package vaapi

import "testing"

func requireAdapter(t *testing.T) {
	t.Helper()
	if !Available() {
		t.Skip("libva.so.2 or libva-drm.so.2 is not available on this machine")
	}
	if _, err := openDisplay(); err != nil {
		t.Skipf("no VA-API render node could be opened: %v", err)
	}
}

func TestConfigValidation(t *testing.T) {
	cases := []struct {
		name string
		cfg  Config
		ok   bool
	}{
		{"zero width", Config{Width: 0, Height: 144, FPSNum: 25, FPSDen: 1}, false},
		{"odd width", Config{Width: 177, Height: 144, FPSNum: 25, FPSDen: 1}, false},
		{"odd height", Config{Width: 176, Height: 145, FPSNum: 25, FPSDen: 1}, false},
		{"zero fps num", Config{Width: 176, Height: 144, FPSNum: 0, FPSDen: 1}, false},
		{"zero fps den", Config{Width: 176, Height: 144, FPSNum: 25, FPSDen: 0}, false},
		{"not mb aligned but even", Config{Width: 1920, Height: 1080, FPSNum: 25, FPSDen: 1}, true},
		{"mb aligned", Config{Width: 176, Height: 144, FPSNum: 25, FPSDen: 1}, true},
	}
	for _, c := range cases {
		err := c.cfg.valid()
		if c.ok && err != nil {
			t.Errorf("%s: valid() = %v, want nil", c.name, err)
		}
		if !c.ok && err == nil {
			t.Errorf("%s: valid() = nil, want an error", c.name)
		}
	}
}

func TestMbAlign(t *testing.T) {
	cases := []struct{ in, want int }{
		{0, 0}, {1, 16}, {15, 16}, {16, 16}, {17, 32}, {1080, 1088}, {1920, 1920},
	}
	for _, c := range cases {
		if got := mbAlign(c.in); got != c.want {
			t.Errorf("mbAlign(%d) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestClampQP(t *testing.T) {
	cases := []struct {
		in   int
		want uint8
	}{
		{0, 26}, {-5, 26}, {1, 1}, {51, 51}, {52, 51}, {26, 26},
	}
	for _, c := range cases {
		if got := clampQP(c.in); got != c.want {
			t.Errorf("clampQP(%d) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestOpenRejectsBadConfig(t *testing.T) {
	if _, err := Open(Config{Width: 177, Height: 144, FPSNum: 25, FPSDen: 1}); err == nil {
		t.Fatal("Open accepted an odd width")
	}
}

func TestEncoderProbeWithoutAdapter(t *testing.T) {
	if Available() {
		t.Skip("libva is available on this machine; this test only covers the no-adapter path")
	}
	if _, err := Open(Config{Width: 176, Height: 144, FPSNum: 25, FPSDen: 1}); err == nil {
		t.Fatal("Open succeeded without libva.so.2 present")
	}
}

const (
	probeWidth  = 176
	probeHeight = 144
	probeFrames = 12
)

func probeConfig() Config {
	return Config{Width: probeWidth, Height: probeHeight, FPSNum: 25, FPSDen: 1, GOPLength: 30, QP: 26}
}

func movingPattern(i int) []byte {
	f := make([]byte, I420Size(probeWidth, probeHeight))
	for y := 0; y < probeHeight; y++ {
		for x := 0; x < probeWidth; x++ {
			f[y*probeWidth+x] = byte((x*3 + y*2 + i*9) & 0xFF)
		}
	}
	cw, ch := probeWidth/2, probeHeight/2
	cb := probeWidth * probeHeight
	cr := cb + cw*ch
	for y := 0; y < ch; y++ {
		for x := 0; x < cw; x++ {
			f[cb+y*cw+x] = byte(96 + (x+i)%64)
			f[cr+y*cw+x] = byte(160 - (y+i)%64)
		}
	}
	return f
}

func TestEncoderProducesAStream(t *testing.T) {
	requireAdapter(t)
	enc, err := Open(probeConfig())
	if err != nil {
		t.Skipf("the encoder could not be opened: %v", err)
	}
	defer enc.Close()

	var stream []byte
	for i := 0; i < probeFrames; i++ {
		out, err := enc.Encode(movingPattern(i))
		if err != nil {
			t.Fatalf("%s: frame %d: %v", enc.Name(), i, err)
		}
		stream = append(stream, out...)
	}
	if len(stream) == 0 {
		t.Fatalf("%s produced no bytes from %d frames", enc.Name(), probeFrames)
	}
	if len(stream) < 4 || stream[0] != 0 || stream[1] != 0 || stream[2] != 0 || stream[3] != 1 {
		t.Fatalf("%s did not begin with a start code", enc.Name())
	}
}

func TestEncoderRefusesShortFrames(t *testing.T) {
	requireAdapter(t)
	enc, err := Open(probeConfig())
	if err != nil {
		t.Skipf("the encoder could not be opened: %v", err)
	}
	defer enc.Close()
	if _, err := enc.Encode(make([]byte, 16)); err == nil {
		t.Fatal("a frame far shorter than the configured picture was accepted")
	}
}

func TestCloseIsSafeTwiceAndStopsFurtherWork(t *testing.T) {
	requireAdapter(t)
	enc, err := Open(probeConfig())
	if err != nil {
		t.Skipf("the encoder could not be opened: %v", err)
	}
	if err := enc.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := enc.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
	if _, err := enc.Encode(movingPattern(0)); err == nil {
		t.Fatal("a closed encoder accepted a frame")
	}
}
