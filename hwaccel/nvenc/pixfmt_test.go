package nvenc

import (
	"math/rand"
	"testing"
)

func randomI420(t *testing.T, w, h int) []byte {
	t.Helper()
	b := make([]byte, I420Size(w, h))
	r := rand.New(rand.NewSource(int64(w*7919 + h)))
	for i := range b {
		b[i] = byte(r.Intn(256))
	}
	return b
}

func TestPlanarAndInterleavedRoundTrip(t *testing.T) {
	cases := []struct {
		w, h, stride int
	}{
		{16, 16, 16},
		{176, 144, 176},
		{176, 144, 192},
		{1280, 720, 1280},
		{1280, 720, 1408},
	}
	for _, c := range cases {
		src := randomI420(t, c.w, c.h)
		mid := make([]byte, NV12Size(c.stride, c.h))
		back := make([]byte, len(src))
		I420ToNV12(mid, c.stride, src, c.w, c.h)
		NV12ToI420(back, mid, c.stride, c.w, c.h)
		for i := range src {
			if src[i] != back[i] {
				t.Fatalf("%dx%d at stride %d: byte %d came back as %d, want %d",
					c.w, c.h, c.stride, i, back[i], src[i])
			}
		}
	}
}

func TestInterleavingPutsBlueDifferenceFirst(t *testing.T) {
	const w, h = 2, 2
	src := make([]byte, I420Size(w, h))
	src[w*h] = 0x11
	src[w*h+1] = 0x22
	dst := make([]byte, NV12Size(w, h))
	I420ToNV12(dst, w, src, w, h)
	if dst[w*h] != 0x11 || dst[w*h+1] != 0x22 {
		t.Fatalf("the chroma pair reads %02X %02X, want 11 22 with the blue difference first",
			dst[w*h], dst[w*h+1])
	}
}

func TestConversionLeavesStridePaddingAlone(t *testing.T) {
	const w, h, stride = 8, 8, 12
	src := randomI420(t, w, h)
	dst := make([]byte, NV12Size(stride, h))
	for i := range dst {
		dst[i] = 0xA5
	}
	I420ToNV12(dst, stride, src, w, h)
	for y := 0; y < h+h/2; y++ {
		for x := w; x < stride; x++ {
			if dst[y*stride+x] != 0xA5 {
				t.Fatalf("padding at row %d column %d was overwritten with %02X", y, x, dst[y*stride+x])
			}
		}
	}
}

func TestConversionRefusesShapesItCannotHold(t *testing.T) {
	dst := make([]byte, 1<<16)
	src := make([]byte, 1<<16)
	for _, c := range []struct {
		name         string
		w, h, stride int
	}{
		{"odd width", 15, 16, 16},
		{"odd height", 16, 15, 16},
		{"zero width", 0, 16, 16},
		{"stride below width", 16, 16, 8},
	} {
		before := append([]byte(nil), dst...)
		I420ToNV12(dst, c.stride, src, c.w, c.h)
		for i := range dst {
			if dst[i] != before[i] {
				t.Fatalf("%s: the destination was written at byte %d", c.name, i)
			}
		}
	}
}

func TestSizesAgreeWithWhatTheConvertersTouch(t *testing.T) {
	if got := I420Size(176, 144); got != 176*144*3/2 {
		t.Fatalf("I420Size(176, 144) = %d, want %d", got, 176*144*3/2)
	}
	if got := NV12Size(192, 144); got != 192*144*3/2 {
		t.Fatalf("NV12Size(192, 144) = %d, want %d", got, 192*144*3/2)
	}
	if got := I420Size(15, 16); got != 0 {
		t.Fatalf("an odd width reported a size of %d", got)
	}
}
