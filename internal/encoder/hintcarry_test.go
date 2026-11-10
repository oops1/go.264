package encoder

import (
	"bytes"
	"image"
	"math/rand"
	"testing"

	"github.com/oops1/go.264/internal/frame"
)

func greyFrame(w, h int, luma byte) []byte {
	f := make([]byte, w*h*3/2)
	for i := range f {
		f[i] = 128
	}
	for i := 0; i < w*h; i++ {
		f[i] = luma
	}
	return f
}

func paintLuma(f []byte, w int, r image.Rectangle, value func(x, y int) byte) []byte {
	out := append([]byte{}, f...)
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			out[y*w+x] = value(x, y)
		}
	}
	return out
}

func lumaError(pic *frame.Picture, src []byte, w int, r image.Rectangle) float64 {
	var sum, n int
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			d := int(pic.Y[pic.LumaOffset(x, y)]) - int(src[y*w+x])
			if d < 0 {
				d = -d
			}
			sum += d
			n++
		}
	}
	return float64(sum) / float64(n)
}

func encodeAll(t *testing.T, cfg Config, frames [][]byte, hints []Hints) []*frame.Picture {
	t.Helper()
	enc, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	var units [][]byte
	for i, f := range frames {
		pkt, err := enc.EncodeWithHints(f, hints[i])
		if err != nil {
			t.Fatalf("frame %d: %v", i, err)
		}
		units = append(units, pkt)
	}
	tail, err := enc.Flush()
	if err != nil {
		t.Fatal(err)
	}
	pics := decodeUnits(t, append(units, tail))
	if len(pics) != len(frames) {
		t.Fatalf("%d pictures decoded, want %d", len(pics), len(frames))
	}
	return pics
}

func TestAChangeInABPictureReachesTheNextP(t *testing.T) {
	const w, h = 64, 64
	cfg := Config{Width: w, Height: h, FPSNum: 30, FPSDen: 1, GOPSize: 300, QP: 22, BFrames: 2}
	area := image.Rect(0, 0, 32, 32)
	base := greyFrame(w, h, 60)
	changed := paintLuma(base, w, area, func(x, y int) byte { return byte(180 + (x+y)%16) })
	frames := [][]byte{base, changed, changed, changed}
	hints := []Hints{{}, {Changed: []image.Rectangle{area}}, {Changed: []image.Rectangle{}}, {Changed: []image.Rectangle{}}}

	pics := encodeAll(t, cfg, frames, hints)
	for i := 1; i < len(pics); i++ {
		if e := lumaError(pics[i], frames[i], w, area); e > 4 {
			t.Errorf("picture %d: the area that changed in picture 1 is %.1f levels from its source on average, as if it were still picture 0", i, e)
		}
	}
}

func TestAChangeInADroppedPictureReachesTheNextOne(t *testing.T) {
	const w, h = 512, 384
	rng := rand.New(rand.NewSource(7))
	noise := func(chroma byte) []byte {
		f := greyFrame(w, h, 0)
		rng.Read(f[:w*h])
		for i := w * h; i < len(f); i++ {
			f[i] = chroma
		}
		return f
	}
	dark, light := noise(40), noise(216)
	frames := [][]byte{dark, light}
	hints := []Hints{{}, {}}
	for i := 0; i < 14; i++ {
		frames = append(frames, light)
		hints = append(hints, Hints{Changed: []image.Rectangle{}})
	}

	drops := 0
	for _, buffer := range []int{160, 130, 200, 250} {
		cfg := Config{Width: w, Height: h, FPSNum: 25, FPSDen: 1, GOPSize: 300, QP: 26, RefFrames: 1,
			VBVBufferKbits: buffer, VBVMaxrateKbps: buffer * 25 / 8}
		pics := encodeAll(t, cfg, frames, hints)
		if !bytes.Equal(pics[1].Y, pics[0].Y) {
			continue
		}
		drops++
		if meanCb(pics[len(pics)-1], w, h) >= 180 {
			return
		}
	}
	if drops == 0 {
		t.Fatal("no buffer size made the encoder drop picture 1, so nothing was measured")
	}
	t.Fatalf("picture 1 was dropped under %d buffer sizes and its change never arrived: the last picture kept the Cb of the picture before the drop, 40, where its source has 216", drops)
}

func meanCb(pic *frame.Picture, w, h int) float64 {
	sum := 0
	for y := 0; y < h/2; y++ {
		for x := 0; x < w/2; x++ {
			sum += int(pic.Cb[pic.ChromaOffset(x, y)])
		}
	}
	return float64(sum) / float64(w*h/4)
}
