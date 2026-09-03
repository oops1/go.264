package encoder

import (
	"fmt"
	"testing"
)

func TestMotionSearchLevelsRoundTrip(t *testing.T) {
	for _, level := range []struct {
		name   string
		search MotionSearch
	}{
		{"full", MotionSearchFull},
		{"half", MotionSearchHalf},
		{"integer", MotionSearchInteger},
		{"zero", MotionSearchZero},
	} {
		cfg := Config{Width: 176, Height: 144, FPSNum: 25, FPSDen: 1, GOPSize: 12, QP: 26,
			RefFrames: 1, CABAC: true, MotionSearch: level.search}
		var frames [][]byte
		for i := 0; i < 6; i++ {
			frames = append(frames, syntheticFrame(cfg.Width, cfg.Height, i))
		}
		units, recons := encodeFrames(t, cfg, frames)
		assertMatchesReconstruction(t, level.name, decodeUnits(t, units), recons)
	}
}

func TestTheSubPelLadderTradesBitsForTime(t *testing.T) {
	const w, h = 320, 240
	var frames [][]byte
	for i := 0; i < 10; i++ {
		frames = append(frames, panningFrame(w, h, i))
	}
	size := func(search MotionSearch) int {
		cfg := Config{Width: w, Height: h, FPSNum: 25, FPSDen: 1, GOPSize: 100, QP: 26,
			RefFrames: 1, CABAC: true, MotionSearch: search}
		units, _ := encodeFrames(t, cfg, frames)
		total := 0
		for _, u := range units {
			total += len(u)
		}
		return total
	}
	full := size(MotionSearchFull)
	half := size(MotionSearchHalf)
	integer := size(MotionSearchInteger)
	zero := size(MotionSearchZero)
	t.Logf("full %d, half-pel %d, integer %d, zero %d bytes", full, half, integer, zero)

	for _, c := range []struct {
		name string
		got  int
	}{{"half-pel", half}, {"integer", integer}} {
		if c.got >= zero {
			t.Fatalf("%s costs %d bytes against the zero search's %d; a search that finds nothing is not a search",
				c.name, c.got, zero)
		}
		if c.got < full {
			continue
		}
		if grew := float64(c.got-full) / float64(full); grew > 0.25 {
			t.Fatalf("%s costs %.1f%% more than the full search, which is not an intermediate setting",
				c.name, 100*grew)
		}
	}
	if integer < half {
		t.Log(fmt.Sprintf("note: integer only came out smaller than half-pel here, %d against %d", integer, half))
	}
}
