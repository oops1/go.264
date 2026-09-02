package mf

import (
	"testing"

	"github.com/oops1/go.264/internal/hwaccel"
	"github.com/oops1/go.264/internal/testutil"
)

func TestDirect3DBackendIsRegistered(t *testing.T) {
	for _, name := range hwaccel.Available() {
		if name == Direct3DBackend {
			return
		}
	}
	t.Fatalf("the Direct3D backend is absent from %v", hwaccel.Available())
}

func TestWorthAcceleratingFollowsTheMeasuredCrossover(t *testing.T) {
	if WorthAccelerating(176, 144) {
		t.Fatal("a quarter common intermediate picture was called worth accelerating")
	}
	if WorthAccelerating(352, 288) {
		t.Fatal("a common intermediate picture was called worth accelerating")
	}
	if !WorthAccelerating(640, 480) {
		t.Fatal("a 640 by 480 picture was not called worth accelerating")
	}
	if !WorthAccelerating(1920, 1080) {
		t.Fatal("a 1920 by 1080 picture was not called worth accelerating")
	}
	if WorthAccelerating(0, 0) || WorthAccelerating(-1920, -1080) {
		t.Fatal("an absent picture size was called worth accelerating")
	}
}

func TestTheBackendRefusesASmallPicture(t *testing.T) {
	if dec, ok := openDirect3DDecoder(hwaccel.DecoderParams{Width: 176, Height: 144}); ok {
		dec.Close()
		t.Fatal("the backend took a picture it measures as slower than our own decoder")
	}
}

func TestTheBackendDecodesThroughTheFacade(t *testing.T) {
	if !Loaded() || !d3d11Available() {
		t.Skip("Media Foundation or Direct3D 11 is not present on this machine")
	}
	dec, name, ok := hwaccel.OpenDecoder(hwaccel.DecoderParams{Width: 1280, Height: 720})
	if !ok {
		t.Skip("no hardware decoding backend answered")
	}
	defer dec.Close()
	if name != Direct3DBackend {
		t.Skipf("the facade chose %q", name)
	}

	stream := syntheticStream(t, 640, 480, 12)
	pics, err := dec.Decode(stream.data)
	if err != nil {
		t.Fatalf("Decode through the facade: %v", err)
	}
	rest, err := dec.Flush()
	if err != nil {
		t.Fatalf("Flush through the facade: %v", err)
	}
	pics = append(pics, rest...)
	if len(pics) == 0 {
		t.Fatal("the facade produced no pictures")
	}
	for i, p := range pics {
		if p.Width != 640 || p.Height != 480 {
			t.Fatalf("picture %d came back as %dx%d", i, p.Width, p.Height)
		}
		if p.StrideY != p.Width || p.StrideC != p.Width/2 {
			t.Fatalf("picture %d came back with strides %d and %d", i, p.StrideY, p.StrideC)
		}
		if len(p.Y) != p.Width*p.Height {
			t.Fatalf("picture %d holds %d luma samples, want %d", i, len(p.Y), p.Width*p.Height)
		}
		if len(p.Cb) != len(p.Cr) || len(p.Cb) != (p.Width/2)*(p.Height/2) {
			t.Fatalf("picture %d holds %d and %d chroma samples", i, len(p.Cb), len(p.Cr))
		}
	}
}

func TestTheBackendPlanesPointIntoTheSamePicture(t *testing.T) {
	d := &direct3DDecoder{}
	src := []*DecodedPicture{{I420: make([]byte, I420Size(16, 16)), Width: 16, Height: 16}}
	for i := range src[0].I420 {
		src[0].I420[i] = byte(i)
	}
	out := d.pictures(src)
	if len(out) != 1 {
		t.Fatalf("one picture came back as %d", len(out))
	}
	p := out[0]
	if p.Y[0] != src[0].I420[0] || p.Cb[0] != src[0].I420[16*16] || p.Cr[0] != src[0].I420[16*16+64] {
		t.Fatalf("the planes start at %d, %d and %d", p.Y[0], p.Cb[0], p.Cr[0])
	}
	if len(d.pictures(nil)) != 0 {
		t.Fatal("no pictures came back as some")
	}
}

func TestTheCorpusIsBelowTheCrossover(t *testing.T) {
	for _, c := range append(append([]testutil.Clip{}, testutil.Corpus...), testutil.MainCorpus...) {
		if WorthAccelerating(c.Width, c.Height) {
			t.Fatalf("the clip %s at %dx%d is above the crossover", c.Name, c.Width, c.Height)
		}
	}
}
