package encoder

import (
	"fmt"
	"image"
	"testing"

	"github.com/oops1/go.264/internal/frame"
)

func (e *Encoder) markingForTest() refMarking { return e.mark }

func (e *Encoder) longTermForTest() []*frame.Picture { return e.appendLongTerm(nil) }

func desktopBackground(w, h, theme int) []byte {
	buf := make([]byte, w*h*3/2)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			v := 214
			if (x/4+y/3+theme)%7 == 0 {
				v = 32
			}
			if (x*x+y*7+theme*13)%23 < 3 {
				v = 130
			}
			if y%24 < 2 {
				v = 96
			}
			buf[y*w+x] = byte(v)
		}
	}
	cw, ch := w/2, h/2
	for i := 0; i < 2; i++ {
		base := w*h + i*cw*ch
		for j := 0; j < cw*ch; j++ {
			buf[base+j] = 128
		}
	}
	return buf
}

func paintWindow(buf []byte, w, h int, r image.Rectangle, t int) {
	for y := r.Min.Y; y < r.Max.Y && y < h; y++ {
		for x := r.Min.X; x < r.Max.X && x < w; x++ {
			if x < 0 || y < 0 {
				continue
			}
			buf[y*w+x] = byte((x*5 + y*3 + t*29) % 256)
		}
	}
	cw, ch := w/2, h/2
	for y := r.Min.Y / 2; y < r.Max.Y/2 && y < ch; y++ {
		for x := r.Min.X / 2; x < r.Max.X/2 && x < cw; x++ {
			if x < 0 || y < 0 {
				continue
			}
			buf[w*h+y*cw+x] = byte(90 + (x+t)%40)
			buf[w*h+cw*ch+y*cw+x] = byte(160 - (y+t)%40)
		}
	}
}

func windowRect(w, h, t int) image.Rectangle {
	cols := (w - 128) / 16
	rows := (h - 96) / 32
	if cols < 1 {
		cols = 1
	}
	if rows < 1 {
		rows = 1
	}
	x := 16 * (t % cols)
	y := 32 * ((t / cols) % rows)
	return image.Rect(x, y, x+128, y+96)
}

func screenContent(w, h, frames int) [][]byte {
	base := desktopBackground(w, h, 0)
	out := make([][]byte, 0, frames)
	for t := 0; t < frames; t++ {
		f := append([]byte(nil), base...)
		paintWindow(f, w, h, windowRect(w, h, t), t)
		out = append(out, f)
	}
	return out
}

func encodeLongTermStream(t *testing.T, cfg Config, frames [][]byte, hints []Hints) ([][]byte, []*frame.Picture, []byte) {
	t.Helper()
	enc, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	var units [][]byte
	var recons []*frame.Picture
	var stream []byte
	for i, f := range frames {
		var h Hints
		if hints != nil {
			h = hints[i]
		}
		pkt, err := enc.EncodeWithHints(f, h)
		if err != nil {
			t.Fatalf("frame %d: %v", i, err)
		}
		units = append(units, pkt)
		stream = append(stream, pkt...)
		rec := enc.Reconstruction()
		snap := frame.NewPicture((cfg.Width+15)/16, (cfg.Height+15)/16)
		copy(snap.Y, rec.Y)
		copy(snap.Cb, rec.Cb)
		copy(snap.Cr, rec.Cr)
		snap.Width = rec.Width
		snap.Height = rec.Height
		recons = append(recons, snap)
	}
	rest, err := enc.Flush()
	if err != nil {
		t.Fatalf("Flush: %v", err)
	}
	if len(rest) != 0 {
		units = append(units, rest)
		stream = append(stream, rest...)
	}
	return units, recons, stream
}

func streamPSNR(t *testing.T, pics []*frame.Picture, frames [][]byte) float64 {
	t.Helper()
	if len(pics) != len(frames) {
		t.Fatalf("decoded %d pictures against %d sources", len(pics), len(frames))
	}
	var total float64
	for i := range pics {
		got := make([]byte, pics[i].Size())
		pics[i].CopyOut(got)
		total += psnr(got, frames[i])
	}
	return total / float64(len(pics))
}

func longTermConfig(slots int) Config {
	return Config{
		Width: 320, Height: 240, FPSNum: 25, FPSDen: 1,
		GOPSize: 1000, QP: 26, RefFrames: 1,
		LongTermReferences: slots,
	}
}

