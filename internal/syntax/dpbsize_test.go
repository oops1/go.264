package syntax

import "testing"

func TestMaxDpbMbsRisesWithTheLevel(t *testing.T) {
	prev := 0
	for _, e := range maxDpbMbsByLevel {
		if e.levelIDC == 9 || e.constraint3 {
			continue
		}
		if e.maxDpbMbs < prev {
			t.Fatalf("level %d has %d macroblocks of buffer, below the %d before it",
				e.levelIDC, e.maxDpbMbs, prev)
		}
		prev = e.maxDpbMbs
	}
}

func TestMaxDpbFramesAtTheLevelsTableAnchors(t *testing.T) {
	cases := []struct {
		level     uint8
		widthMBs  int
		heightMBs int
		want      int
	}{
		{30, 45, 36, 5},
		{31, 80, 45, 5},
		{40, 120, 68, 4},
		{10, 11, 9, 4},
		{51, 120, 68, 16},
	}
	for _, c := range cases {
		s := &SPS{
			LevelIDC:                  c.level,
			PicWidthInMbsMinus1:       uint32(c.widthMBs - 1),
			PicHeightInMapUnitsMinus1: uint32(c.heightMBs - 1),
			FrameMbsOnly:              true,
		}
		if got := s.MaxDpbFrames(); got != c.want {
			t.Fatalf("level %d at %dx%d macroblocks: MaxDpbFrames() = %d, want %d",
				c.level, c.widthMBs, c.heightMBs, got, c.want)
		}
	}
}

func TestLevel1bTakesTheSmallerBuffer(t *testing.T) {
	base := SPS{LevelIDC: 11, PicWidthInMbsMinus1: 10, PicHeightInMapUnitsMinus1: 8, FrameMbsOnly: true}
	full := base
	if full.MaxDpbFrames() != 9 {
		t.Fatalf("level 1.1 MaxDpbFrames() = %d, want 9", full.MaxDpbFrames())
	}
	oneB := base
	oneB.ConstraintSet = 0x10
	if oneB.MaxDpbFrames() != 4 {
		t.Fatalf("level 1b MaxDpbFrames() = %d, want 4", oneB.MaxDpbFrames())
	}
}

func TestMaxNumReorderPrefersTheBitstreamRestriction(t *testing.T) {
	s := &SPS{LevelIDC: 51, PicWidthInMbsMinus1: 10, PicHeightInMapUnitsMinus1: 8, FrameMbsOnly: true}
	if got := s.MaxNumReorder(); got != 16 {
		t.Fatalf("without a restriction MaxNumReorder() = %d, want 16", got)
	}
	s.VUIPresent = true
	s.VUI.BitstreamRestriction = true
	s.VUI.MaxNumReorderFrames = 2
	if got := s.MaxNumReorder(); got != 2 {
		t.Fatalf("with a restriction MaxNumReorder() = %d, want 2", got)
	}
}
