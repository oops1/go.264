package encoder

import (
	"image"
	"testing"

	"github.com/oops1/go.264/internal/decoder"
	"github.com/oops1/go.264/internal/frame"
)

func flatFrame(w, h int, luma, cb, cr byte) []byte {
	f := make([]byte, w*h*3/2)
	for i := 0; i < w*h; i++ {
		f[i] = luma
	}
	cw, ch := w/2, h/2
	for i := 0; i < cw*ch; i++ {
		f[w*h+i] = cb
		f[w*h+cw*ch+i] = cr
	}
	return f
}

func paintRect(f []byte, w, h int, r image.Rectangle, luma byte) {
	for y := r.Min.Y; y < r.Max.Y && y < h; y++ {
		for x := r.Min.X; x < r.Max.X && x < w; x++ {
			f[y*w+x] = luma
		}
	}
}

func encodeWithHints(t *testing.T, cfg Config, frames [][]byte, hints []Hints) ([][]byte, []*frame.Picture) {
	t.Helper()
	enc, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	var units [][]byte
	var recons []*frame.Picture
	for i, f := range frames {
		pkt, err := enc.EncodeWithHints(f, hints[i])
		if err != nil {
			t.Fatalf("frame %d: EncodeWithHints: %v", i, err)
		}
		units = append(units, pkt)
		rec := enc.Reconstruction()
		snapshot := frame.NewPicture((cfg.Width+15)/16, (cfg.Height+15)/16)
		copy(snapshot.Y, rec.Y)
		copy(snapshot.Cb, rec.Cb)
		copy(snapshot.Cr, rec.Cr)
		snapshot.Width = rec.Width
		snapshot.Height = rec.Height
		recons = append(recons, snapshot)
	}
	return units, recons
}

func decodeUnits(t *testing.T, units [][]byte) []*frame.Picture {
	t.Helper()
	var stream []byte
	for _, u := range units {
		stream = append(stream, u...)
	}
	d := decoder.New()
	pics, err := d.Decode(stream)
	if err != nil {
		t.Fatalf("decoding our own stream: %v", err)
	}
	rest, err := d.Flush()
	if err != nil {
		t.Fatalf("flushing our own stream: %v", err)
	}
	return append(pics, rest...)
}

func assertMatchesReconstruction(t *testing.T, label string, pics, recons []*frame.Picture) {
	t.Helper()
	if len(pics) != len(recons) {
		t.Fatalf("%s: decoded %d pictures against %d reconstructions", label, len(pics), len(recons))
	}
	for i := range pics {
		got := make([]byte, pics[i].Size())
		pics[i].CopyOut(got)
		want := make([]byte, recons[i].Size())
		recons[i].CopyOut(want)
		for j := range got {
			if got[j] != want[j] {
				t.Fatalf("%s frame %d: the decoder and the encoder disagree at sample %d, decoder %d encoder %d",
					label, i, j, got[j], want[j])
			}
		}
	}
}

func hintsConfig(qp int) Config {
	return Config{Width: 176, Height: 144, FPSNum: 25, FPSDen: 1, GOPSize: 100, QP: qp, RefFrames: 1}
}

func TestAnUnchangedFrameCostsAlmostNothing(t *testing.T) {
	cfg := hintsConfig(26)
	base := syntheticFrame(cfg.Width, cfg.Height, 0)
	frames := [][]byte{base, base, base, base}
	hints := []Hints{{}, {Changed: []image.Rectangle{}}, {Changed: []image.Rectangle{}}, {Changed: []image.Rectangle{}}}

	units, recons := encodeWithHints(t, cfg, frames, hints)
	assertMatchesReconstruction(t, "unchanged", decodeUnits(t, units), recons)

	for i := 1; i < len(units); i++ {
		if len(units[i]) > len(units[0])/20 {
			t.Fatalf("frame %d cost %d bytes against %d for the key frame, which is not the near nothing an unchanged screen should cost",
				i, len(units[i]), len(units[0]))
		}
	}
	t.Logf("key frame %d bytes, then %d, %d, %d", len(units[0]), len(units[1]), len(units[2]), len(units[3]))
}

func TestOnlyTheChangedRectangleIsCoded(t *testing.T) {
	cfg := hintsConfig(26)
	first := flatFrame(cfg.Width, cfg.Height, 90, 128, 128)
	second := append([]byte(nil), first...)
	changed := image.Rect(32, 32, 64, 64)
	paintRect(second, cfg.Width, cfg.Height, changed, 200)

	narrow, narrowRec := encodeWithHints(t, cfg, [][]byte{first, second},
		[]Hints{{}, {Changed: []image.Rectangle{changed}}})
	assertMatchesReconstruction(t, "narrow", decodeUnits(t, narrow), narrowRec)

	wide, wideRec := encodeWithHints(t, cfg, [][]byte{first, second}, []Hints{{}, {}})
	assertMatchesReconstruction(t, "wide", decodeUnits(t, wide), wideRec)

	if len(narrow[1]) > len(wide[1]) {
		t.Fatalf("naming the changed rectangle cost %d bytes against %d when the whole frame was offered",
			len(narrow[1]), len(wide[1]))
	}
	t.Logf("changed rectangle %d bytes, whole frame %d bytes", len(narrow[1]), len(wide[1]))
}

