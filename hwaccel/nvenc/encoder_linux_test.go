package nvenc

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

const (
	probeWidth  = 176
	probeHeight = 144
	probeFrames = 12
)

func probeConfig() Config {
	return Config{
		Width: probeWidth, Height: probeHeight,
		FPSNum: 25, FPSDen: 1,
		BitrateBitsPerSecond: 2000000,
		GOPLength:            30,
		Profile:              ProfileMain,
	}
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

func requireAdapter(t *testing.T) {
	t.Helper()
	if !Available() {
		t.Skip("no NVIDIA encoder is reachable on this machine")
	}
}

func encodeProbeStream(t *testing.T) ([]byte, string) {
	t.Helper()
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
	tail, err := enc.Drain()
	if err != nil {
		t.Fatalf("%s: Drain: %v", enc.Name(), err)
	}
	return append(stream, tail...), enc.Name()
}

func decodeWithFFmpeg(t *testing.T, stream []byte) []byte {
	t.Helper()
	bin, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg is not installed, skipping the external check")
	}
	dir := t.TempDir()
	in := filepath.Join(dir, "in.264")
	out := filepath.Join(dir, "out.yuv")
	if err := os.WriteFile(in, stream, 0o600); err != nil {
		t.Fatal(err)
	}
	combined, err := exec.Command(bin, "-hide_banner", "-loglevel", "error", "-y",
		"-i", in, "-pix_fmt", "yuv420p", "-f", "rawvideo", out).CombinedOutput()
	if err != nil {
		t.Fatalf("ffmpeg refused the stream: %v\n%s", err, combined)
	}
	if len(combined) != 0 {
		t.Fatalf("ffmpeg reported problems reading the stream:\n%s", combined)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestMaxSupportedVersionIsAtLeastWhatWeTarget(t *testing.T) {
	requireAdapter(t)
	major, minor, err := MaxSupportedVersion()
	if err != nil {
		t.Fatalf("MaxSupportedVersion: %v", err)
	}
	if major < NvEncAPIMajorVersion || (major == NvEncAPIMajorVersion && minor < NvEncAPIMinorVersion) {
		t.Fatalf("the driver supports interface %d.%d, below the %d.%d this package targets",
			major, minor, NvEncAPIMajorVersion, NvEncAPIMinorVersion)
	}
	t.Logf("driver interface %d.%d, targeting %d.%d", major, minor, NvEncAPIMajorVersion, NvEncAPIMinorVersion)
}

func TestEncoderProducesAStream(t *testing.T) {
	requireAdapter(t)
	stream, name := encodeProbeStream(t)
	if len(stream) == 0 {
		t.Fatalf("%s produced no bytes from %d frames", name, probeFrames)
	}
	if len(stream) < 4 || stream[0] != 0 || stream[1] != 0 || stream[2] != 0 || stream[3] != 1 {
		t.Fatalf("%s did not begin with a start code", name)
	}
	t.Logf("%s produced %d bytes from %d frames", name, len(stream), probeFrames)
}

func TestFFmpegReadsWhatTheAdapterWrote(t *testing.T) {
	requireAdapter(t)
	stream, name := encodeProbeStream(t)
	data := decodeWithFFmpeg(t, stream)
	size := I420Size(probeWidth, probeHeight)
	if len(data) == 0 || len(data)%size != 0 {
		t.Fatalf("ffmpeg produced %d bytes, not a whole number of %d byte frames", len(data), size)
	}
	t.Logf("ffmpeg read %d pictures from %s", len(data)/size, name)
}

func TestEncoderRejectsShapesItCannotEncode(t *testing.T) {
	for name, cfg := range map[string]Config{
		"odd width":     {Width: 177, Height: 144, FPSNum: 25, FPSDen: 1},
		"zero height":   {Width: 176, Height: 0, FPSNum: 25, FPSDen: 1},
		"zero rate":     {Width: 176, Height: 144, FPSNum: 0, FPSDen: 1},
		"zero interval": {Width: 176, Height: 144, FPSNum: 25, FPSDen: 0},
	} {
		if _, err := Open(cfg); err == nil {
			t.Fatalf("%s was accepted", name)
		}
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

func TestDrainingTwiceIsHarmless(t *testing.T) {
	requireAdapter(t)
	enc, err := Open(probeConfig())
	if err != nil {
		t.Skipf("the encoder could not be opened: %v", err)
	}
	defer enc.Close()
	if _, err := enc.Encode(movingPattern(0)); err != nil {
		t.Fatalf("Encode: %v", err)
	}
	if _, err := enc.Drain(); err != nil {
		t.Fatalf("first Drain: %v", err)
	}
	second, err := enc.Drain()
	if err != nil {
		t.Fatalf("second Drain: %v", err)
	}
	if len(second) != 0 {
		t.Fatalf("the second drain produced %d bytes", len(second))
	}
}

func TestEncodersSurviveBeingCycledWithCollectionBetween(t *testing.T) {
	requireAdapter(t)
	for round := 0; round < 8; round++ {
		enc, err := Open(probeConfig())
		if err != nil {
			t.Skipf("round %d: the encoder could not be opened: %v", round, err)
		}
		for i := 0; i < 4; i++ {
			if _, err := enc.Encode(movingPattern(i)); err != nil {
				enc.Close()
				t.Fatalf("round %d frame %d: %v", round, i, err)
			}
		}
		if err := enc.Close(); err != nil {
			t.Fatalf("round %d: Close: %v", round, err)
		}
		runtime.GC()
	}
}

func TestEncodingSurvivesCollectionBetweenFrames(t *testing.T) {
	requireAdapter(t)
	enc, err := Open(probeConfig())
	if err != nil {
		t.Skipf("the encoder could not be opened: %v", err)
	}
	defer enc.Close()
	produced := 0
	for i := 0; i < 24; i++ {
		out, err := enc.Encode(movingPattern(i))
		if err != nil {
			t.Fatalf("frame %d: %v", i, err)
		}
		produced += len(out)
		runtime.GC()
	}
	if produced == 0 {
		t.Fatal("no bytes came back from twenty four frames")
	}
}