func TestLongTermReferencesAreOffByDefault(t *testing.T) {
	cfg := longTermConfig(0)
	enc, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if got := enc.SPS().MaxNumRefFrames; got != 1 {
		t.Fatalf("max_num_ref_frames is %d with the feature off, want 1", got)
	}
	frames := screenContent(cfg.Width, cfg.Height, 6)
	for i, f := range frames {
		if _, err := enc.Encode(f); err != nil {
			t.Fatalf("frame %d: %v", i, err)
		}
		if m := enc.markingForTest(); m.adaptive {
			t.Fatalf("frame %d planned adaptive marking although the feature is off", i)
		}
	}
	if n := len(enc.longTermForTest()); n != 0 {
		t.Fatalf("%d long-term references were kept with the feature off", n)
	}
}

func TestLongTermReferencesEnlargeTheDecodedPictureBuffer(t *testing.T) {
	cfg := longTermConfig(2)
	cfg.RefFrames = 3
	enc, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if got := enc.SPS().MaxNumRefFrames; got != 5 {
		t.Fatalf("max_num_ref_frames is %d, want 5 for three short-term and two long-term slots", got)
	}
}

func TestLongTermConfigurationIsChecked(t *testing.T) {
	if _, err := New(longTermConfig(-1)); err == nil {
		t.Fatal("a negative slot count was accepted")
	}
	cfg := longTermConfig(12)
	cfg.RefFrames = 8
	if _, err := New(cfg); err == nil {
		t.Fatal("a reference count above sixteen was accepted")
	}
}

func TestLongTermMarkingRoundTrips(t *testing.T) {
	frames := screenContent(320, 240, 24)
	cases := []struct {
		name string
		cfg  func(Config) Config
	}{
		{"cavlc", func(c Config) Config { return c }},
		{"cabac", func(c Config) Config { c.CABAC = true; return c }},
		{"transform8x8", func(c Config) Config { c.CABAC = true; c.Transform8x8 = true; return c }},
		{"slices", func(c Config) Config { c.Slices = 3; return c }},
		{"trellis", func(c Config) Config { c.CABAC = true; c.Trellis = true; return c }},
		{"weighted", func(c Config) Config { c.WeightedPrediction = WeightedPredictionExplicit; return c }},
		{"refresh", func(c Config) Config { c.IntraRefresh = 8; return c }},
		{"threerefs", func(c Config) Config { c.RefFrames = 3; return c }},
		{"twoslots", func(c Config) Config { c.LongTermReferences = 2; return c }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := tc.cfg(longTermConfig(1))
			units, recons, _ := encodeLongTermStream(t, cfg, frames, nil)
			assertMatchesReconstruction(t, tc.name, decodeUnits(t, units), recons)
		})
	}
}

func TestLongTermMarkingRoundTripsWithBPictures(t *testing.T) {
	for _, cabac := range []bool{false, true} {
		for _, bframes := range []int{1, 3} {
			cfg := longTermConfig(1)
			cfg.RefFrames = 2
			cfg.BFrames = bframes
			cfg.CABAC = cabac
			frames := screenContent(cfg.Width, cfg.Height, 20)
			stream, recons := encodeBStream(t, cfg, frames, nil)
			label := fmt.Sprintf("cabac %v, %d B pictures with a long-term slot", cabac, bframes)
			assertMatchesReconstruction(t, label, decodeUnits(t, [][]byte{stream}), recons)

			ref := decodeWithFFmpeg(t, stream)
			size := cfg.Width * cfg.Height * 3 / 2
			if len(ref) != size*len(frames) {
				t.Fatalf("%s: ffmpeg produced %d bytes, want %d", label, len(ref), size*len(frames))
			}
			for i := range recons {
				want := make([]byte, recons[i].Size())
				recons[i].CopyOut(want)
				got := ref[i*size : (i+1)*size]
				for j := range want {
					if got[j] != want[j] {
						t.Fatalf("%s frame %d: ffmpeg disagrees at sample %d, ffmpeg %d ours %d",
							label, i, j, got[j], want[j])
					}
				}
			}
		}
	}
}

func TestLongTermMarkingRoundTripsAcrossQuantisers(t *testing.T) {
	frames := screenContent(320, 240, 12)
	for _, qp := range []int{0, 1, 18, 26, 37, 51} {
		cfg := longTermConfig(1)
		cfg.QP = qp
		cfg.CABAC = qp%2 == 0
		units, recons, _ := encodeLongTermStream(t, cfg, frames, nil)
		assertMatchesReconstruction(t, "quantiser", decodeUnits(t, units), recons)
	}
}

func TestLongTermMarkingRoundTripsWithTheBufferModel(t *testing.T) {
	frames := screenContent(320, 240, 20)
	cfg := longTermConfig(1)
	cfg.CABAC = true
	cfg.BitrateKbps = 400
	cfg.VBVBufferKbits = 400
	cfg.VBVMaxrateKbps = 400
	cfg.CBR = true
	units, recons, _ := encodeLongTermStream(t, cfg, frames, nil)
	assertMatchesReconstruction(t, "buffer model", decodeUnits(t, units), recons)
}

