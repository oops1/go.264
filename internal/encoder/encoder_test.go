package encoder

import (
	"math"
	"math/rand"
	"testing"

	"github.com/oops1/go264/internal/decoder"
	"github.com/oops1/go264/internal/frame"
)

func syntheticFrame(w, h, t int) []byte {
	buf := make([]byte, w*h*3/2)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			v := (x*3 + y*5 + t*7) % 256
			if (x/8+y/8)%2 == 0 {
				v = 255 - v
			}
			if x > w/3 && x < 2*w/3 && y > h/3 && y < 2*h/3 {
				v = 128 + (x+y+t*11)%64
			}
			buf[y*w+x] = byte(v)
		}
	}
	cw, ch := w/2, h/2
	for i := 0; i < 2; i++ {
		base := w*h + i*cw*ch
		for y := 0; y < ch; y++ {
			for x := 0; x < cw; x++ {
				buf[base+y*cw+x] = byte((x*2 + y*3 + t*5 + i*40) % 256)
			}
		}
	}
	return buf
}

func noisyFrame(rng *rand.Rand, w, h int) []byte {
	buf := make([]byte, w*h*3/2)
	for i := range buf {
		buf[i] = byte(rng.Intn(256))
	}
	return buf
}

func psnr(a, b []byte) float64 {
	var sum float64
	for i := range a {
		d := float64(a[i]) - float64(b[i])
		sum += d * d
	}
	if sum == 0 {
		return math.Inf(1)
	}
	mse := sum / float64(len(a))
	return 10 * math.Log10(255*255/mse)
}

func (e *Encoder) Reconstruction() *frame.Picture { return e.ref }

