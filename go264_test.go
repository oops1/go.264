package go264

import (
	"math"
	"testing"
)

func pattern(w, h, t int) []byte {
	buf := make([]byte, w*h*3/2)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			v := (x*3 + y*5 + t*9) % 256
			if (x/8+y/8)%2 == 0 {
				v = 255 - v
			}
			buf[y*w+x] = byte(v)
		}
	}
	cw, ch := w/2, h/2
	for i := 0; i < 2; i++ {
		base := w*h + i*cw*ch
		for y := 0; y < ch; y++ {
			for x := 0; x < cw; x++ {
				buf[base+y*cw+x] = byte((x + y*2 + t*3 + i*60) % 256)
			}
		}
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
	return 10 * math.Log10(255*255/(sum/float64(len(a))))
}

func TestRoundTrip(t *testing.T) {
	sizes := [][2]int{{176, 144}, {320, 240}, {160, 120}, {100, 60}}
	for _, size := range sizes {
		w, h := size[0], size[1]
		cfg := EncoderConfig{Width: w, Height: h, FPSNum: 25, FPSDen: 1, GOPSize: 4, QP: 24, ForceSoftware: true}
		enc, err := NewEncoder(cfg)
		if err != nil {
			t.Fatalf("%dx%d: %v", w, h, err)
		}
		defer enc.Close()
		if enc.Backend() != "cpu" {
			t.Fatalf("ForceSoftware still selected backend %q", enc.Backend())
		}

		var stream []byte
		var sources [][]byte
		for i := 0; i < 6; i++ {
			src := pattern(w, h, i)
			sources = append(sources, src)
			pkt, err := enc.Encode(src)
			if err != nil {
				t.Fatalf("%dx%d frame %d: %v", w, h, i, err)
			}
			stream = append(stream, pkt...)
		}

		dec := NewDecoderWithConfig(DecoderConfig{ForceSoftware: true})
		defer dec.Close()
		frames, err := dec.Decode(stream)
		if err != nil {
			t.Fatalf("%dx%d decode: %v", w, h, err)
		}
		rest, err := dec.Flush()
		if err != nil {
			t.Fatalf("%dx%d flush: %v", w, h, err)
		}
		frames = append(frames, rest...)
		if len(frames) != len(sources) {
			t.Fatalf("%dx%d: decoded %d frames, want %d", w, h, len(frames), len(sources))
		}
		for i, f := range frames {
			if f.Width != w || f.Height != h {
				t.Fatalf("%dx%d frame %d: decoded size %dx%d", w, h, i, f.Width, f.Height)
			}
			got := f.AppendI420(nil)
			if len(got) != len(sources[i]) {
				t.Fatalf("%dx%d frame %d: %d bytes, want %d", w, h, i, len(got), len(sources[i]))
			}
			if q := psnr(sources[i], got); q < 32 {
				t.Errorf("%dx%d frame %d: PSNR only %.2f dB", w, h, i, q)
			}
		}
	}
}

func TestClosedCodecsRefuseWork(t *testing.T) {
	enc, err := NewEncoder(EncoderConfig{Width: 32, Height: 32, QP: 26, ForceSoftware: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := enc.Close(); err != nil {
		t.Fatal(err)
	}
	if err := enc.Close(); err != nil {
		t.Fatalf("second Close returned %v", err)
	}
	if _, err := enc.Encode(make([]byte, 32*32*3/2)); err != ErrClosed {
		t.Errorf("Encode after Close returned %v", err)
	}

	dec := NewDecoderWithConfig(DecoderConfig{ForceSoftware: true})
	if err := dec.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := dec.Decode(nil); err != ErrClosed {
		t.Errorf("Decode after Close returned %v", err)
	}
	if _, err := dec.Flush(); err != ErrClosed {
		t.Errorf("Flush after Close returned %v", err)
	}
}

func TestBackendsAlwaysIncludesCPU(t *testing.T) {
	b := Backends()
	if len(b) == 0 || b[0] != "cpu" {
		t.Fatalf("Backends() = %v", b)
	}
	dec := NewDecoder()
	defer dec.Close()
	if dec.Backend() == "" {
		t.Error("decoder reported an empty backend name")
	}
	enc, err := NewEncoder(EncoderConfig{Width: 64, Height: 64, QP: 26})
	if err != nil {
		t.Fatal(err)
	}
	defer enc.Close()
	if enc.Backend() == "" {
		t.Error("encoder reported an empty backend name")
	}
	t.Logf("available backends: %v, encoder using %q, decoder using %q", b, enc.Backend(), dec.Backend())
}

func TestFrameHelpers(t *testing.T) {
	f := &Frame{
		Y:       []byte{1, 2, 9, 3, 4, 9},
		Cb:      []byte{5, 9, 9},
		Cr:      []byte{6, 9, 9},
		StrideY: 3,
		StrideC: 3,
		Width:   2,
		Height:  2,
	}
	if got := f.I420Size(); got != 6 {
		t.Fatalf("I420Size = %d, want 6", got)
	}
	got := f.AppendI420(nil)
	want := []byte{1, 2, 3, 4, 5, 6}
	if len(got) != len(want) {
		t.Fatalf("AppendI420 produced %d bytes, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("AppendI420 = %v, want %v", got, want)
		}
	}
	prefix := []byte{0xAA}
	got = f.AppendI420(prefix)
	if got[0] != 0xAA || len(got) != 7 {
		t.Fatalf("AppendI420 did not append to the given slice: %v", got)
	}
}

func BenchmarkRoundTripQCIF(b *testing.B) {
	cfg := EncoderConfig{Width: 176, Height: 144, FPSNum: 25, FPSDen: 1, GOPSize: 10, QP: 26, ForceSoftware: true}
	src := pattern(cfg.Width, cfg.Height, 0)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		enc, err := NewEncoder(cfg)
		if err != nil {
			b.Fatal(err)
		}
		if _, err := enc.Encode(src); err != nil {
			b.Fatal(err)
		}
		enc.Close()
	}
}