func TestUntouchedAreaSurvivesUnchanged(t *testing.T) {
	cfg := hintsConfig(30)
	first := flatFrame(cfg.Width, cfg.Height, 90, 128, 128)
	second := append([]byte(nil), first...)
	changed := image.Rect(64, 64, 96, 96)
	paintRect(second, cfg.Width, cfg.Height, changed, 210)

	units, _ := encodeWithHints(t, cfg, [][]byte{first, second},
		[]Hints{{}, {Changed: []image.Rectangle{changed}}})
	pics := decodeUnits(t, units)
	if len(pics) != 2 {
		t.Fatalf("decoded %d pictures, want 2", len(pics))
	}

	size := cfg.Width * cfg.Height * 3 / 2
	before := make([]byte, size)
	after := make([]byte, size)
	pics[0].CopyOut(before)
	pics[1].CopyOut(after)

	for y := 0; y < cfg.Height; y++ {
		for x := 0; x < cfg.Width; x++ {
			if image.Pt(x, y).In(changed.Inset(-16)) {
				continue
			}
			if before[y*cfg.Width+x] != after[y*cfg.Width+x] {
				t.Fatalf("the sample at (%d,%d) moved from %d to %d although it was never named as changed",
					x, y, before[y*cfg.Width+x], after[y*cfg.Width+x])
			}
		}
	}
}

func TestChangedNilStillMeansTheWholeFrame(t *testing.T) {
	cfg := hintsConfig(26)
	first := syntheticFrame(cfg.Width, cfg.Height, 0)
	second := syntheticFrame(cfg.Width, cfg.Height, 1)

	withNil, recNil := encodeWithHints(t, cfg, [][]byte{first, second}, []Hints{{}, {Changed: nil}})
	assertMatchesReconstruction(t, "nil change list", decodeUnits(t, withNil), recNil)

	empty, _ := encodeWithHints(t, cfg, [][]byte{first, second},
		[]Hints{{}, {Changed: []image.Rectangle{}}})
	if len(empty[1]) >= len(withNil[1]) {
		t.Fatalf("an empty change list cost %d bytes and a nil one %d, so the two are not being told apart",
			len(empty[1]), len(withNil[1]))
	}
}

func TestReconstructionHoldsWithHintsAcrossQuantisers(t *testing.T) {
	for _, qp := range []int{0, 18, 32, 45} {
		cfg := hintsConfig(qp)
		frames := [][]byte{syntheticFrame(cfg.Width, cfg.Height, 0)}
		hints := []Hints{{}}
		for i := 1; i < 8; i++ {
			f := syntheticFrame(cfg.Width, cfg.Height, i)
			frames = append(frames, f)
			r := image.Rect(0, (i*16)%cfg.Height, cfg.Width, (i*16)%cfg.Height+32)
			hints = append(hints, Hints{Changed: []image.Rectangle{r}})
		}
		units, recons := encodeWithHints(t, cfg, frames, hints)
		assertMatchesReconstruction(t, "quantiser", decodeUnits(t, units), recons)
		_ = units
	}
}

func TestForceKeyFrameEmitsAnInstantaneousRefresh(t *testing.T) {
	cfg := hintsConfig(26)
	enc, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	frames := [][]byte{
		syntheticFrame(cfg.Width, cfg.Height, 0),
		syntheticFrame(cfg.Width, cfg.Height, 1),
		syntheticFrame(cfg.Width, cfg.Height, 2),
	}
	var sizes []int
	for i, f := range frames {
		if i == 2 {
			enc.ForceKeyFrame()
		}
		pkt, err := enc.Encode(f)
		if err != nil {
			t.Fatalf("frame %d: %v", i, err)
		}
		sizes = append(sizes, len(pkt))
	}
	if sizes[2] <= sizes[1] {
		t.Fatalf("the forced key frame cost %d bytes against %d for the predicted frame before it, so it was not a refresh",
			sizes[2], sizes[1])
	}
	t.Logf("key %d, predicted %d, forced key %d", sizes[0], sizes[1], sizes[2])
}

func TestMotionSearchCanBeTurnedOff(t *testing.T) {
	cfg := hintsConfig(26)
	cfg.MotionSearch = MotionSearchZero
	var frames [][]byte
	hints := []Hints{}
	for i := 0; i < 6; i++ {
		frames = append(frames, syntheticFrame(cfg.Width, cfg.Height, i))
		hints = append(hints, Hints{})
	}
	units, recons := encodeWithHints(t, cfg, frames, hints)
	assertMatchesReconstruction(t, "without motion search", decodeUnits(t, units), recons)
}

func TestRegionsMoveTheQuantiserWhereAsked(t *testing.T) {
	cfg := hintsConfig(34)
	frames := [][]byte{
		syntheticFrame(cfg.Width, cfg.Height, 0),
		syntheticFrame(cfg.Width, cfg.Height, 1),
	}
	text := image.Rect(0, 0, 80, 80)
	plain, _ := encodeWithHints(t, cfg, frames, []Hints{{}, {}})
	marked, markedRec := encodeWithHints(t, cfg, frames,
		[]Hints{{}, {Regions: []Region{{Rect: text, Kind: RegionText}}}})
	assertMatchesReconstruction(t, "regions", decodeUnits(t, marked), markedRec)

	if len(marked[1]) <= len(plain[1]) {
		t.Fatalf("asking for text quality cost %d bytes against %d without the hint, so the quantiser did not move",
			len(marked[1]), len(plain[1]))
	}
	t.Logf("plain %d bytes, text region %d bytes", len(plain[1]), len(marked[1]))
}

func TestRectanglesOutsideThePictureAreIgnored(t *testing.T) {
	cfg := hintsConfig(26)
	frames := [][]byte{
		syntheticFrame(cfg.Width, cfg.Height, 0),
		syntheticFrame(cfg.Width, cfg.Height, 1),
	}
	hints := []Hints{{}, {Changed: []image.Rectangle{
		image.Rect(-100, -100, -50, -50),
		image.Rect(1000, 1000, 2000, 2000),
		image.Rect(0, 0, 0, 0),
	}}}
	units, recons := encodeWithHints(t, cfg, frames, hints)
	assertMatchesReconstruction(t, "out of range", decodeUnits(t, units), recons)
}