func encodeAndDecode(t *testing.T, cfg Config, frames [][]byte) ([]*frame.Picture, [][]byte, []*frame.Picture) {
	t.Helper()
	enc, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	var stream []byte
	var recons []*frame.Picture
	var units [][]byte
	for _, f := range frames {
		pkt, err := enc.Encode(f)
		if err != nil {
			t.Fatalf("Encode: %v", err)
		}
		if len(pkt) == 0 {
			t.Fatal("Encode produced no bytes")
		}
		units = append(units, pkt)
		stream = append(stream, pkt...)
		rec := enc.Reconstruction()
		snapshot := frame.NewPicture((cfg.Width+15)/16, (cfg.Height+15)/16)
		copy(snapshot.Y, rec.Y)
		copy(snapshot.Cb, rec.Cb)
		copy(snapshot.Cr, rec.Cr)
		snapshot.Width = rec.Width
		snapshot.Height = rec.Height
		recons = append(recons, snapshot)
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
	pics = append(pics, rest...)
	return pics, units, recons
}

func TestEncodeDecodeReconstructionMatches(t *testing.T) {
	for _, qp := range []int{0, 10, 26, 40, 51} {
		cfg := Config{Width: 176, Height: 144, FPSNum: 25, FPSDen: 1, GOPSize: 1, QP: qp}
		var frames [][]byte
		for i := 0; i < 3; i++ {
			frames = append(frames, syntheticFrame(cfg.Width, cfg.Height, i))
		}
		pics, _, recons := encodeAndDecode(t, cfg, frames)
		if len(pics) != len(frames) {
			t.Fatalf("qp %d: decoded %d frames, want %d", qp, len(pics), len(frames))
		}
		for i := range pics {
			got := make([]byte, pics[i].Size())
			pics[i].CopyOut(got)
			want := make([]byte, recons[i].Size())
			recons[i].CopyOut(want)
			for j := range got {
				if got[j] != want[j] {
					t.Fatalf("qp %d frame %d: decoder and encoder reconstructions differ at sample %d, got %d want %d",
						qp, i, j, got[j], want[j])
				}
			}
		}
	}
}

func TestEncodeQualityImprovesAsQPFalls(t *testing.T) {
	cfg := Config{Width: 176, Height: 144, FPSNum: 25, FPSDen: 1, GOPSize: 1}
	src := syntheticFrame(cfg.Width, cfg.Height, 0)
	var prev float64
	var prevSize int
	for i, qp := range []int{40, 30, 20, 10} {
		cfg.QP = qp
		pics, units, _ := encodeAndDecode(t, cfg, [][]byte{src})
		got := make([]byte, pics[0].Size())
		pics[0].CopyOut(got)
		q := psnr(src, got)
		size := len(units[0])
		t.Logf("qp %d: %d bytes, PSNR %.2f dB", qp, size, q)
		if i > 0 {
			if q <= prev {
				t.Errorf("qp %d: PSNR %.2f did not improve on %.2f", qp, q, prev)
			}
			if size <= prevSize {
				t.Errorf("qp %d: size %d did not grow beyond %d", qp, size, prevSize)
			}
		}
		prev = q
		prevSize = size
	}
	if prev < 40 {
		t.Errorf("PSNR at qp 10 is only %.2f dB", prev)
	}
}

func TestEncodeLosslessAtQP0IsNearPerfect(t *testing.T) {
	cfg := Config{Width: 176, Height: 144, FPSNum: 25, FPSDen: 1, GOPSize: 1, QP: 0}
	src := syntheticFrame(cfg.Width, cfg.Height, 0)
	pics, _, _ := encodeAndDecode(t, cfg, [][]byte{src})
	got := make([]byte, pics[0].Size())
	pics[0].CopyOut(got)
	q := psnr(src, got)
	t.Logf("qp 0 PSNR %.2f dB", q)
	if q < 48 {
		t.Errorf("PSNR at qp 0 is only %.2f dB", q)
	}
}

func TestEncodeOddDimensions(t *testing.T) {
	for _, dim := range [][2]int{{176, 144}, {160, 120}, {64, 48}, {32, 18}, {2, 2}} {
		cfg := Config{Width: dim[0], Height: dim[1], FPSNum: 30, FPSDen: 1, GOPSize: 1, QP: 26}
		src := syntheticFrame(dim[0], dim[1], 1)
		pics, _, _ := encodeAndDecode(t, cfg, [][]byte{src})
		if len(pics) != 1 {
			t.Fatalf("%dx%d: decoded %d pictures", dim[0], dim[1], len(pics))
		}
	}
}

func TestEncodeNoisySource(t *testing.T) {
	rng := rand.New(rand.NewSource(11))
	cfg := Config{Width: 96, Height: 64, FPSNum: 25, FPSDen: 1, GOPSize: 1, QP: 30}
	var frames [][]byte
	for i := 0; i < 3; i++ {
		frames = append(frames, noisyFrame(rng, cfg.Width, cfg.Height))
	}
	pics, _, recons := encodeAndDecode(t, cfg, frames)
	for i := range pics {
		got := make([]byte, pics[i].Size())
		pics[i].CopyOut(got)
		want := make([]byte, recons[i].Size())
		recons[i].CopyOut(want)
		for j := range got {
			if got[j] != want[j] {
				t.Fatalf("noisy frame %d differs at sample %d", i, j)
			}
		}
	}
}

func TestConfigValidation(t *testing.T) {
	bad := []Config{
		{Width: 0, Height: 16, QP: 26},
		{Width: 16, Height: 0, QP: 26},
		{Width: 17, Height: 16, QP: 26},
		{Width: 16, Height: 16, QP: -1},
		{Width: 16, Height: 16, QP: 52},
		{Width: 20000, Height: 16, QP: 26},
	}
	for _, c := range bad {
		if _, err := New(c); err == nil {
			t.Errorf("New(%+v) accepted an invalid configuration", c)
		}
	}
	e, err := New(Config{Width: 16, Height: 16, QP: 26})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.Encode(make([]byte, 10)); err == nil {
		t.Error("Encode accepted a frame of the wrong size")
	}
}

func BenchmarkEncodeIntraQCIF(b *testing.B) {
	cfg := Config{Width: 176, Height: 144, FPSNum: 25, FPSDen: 1, GOPSize: 1, QP: 26}
	src := syntheticFrame(cfg.Width, cfg.Height, 0)
	enc, err := New(cfg)
	if err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := enc.Encode(src); err != nil {
			b.Fatal(err)
		}
	}
}

