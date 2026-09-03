package mf

import (
	"errors"
	"runtime"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
	"unsafe"
)

type fakeObject struct {
	vtblPtr unsafe.Pointer
	vtbl    []uintptr
}

func newFakeObject(methods ...uintptr) *fakeObject {
	f := &fakeObject{vtbl: methods}
	f.vtblPtr = unsafe.Pointer(&f.vtbl[0])
	return f
}

func (f *fakeObject) this() unsafe.Pointer { return unsafe.Pointer(&f.vtblPtr) }

func constantMethod(v uintptr) uintptr {
	return syscall.NewCallback(func(unsafe.Pointer) uintptr { return v })
}

func TestVtblCallDispatchesByIndex(t *testing.T) {
	obj := newFakeObject(
		constantMethod(0x11),
		constantMethod(0x22),
		constantMethod(0x33),
		constantMethod(0x44),
	)
	for i, want := range []uintptr{0x11, 0x22, 0x33, 0x44} {
		if got := vtblCall(obj.this(), i); got != want {
			t.Fatalf("method %d returned %#x, want %#x", i, got, want)
		}
	}
}

func TestVtblCallPassesTheInterfacePointerFirst(t *testing.T) {
	var seen unsafe.Pointer
	obj := newFakeObject(syscall.NewCallback(func(this unsafe.Pointer) uintptr {
		seen = this
		return 0
	}))
	vtblCall(obj.this(), 0)
	if seen != obj.this() {
		t.Fatalf("the method received %p as its own pointer, want %p", seen, obj.this())
	}
}

func TestVtblCallPassesArgumentsInOrder(t *testing.T) {
	var got [4]uintptr
	obj := newFakeObject(syscall.NewCallback(func(_ unsafe.Pointer, a, b, c, d uintptr) uintptr {
		got = [4]uintptr{a, b, c, d}
		return 0
	}))
	vtblCall(obj.this(), 0, 7, 8, 9, 10)
	if got != [4]uintptr{7, 8, 9, 10} {
		t.Fatalf("the method received %v, want [7 8 9 10]", got)
	}
}

func TestReleaseOnAnAbsentInterfaceIsHarmless(t *testing.T) {
	if n := (unknown{}).release(); n != 0 {
		t.Fatalf("releasing nothing reported %d references", n)
	}
}

func TestAddRefAndReleaseReachTheirSlots(t *testing.T) {
	refs := 1
	obj := newFakeObject(
		constantMethod(0),
		syscall.NewCallback(func(unsafe.Pointer) uintptr { refs++; return uintptr(refs) }),
		syscall.NewCallback(func(unsafe.Pointer) uintptr { refs--; return uintptr(refs) }),
	)
	u := unknown{obj.this()}
	if got := u.addRef(); got != 2 || refs != 2 {
		t.Fatalf("addRef reported %d with a count of %d, want 2 and 2", got, refs)
	}
	if got := u.release(); got != 1 || refs != 1 {
		t.Fatalf("release reported %d with a count of %d, want 1 and 1", got, refs)
	}
}

func TestQueryInterfaceHandsBackThePointerItWasGiven(t *testing.T) {
	wantIID := GUID{Data1: 0xDEADBEEF}
	var sawIID GUID
	target := new(int)
	obj := newFakeObject(syscall.NewCallback(func(_ unsafe.Pointer, iid *GUID, out *unsafe.Pointer) uintptr {
		sawIID = *iid
		*out = unsafe.Pointer(target)
		return uintptr(sOK)
	}))
	got, err := (unknown{obj.this()}).queryInterface(&wantIID)
	if err != nil {
		t.Fatalf("queryInterface: %v", err)
	}
	if got.p != unsafe.Pointer(target) {
		t.Fatalf("queryInterface produced %p, want %p", got.p, target)
	}
	if sawIID != wantIID {
		t.Fatalf("the method was asked for %s, want %s", sawIID, wantIID)
	}
}

func TestQueryInterfaceReportsTheFailingResult(t *testing.T) {
	const eNoInterface = HRESULT(0x80004002)
	obj := newFakeObject(syscall.NewCallback(func(_ unsafe.Pointer, _ *GUID, _ *unsafe.Pointer) uintptr {
		return uintptr(eNoInterface)
	}))
	_, err := (unknown{obj.this()}).queryInterface(&GUID{})
	var mfErr *Error
	if !errors.As(err, &mfErr) {
		t.Fatalf("queryInterface returned %v, want an *Error", err)
	}
	if mfErr.Code != eNoInterface {
		t.Fatalf("queryInterface reported %s, want %s", mfErr.Code, eNoInterface)
	}
	if mfErr.Op != "QueryInterface" {
		t.Fatalf("the failure names %q, want QueryInterface", mfErr.Op)
	}
}

func waitForFinalizer(flag *int32, budget time.Duration) bool {
	deadline := time.Now().Add(budget)
	for atomic.LoadInt32(flag) == 0 && time.Now().Before(deadline) {
		runtime.GC()
		time.Sleep(time.Millisecond)
	}
	return atomic.LoadInt32(flag) == 1
}

// TestQueryInterfaceKeepsTheIIDAliveAcrossTheCall reproduces the shape of
// the bug fixed by the runtime.KeepAlive calls added around this package's
// hr/vtblCall sites: an argument built by the caller and referenced only
// through its address (converted to a uintptr, staged through vtblCall's
// []uintptr slice, and handed to syscall.SyscallN) is, from the compiler's
// point of view, dead as soon as the conversion happens, because nothing
// in the call chain down to SyscallN keeps a typed reference to it. Without
// an explicit runtime.KeepAlive, a GC cycle that lands while the native
// call is still running is free to collect it.
func TestQueryInterfaceKeepsTheIIDAliveAcrossTheCall(t *testing.T) {
	var finalized, collectedDuringCall int32

	obj := newFakeObject(syscall.NewCallback(func(_ unsafe.Pointer, _ *GUID, out *unsafe.Pointer) uintptr {
		if waitForFinalizer(&finalized, 300*time.Millisecond) {
			atomic.StoreInt32(&collectedDuringCall, 1)
		}
		*out = nil
		return uintptr(sOK)
	}))

	func() {
		iid := new(GUID)
		*iid = GUID{Data1: 0xAABBCCDD}
		runtime.SetFinalizer(iid, func(*GUID) {
			atomic.StoreInt32(&finalized, 1)
		})
		if _, err := (unknown{obj.this()}).queryInterface(iid); err != nil {
			t.Fatalf("queryInterface: %v", err)
		}
	}()

	if atomic.LoadInt32(&collectedDuringCall) == 1 {
		t.Fatal("the iid was garbage collected while the native QueryInterface call still held only its raw address")
	}
}
