package go264

import (
	"errors"
	"testing"
)

func TestEncodeFrameRefusesAFrameItCannotRead(t *testing.T) {
	const w, h = 64, 48
	e, err := NewEncoder(EncoderConfig{Width: w, Height: h, FPSNum: 30, FPSDen: 1, GOPSize: 30, QP: 26, ForceSoftware: true})
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	good := func() *Frame {
		return &Frame{
			Y: make([]byte, w*h), Cb: make([]byte, w*h/4), Cr: make([]byte, w*h/4),
			StrideY: w, StrideC: w / 2, Width: w, Height: h,
		}
	}
	cases := []struct {
		name string
		edit func(*Frame)
	}{
		{"a short luma plane", func(f *Frame) { f.Y = f.Y[:len(f.Y)-1] }},
		{"a short chroma plane", func(f *Frame) { f.Cr = f.Cr[:w/2] }},
		{"a stride narrower than the picture", func(f *Frame) { f.StrideY = w - 1 }},
		{"another width", func(f *Frame) { f.Width, f.StrideY, f.StrideC = w-16, w-16, (w-16)/2 }},
		{"another height", func(f *Frame) { f.Height = h - 16 }},
	}
	for _, c := range cases {
		f := good()
		c.edit(f)
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("%s: EncodeFrame panicked: %v", c.name, r)
				}
			}()
			if _, err := e.EncodeFrame(f, nil, nil); !errors.Is(err, ErrFrameSize) {
				t.Errorf("%s: EncodeFrame = %v, want ErrFrameSize", c.name, err)
			}
		}()
	}
	if _, err := e.EncodeFrame(good(), nil, nil); err != nil {
		t.Fatalf("a well formed frame was refused: %v", err)
	}
}

func TestEveryBackendRefusesABufferOfTheWrongSize(t *testing.T) {
	const w, h = 320, 240
	e, err := NewEncoder(EncoderConfig{Width: w, Height: h, FPSNum: 30, FPSDen: 1, GOPSize: 30, QP: 26})
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	for _, n := range []int{w*h*3/2 - 1, w*h*3/2 + w} {
		if _, err := e.Encode(make([]byte, n)); !errors.Is(err, ErrFrameSize) {
			t.Errorf("%s: Encode of %d bytes = %v, want ErrFrameSize", e.Backend(), n, err)
		}
		if _, err := e.EncodeWithHints(make([]byte, n), nil, nil); !errors.Is(err, ErrFrameSize) {
			t.Errorf("%s: EncodeWithHints of %d bytes = %v, want ErrFrameSize", e.Backend(), n, err)
		}
	}
}
