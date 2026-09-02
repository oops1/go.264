package vaapi

import (
	"testing"

	go264 "github.com/oops1/go.264"
)

func TestBackendNameIsRegistered(t *testing.T) {
	found := false
	for _, name := range go264.Backends() {
		if name == backendName {
			found = true
		}
	}
	if !found {
		t.Fatalf("backend %q did not register itself with internal/hwaccel", backendName)
	}
}

func TestPublicEncoderPicksTheAdapterUp(t *testing.T) {
	requireAdapter(t)
	enc, err := go264.NewEncoder(go264.EncoderConfig{
		Width: probeWidth, Height: probeHeight,
		FPSNum: 25, FPSDen: 1, GOPSize: 30, QP: 26,
	})
	if err != nil {
		t.Fatalf("NewEncoder: %v", err)
	}
	defer enc.Close()
	if enc.Backend() == "cpu" {
		t.Skip("the adapter refused this configuration")
	}
	if enc.Backend() != backendName {
		t.Fatalf("the encoder chose backend %q", enc.Backend())
	}
	total := 0
	for i := 0; i < probeFrames; i++ {
		out, err := enc.Encode(movingPattern(i))
		if err != nil {
			t.Fatalf("frame %d: %v", i, err)
		}
		total += len(out)
	}
	tail, err := enc.Flush()
	if err != nil {
		t.Fatalf("Flush: %v", err)
	}
	total += len(tail)
	if total == 0 {
		t.Fatal("the adapter produced nothing through the public encoder")
	}
}
