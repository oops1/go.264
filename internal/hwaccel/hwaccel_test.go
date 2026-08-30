package hwaccel

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func resetRegistry(t *testing.T) {
	t.Helper()
	mu.Lock()
	savedBackends := make([]Backend, len(backends))
	copy(savedBackends, backends)
	savedDisabled := disabled
	backends = nil
	disabled = false
	mu.Unlock()
	t.Cleanup(func() {
		mu.Lock()
		backends = savedBackends
		disabled = savedDisabled
		mu.Unlock()
	})
}

type mockEncoder struct {
	mu          sync.Mutex
	encodeCalls [][]byte
	encodeRet   []byte
	encodeErr   error
	closeCalls  int
	closeErr    error
}

func (m *mockEncoder) Encode(i420 []byte) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := append([]byte(nil), i420...)
	m.encodeCalls = append(m.encodeCalls, cp)
	return m.encodeRet, m.encodeErr
}

func (m *mockEncoder) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closeCalls++
	return m.closeErr
}

type mockDecoder struct {
	mu          sync.Mutex
	decodeCalls int
	flushCalls  int
	closeCalls  int
	decodeRet   []*Picture
	flushRet    []*Picture
}

func (m *mockDecoder) Decode(annexB []byte) ([]*Picture, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.decodeCalls++
	return m.decodeRet, nil
}

func (m *mockDecoder) Flush() ([]*Picture, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.flushCalls++
	return m.flushRet, nil
}

func (m *mockDecoder) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closeCalls++
	return nil
}

func TestEmptyRegistryReportsNothing(t *testing.T) {
	resetRegistry(t)

	if got := Available(); len(got) != 0 {
		t.Fatalf("Available() = %v, want empty", got)
	}

	enc, name, ok := OpenEncoder(EncoderParams{Width: 64, Height: 64})
	if ok || enc != nil || name != "" {
		t.Fatalf("OpenEncoder() = (%v, %q, %v), want (nil, \"\", false)", enc, name, ok)
	}

	dec, name, ok := OpenDecoder()
	if ok || dec != nil || name != "" {
		t.Fatalf("OpenDecoder() = (%v, %q, %v), want (nil, \"\", false)", dec, name, ok)
	}
}

func TestOpenEncoderReturnsSucceedingBackend(t *testing.T) {
	resetRegistry(t)

	me := &mockEncoder{encodeRet: []byte{1, 2, 3}}
	Register(Backend{
		Name: "mock",
		ProbeEncode: func(EncoderParams) (Encoder, bool) {
			return me, true
		},
	})

	enc, name, ok := OpenEncoder(EncoderParams{Width: 32, Height: 32})
	if !ok {
		t.Fatal("OpenEncoder() reported not-ok, want ok")
	}
	if name != "mock" {
		t.Fatalf("OpenEncoder() name = %q, want %q", name, "mock")
	}
	if enc != Encoder(me) {
		t.Fatal("OpenEncoder() did not return the registered mock encoder")
	}

	pkt, err := enc.Encode([]byte{9, 9, 9})
	if err != nil {
		t.Fatalf("Encode returned error: %v", err)
	}
	if len(pkt) != 3 || pkt[0] != 1 || pkt[1] != 2 || pkt[2] != 3 {
		t.Fatalf("Encode returned %v, want the mock's payload", pkt)
	}
	if len(me.encodeCalls) != 1 || len(me.encodeCalls[0]) != 3 {
		t.Fatalf("mock encoder did not record the Encode call: %v", me.encodeCalls)
	}

	if err := enc.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}
	if me.closeCalls != 1 {
		t.Fatalf("mock encoder Close call count = %d, want 1", me.closeCalls)
	}
}