func TestLongTermSlotIsSeededFromTheKeyFrame(t *testing.T) {
	cfg := longTermConfig(1)
	frames := screenContent(cfg.Width, cfg.Height, 4)
	enc, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := enc.Encode(frames[0]); err != nil {
		t.Fatal(err)
	}
	if _, err := enc.Encode(frames[1]); err != nil {
		t.Fatal(err)
	}
	m := enc.markingForTest()
	if !m.adaptive {
		t.Fatal("the picture after the key frame did not carry adaptive marking")
	}
	var ops []uint32
	for _, op := range m.mmcos {
		ops = append(ops, op.Op)
	}
	if len(ops) < 2 || ops[0] != 4 || ops[1] != 3 {
		t.Fatalf("the operations were %v, want the long-term index limit followed by a promotion", ops)
	}
	if n := len(enc.longTermForTest()); n != 1 {
		t.Fatalf("%d long-term references were kept, want 1", n)
	}
}

func TestLongTermSlotIsRefreshedWhenTheDesktopChanges(t *testing.T) {
	cfg := longTermConfig(1)
	cfg.CABAC = true
	first := desktopBackground(cfg.Width, cfg.Height, 0)
	second := desktopBackground(cfg.Width, cfg.Height, 3)
	var frames [][]byte
	for i := 0; i < 24; i++ {
		if i < 12 {
			frames = append(frames, first)
		} else {
			frames = append(frames, second)
		}
	}
	enc, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	promotions := 0
	releases := 0
	for i, f := range frames {
		if _, err := enc.Encode(f); err != nil {
			t.Fatalf("frame %d: %v", i, err)
		}
		for _, op := range enc.markingForTest().mmcos {
			switch op.Op {
			case 2:
				releases++
			case 3:
				promotions++
			}
		}
	}
	if promotions < 2 || releases < 1 {
		t.Fatalf("the slot was promoted %d times and released %d times, so a wholly new desktop never refreshed it",
			promotions, releases)
	}
	t.Logf("%d promotions, %d releases over %d frames", promotions, releases, len(frames))
}

func TestLongTermMarkingNeverMixesWithTheSlidingWindow(t *testing.T) {
	cfg := longTermConfig(2)
	cfg.RefFrames = 2
	frames := screenContent(cfg.Width, cfg.Height, 30)
	enc, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	for i, f := range frames {
		if _, err := enc.Encode(f); err != nil {
			t.Fatalf("frame %d: %v", i, err)
		}
		m := enc.markingForTest()
		if !m.adaptive {
			continue
		}
		if len(m.mmcos) == 0 {
			t.Fatalf("frame %d claimed adaptive marking with no operations", i)
		}
		if got := len(enc.longTermForTest()); got > cfg.LongTermReferences {
			t.Fatalf("frame %d left %d long-term references, want at most %d",
				i, got, cfg.LongTermReferences)
		}
		var promotions, limits int
		for _, op := range m.mmcos {
			switch op.Op {
			case 3:
				promotions++
			case 4:
				limits++
			case 1, 2:
			default:
				t.Fatalf("frame %d emitted an unexpected operation %d", i, op.Op)
			}
			if op.Op == 1 || op.Op == 3 {
				if op.DifferenceOfPicNumsMinus1 > 16 {
					t.Fatalf("frame %d named a picture %d places back", i, op.DifferenceOfPicNumsMinus1+1)
				}
			}
		}
		if promotions != 1 {
			t.Fatalf("frame %d emitted %d promotions, want exactly one", i, promotions)
		}
		if limits > 1 {
			t.Fatalf("frame %d emitted the index limit %d times", i, limits)
		}
	}
}

func TestLongTermMarkingSurvivesAKeyFrame(t *testing.T) {
	cfg := longTermConfig(1)
	cfg.GOPSize = 8
	frames := screenContent(cfg.Width, cfg.Height, 24)
	units, recons, _ := encodeLongTermStream(t, cfg, frames, nil)
	assertMatchesReconstruction(t, "key frames", decodeUnits(t, units), recons)
}

func TestLongTermMarkingFollowsTheChangeHints(t *testing.T) {
	cfg := longTermConfig(2)
	frames := screenContent(cfg.Width, cfg.Height, 16)
	hints := make([]Hints, len(frames))
	for i := range frames {
		if i == 0 {
			continue
		}
		hints[i] = Hints{Changed: []image.Rectangle{windowRect(cfg.Width, cfg.Height, i-1).Union(
			windowRect(cfg.Width, cfg.Height, i))}}
	}
	units, recons, _ := encodeLongTermStream(t, cfg, frames, hints)
	assertMatchesReconstruction(t, "hints", decodeUnits(t, units), recons)
}

