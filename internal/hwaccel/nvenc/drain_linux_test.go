//go:build linux && (amd64 || arm64)

package nvenc

import (
	"testing"
	"time"
)

const callDeadline = 10 * time.Second

func returnsWithin(fn func()) bool {
	done := make(chan struct{})
	go func() {
		defer close(done)
		fn()
	}()
	select {
	case <-done:
		return true
	case <-time.After(callDeadline):
		return false
	}
}

func drainWithin(t *testing.T, enc *Encoder) []byte {
	t.Helper()
	var out []byte
	var err error
	if !returnsWithin(func() { out, err = enc.Drain() }) {
		t.Fatalf("%s: Drain did not return within %v", enc.Name(), callDeadline)
	}
	if err != nil {
		t.Fatalf("%s: Drain: %v", enc.Name(), err)
	}
	return out
}

func closeWithin(t *testing.T, enc *Encoder) {
	t.Helper()
	var err error
	if !returnsWithin(func() { err = enc.Close() }) {
		t.Errorf("%s: Close did not return within %v", enc.Name(), callDeadline)
		return
	}
	if err != nil {
		t.Errorf("%s: Close: %v", enc.Name(), err)
	}
}

func openProbe(t *testing.T) *Encoder {
	t.Helper()
	requireAdapter(t)
	enc, err := Open(probeConfig())
	if err != nil {
		t.Skipf("the encoder could not be opened: %v", err)
	}
	return enc
}

func TestDrainAfterFramesReturns(t *testing.T) {
	enc := openProbe(t)
	defer closeWithin(t, enc)
	for i := 0; i < probeFrames; i++ {
		if _, err := enc.Encode(movingPattern(i)); err != nil {
			t.Fatalf("frame %d: %v", i, err)
		}
	}
	drainWithin(t, enc)
}

func TestDrainBeforeAnyFrameReturns(t *testing.T) {
	enc := openProbe(t)
	defer closeWithin(t, enc)
	if out := drainWithin(t, enc); len(out) != 0 {
		t.Fatalf("draining an encoder that was given no frame produced %d bytes", len(out))
	}
}

func TestCloseWithoutDrainReturns(t *testing.T) {
	enc := openProbe(t)
	for i := 0; i < 4; i++ {
		if _, err := enc.Encode(movingPattern(i)); err != nil {
			closeWithin(t, enc)
			t.Fatalf("frame %d: %v", i, err)
		}
	}
	closeWithin(t, enc)
}

func TestEncodersCycleThroughDrainAndClose(t *testing.T) {
	for round := 0; round < 4; round++ {
		enc := openProbe(t)
		for i := 0; i < 3; i++ {
			if _, err := enc.Encode(movingPattern(i)); err != nil {
				closeWithin(t, enc)
				t.Fatalf("round %d frame %d: %v", round, i, err)
			}
		}
		drainWithin(t, enc)
		closeWithin(t, enc)
	}
}
