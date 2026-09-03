package encoder

import (
	"bytes"
	"fmt"
	"runtime"
	"testing"
	"time"

	"github.com/oops1/go.264/internal/frame"
	"github.com/oops1/go.264/internal/nal"
)

func slicedConfig(w, h, qp, slices int, cabac bool) Config {
	return Config{Width: w, Height: h, FPSNum: 25, FPSDen: 1, GOPSize: 8, QP: qp,
		RefFrames: 1, Slices: slices, CABAC: cabac}
}

func countSliceUnits(t *testing.T, pkt []byte) int {
	t.Helper()
	n := 0
	for _, ebsp := range nal.SplitAnnexB(pkt) {
		u, err := nal.Parse(ebsp)
		if err != nil {
			t.Fatalf("parsing our own unit: %v", err)
		}
		if u.Header.Type == nal.TypeSliceIDR || u.Header.Type == nal.TypeSliceNonIDR {
			n++
		}
	}
	return n
}

func encodeFrames(t *testing.T, cfg Config, frames [][]byte) ([][]byte, []*frame.Picture) {
	t.Helper()
	enc, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	var units [][]byte
	var recons []*frame.Picture
	for i, f := range frames {
		pkt, err := enc.Encode(f)
		if err != nil {
			t.Fatalf("frame %d: %v", i, err)
		}
		units = append(units, pkt)
		recons = append(recons, snapshotOf(cfg, enc.Reconstruction()))
	}
	return units, recons
}

func TestSlicedPicturesRoundTrip(t *testing.T) {
	for _, cabac := range []bool{false, true} {
		for _, slices := range []int{2, 3, 4, 9} {
			for _, qp := range []int{0, 26, 45} {
				cfg := slicedConfig(176, 144, qp, slices, cabac)
				var frames [][]byte
				for i := 0; i < 5; i++ {
					frames = append(frames, syntheticFrame(cfg.Width, cfg.Height, i))
				}
				units, recons := encodeFrames(t, cfg, frames)
				label := fmt.Sprintf("cabac %v, %d slices, qp %d", cabac, slices, qp)
				assertMatchesReconstruction(t, label, decodeUnits(t, units), recons)
			}
		}
	}
}

func TestEveryRequestedSliceReachesTheStream(t *testing.T) {
	cfg := slicedConfig(176, 144, 26, 3, false)
	units, _ := encodeFrames(t, cfg, [][]byte{
		syntheticFrame(cfg.Width, cfg.Height, 0),
		syntheticFrame(cfg.Width, cfg.Height, 1),
	})
	for i, pkt := range units {
		if got := countSliceUnits(t, pkt); got != 3 {
			t.Fatalf("frame %d carries %d slice units, want 3", i, got)
		}
	}
}

func TestSliceCountIsClampedToTheRowCount(t *testing.T) {
	cfg := slicedConfig(176, 144, 26, 64, false)
	units, recons := encodeFrames(t, cfg, [][]byte{syntheticFrame(cfg.Width, cfg.Height, 0)})
	rows := (cfg.Height + 15) / 16
	if got := countSliceUnits(t, units[0]); got != rows {
		t.Fatalf("asking for 64 slices in a picture of %d macroblock rows produced %d slices", rows, got)
	}
	assertMatchesReconstruction(t, "clamped", decodeUnits(t, units), recons)
}

func TestOneSliceIsStillOneSlice(t *testing.T) {
	for _, slices := range []int{0, 1} {
		cfg := slicedConfig(176, 144, 26, slices, false)
		units, _ := encodeFrames(t, cfg, [][]byte{syntheticFrame(cfg.Width, cfg.Height, 0)})
		if got := countSliceUnits(t, units[0]); got != 1 {
			t.Fatalf("Slices=%d produced %d slice units, want 1", slices, got)
		}
	}
}

func TestParallelSlicesProduceTheSameBytesEveryRun(t *testing.T) {
	cfg := slicedConfig(320, 240, 27, 6, true)
	var frames [][]byte
	for i := 0; i < 6; i++ {
		frames = append(frames, syntheticFrame(cfg.Width, cfg.Height, i))
	}
	first, _ := encodeFrames(t, cfg, frames)
	for run := 0; run < 3; run++ {
		again, _ := encodeFrames(t, cfg, frames)
		for i := range first {
			if !bytes.Equal(first[i], again[i]) {
				t.Fatalf("run %d frame %d differs from the first run, so the parallel encode is not deterministic",
					run, i)
			}
		}
	}
}