func TestEncodeInterReconstructionMatches(t *testing.T) {
	for _, qp := range []int{10, 26, 40} {
		cfg := Config{Width: 176, Height: 144, FPSNum: 25, FPSDen: 1, GOPSize: 8, QP: qp}
		var frames [][]byte
		for i := 0; i < 8; i++ {
			frames = append(frames, syntheticFrame(cfg.Width, cfg.Height, i))
		}
		pics, _, recons := encodeAndDecode(t, cfg, frames)
		if len(pics) != len(frames) {
			t.Fatalf("qp %d: decoded %d frames, want %d", qp, len(pics), len(frames))
		}
		for i := range pics {
			got := make([]byte, pics[i].Size())
			pics[i].CopyOut(got)
			want := make([]byte, recons[i].Size())
			recons[i].CopyOut(want)
			for j := range got {
				if got[j] != want[j] {
					t.Fatalf("qp %d frame %d: reconstructions differ at sample %d, got %d want %d",
						qp, i, j, got[j], want[j])
				}
			}
		}
	}
}

func TestInterFramesAreSmallerThanIntra(t *testing.T) {
	cfg := Config{Width: 176, Height: 144, FPSNum: 25, FPSDen: 1, GOPSize: 6, QP: 26}
	enc, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	src := syntheticFrame(cfg.Width, cfg.Height, 0)
	var sizes []int
	for i := 0; i < 6; i++ {
		pkt, err := enc.Encode(src)
		if err != nil {
			t.Fatal(err)
		}
		sizes = append(sizes, len(pkt))
	}
	t.Logf("access unit sizes: %v", sizes)
	for i := 1; i < len(sizes); i++ {
		if sizes[i] >= sizes[0] {
			t.Errorf("frame %d is %d bytes, not smaller than the intra frame at %d", i, sizes[i], sizes[0])
		}
	}
}

func TestRateControlHitsTheTargetBitrate(t *testing.T) {
	for _, kbps := range []int{200, 500, 1500} {
		cfg := Config{
			Width: 176, Height: 144, FPSNum: 25, FPSDen: 1,
			GOPSize: 25, QP: 30, BitrateKbps: kbps,
		}
		enc, err := New(cfg)
		if err != nil {
			t.Fatal(err)
		}
		total := 0
		const frames = 50
		for i := 0; i < frames; i++ {
			pkt, err := enc.Encode(syntheticFrame(cfg.Width, cfg.Height, i))
			if err != nil {
				t.Fatal(err)
			}
			total += len(pkt)
		}
		fps := float64(cfg.FPSNum) / float64(cfg.FPSDen)
		got := float64(total) * 8 / float64(frames) * fps / 1000
		t.Logf("target %d kbps, achieved %.0f kbps", kbps, got)
		if got < float64(kbps)*0.75 || got > float64(kbps)*1.35 {
			t.Errorf("target %d kbps but produced %.0f kbps", kbps, got)
		}
	}
}

func TestRateControlDisabledKeepsConstantQP(t *testing.T) {
	cfg := Config{Width: 96, Height: 64, FPSNum: 25, FPSDen: 1, GOPSize: 4, QP: 33}
	enc, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 8; i++ {
		if _, err := enc.Encode(syntheticFrame(cfg.Width, cfg.Height, i)); err != nil {
			t.Fatal(err)
		}
		if q := enc.rc.frameQP(false); q != 33 {
			t.Fatalf("frame %d: quantiser drifted to %d with rate control off", i, q)
		}
	}
}

func TestRateControlStreamsStayDecodable(t *testing.T) {
	cfg := Config{Width: 176, Height: 144, FPSNum: 25, FPSDen: 1, GOPSize: 10, QP: 30, BitrateKbps: 400}
	var frames [][]byte
	for i := 0; i < 20; i++ {
		frames = append(frames, syntheticFrame(cfg.Width, cfg.Height, i))
	}
	pics, _, recons := encodeAndDecode(t, cfg, frames)
	if len(pics) != len(frames) {
		t.Fatalf("decoded %d frames, want %d", len(pics), len(frames))
	}
	for i := range pics {
		got := make([]byte, pics[i].Size())
		pics[i].CopyOut(got)
		want := make([]byte, recons[i].Size())
		recons[i].CopyOut(want)
		for j := range got {
			if got[j] != want[j] {
				t.Fatalf("frame %d differs at sample %d under rate control", i, j)
			}
		}
	}
}