func TestOpenEncoderPriorityOrder(t *testing.T) {
	resetRegistry(t)

	var calledFail, calledFirstOK, calledSecondOK int32

	Register(Backend{
		Name: "failing",
		ProbeEncode: func(EncoderParams) (Encoder, bool) {
			atomic.AddInt32(&calledFail, 1)
			return nil, false
		},
	})
	Register(Backend{
		Name: "succeeding",
		ProbeEncode: func(EncoderParams) (Encoder, bool) {
			atomic.AddInt32(&calledFirstOK, 1)
			return &mockEncoder{}, true
		},
	})
	Register(Backend{
		Name: "also-succeeding",
		ProbeEncode: func(EncoderParams) (Encoder, bool) {
			atomic.AddInt32(&calledSecondOK, 1)
			return &mockEncoder{}, true
		},
	})

	_, name, ok := OpenEncoder(EncoderParams{})
	if !ok {
		t.Fatal("OpenEncoder() reported not-ok")
	}
	if name != "succeeding" {
		t.Fatalf("OpenEncoder() chose %q, want %q", name, "succeeding")
	}
	if calledFail != 1 {
		t.Fatalf("failing probe called %d times, want 1", calledFail)
	}
	if calledFirstOK != 1 {
		t.Fatalf("succeeding probe called %d times, want 1", calledFirstOK)
	}
	if calledSecondOK != 0 {
		t.Fatalf("also-succeeding probe called %d times, want 0 (should not be reached)", calledSecondOK)
	}
}

func TestOpenEncoderFirstSucceedingWinsAmongMultiple(t *testing.T) {
	resetRegistry(t)

	firstEnc := &mockEncoder{}
	secondEnc := &mockEncoder{}
	Register(Backend{
		Name: "first",
		ProbeEncode: func(EncoderParams) (Encoder, bool) {
			return firstEnc, true
		},
	})
	Register(Backend{
		Name: "second",
		ProbeEncode: func(EncoderParams) (Encoder, bool) {
			return secondEnc, true
		},
	})

	enc, name, ok := OpenEncoder(EncoderParams{})
	if !ok {
		t.Fatal("OpenEncoder() reported not-ok")
	}
	if name != "first" {
		t.Fatalf("OpenEncoder() chose %q, want %q", name, "first")
	}
	if enc != Encoder(firstEnc) {
		t.Fatal("OpenEncoder() did not return the first registered backend's encoder")
	}
}

func TestBackendNilProbeIsSkippedButOtherProbeStillWorks(t *testing.T) {
	resetRegistry(t)

	me := &mockEncoder{}
	md := &mockDecoder{}
	Register(Backend{
		Name:        "encode-only",
		ProbeEncode: func(EncoderParams) (Encoder, bool) { return me, true },
		ProbeDecode: nil,
	})

	if _, _, ok := OpenDecoder(); ok {
		t.Fatal("OpenDecoder() succeeded using a backend with a nil ProbeDecode")
	}
	if _, name, ok := OpenEncoder(EncoderParams{}); !ok || name != "encode-only" {
		t.Fatalf("OpenEncoder() = (_, %q, %v), want (_, \"encode-only\", true)", name, ok)
	}

	resetRegistry(t)

	Register(Backend{
		Name:        "decode-only",
		ProbeEncode: nil,
		ProbeDecode: func() (Decoder, bool) { return md, true },
	})

	if _, _, ok := OpenEncoder(EncoderParams{}); ok {
		t.Fatal("OpenEncoder() succeeded using a backend with a nil ProbeEncode")
	}
	if _, name, ok := OpenDecoder(); !ok || name != "decode-only" {
		t.Fatalf("OpenDecoder() = (_, %q, %v), want (_, \"decode-only\", true)", name, ok)
	}
}

func TestDisableAndEnableAreReversibleAndPreserveRegistry(t *testing.T) {
	resetRegistry(t)

	Register(Backend{
		Name:        "a",
		ProbeEncode: func(EncoderParams) (Encoder, bool) { return &mockEncoder{}, true },
		ProbeDecode: func() (Decoder, bool) { return &mockDecoder{}, true },
	})
	Register(Backend{
		Name:        "b",
		ProbeEncode: func(EncoderParams) (Encoder, bool) { return &mockEncoder{}, true },
		ProbeDecode: func() (Decoder, bool) { return &mockDecoder{}, true },
	})

	before := Available()
	if len(before) != 2 || before[0] != "a" || before[1] != "b" {
		t.Fatalf("Available() before Disable = %v, want [a b]", before)
	}

	Disable()
	if got := Available(); len(got) != 0 {
		t.Fatalf("Available() after Disable = %v, want empty", got)
	}
	if _, _, ok := OpenEncoder(EncoderParams{}); ok {
		t.Fatal("OpenEncoder() succeeded while the registry was disabled")
	}
	if _, _, ok := OpenDecoder(); ok {
		t.Fatal("OpenDecoder() succeeded while the registry was disabled")
	}

	Enable()
	after := Available()
	if len(after) != 2 || after[0] != "a" || after[1] != "b" {
		t.Fatalf("Available() after Enable = %v, want [a b] (registry must not be cleared by Disable)", after)
	}
	if _, name, ok := OpenEncoder(EncoderParams{}); !ok || name != "a" {
		t.Fatalf("OpenEncoder() after Enable = (_, %q, %v), want (_, \"a\", true)", name, ok)
	}
	if _, name, ok := OpenDecoder(); !ok || name != "a" {
		t.Fatalf("OpenDecoder() after Enable = (_, %q, %v), want (_, \"a\", true)", name, ok)
	}
}