func TestSlicesCostBitsButNotCorrectness(t *testing.T) {
	var sizes []int
	for _, slices := range []int{1, 4} {
		cfg := slicedConfig(320, 240, 26, slices, false)
		var frames [][]byte
		for i := 0; i < 6; i++ {
			frames = append(frames, syntheticFrame(cfg.Width, cfg.Height, i))
		}
		units, recons := encodeFrames(t, cfg, frames)
		assertMatchesReconstruction(t, fmt.Sprintf("%d slices", slices), decodeUnits(t, units), recons)
		total := 0
		for _, u := range units {
			total += len(u)
		}
		sizes = append(sizes, total)
	}
	if sizes[1] < sizes[0] {
		t.Fatalf("four slices cost %d bytes against one slice's %d; cutting prediction at a boundary cannot save bits",
			sizes[1], sizes[0])
	}
	t.Logf("one slice %d bytes, four slices %d bytes, %.1f%% more",
		sizes[0], sizes[1], 100*float64(sizes[1]-sizes[0])/float64(sizes[0]))
}

func TestSlicedEncodingWithHints(t *testing.T) {
	cfg := slicedConfig(320, 240, 30, 4, true)
	frames := [][]byte{syntheticFrame(cfg.Width, cfg.Height, 0)}
	hints := []Hints{{}}
	for i := 1; i < 6; i++ {
		frames = append(frames, syntheticFrame(cfg.Width, cfg.Height, i))
		hints = append(hints, Hints{Changed: nil})
	}
	units, recons := encodeWithHints(t, cfg, frames, hints)
	assertMatchesReconstruction(t, "sliced with hints", decodeUnits(t, units), recons)
}

func TestNegativeSliceCountFollowsTheProcessorCount(t *testing.T) {
	cfg := slicedConfig(320, 240, 26, -1, false)
	units, recons := encodeFrames(t, cfg, []([]byte){
		syntheticFrame(cfg.Width, cfg.Height, 0),
		syntheticFrame(cfg.Width, cfg.Height, 1),
	})
	rows := (cfg.Height + 15) / 16
	want := automaticSliceCount(runtime.GOMAXPROCS(0), rows)
	if got := countSliceUnits(t, units[0]); got != want {
		t.Fatalf("Slices=-1 produced %d slices on a machine with GOMAXPROCS %d and %d macroblock rows",
			got, runtime.GOMAXPROCS(0), rows)
	}
	assertMatchesReconstruction(t, "automatic slice count", decodeUnits(t, units), recons)
}

func TestTheAutomaticSliceCountOvershootsTheProcessors(t *testing.T) {
	cases := []struct{ procs, rows, want int }{
		{1, 68, 2},
		{4, 68, 8},
		{20, 68, 40},
		{20, 30, 30},
		{36, 68, 68},
		{64, 9, 9},
		{0, 68, 2},
	}
	for _, c := range cases {
		if got := automaticSliceCount(c.procs, c.rows); got != c.want {
			t.Errorf("%d processors and %d macroblock rows gave %d slices, want %d",
				c.procs, c.rows, got, c.want)
		}
	}
}

func TestTheAutomaticSliceCountBeatsOnePerProcessor(t *testing.T) {
	if testing.Short() {
		t.Skip("this measures encoding time")
	}
	const w, h = 640, 480
	frames := make([][]byte, 8)
	for i := range frames {
		frames[i] = syntheticFrame(w, h, i)
	}
	measure := func(slices int) time.Duration {
		cfg := Config{Width: w, Height: h, FPSNum: 30, FPSDen: 1, GOPSize: 1000, QP: 26,
			RefFrames: 1, Slices: slices}
		enc, err := New(cfg)
		if err != nil {
			t.Fatal(err)
		}
		for i := 0; i < 3; i++ {
			if _, err := enc.Encode(frames[i%len(frames)]); err != nil {
				t.Fatal(err)
			}
		}
		start := time.Now()
		for i := 0; i < 12; i++ {
			if _, err := enc.Encode(frames[i%len(frames)]); err != nil {
				t.Fatal(err)
			}
		}
		return time.Since(start)
	}
	rows := (h + 15) / 16
	procs := runtime.GOMAXPROCS(0)
	if procs >= rows {
		t.Skipf("this machine has %d processors for %d macroblock rows, so the two settings coincide", procs, rows)
	}
	auto := measure(-1)
	onePer := measure(procs)
	t.Logf("one slice per processor (%d) took %v, the automatic count (%d) took %v",
		procs, onePer, automaticSliceCount(procs, rows), auto)
	if auto > onePer*6/5 {
		t.Fatalf("the automatic slice count is more than a fifth slower than one slice per processor: %v against %v",
			auto, onePer)
	}
}
