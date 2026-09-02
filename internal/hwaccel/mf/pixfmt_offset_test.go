package mf

import "testing"

func TestNV12ToI420OffsetReadsAChromaPlaneBelowTheAlignedHeight(t *testing.T) {
	const w, h = 4, 4
	const stride = 8
	const surfaceHeight = 6
	src := make([]byte, stride*surfaceHeight+stride*surfaceHeight/2)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			src[y*stride+x] = byte(10*y + x)
		}
	}
	chroma := stride * surfaceHeight
	for y := 0; y < h/2; y++ {
		for x := 0; x < w/2; x++ {
			src[chroma+y*stride+2*x] = byte(100 + 10*y + x)
			src[chroma+y*stride+2*x+1] = byte(200 + 10*y + x)
		}
	}

	dst := make([]byte, I420Size(w, h))
	NV12ToI420Offset(dst, src, stride, chroma, w, h)

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if got, want := dst[y*w+x], byte(10*y+x); got != want {
				t.Fatalf("luma at %d,%d came back as %d, want %d", x, y, got, want)
			}
		}
	}
	cb := dst[w*h:]
	cr := dst[w*h+(w/2)*(h/2):]
	for y := 0; y < h/2; y++ {
		for x := 0; x < w/2; x++ {
			if got, want := cb[y*(w/2)+x], byte(100+10*y+x); got != want {
				t.Fatalf("Cb at %d,%d came back as %d, want %d", x, y, got, want)
			}
			if got, want := cr[y*(w/2)+x], byte(200+10*y+x); got != want {
				t.Fatalf("Cr at %d,%d came back as %d, want %d", x, y, got, want)
			}
		}
	}
}

func TestNV12ToI420OffsetMatchesTheContiguousFormOnATightSurface(t *testing.T) {
	const w, h = 8, 6
	const stride = 12
	src := make([]byte, NV12Size(stride, h))
	for i := range src {
		src[i] = byte(i * 7)
	}
	tight := make([]byte, I420Size(w, h))
	NV12ToI420(tight, src, stride, w, h)
	offset := make([]byte, I420Size(w, h))
	NV12ToI420Offset(offset, src, stride, stride*h, w, h)
	for i := range tight {
		if tight[i] != offset[i] {
			t.Fatalf("sample %d came back as %d against %d", i, offset[i], tight[i])
		}
	}
}

func TestNV12ToI420OffsetRefusesAChromaPlaneInsideTheLuma(t *testing.T) {
	const w, h = 4, 4
	dst := make([]byte, I420Size(w, h))
	for i := range dst {
		dst[i] = 0xAB
	}
	src := make([]byte, NV12Size(w, h))
	NV12ToI420Offset(dst, src, w, w*h-1, w, h)
	for i := range dst {
		if dst[i] != 0xAB {
			t.Fatalf("a chroma plane inside the luma still wrote sample %d", i)
		}
	}
}

func TestNV12ToI420OffsetRefusesAShortSource(t *testing.T) {
	const w, h = 4, 4
	dst := make([]byte, I420Size(w, h))
	for i := range dst {
		dst[i] = 0xCD
	}
	src := make([]byte, w*h)
	NV12ToI420Offset(dst, src, w, w*h, w, h)
	for i := range dst {
		if dst[i] != 0xCD {
			t.Fatalf("a short source still wrote sample %d", i)
		}
	}
}
