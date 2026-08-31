package encoder

import (
	"fmt"
	"image"
	"math/rand"
	"testing"

	"github.com/oops1/go.264/internal/frame"
)

func cabacConfig(qp, gop int) Config {
	return Config{Width: 176, Height: 144, FPSNum: 25, FPSDen: 1, GOPSize: gop, QP: qp,
		RefFrames: 1, CABAC: true}
}

func snapshotOf(cfg Config, rec *frame.Picture) *frame.Picture {
	snapshot := frame.NewPicture((cfg.Width+15)/16, (cfg.Height+15)/16)
	copy(snapshot.Y, rec.Y)
	copy(snapshot.Cb, rec.Cb)
	copy(snapshot.Cr, rec.Cr)
	snapshot.Width = rec.Width
	snapshot.Height = rec.Height
	return snapshot
}

func TestCABACIntraSlicesRoundTrip(t *testing.T) {
	for _, qp := range []int{0, 12, 26, 40, 51} {
		cfg := cabacConfig(qp, 1)
		var frames [][]byte
		for i := 0; i < 3; i++ {
			frames = append(frames, syntheticFrame(cfg.Width, cfg.Height, i))
		}
		pics, _, recons := encodeAndDecode(t, cfg, frames)
		assertMatchesReconstruction(t, fmt.Sprintf("intra qp %d", qp), pics, recons)
	}
}

func TestCABACPredictedSlicesRoundTrip(t *testing.T) {
	for _, qp := range []int{0, 18, 26, 37, 51} {
		cfg := cabacConfig(qp, 12)
		var frames [][]byte
		for i := 0; i < 8; i++ {
			frames = append(frames, syntheticFrame(cfg.Width, cfg.Height, i))
		}
		pics, _, recons := encodeAndDecode(t, cfg, frames)
		assertMatchesReconstruction(t, fmt.Sprintf("predicted qp %d", qp), pics, recons)
	}
}

func TestCABACSurvivesNoise(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	cfg := cabacConfig(22, 6)
	var frames [][]byte
	for i := 0; i < 6; i++ {
		frames = append(frames, noisyFrame(rng, cfg.Width, cfg.Height))
	}
	pics, _, recons := encodeAndDecode(t, cfg, frames)
	assertMatchesReconstruction(t, "noise", pics, recons)
}

func TestCABACWithMultipleReferences(t *testing.T) {
	cfg := cabacConfig(28, 16)
	cfg.RefFrames = 4
	var frames [][]byte
	for i := 0; i < 10; i++ {
		frames = append(frames, syntheticFrame(cfg.Width, cfg.Height, i))
	}
	pics, _, recons := encodeAndDecode(t, cfg, frames)
	assertMatchesReconstruction(t, "four references", pics, recons)
}

func TestCABACWithEveryPartitioning(t *testing.T) {
	cfg := cabacConfig(24, 20)
	cfg.RefFrames = 2
	enc, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	var units [][]byte
	var recons []*frame.Picture
	for i := 0; i < 12; i++ {
		pkt, err := enc.Encode(syntheticFrame(cfg.Width, cfg.Height, i))
		if err != nil {
			t.Fatalf("frame %d: %v", i, err)
		}
		units = append(units, pkt)
		recons = append(recons, snapshotOf(cfg, enc.Reconstruction()))
	}
	hist := kindHistogram(enc)
	for _, kind := range []int{mbTypeP16x16, mbTypeP16x8, mbTypeP8x16, mbTypeP8x8} {
		if hist[kind] == 0 {
			t.Fatalf("partitioning %d never appeared in the last frame, so its CABAC writer is untested", kind)
		}
	}
	assertMatchesReconstruction(t, "partitionings", decodeUnits(t, units), recons)
}

func TestCABACWithHints(t *testing.T) {
	cfg := cabacConfig(30, 40)
	frames := [][]byte{syntheticFrame(cfg.Width, cfg.Height, 0)}
	hints := []Hints{{}}
	for i := 1; i < 8; i++ {
		frames = append(frames, syntheticFrame(cfg.Width, cfg.Height, i))
		band := image.Rect(0, (i*16)%cfg.Height, cfg.Width, (i*16)%cfg.Height+32)
		hints = append(hints, Hints{
			Changed: []image.Rectangle{band},
			Regions: []Region{{Rect: image.Rect(0, 0, 64, 64), Kind: RegionText}},
		})
	}
	units, recons := encodeWithHints(t, cfg, frames, hints)
	assertMatchesReconstruction(t, "hinted CABAC", decodeUnits(t, units), recons)
}

func TestCABACHandlesAnUnchangedScreen(t *testing.T) {
	cfg := cabacConfig(26, 100)
	base := syntheticFrame(cfg.Width, cfg.Height, 0)
	frames := [][]byte{base, base, base, base}
	hints := []Hints{{},
		{Changed: []image.Rectangle{}},
		{Changed: []image.Rectangle{}},
		{Changed: []image.Rectangle{}},
	}
	units, recons := encodeWithHints(t, cfg, frames, hints)
	assertMatchesReconstruction(t, "unchanged CABAC", decodeUnits(t, units), recons)
	for i := 1; i < len(units); i++ {
		if len(units[i]) > len(units[0])/20 {
			t.Fatalf("frame %d cost %d bytes against %d for the key frame, which is far from the near nothing a still screen should cost",
				i, len(units[i]), len(units[0]))
		}
	}
}

func TestCABACIsSmallerThanCAVLC(t *testing.T) {
	for _, qp := range []int{18, 26, 34} {
		var sizes [2]int
		for i, on := range []bool{false, true} {
			cfg := cabacConfig(qp, 12)
			cfg.CABAC = on
			enc, err := New(cfg)
			if err != nil {
				t.Fatal(err)
			}
			for n := 0; n < 10; n++ {
				pkt, err := enc.Encode(syntheticFrame(cfg.Width, cfg.Height, n))
				if err != nil {
					t.Fatal(err)
				}
				sizes[i] += len(pkt)
			}
		}
		if sizes[1] >= sizes[0] {
			t.Fatalf("qp %d: CABAC produced %d bytes against CAVLC's %d, so the arithmetic coder is not paying for itself",
				qp, sizes[1], sizes[0])
		}
		t.Logf("qp %d: CAVLC %d bytes, CABAC %d bytes, saving %.1f%%",
			qp, sizes[0], sizes[1], 100*float64(sizes[0]-sizes[1])/float64(sizes[0]))
	}
}

func TestCABACStreamsDeclareTheMainProfile(t *testing.T) {
	enc, err := New(cabacConfig(26, 1))
	if err != nil {
		t.Fatal(err)
	}
	if got := enc.SPS().ProfileIDC; got != 77 {
		t.Fatalf("a CABAC stream announced profile_idc %d, but CABAC is not permitted below Main", got)
	}
	plain, err := New(Config{Width: 176, Height: 144, FPSNum: 25, FPSDen: 1, GOPSize: 1, QP: 26})
	if err != nil {
		t.Fatal(err)
	}
	if got := plain.SPS().ProfileIDC; got != 66 {
		t.Fatalf("a CAVLC stream announced profile_idc %d, want Baseline", got)
	}
}
