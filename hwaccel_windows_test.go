package go264

import (
	"testing"

	"github.com/oops1/go.264/internal/hwaccel"
	"github.com/oops1/go.264/internal/hwaccel/mf"
)

func TestMediaFoundationIsRegistered(t *testing.T) {
	for _, name := range Backends() {
		if name == mediaFoundationBackend {
			return
		}
	}
	t.Fatalf("the Media Foundation backend is absent from %v", Backends())
}

func TestDefaultBitrateFollowsTheRequestWhenGiven(t *testing.T) {
	got := defaultBitrateBitsPerSecond(hwaccel.EncoderParams{BitrateKbps: 750})
	if got != 750000 {
		t.Fatalf("a request for 750 kbit produced %d bit per second", got)
	}
}

func TestDefaultBitrateGrowsWithThePictureAndTheRate(t *testing.T) {
	small := defaultBitrateBitsPerSecond(hwaccel.EncoderParams{Width: 176, Height: 144, FPSNum: 10, FPSDen: 1})
	large := defaultBitrateBitsPerSecond(hwaccel.EncoderParams{Width: 1280, Height: 720, FPSNum: 10, FPSDen: 1})
	faster := defaultBitrateBitsPerSecond(hwaccel.EncoderParams{Width: 1280, Height: 720, FPSNum: 60, FPSDen: 1})
	if large <= small {
		t.Fatalf("a larger picture asked for %d bit per second against %d for a smaller one", large, small)
	}
	if faster <= large {
		t.Fatalf("a faster rate asked for %d bit per second against %d for a slower one", faster, large)
	}
}

func TestDefaultBitrateHasAFloor(t *testing.T) {
	if got := defaultBitrateBitsPerSecond(hwaccel.EncoderParams{Width: 16, Height: 16, FPSNum: 1, FPSDen: 1}); got < 100000 {
		t.Fatalf("a tiny picture asked for %d bit per second, below the floor", got)
	}
}

func TestDefaultBitrateSurvivesAnAbsentFrameRate(t *testing.T) {
	if got := defaultBitrateBitsPerSecond(hwaccel.EncoderParams{Width: 176, Height: 144}); got <= 0 {
		t.Fatalf("an absent frame rate produced %d bit per second", got)
	}
}

func TestJoinPacketsConcatenatesInOrder(t *testing.T) {
	got := joinPackets([][]byte{{1, 2}, nil, {3}, {4, 5, 6}})
	want := []byte{1, 2, 3, 4, 5, 6}
	if len(got) != len(want) {
		t.Fatalf("joining produced %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("joining produced %v, want %v", got, want)
		}
	}
	if joinPackets(nil) != nil {
		t.Fatal("joining nothing produced a non-empty result")
	}
}

func TestEncoderPrefersTheHardwareBackendWhenItOpens(t *testing.T) {
	if !mf.Loaded() {
		t.Skip("Media Foundation is not present on this machine")
	}
	enc, err := NewEncoder(EncoderConfig{Width: 176, Height: 144, FPSNum: 10, FPSDen: 1, GOPSize: 10, QP: 26})
	if err != nil {
		t.Fatalf("NewEncoder: %v", err)
	}
	defer enc.Close()
	if enc.Backend() == "cpu" {
		t.Skip("no hardware encoder could be opened on this machine")
	}
	if enc.Backend() != mediaFoundationBackend {
		t.Fatalf("the encoder chose backend %q", enc.Backend())
	}

	frame := make([]byte, 176*144*3/2)
	total := 0
	for i := 0; i < 12; i++ {
		for j := range frame {
			frame[j] = byte(j + i*13)
		}
		out, err := enc.Encode(frame)
		if err != nil {
			t.Fatalf("Encode %d: %v", i, err)
		}
		total += len(out)
	}
	tail, err := enc.Flush()
	if err != nil {
		t.Fatalf("Flush: %v", err)
	}
	total += len(tail)
	if total == 0 {
		t.Fatal("the hardware encoder produced nothing at all")
	}
	t.Logf("%s produced %d bytes from twelve frames", enc.Backend(), total)
}

func TestSoftwareEncoderFlushesToNothing(t *testing.T) {
	enc, err := NewEncoder(EncoderConfig{Width: 176, Height: 144, FPSNum: 10, FPSDen: 1, GOPSize: 10, QP: 26, ForceSoftware: true})
	if err != nil {
		t.Fatalf("NewEncoder: %v", err)
	}
	defer enc.Close()
	tail, err := enc.Flush()
	if err != nil {
		t.Fatalf("Flush: %v", err)
	}
	if len(tail) != 0 {
		t.Fatalf("the processor encoder held back %d bytes", len(tail))
	}
}

func TestFlushOnAClosedEncoderReports(t *testing.T) {
	enc, err := NewEncoder(EncoderConfig{Width: 176, Height: 144, FPSNum: 10, FPSDen: 1, GOPSize: 10, QP: 26, ForceSoftware: true})
	if err != nil {
		t.Fatalf("NewEncoder: %v", err)
	}
	if err := enc.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if _, err := enc.Flush(); err != ErrClosed {
		t.Fatalf("Flush on a closed encoder returned %v, want ErrClosed", err)
	}
}
