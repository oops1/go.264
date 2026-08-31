package mf

import (
	"math/rand"
	"testing"
)

func randI420(rng *rand.Rand, w, h int) []byte {
	b := make([]byte, I420Size(w, h))
	rng.Read(b)
	return b
}

func TestI420NV12RoundTrip(t *testing.T) {
	cases := []struct {
		name   string
		w, h   int
		stride int
	}{
		{"16x16 tight", 16, 16, 16},
		{"176x144 tight", 176, 144, 176},
		{"176x144 padded stride 192", 176, 144, 192},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rng := rand.New(rand.NewSource(1))
			src := randI420(rng, c.w, c.h)
			nv12 := make([]byte, NV12Size(c.stride, c.h))
			I420ToNV12(nv12, c.stride, src, c.w, c.h)
			got := make([]byte, I420Size(c.w, c.h))
			NV12ToI420(got, nv12, c.stride, c.w, c.h)
			for i := range src {
				if got[i] != src[i] {
					t.Fatalf("round trip byte %d = %d, want %d", i, got[i], src[i])
				}
			}
		})
	}
}

func TestI420ToNV12InterleavesCbThenCr(t *testing.T) {
	w, h, stride := 2, 2, 2
	src := []byte{10, 20, 30, 40, 0xAA, 0xBB}
	dst := make([]byte, NV12Size(stride, h))
	I420ToNV12(dst, stride, src, w, h)
	want := []byte{10, 20, 30, 40, 0xAA, 0xBB}
	for i := range want {
		if dst[i] != want[i] {
			t.Fatalf("dst[%d] = %#x, want %#x (Cb must precede Cr in the interleave)", i, dst[i], want[i])
		}
	}
}

func TestNV12ToI420DeinterleavesCbThenCr(t *testing.T) {
	w, h, stride := 2, 2, 2
	nv12 := []byte{10, 20, 30, 40, 0xAA, 0xBB}
	dst := make([]byte, I420Size(w, h))
	NV12ToI420(dst, nv12, stride, w, h)
	want := []byte{10, 20, 30, 40, 0xAA, 0xBB}
	for i := range want {
		if dst[i] != want[i] {
			t.Fatalf("dst[%d] = %#x, want %#x (Cb must be read before Cr from the interleave)", i, dst[i], want[i])
		}
	}
}

func TestI420ToNV12LeavesStridePaddingUntouched(t *testing.T) {
	w, h, stride := 176, 144, 192
	rng := rand.New(rand.NewSource(2))
	src := randI420(rng, w, h)
	dst := make([]byte, NV12Size(stride, h))
	for i := range dst {
		dst[i] = 0x5A
	}
	I420ToNV12(dst, stride, src, w, h)
	for y := 0; y < h; y++ {
		for x := w; x < stride; x++ {
			off := y*stride + x
			if dst[off] != 0x5A {
				t.Fatalf("luma padding at row %d col %d = %#x, want sentinel 0x5A", y, x, dst[off])
			}
		}
	}
	cw, ch := w/2, h/2
	base := stride * h
	for y := 0; y < ch; y++ {
		for x := 2 * cw; x < stride; x++ {
			off := base + y*stride + x
			if dst[off] != 0x5A {
				t.Fatalf("chroma padding at row %d col %d = %#x, want sentinel 0x5A", y, x, dst[off])
			}
		}
	}
}

func TestNV12ToI420IgnoresStridePadding(t *testing.T) {
	w, h, stride := 176, 144, 192
	rng := rand.New(rand.NewSource(3))
	src := randI420(rng, w, h)
	clean := make([]byte, NV12Size(stride, h))
	I420ToNV12(clean, stride, src, w, h)

	garbage := make([]byte, NV12Size(stride, h))
	copy(garbage, clean)
	rngGarbage := rand.New(rand.NewSource(4))
	cw, ch := w/2, h/2
	for y := 0; y < h; y++ {
		for x := w; x < stride; x++ {
			garbage[y*stride+x] = byte(rngGarbage.Intn(256))
		}
	}
	base := stride * h
	for y := 0; y < ch; y++ {
		for x := 2 * cw; x < stride; x++ {
			garbage[base+y*stride+x] = byte(rngGarbage.Intn(256))
		}
	}

	want := make([]byte, I420Size(w, h))
	NV12ToI420(want, clean, stride, w, h)
	got := make([]byte, I420Size(w, h))
	NV12ToI420(got, garbage, stride, w, h)
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("I420 byte %d = %d, want %d (stride padding must be ignored)", i, got[i], want[i])
		}
	}
}

func TestI420Size(t *testing.T) {
	cases := []struct {
		w, h, want int
	}{
		{0, 0, 0},
		{-4, 8, 0},
		{16, 16, 16*16 + 2*8*8},
		{176, 144, 176*144 + 2*88*72},
	}
	for _, c := range cases {
		if got := I420Size(c.w, c.h); got != c.want {
			t.Fatalf("I420Size(%d, %d) = %d, want %d", c.w, c.h, got, c.want)
		}
	}
}