func TestEncoderParamsPassedThroughUnchanged(t *testing.T) {
	resetRegistry(t)

	want := EncoderParams{
		Width:   1920,
		Height:  1080,
		FPSNum:  30001,
		FPSDen:  1001,
		GOPSize: 12,
		QP:      23,
	}

	var got EncoderParams
	var gotCalled bool
	Register(Backend{
		Name: "recorder",
		ProbeEncode: func(p EncoderParams) (Encoder, bool) {
			got = p
			gotCalled = true
			return &mockEncoder{}, true
		},
	})

	if _, _, ok := OpenEncoder(want); !ok {
		t.Fatal("OpenEncoder() reported not-ok")
	}
	if !gotCalled {
		t.Fatal("probe was never called")
	}
	if got != want {
		t.Fatalf("probe received %+v, want %+v", got, want)
	}
}

func TestRegistryIsConcurrencySafe(t *testing.T) {
	resetRegistry(t)

	const goroutines = 16
	const iterations = 500

	var wg sync.WaitGroup
	stop := make(chan struct{})

	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			n := 0
			for {
				select {
				case <-stop:
					return
				default:
				}
				switch n % 6 {
				case 0:
					Register(Backend{
						Name: "concurrent",
						ProbeEncode: func(EncoderParams) (Encoder, bool) {
							return &mockEncoder{}, true
						},
						ProbeDecode: func() (Decoder, bool) {
							return &mockDecoder{}, true
						},
					})
				case 1:
					_ = Available()
				case 2:
					_, _, _ = OpenEncoder(EncoderParams{Width: id, Height: n})
				case 3:
					_, _, _ = OpenDecoder()
				case 4:
					Disable()
				case 5:
					Enable()
				}
				n++
				if n >= iterations {
					return
				}
			}
		}(g)
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		close(stop)
		t.Fatal("timed out waiting for concurrent registry access to finish")
	}
}

func TestPictureZeroValueIsUsableThroughDecoderInterface(t *testing.T) {
	var zero Picture
	if zero.Y != nil || zero.Cb != nil || zero.Cr != nil {
		t.Fatalf("zero Picture has non-nil planes: %+v", zero)
	}
	if zero.StrideY != 0 || zero.StrideC != 0 || zero.Width != 0 || zero.Height != 0 {
		t.Fatalf("zero Picture has non-zero geometry: %+v", zero)
	}

	populated := &Picture{
		Y: []byte{1, 2, 3, 4}, Cb: []byte{5, 6}, Cr: []byte{7, 8},
		StrideY: 2, StrideC: 1, Width: 2, Height: 2,
	}
	md := &mockDecoder{
		decodeRet: []*Picture{&zero, populated},
		flushRet:  []*Picture{&zero},
	}

	var d Decoder = md
	pics, err := d.Decode(nil)
	if err != nil {
		t.Fatalf("Decode returned error: %v", err)
	}
	if len(pics) != 2 {
		t.Fatalf("Decode returned %d pictures, want 2", len(pics))
	}
	if pics[0].Width != 0 || pics[0].Y != nil {
		t.Fatalf("zero-value Picture came back mutated: %+v", *pics[0])
	}
	if pics[1].Width != 2 || len(pics[1].Y) != 4 {
		t.Fatalf("populated Picture came back wrong: %+v", *pics[1])
	}

	flushed, err := d.Flush()
	if err != nil {
		t.Fatalf("Flush returned error: %v", err)
	}
	if len(flushed) != 1 || flushed[0] != &zero {
		t.Fatal("Flush did not return the expected picture through the interface")
	}

	if err := d.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}
	if md.closeCalls != 1 {
		t.Fatalf("Close call count = %d, want 1", md.closeCalls)
	}
}
