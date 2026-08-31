package mf

import (
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/oops1/go.264/internal/decoder"
	"github.com/oops1/go.264/internal/frame"
)

const (
	probeWidth  = 176
	probeHeight = 144
	probeFrames = 12
)

func probeFormat() EncoderFormat {
	return EncoderFormat{
		Width: probeWidth, Height: probeHeight,
		FPSNum: 10, FPSDen: 1,
		BitrateBitsPerSecond: 2000000,
		Profile:              AVEncH264VProfileMain,
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

func encodeProbeStream(t *testing.T, hardwareOnly bool) ([]byte, [][]byte, string) {
	t.Helper()
	enc, err := OpenEncoder(probeFormat(), hardwareOnly)
	if err != nil {
		t.Skipf("no encoder transform could be opened: %v", err)
	}
	defer enc.Close()

	var stream []byte
	sources := make([][]byte, 0, probeFrames)
	for i := 0; i < probeFrames; i++ {
		src := movingPattern(i)
		sources = append(sources, src)
		out, err := enc.Encode(src)
		if err != nil {
			t.Fatalf("%s: Encode frame %d: %v", enc.Name(), i, err)
		}
		for _, p := range out {
			stream = append(stream, p...)
		}
	}
	rest, err := enc.Drain()
	if err != nil {
		t.Fatalf("%s: Drain: %v", enc.Name(), err)
	}
	for _, p := range rest {
		stream = append(stream, p...)
	}
	return stream, sources, enc.Name()
}

func decodeWithCPU(t *testing.T, stream []byte) []*frame.Picture {
	t.Helper()
	d := decoder.New()
	pics, err := d.Decode(stream)
	if err != nil {
		t.Fatalf("our decoder refused the stream: %v", err)
	}
	rest, err := d.Flush()
	if err != nil {
		t.Fatalf("our decoder refused to flush: %v", err)
	}
	return append(pics, rest...)
}

func psnr(a, b []byte) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var sum float64
	for i := range a {
		d := float64(int(a[i]) - int(b[i]))
		sum += d * d
	}
	mse := sum / float64(len(a))
	if mse == 0 {
		return math.Inf(1)
	}
	return 10 * math.Log10(255*255/mse)
}

func TestHardwareEncoderOutputIsReadableByOurDecoder(t *testing.T) {
	if !Loaded() {
		t.Skip("Media Foundation is not present on this machine")
	}
	stream, sources, name := encodeProbeStream(t, true)
	if len(stream) == 0 {
		t.Fatalf("%s produced no bytes at all", name)
	}
	pics := decodeWithCPU(t, stream)
	if len(pics) == 0 {
		t.Fatalf("%s produced a stream our decoder read as no pictures", name)
	}
	if len(pics) > len(sources) {
		t.Fatalf("%s produced %d pictures from %d frames", name, len(pics), len(sources))
	}
	for i, p := range pics {
		if p.CropWidth != probeWidth || p.CropHeight != probeHeight {
			t.Fatalf("%s picture %d is %dx%d, want %dx%d",
				name, i, p.CropWidth, p.CropHeight, probeWidth, probeHeight)
		}
	}
	t.Logf("%s: %d bytes, %d pictures out of %d frames", name, len(stream), len(pics), len(sources))
}

func TestHardwareEncoderKeepsTheLumaItWasGiven(t *testing.T) {
	if !Loaded() {
		t.Skip("Media Foundation is not present on this machine")
	}
	stream, sources, name := encodeProbeStream(t, true)
	pics := decodeWithCPU(t, stream)
	if len(pics) == 0 {
		t.Skipf("%s produced no pictures to compare", name)
	}

	got := make([]byte, I420Size(probeWidth, probeHeight))
	pics[0].CopyOut(got)
	luma := probeWidth * probeHeight
	best := 0.0
	for _, src := range sources {
		if v := psnr(got[:luma], src[:luma]); v > best {
			best = v
		}
	}
	if best < 30 {
		t.Fatalf("%s: the first decoded picture matches no source frame above 30 dB, best was %.1f dB",
			name, best)
	}
	t.Logf("%s: first picture matches its source at %.1f dB", name, best)
}

func TestEncoderRefusesFramesItCannotHold(t *testing.T) {
	if !Loaded() {
		t.Skip("Media Foundation is not present on this machine")
	}
	enc, err := OpenEncoder(probeFormat(), false)
	if err != nil {
		t.Skipf("no encoder transform could be opened: %v", err)
	}
	defer enc.Close()
	if _, err := enc.Encode(make([]byte, 16)); err == nil {
		t.Fatal("a frame far too short for the configured size was accepted")
	}
}

func TestOpenEncoderRejectsImpossibleFormats(t *testing.T) {
	cases := map[string]EncoderFormat{
		"odd width":     {Width: 177, Height: 144, FPSNum: 10, FPSDen: 1},
		"zero height":   {Width: 176, Height: 0, FPSNum: 10, FPSDen: 1},
		"zero rate":     {Width: 176, Height: 144, FPSNum: 0, FPSDen: 1},
		"zero interval": {Width: 176, Height: 144, FPSNum: 10, FPSDen: 0},
	}
	for name, f := range cases {
		if _, err := OpenEncoder(f, false); err == nil {
			t.Fatalf("%s was accepted", name)
		}
	}
}

func TestEncoderCloseIsSafeTwice(t *testing.T) {
	if !Loaded() {
		t.Skip("Media Foundation is not present on this machine")
	}
	enc, err := OpenEncoder(probeFormat(), false)
	if err != nil {
		t.Skipf("no encoder transform could be opened: %v", err)
	}
	if err := enc.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := enc.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
	if _, err := enc.Encode(make([]byte, I420Size(probeWidth, probeHeight))); err == nil {
		t.Fatal("a closed encoder accepted a frame")
	}
}

func decodeWithFFmpeg(t *testing.T, stream []byte) []byte {
	t.Helper()
	bin, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg is not installed, skipping the external conformance check")
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
		t.Fatalf("ffmpeg rejected the hardware encoder's stream: %v\n%s", err, combined)
	}
	if len(combined) != 0 {
		t.Fatalf("ffmpeg reported problems decoding the hardware encoder's stream:\n%s", combined)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestOurDecoderAgreesWithFFmpegOnAHardwareStream(t *testing.T) {
	if !Loaded() {
		t.Skip("Media Foundation is not present on this machine")
	}
	stream, _, name := encodeProbeStream(t, true)
	pics := decodeWithCPU(t, stream)
	if len(pics) == 0 {
		t.Skipf("%s produced no pictures to compare", name)
	}
	ref := decodeWithFFmpeg(t, stream)

	size := I420Size(probeWidth, probeHeight)
	if len(ref) != size*len(pics) {
		t.Fatalf("ffmpeg produced %d bytes for %d pictures, want %d", len(ref), len(pics), size*len(pics))
	}
	got := make([]byte, size)
	for i, p := range pics {
		p.CopyOut(got)
		want := ref[i*size : (i+1)*size]
		for j := range got {
			if got[j] != want[j] {
				t.Fatalf("%s picture %d: ffmpeg and our decoder disagree at sample %d, ffmpeg %d ours %d",
					name, i, j, want[j], got[j])
			}
		}
	}
	t.Logf("%s: %d pictures decode identically in ffmpeg and in our decoder", name, len(pics))
}
