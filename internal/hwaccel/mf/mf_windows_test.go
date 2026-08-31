package mf

import "testing"

func TestPlatformStartupAndShutdownBalance(t *testing.T) {
	if !Loaded() {
		t.Skip("Media Foundation is not present on this machine")
	}
	if err := Startup(); err != nil {
		t.Fatalf("Startup: %v", err)
	}
	if err := Startup(); err != nil {
		t.Fatalf("nested Startup: %v", err)
	}
	if platform.count != 2 {
		t.Fatalf("two starts left a count of %d, want 2", platform.count)
	}
	if err := Shutdown(); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	if platform.count != 1 {
		t.Fatalf("one shutdown left a count of %d, want 1", platform.count)
	}
	if err := Shutdown(); err != nil {
		t.Fatalf("final Shutdown: %v", err)
	}
	if platform.count != 0 {
		t.Fatalf("the final shutdown left a count of %d, want 0", platform.count)
	}
	if err := Shutdown(); err != nil {
		t.Fatalf("shutting down what was never started: %v", err)
	}
}

func TestPlatformStartsAgainAfterAFullShutdown(t *testing.T) {
	if !Loaded() {
		t.Skip("Media Foundation is not present on this machine")
	}
	for i := 0; i < 3; i++ {
		if err := Startup(); err != nil {
			t.Fatalf("Startup round %d: %v", i, err)
		}
		if err := Shutdown(); err != nil {
			t.Fatalf("Shutdown round %d: %v", i, err)
		}
	}
}