func screenContentWithDither(w, h, frames int) [][]byte {
	out := screenContent(w, h, frames)
	for t, f := range out {
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				if (x*7+y*11+t*13)%17 != 0 {
					continue
				}
				v := int(f[y*w+x])
				if (x+y+t)%2 == 0 {
					v++
				} else {
					v--
				}
				f[y*w+x] = byte(clip3(0, 255, v))
			}
		}
	}
	return out
}

func TestLongTermReferenceOnScreenContent(t *testing.T) {
	for _, content := range []struct {
		name   string
		frames [][]byte
	}{
		{"a still desktop behind a moving window", screenContent(320, 240, 60)},
		{"the same with the desktop dithered every frame", screenContentWithDither(320, 240, 60)},
	} {
		t.Run(content.name, func(t *testing.T) {
			measureLongTermArms(t, content.frames)
		})
	}
}

func measureLongTermArms(t *testing.T, source [][]byte) {
	t.Helper()
	arms := []struct {
		name  string
		refs  int
		slots int
	}{
		{"one short-term reference", 1, 0},
		{"two short-term references", 2, 0},
		{"one short-term and one long-term", 1, 1},
	}
	type result struct {
		bytes int
		psnr  float64
	}
	got := make([]result, len(arms))
	for i, arm := range arms {
		cfg := longTermConfig(arm.slots)
		cfg.RefFrames = arm.refs
		cfg.CABAC = true
		units, _, stream := encodeLongTermStream(t, cfg, source, nil)
		pics := decodeUnits(t, units)
		got[i] = result{bytes: len(stream), psnr: streamPSNR(t, pics, source)}
	}
	for i, arm := range arms {
		change := 100 * float64(got[i].bytes-got[0].bytes) / float64(got[0].bytes)
		t.Logf("%-34s %7d bytes  %6.2f dB  %+6.1f%%", arm.name, got[i].bytes, got[i].psnr, change)
	}
	if got[2].psnr < got[1].psnr-0.25 {
		t.Fatalf("the long-term reference cost %.2f dB against the same buffer size, %.2f against %.2f",
			got[1].psnr-got[2].psnr, got[1].psnr, got[2].psnr)
	}
}

func TestFFmpegDecodesLongTermMarkingIdentically(t *testing.T) {
	source := screenContent(320, 240, 24)
	for _, cabac := range []bool{false, true} {
		cfg := longTermConfig(2)
		cfg.RefFrames = 2
		cfg.CABAC = cabac
		units, recons, stream := encodeLongTermStream(t, cfg, source, nil)
		assertMatchesReconstruction(t, "ours", decodeUnits(t, units), recons)

		ref := decodeWithFFmpeg(t, stream)
		size := cfg.Width * cfg.Height * 3 / 2
		if len(ref) != size*len(source) {
			t.Fatalf("cabac %v: ffmpeg produced %d bytes, want %d", cabac, len(ref), size*len(source))
		}
		for i := range recons {
			want := make([]byte, recons[i].Size())
			recons[i].CopyOut(want)
			got := ref[i*size : (i+1)*size]
			for j := range want {
				if got[j] != want[j] {
					t.Fatalf("cabac %v frame %d: ffmpeg and our reconstruction disagree at sample %d, ffmpeg %d ours %d",
						cabac, i, j, got[j], want[j])
				}
			}
		}
		t.Logf("cabac %v: %d bytes, %d frames decode identically in ffmpeg", cabac, len(stream), len(source))
	}
}

func TestLongTermOperationsAreWellFormed(t *testing.T) {
	cfg := longTermConfig(2)
	cfg.RefFrames = 2
	frames := screenContent(cfg.Width, cfg.Height, 40)
	enc, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	seenLimit := false
	for i, f := range frames {
		if _, err := enc.Encode(f); err != nil {
			t.Fatalf("frame %d: %v", i, err)
		}
		for _, op := range enc.markingForTest().mmcos {
			if op.Op == 4 {
				if seenLimit {
					t.Fatalf("frame %d repeated the long-term index limit", i)
				}
				seenLimit = true
				if op.MaxLongTermFrameIdxPlus1 != uint32(cfg.LongTermReferences) {
					t.Fatalf("frame %d set the limit to %d, want %d",
						i, op.MaxLongTermFrameIdxPlus1, cfg.LongTermReferences)
				}
			}
			if op.Op == 3 && op.LongTermFrameIdx >= uint32(cfg.LongTermReferences) {
				t.Fatalf("frame %d promoted into slot %d, outside 0..%d",
					i, op.LongTermFrameIdx, cfg.LongTermReferences-1)
			}
		}
	}
	if !seenLimit {
		t.Fatal("the long-term index limit was never sent")
	}
}
