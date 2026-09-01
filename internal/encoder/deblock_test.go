package encoder

import (
	"bytes"
	"testing"

	"github.com/oops1/go.264/internal/frame"
)

func deblockConfig(qp int) Config {
	return Config{Width: 176, Height: 144, FPSNum: 25, FPSDen: 1, GOPSize: 8, QP: qp, RefFrames: 1}
}

func decodedBytes(t *testing.T, pics []*frame.Picture) [][]byte {
	t.Helper()
	out := make([][]byte, len(pics))
	for i, p := range pics {
		buf := make([]byte, p.Size())
		p.CopyOut(buf)
		out[i] = buf
	}
	return out
}

func TestDeblockingDefaultsToTheOldStream(t *testing.T) {
	cfg := deblockConfig(28)
	frames := screenSequence(cfg.Width, cfg.Height, 6)
	before, _ := encodeUnits(t, cfg, frames)

	cfg.Deblocking = DeblockingOn
	cfg.DeblockAlphaOffset = 0
	cfg.DeblockBetaOffset = 0
	after, _ := encodeUnits(t, cfg, frames)
	for i := range before {
		if !bytes.Equal(before[i], after[i]) {
			t.Fatalf("frame %d differs although the deblocking settings are the defaults", i)
		}
	}
}

func TestDeblockingRejectsOutOfRangeSettings(t *testing.T) {
	cases := []struct {
		name string
		edit func(c *Config)
	}{
		{"idc 3", func(c *Config) { c.Deblocking = 3 }},
		{"alpha 7", func(c *Config) { c.DeblockAlphaOffset = 7 }},
		{"alpha -7", func(c *Config) { c.DeblockAlphaOffset = -7 }},
		{"beta 7", func(c *Config) { c.DeblockBetaOffset = 7 }},
		{"beta -7", func(c *Config) { c.DeblockBetaOffset = -7 }},
	}
	for _, tc := range cases {
		cfg := deblockConfig(26)
		tc.edit(&cfg)
		if _, err := New(cfg); err == nil {
			t.Fatalf("%s was accepted", tc.name)
		}
	}
}

func TestDeblockingReconstructionSurvivesTheDecoder(t *testing.T) {
	for _, cabac := range []bool{false, true} {
		for _, mode := range []DeblockMode{DeblockingOn, DeblockingOff, DeblockingNotAcrossSlices} {
			for _, offsets := range trim([][2]int{{0, 0}, {-6, -6}, {6, 6}, {3, -2}, {-1, 5}}, 1) {
				for _, qp := range trim([]int{0, 21, 33, 51}, 1) {
					cfg := deblockConfig(qp)
					cfg.CABAC = cabac
					cfg.Slices = 3
					cfg.Deblocking = mode
					cfg.DeblockAlphaOffset = offsets[0]
					cfg.DeblockBetaOffset = offsets[1]
					frames := screenSequence(cfg.Width, cfg.Height, 5)
					units, recons := encodeUnits(t, cfg, frames)
					assertMatchesReconstruction(t, "deblocking", decodeUnits(t, units), recons)
				}
			}
		}
	}
}

func TestDeblockingReconstructionAcrossFeatures(t *testing.T) {
	cases := []struct {
		name string
		edit func(c *Config)
	}{
		{"b pictures", func(c *Config) { c.BFrames = 2; c.CABAC = true }},
		{"transform 8x8", func(c *Config) { c.Transform8x8 = true; c.CABAC = true }},
		{"multiple references", func(c *Config) { c.RefFrames = 4 }},
		{"intra refresh", func(c *Config) { c.IntraRefresh = 5 }},
		{"nine slices", func(c *Config) { c.Slices = 9 }},
	}
	for _, tc := range trim(cases, 2) {
		for _, mode := range trim([]DeblockMode{DeblockingOff, DeblockingNotAcrossSlices}, 1) {
			cfg := deblockConfig(27)
			cfg.Deblocking = mode
			cfg.DeblockAlphaOffset = 2
			cfg.DeblockBetaOffset = -3
			tc.edit(&cfg)
			frames := screenSequence(cfg.Width, cfg.Height, 9)
			if cfg.BFrames > 0 {
				stream, recons := encodeBStream(t, cfg, frames, nil)
				assertMatchesReconstruction(t, tc.name, decodeUnits(t, [][]byte{stream}), recons)
				continue
			}
			units, recons := encodeUnits(t, cfg, frames)
			assertMatchesReconstruction(t, tc.name, decodeUnits(t, units), recons)
		}
	}
}

func TestDeblockingOffLeavesTheBlockEdgesAlone(t *testing.T) {
	cfg := deblockConfig(34)
	frames := screenSequence(cfg.Width, cfg.Height, 3)

	on, _ := encodeUnits(t, cfg, frames)
	cfg.Deblocking = DeblockingOff
	off, _ := encodeUnits(t, cfg, frames)

	filtered := decodedBytes(t, decodeUnits(t, on))
	raw := decodedBytes(t, decodeUnits(t, off))
	if bytes.Equal(filtered[0], raw[0]) {
		t.Fatal("turning the deblocking filter off changed nothing in the key frame")
	}
}

func TestDeblockingOffsetsChangeThePicture(t *testing.T) {
	cfg := deblockConfig(34)
	frames := screenSequence(cfg.Width, cfg.Height, 3)
	plain, _ := encodeUnits(t, cfg, frames)

	cfg.DeblockAlphaOffset = 6
	cfg.DeblockBetaOffset = 6
	strong, _ := encodeUnits(t, cfg, frames)

	a := decodedBytes(t, decodeUnits(t, plain))
	b := decodedBytes(t, decodeUnits(t, strong))
	if bytes.Equal(a[0], b[0]) {
		t.Fatal("the deblocking offsets did not reach the filter")
	}
}

func TestDeblockingIDC2StopsAtSliceBoundariesOnly(t *testing.T) {
	const slices = 3
	cfg := deblockConfig(34)
	cfg.Slices = slices
	frames := screenSequence(cfg.Width, cfg.Height, 1)

	across, _ := encodeUnits(t, cfg, frames)
	cfg.Deblocking = DeblockingNotAcrossSlices
	within, _ := encodeUnits(t, cfg, frames)

	a := decodedBytes(t, decodeUnits(t, across))[0]
	b := decodedBytes(t, decodeUnits(t, within))[0]
	if bytes.Equal(a, b) {
		t.Fatal("idc 2 produced the same picture as idc 0, so slice boundaries were still filtered")
	}

	heightMBs := (cfg.Height + 15) / 16
	first := (heightMBs / slices) * 16
	for y := 0; y < first-3; y++ {
		for x := 0; x < cfg.Width; x++ {
			if a[y*cfg.Width+x] != b[y*cfg.Width+x] {
				t.Fatalf("idc 2 changed the sample at (%d,%d), which sits above the first slice boundary at %d",
					x, y, first)
			}
		}
	}
	changed := false
	for y := first - 3; y < first+3 && !changed; y++ {
		for x := 0; x < cfg.Width; x++ {
			if a[y*cfg.Width+x] != b[y*cfg.Width+x] {
				changed = true
				break
			}
		}
	}
	if !changed {
		t.Fatalf("idc 2 left the edge at the slice boundary %d filtered", first)
	}
}
