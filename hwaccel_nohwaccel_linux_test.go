//go:build linux && go264_nohwaccel

package go264

import (
	"slices"
	"testing"
)

func TestTheSwitchLeavesOnlyTheProcessor(t *testing.T) {
	if got := Backends(); !slices.Equal(got, []string{"cpu"}) {
		t.Fatalf("Backends() = %v under go264_nohwaccel, want only the processor", got)
	}
}
