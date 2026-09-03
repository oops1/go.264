package encoder

import "testing"

func codedSize(t *testing.T, cfg Config, frames [][]byte) int {
	t.Helper()
	units, _ := encodeFrames(t, cfg, frames)
	total := 0
	for _, u := range units {
		total += len(u)
	}
	return total
}

func TestSlicesStayCheapOnMovingContent(t *testing.T) {
	var frames [][]byte
	for i := 0; i < 8; i++ {
		frames = append(frames, panningFrame(320, 240, i))
	}
	base := Config{Width: 320, Height: 240, FPSNum: 30, FPSDen: 1, GOPSize: 60,
		QP: 22, RefFrames: 1, Slices: 1, CABAC: true}
	one := codedSize(t, base, frames)
	for _, slices := range []int{2, 4, 8} {
		cfg := base
		cfg.Slices = slices
		got := codedSize(t, cfg, frames)
		grew := 100 * float64(got-one) / float64(one)
		t.Logf("%d slices cost %d bytes against one slice's %d, %+.1f%%", slices, got, one, grew)
		if grew > 25 {
			t.Fatalf("%d slices cost %+.1f%% more than one slice on panning content. Cutting prediction at a slice boundary should cost a few per cent; this much means a macroblock at the top of a slice has lost its motion vector predictor and the search is starting from nothing, which then propagates down the slice",
				slices, grew)
		}
	}
}

func TestCarriedMotionHelpsRatherThanHurts(t *testing.T) {
	var panning, synth [][]byte
	for i := 0; i < 8; i++ {
		panning = append(panning, panningFrame(320, 240, i))
		synth = append(synth, syntheticFrame(320, 240, i))
	}
	cfg := Config{Width: 320, Height: 240, FPSNum: 30, FPSDen: 1, GOPSize: 60,
		QP: 22, RefFrames: 1, Slices: 1, CABAC: true}
	for _, c := range []struct {
		name   string
		frames [][]byte
		ceil   int
	}{
		{"panning", panning, 42000},
		{"synthetic", synth, 92000},
	} {
		got := codedSize(t, cfg, c.frames)
		t.Logf("%s codes to %d bytes", c.name, got)
		if got > c.ceil {
			t.Fatalf("%s codes to %d bytes, above the %d the reference picture's own motion field brought it to; the search has lost a starting candidate",
				c.name, got, c.ceil)
		}
	}
}
