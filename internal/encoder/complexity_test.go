package encoder

import (
	"testing"

	"github.com/oops1/go.264/internal/frame"
)

func probeOver(t *testing.T, cfg Config, frames [][]byte) []float64 {
	t.Helper()
	enc, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	probe := newComplexityProbe(enc.widthMBs, enc.heightMBs)
	var out []float64
	for _, f := range frames {
		enc.loadSourceInto(enc.src, f)
		v, ok := probe.measure(enc.src)
		if !ok {
			continue
		}
		out = append(out, v)
	}
	return out
}

func probeConfig(w, h int) Config {
	return Config{Width: w, Height: h, FPSNum: 25, FPSDen: 1, GOPSize: 250, QP: 26, RefFrames: 1}
}

func TestComplexityProbeSaysNothingAboutTheFirstPicture(t *testing.T) {
	const w, h = 320, 240
	cfg := probeConfig(w, h)
	enc, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	probe := newComplexityProbe(enc.widthMBs, enc.heightMBs)
	enc.loadSourceInto(enc.src, screencastClips()[4].frames(w, h, 1)[0])
	if v, ok := probe.measure(enc.src); ok {
		t.Fatalf("the first picture reported a complexity of %g with nothing to compare it against", v)
	}
}

func TestComplexityProbeIsBlindToTheQuantiser(t *testing.T) {
	const w, h = 320, 240
	frames := screencastClips()[2].frames(w, h, 8)
	var first []float64
	for _, qp := range []int{0, 12, 26, 40, 51} {
		cfg := probeConfig(w, h)
		cfg.QP = qp
		got := probeOver(t, cfg, frames)
		if first == nil {
			first = got
			continue
		}
		for i := range got {
			if got[i] != first[i] {
				t.Fatalf("at qp %d picture %d measured %g against %g at qp 0; the estimate is taken before the encode and must not know the quantiser",
					qp, i, got[i], first[i])
			}
		}
	}
	t.Logf("eight pictures measured identically at five quantisers, first %g last %g", first[0], first[len(first)-1])
}

func TestComplexityProbeSeparatesAStillScreenFromABusyOne(t *testing.T) {
	const w, h = 320, 240
	still := probeOver(t, probeConfig(w, h), screencastClips()[0].frames(w, h, 8))
	busy := probeOver(t, probeConfig(w, h), screencastClips()[4].frames(w, h, 8))
	quiet, loud := still[len(still)-1], busy[len(busy)-1]
	t.Logf("a still desktop measures %g and a window playing video %g, %.1f times as much", quiet, loud, loud/quiet)
	if loud <= 4*quiet {
		t.Fatalf("a window playing video measures %g against %g for a still desktop; the estimate is not separating them",
			loud, quiet)
	}
}

func shiftedFrame(src []byte, w, h, by int) []byte {
	out := make([]byte, len(src))
	for y := 0; y < h; y++ {
		from := y + by
		if from < 0 {
			from = 0
		}
		if from >= h {
			from = h - 1
		}
		copy(out[y*w:(y+1)*w], src[from*w:(from+1)*w])
	}
	cw, ch := w/2, h/2
	for p := 0; p < 2; p++ {
		base := w * h
		if p == 1 {
			base += cw * ch
		}
		for y := 0; y < ch; y++ {
			from := y + by/2
			if from < 0 {
				from = 0
			}
			if from >= ch {
				from = ch - 1
			}
			copy(out[base+y*cw:base+(y+1)*cw], src[base+from*cw:base+(from+1)*cw])
		}
	}
	return out
}

func TestComplexityProbeLearnsASteadyScroll(t *testing.T) {
	const w, h = 320, 240
	page := screencastClips()[2].frames(w, h, 1)[0]
	frames := [][]byte{page}
	for i := 1; i < 8; i++ {
		frames = append(frames, shiftedFrame(page, w, h, 11*i))
	}
	got := probeOver(t, probeConfig(w, h), frames)
	t.Logf("a page scrolled eleven rows a picture measures %v", got)
	if len(got) < 3 {
		t.Fatalf("only %d measurements", len(got))
	}
	settled := got[len(got)-1]
	if settled >= got[0] {
		t.Fatalf("after learning the motion a steady scroll still measures %g against %g on the first picture of it; the estimate never locks on",
			settled, got[0])
	}
	still := probeOver(t, probeConfig(w, h), [][]byte{page, page})
	if settled > 40*still[0]+float64(w*h) {
		t.Fatalf("a scroll the search can follow exactly measures %g, which is not far from coding the page afresh", settled)
	}
}

func TestComplexityProbeIsCappedByCodingTheBlockAlone(t *testing.T) {
	const w, h = 320, 240
	frames := [][]byte{
		flatFrame(w, h, 128, 128, 128),
		screencastClips()[4].frames(w, h, 1)[0],
	}
	got := probeOver(t, probeConfig(w, h), frames)
	if len(got) != 1 {
		t.Fatalf("%d measurements, want one", len(got))
	}
	cfg := probeConfig(w, h)
	enc, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	probe := newComplexityProbe(enc.widthMBs, enc.heightMBs)
	enc.loadSourceInto(enc.src, frames[1])
	probe.take(enc.src)
	alone := 0
	for by := 0; by < probe.blocksY; by++ {
		for bx := 0; bx < probe.blocksX; bx++ {
			alone += probe.alone(bx, by)
		}
	}
	t.Logf("a picture with nothing to predict from measures %g against %d for coding every block alone", got[0], alone)
	if got[0] > float64(alone) {
		t.Fatalf("a picture with nothing to predict from measures %g, past the %d it costs to code every block alone",
			got[0], alone)
	}
}

func TestComplexityProbeOnlyExistsForARateFactor(t *testing.T) {
	const w, h = 176, 144
	plain, err := New(probeConfig(w, h))
	if err != nil {
		t.Fatal(err)
	}
	if plain.probe != nil {
		t.Fatal("a fixed quantiser built the complexity estimate it never reads")
	}
	cfg := probeConfig(w, h)
	cfg.QP = 0
	cfg.RateFactor = 24
	quality, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if quality.probe == nil {
		t.Fatal("a rate factor has no complexity estimate to follow")
	}
}

func TestComplexityProbeStaysInsideThePicture(t *testing.T) {
	for _, dim := range [][2]int{{16, 16}, {32, 48}, {176, 144}, {322, 242}} {
		w, h := dim[0], dim[1]
		cfg := probeConfig(w, h)
		if w%2 != 0 || h%2 != 0 {
			continue
		}
		var frames [][]byte
		for i := 0; i < 4; i++ {
			frames = append(frames, screencastClips()[3].frames(w, h, 4)[i])
		}
		got := probeOver(t, cfg, frames)
		if len(got) != 3 {
			t.Fatalf("%dx%d: %d measurements, want three", w, h, len(got))
		}
		for i, v := range got {
			if v < 0 {
				t.Fatalf("%dx%d picture %d measured %g", w, h, i, v)
			}
		}
	}
}

func BenchmarkComplexityProbe(b *testing.B) {
	const w, h = 1920, 1080
	cfg := probeConfig(w, h)
	enc, err := New(cfg)
	if err != nil {
		b.Fatal(err)
	}
	frames := screencastClips()[2].frames(w, h, 4)
	sources := make([]*frame.Picture, len(frames))
	for i, f := range frames {
		sources[i] = frame.NewPicture(enc.widthMBs, enc.heightMBs)
		enc.loadSourceInto(sources[i], f)
	}
	probe := newComplexityProbe(enc.widthMBs, enc.heightMBs)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		probe.measure(sources[i%len(sources)])
	}
}
