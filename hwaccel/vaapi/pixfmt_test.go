package vaapi

import "testing"

func TestI420Size(t *testing.T) {
	if got := I420Size(16, 16); got != 384 {
		t.Errorf("I420Size(16,16) = %d, want 384", got)
	}
	if got := I420Size(0, 16); got != 0 {
		t.Errorf("I420Size(0,16) = %d, want 0", got)
	}
	if got := I420Size(15, 16); got != 0 {
		t.Errorf("I420Size(15,16) = %d, want 0", got)
	}
}

func TestI420ToNV12RoundTrip(t *testing.T) {
	w, h := 16, 16
	src := make([]byte, I420Size(w, h))
	for i := range src {
		src[i] = byte(i)
	}
	strideY, strideUV := w, w
	dstY := make([]byte, strideY*h)
	dstUV := make([]byte, strideUV*(h/2))
	I420ToNV12(dstY, strideY, dstUV, strideUV, src, w, h)

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			want := src[y*w+x]
			got := dstY[y*strideY+x]
			if got != want {
				t.Fatalf("Y[%d][%d] = %d, want %d", y, x, got, want)
			}
		}
	}
	cw, ch := w/2, h/2
	cb := src[w*h:]
	cr := src[w*h+cw*ch:]
	for y := 0; y < ch; y++ {
		for x := 0; x < cw; x++ {
			wantU := cb[y*cw+x]
			wantV := cr[y*cw+x]
			gotU := dstUV[y*strideUV+2*x]
			gotV := dstUV[y*strideUV+2*x+1]
			if gotU != wantU || gotV != wantV {
				t.Fatalf("UV[%d][%d] = (%d,%d), want (%d,%d)", y, x, gotU, gotV, wantU, wantV)
			}
		}
	}
}

func TestI420ToNV12RejectsShortBuffers(t *testing.T) {
	w, h := 16, 16
	dstY := make([]byte, w*h)
	dstUV := make([]byte, w*(h/2))
	short := make([]byte, 4)
	I420ToNV12(dstY, w, dstUV, w, short, w, h)
	for _, b := range dstY {
		if b != 0 {
			t.Fatal("I420ToNV12 wrote into dstY despite a short source buffer")
		}
	}
}

func TestNV12PlaneSize(t *testing.T) {
	if got := NV12PlaneSize(16, 16, 16); got != 384 {
		t.Errorf("NV12PlaneSize(16,16,16) = %d, want 384", got)
	}
	if got := NV12PlaneSize(0, 16, 16); got != 0 {
		t.Errorf("NV12PlaneSize(0,16,16) = %d, want 0", got)
	}
}
