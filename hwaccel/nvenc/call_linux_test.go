package nvenc

import (
	"runtime"
	"sync/atomic"
	"testing"
	"time"
	"unsafe"

	"github.com/ebitengine/purego"
)

type callKeepAlivePayload struct{ a, b, c, d uint64 }

func waitForFinalizer(flag *int32, budget time.Duration) bool {
	deadline := time.Now().Add(budget)
	for atomic.LoadInt32(flag) == 0 && time.Now().Before(deadline) {
		runtime.GC()
		time.Sleep(time.Millisecond)
	}
	return atomic.LoadInt32(flag) == 1
}

// TestCallKeepsPointerArgumentsAliveAcrossTheCall reproduces the shape of
// bug that motivated the //go:uintptrescapes pragma on (*Encoder).call: a
// struct built locally, addressed only through uintptr(unsafe.Pointer(&x)),
// and never referenced again in Go-typed form afterward, is otherwise dead
// as soon as the compiler emits that conversion. Every NVENC call in this
// package builds exactly such a struct (NvEncOpenEncodeSessionExParams,
// NvEncInitializeParams, NvEncPicParams, ...) and hands its address to the
// driver through call, which stages it through a []uintptr on its way to
// purego.SyscallN. Without uintptrescapes, a GC cycle landing while the
// driver is still running is free to collect it.
func TestCallKeepsPointerArgumentsAliveAcrossTheCall(t *testing.T) {
	var finalized, collectedDuringCall int32

	entry := purego.NewCallback(func(_ uintptr) uintptr {
		if waitForFinalizer(&finalized, 300*time.Millisecond) {
			atomic.StoreInt32(&collectedDuringCall, 1)
		}
		return 0
	})

	e := &Encoder{}
	func() {
		p := new(callKeepAlivePayload)
		*p = callKeepAlivePayload{1, 2, 3, 4}
		runtime.SetFinalizer(p, func(*callKeepAlivePayload) {
			atomic.StoreInt32(&finalized, 1)
		})
		if err := e.call(entry, "test entry point", uintptr(unsafe.Pointer(p))); err != nil {
			t.Fatalf("call: %v", err)
		}
	}()

	if atomic.LoadInt32(&collectedDuringCall) == 1 {
		t.Fatal("the argument was garbage collected while the native call still held only its raw address")
	}
}