func TestI420SizeMatchesWhatConvertersTouch(t *testing.T) {
	w, h, stride := 176, 144, 192
	rng := rand.New(rand.NewSource(5))
	src := randI420(rng, w, h)
	if len(src) != I420Size(w, h) {
		t.Fatalf("randI420 produced %d bytes, I420Size says %d", len(src), I420Size(w, h))
	}
	nv12 := make([]byte, NV12Size(stride, h))
	I420ToNV12(nv12, stride, src, w, h)
	got := make([]byte, I420Size(w, h))
	NV12ToI420(got, nv12, stride, w, h)
	for i := range src {
		if got[i] != src[i] {
			t.Fatalf("byte %d mismatch after round trip through exactly sized buffers", i)
		}
	}
}

func TestNV12Size(t *testing.T) {
	cases := []struct {
		stride, h, want int
	}{
		{0, 0, 0},
		{-4, 8, 0},
		{16, 16, 16*16 + 16*8},
		{192, 144, 192*144 + 192*72},
	}
	for _, c := range cases {
		if got := NV12Size(c.stride, c.h); got != c.want {
			t.Fatalf("NV12Size(%d, %d) = %d, want %d", c.stride, c.h, got, c.want)
		}
	}
}

func TestI420ToNV12RejectsOddDimensionsWithoutPanic(t *testing.T) {
	rng := rand.New(rand.NewSource(6))
	cases := []struct{ w, h int }{{15, 16}, {16, 15}, {-2, 16}, {16, -2}, {0, 0}}
	for _, c := range cases {
		src := make([]byte, 4096)
		rng.Read(src)
		dst := make([]byte, 4096)
		for i := range dst {
			dst[i] = 0x33
		}
		I420ToNV12(dst, 64, src, c.w, c.h)
		for i, v := range dst {
			if v != 0x33 {
				t.Fatalf("I420ToNV12(w=%d, h=%d) wrote to dst[%d], want a no-op", c.w, c.h, i)
			}
		}
	}
}

func TestNV12ToI420RejectsOddDimensionsWithoutPanic(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	cases := []struct{ w, h int }{{15, 16}, {16, 15}, {-2, 16}, {16, -2}, {0, 0}}
	for _, c := range cases {
		src := make([]byte, 4096)
		rng.Read(src)
		dst := make([]byte, 4096)
		for i := range dst {
			dst[i] = 0x77
		}
		NV12ToI420(dst, src, 64, c.w, c.h)
		for i, v := range dst {
			if v != 0x77 {
				t.Fatalf("NV12ToI420(w=%d, h=%d) wrote to dst[%d], want a no-op", c.w, c.h, i)
			}
		}
	}
}

func TestI420ToNV12RejectsStrideBelowWidthWithoutPanic(t *testing.T) {
	rng := rand.New(rand.NewSource(8))
	src := randI420(rng, 16, 16)
	dst := make([]byte, 4096)
	for i := range dst {
		dst[i] = 0x11
	}
	I420ToNV12(dst, 8, src, 16, 16)
	for i, v := range dst {
		if v != 0x11 {
			t.Fatalf("I420ToNV12 with stride < w wrote to dst[%d], want a no-op", i)
		}
	}
}

func TestNV12ToI420RejectsStrideBelowWidthWithoutPanic(t *testing.T) {
	rng := rand.New(rand.NewSource(9))
	src := make([]byte, 4096)
	rng.Read(src)
	dst := make([]byte, 4096)
	for i := range dst {
		dst[i] = 0x22
	}
	NV12ToI420(dst, src, 8, 16, 16)
	for i, v := range dst {
		if v != 0x22 {
			t.Fatalf("NV12ToI420 with stride < w wrote to dst[%d], want a no-op", i)
		}
	}
}

func BenchmarkI420ToNV12(b *testing.B) {
	w, h, stride := 1920, 1080, 1920
	rng := rand.New(rand.NewSource(10))
	src := randI420(rng, w, h)
	dst := make([]byte, NV12Size(stride, h))
	b.SetBytes(int64(len(src)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		I420ToNV12(dst, stride, src, w, h)
	}
}

func BenchmarkNV12ToI420(b *testing.B) {
	w, h, stride := 1920, 1080, 1920
	rng := rand.New(rand.NewSource(11))
	src := randI420(rng, w, h)
	nv12 := make([]byte, NV12Size(stride, h))
	I420ToNV12(nv12, stride, src, w, h)
	dst := make([]byte, I420Size(w, h))
	b.SetBytes(int64(len(dst)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		NV12ToI420(dst, nv12, stride, w, h)
	}
}
