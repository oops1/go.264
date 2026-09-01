package go264

import (
	"image"
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

func TestCABACRoundTripThroughThePublicAPI(t *testing.T) {
	const w, h = 320, 240
	cfg := EncoderConfig{Width: w, Height: h, FPSNum: 25, FPSDen: 1, GOPSize: 5, QP: 24,
		CABAC: true, ForceSoftware: true}
	enc, err := NewEncoder(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer enc.Close()

	var stream []byte
	var sources [][]byte
	for i := 0; i < 10; i++ {
		src := pattern(w, h, i)
		sources = append(sources, src)
		pkt, err := enc.Encode(src)
		if err != nil {
			t.Fatalf("frame %d: %v", i, err)
		}
		stream = append(stream, pkt...)
	}

	dec := NewDecoderWithConfig(DecoderConfig{ForceSoftware: true})
	defer dec.Close()
	frames, err := dec.Decode(stream)
	if err != nil {
		t.Fatalf("decoding our own CABAC stream: %v", err)
	}
	rest, err := dec.Flush()
	if err != nil {
		t.Fatal(err)
	}
	frames = append(frames, rest...)
	if len(frames) != len(sources) {
		t.Fatalf("decoded %d frames, want %d", len(frames), len(sources))
	}
	for i, f := range frames {
		if q := psnr(sources[i], f.AppendI420(nil)); q < 32 {
			t.Errorf("frame %d: PSNR only %.2f dB", i, q)
		}
	}
}

func TestCABACCostsFewerBitsThanCAVLC(t *testing.T) {
	const w, h = 320, 240
	sizes := map[bool]int{}
	for _, on := range []bool{false, true} {
		enc, err := NewEncoder(EncoderConfig{Width: w, Height: h, FPSNum: 25, FPSDen: 1,
			GOPSize: 10, QP: 26, CABAC: on, ForceSoftware: true})
		if err != nil {
			t.Fatal(err)
		}
		for i := 0; i < 12; i++ {
			pkt, err := enc.Encode(pattern(w, h, i))
			if err != nil {
				t.Fatal(err)
			}
			sizes[on] += len(pkt)
		}
		enc.Close()
	}
	if sizes[true] >= sizes[false] {
		t.Fatalf("CABAC spent %d bytes against CAVLC's %d", sizes[true], sizes[false])
	}
	t.Logf("CAVLC %d bytes, CABAC %d bytes, saving %.1f%%",
		sizes[false], sizes[true], 100*float64(sizes[false]-sizes[true])/float64(sizes[false]))
}

func TestSlicesTradeBitsForThreads(t *testing.T) {
	const w, h = 1920, 1080
	video := image.Rect(640, 300, 1280, 660)
	frames := make([][]byte, 6)
	for i := range frames {
		frames[i] = desktopFrame(w, h, i, video)
	}
	sizes := map[int]int{}
	for _, slices := range []int{1, 68} {
		enc, err := NewEncoder(EncoderConfig{Width: w, Height: h, FPSNum: 30, FPSDen: 1,
			GOPSize: 100, QP: 26, RefFrames: 1, Slices: slices, ForceSoftware: true})
		if err != nil {
			t.Fatal(err)
		}
		var stream []byte
		for i, f := range frames {
			pkt, err := enc.Encode(f)
			if err != nil {
				t.Fatalf("slices %d frame %d: %v", slices, i, err)
			}
			sizes[slices] += len(pkt)
			stream = append(stream, pkt...)
		}
		enc.Close()

		dec := NewDecoderWithConfig(DecoderConfig{ForceSoftware: true})
		decoded, err := dec.Decode(stream)
		if err != nil {
			t.Fatalf("slices %d: decoding our own stream: %v", slices, err)
		}
		rest, err := dec.Flush()
		if err != nil {
			t.Fatal(err)
		}
		dec.Close()
		decoded = append(decoded, rest...)
		if len(decoded) != len(frames) {
			t.Fatalf("slices %d: decoded %d frames, want %d", slices, len(decoded), len(frames))
		}
		for i, f := range decoded {
			if q := psnr(frames[i], f.AppendI420(nil)); q < 32 {
				t.Errorf("slices %d frame %d: PSNR only %.2f dB", slices, i, q)
			}
		}
	}
	overhead := 100 * float64(sizes[68]-sizes[1]) / float64(sizes[1])
	if overhead > 25 {
		t.Fatalf("one slice per macroblock row cost %.1f%% more bits, which is too much to pay for threads", overhead)
	}
	t.Logf("one slice %d bytes, 68 slices %d bytes, %.1f%% more", sizes[1], sizes[68], overhead)
}

func TestRoundTripWithBFrames(t *testing.T) {
	const w, h = 176, 144
	cfg := EncoderConfig{Width: w, Height: h, FPSNum: 25, FPSDen: 1, GOPSize: 8, QP: 26,
		BFrames: 2, RefFrames: 2, CABAC: true, ForceSoftware: true}
	enc, err := NewEncoder(cfg)
	if err != nil {
		t.Fatalf("NewEncoder: %v", err)
	}
	defer enc.Close()

	var stream []byte
	var sources [][]byte
	for i := 0; i < 10; i++ {
		src := pattern(w, h, i)
		sources = append(sources, src)
		pkt, err := enc.Encode(src)
		if err != nil {
			t.Fatalf("frame %d: %v", i, err)
		}
		stream = append(stream, pkt...)
	}
	tail, err := enc.Flush()
	if err != nil {
		t.Fatalf("Flush: %v", err)
	}
	stream = append(stream, tail...)

	dec := NewDecoderWithConfig(DecoderConfig{ForceSoftware: true})
	defer dec.Close()
	frames, err := dec.Decode(stream)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	rest, err := dec.Flush()
	if err != nil {
		t.Fatalf("decoder Flush: %v", err)
	}
	frames = append(frames, rest...)

	if len(frames) != len(sources) {
		t.Fatalf("decoded %d frames from %d sources", len(frames), len(sources))
	}
	for i, f := range frames {
		got := f.AppendI420(make([]byte, 0, f.I420Size()))
		if p := psnr(sources[i], got); p < 30 {
			t.Errorf("frame %d came back at %.1f dB, which is below the 30 dB floor", i, p)
		}
	}
}

func TestTransform8x8ReachesTheSoftwareEncoder(t *testing.T) {
	cfg := EncoderConfig{Width: 96, Height: 80, FPSNum: 25, FPSDen: 1, GOPSize: 4, QP: 27,
		Transform8x8: true, ScalingMatrix: ScalingMatrixJVT, ForceSoftware: true}
	enc, err := NewEncoder(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer enc.Close()

	var stream []byte
	for i := 0; i < 4; i++ {
		pkt, err := enc.Encode(pattern(cfg.Width, cfg.Height, i))
		if err != nil {
			t.Fatal(err)
		}
		stream = append(stream, pkt...)
	}
	tail, err := enc.Flush()
	if err != nil {
		t.Fatal(err)
	}
	stream = append(stream, tail...)

	d := NewDecoderWithConfig(DecoderConfig{ForceSoftware: true})
	defer d.Close()
	frames, err := d.Decode(stream)
	if err != nil {
		t.Fatalf("our decoder rejected a High profile stream from the public encoder: %v", err)
	}
	rest, err := d.Flush()
	if err != nil {
		t.Fatal(err)
	}
	if len(frames)+len(rest) != 4 {
		t.Fatalf("decoded %d frames, want 4", len(frames)+len(rest))
	}
}

func encodeThroughPublicAPI(t *testing.T, cfg EncoderConfig, count int) ([]byte, int) {
	t.Helper()
	cfg.ForceSoftware = true
	enc, err := NewEncoder(cfg)
	if err != nil {
		t.Fatalf("NewEncoder: %v", err)
	}
	defer enc.Close()
	var stream []byte
	peak := 0
	for i := 0; i < count; i++ {
		pkt, err := enc.Encode(pattern(cfg.Width, cfg.Height, i))
		if err != nil {
			t.Fatalf("frame %d: %v", i, err)
		}
		if len(pkt) > peak {
			peak = len(pkt)
		}
		stream = append(stream, pkt...)
	}
	tail, err := enc.Flush()
	if err != nil {
		t.Fatal(err)
	}
	return append(stream, tail...), peak
}

func decodeThroughPublicAPI(t *testing.T, stream []byte) int {
	t.Helper()
	d := NewDecoderWithConfig(DecoderConfig{ForceSoftware: true})
	defer d.Close()
	frames, err := d.Decode(stream)
	if err != nil {
		t.Fatalf("our decoder rejected our own stream: %v", err)
	}
	rest, err := d.Flush()
	if err != nil {
		t.Fatal(err)
	}
	return len(frames) + len(rest)
}

func TestIntraRefreshReachesTheSoftwareEncoder(t *testing.T) {
	cfg := EncoderConfig{Width: 176, Height: 144, FPSNum: 25, FPSDen: 1,
		GOPSize: 8, QP: 27, IntraRefresh: 8}
	refreshed, refreshPeak := encodeThroughPublicAPI(t, cfg, 24)
	if n := decodeThroughPublicAPI(t, refreshed); n != 24 {
		t.Fatalf("decoded %d frames, want 24", n)
	}

	cfg.IntraRefresh = 0
	keyed, keyedPeak := encodeThroughPublicAPI(t, cfg, 24)
	if n := decodeThroughPublicAPI(t, keyed); n != 24 {
		t.Fatalf("decoded %d frames, want 24", n)
	}
	if refreshPeak >= keyedPeak {
		t.Fatalf("intra refresh peaked at %d bytes against %d for key frames", refreshPeak, keyedPeak)
	}
	t.Logf("intra refresh %d bytes, peak %d; key frames %d bytes, peak %d",
		len(refreshed), refreshPeak, len(keyed), keyedPeak)
}

func TestDeblockingSettingsReachTheSoftwareEncoder(t *testing.T) {
	for _, mode := range []DeblockMode{DeblockingOn, DeblockingOff, DeblockingNotAcrossSlices} {
		cfg := EncoderConfig{Width: 176, Height: 144, FPSNum: 25, FPSDen: 1,
			GOPSize: 6, QP: 30, Slices: 3, Deblocking: mode,
			DeblockAlphaOffset: 3, DeblockBetaOffset: -3}
		stream, _ := encodeThroughPublicAPI(t, cfg, 8)
		if n := decodeThroughPublicAPI(t, stream); n != 8 {
			t.Fatalf("deblocking %d: decoded %d frames, want 8", mode, n)
		}
	}
	bad := EncoderConfig{Width: 64, Height: 64, QP: 26, DeblockAlphaOffset: 9, ForceSoftware: true}
	if _, err := NewEncoder(bad); err == nil {
		t.Fatal("an out of range deblocking offset was accepted")
	}
}

func TestBufferModelReachesTheSoftwareEncoder(t *testing.T) {
	cfg := EncoderConfig{Width: 176, Height: 144, FPSNum: 25, FPSDen: 1,
		GOPSize: 25, QP: 26, VBVBufferKbits: 300, VBVMaxrateKbps: 900, CBR: true}
	stream, _ := encodeThroughPublicAPI(t, cfg, 30)
	if n := decodeThroughPublicAPI(t, stream); n != 30 {
		t.Fatalf("decoded %d frames, want 30", n)
	}
	bad := EncoderConfig{Width: 64, Height: 64, QP: 26, CBR: true, ForceSoftware: true}
	if _, err := NewEncoder(bad); err == nil {
		t.Fatal("constant bitrate without a buffer was accepted")
	}
}

func TestWeightedPredictionReachesTheSoftwareEncoder(t *testing.T) {
	for _, mode := range []WeightedPrediction{
		WeightedPredictionOff, WeightedPredictionExplicit, WeightedPredictionImplicit} {
		cfg := EncoderConfig{Width: 176, Height: 144, FPSNum: 25, FPSDen: 1,
			GOPSize: 12, QP: 28, RefFrames: 2, BFrames: 2, CABAC: true,
			WeightedPrediction: mode}
		stream, _ := encodeThroughPublicAPI(t, cfg, 12)
		if n := decodeThroughPublicAPI(t, stream); n != 12 {
			t.Fatalf("weighted prediction %d: decoded %d frames, want 12", mode, n)
		}
	}
	bad := EncoderConfig{Width: 64, Height: 64, QP: 26, ForceSoftware: true,
		WeightedPrediction: WeightedPredictionImplicit + 1}
	if _, err := NewEncoder(bad); err == nil {
		t.Fatal("an unknown weighted prediction mode was accepted")
	}
}

func TestWeightedPredictionKeepsTheHardwarePathForTheDefault(t *testing.T) {
	plain := EncoderConfig{Width: 176, Height: 144, QP: 26}
	if plain.needsSoftware() {
		t.Fatal("a plain configuration was pushed onto the software encoder")
	}
	weighted := plain
	weighted.WeightedPrediction = WeightedPredictionExplicit
	if !weighted.needsSoftware() {
		t.Fatal("weighted prediction must select the software encoder")
	}
}

func TestDirectModeReachesTheSoftwareEncoder(t *testing.T) {
	for _, mode := range []DirectMode{DirectSpatial, DirectTemporal} {
		cfg := EncoderConfig{Width: 176, Height: 144, FPSNum: 25, FPSDen: 1,
			GOPSize: 12, QP: 28, RefFrames: 2, BFrames: 2, CABAC: true, DirectMode: mode}
		stream, _ := encodeThroughPublicAPI(t, cfg, 12)
		if n := decodeThroughPublicAPI(t, stream); n != 12 {
			t.Fatalf("direct mode %d: decoded %d frames, want 12", mode, n)
		}
	}
	bad := EncoderConfig{Width: 64, Height: 64, QP: 26, ForceSoftware: true,
		DirectMode: DirectTemporal + 1}
	if _, err := NewEncoder(bad); err == nil {
		t.Fatal("an unknown direct mode was accepted")
	}
}

func TestRepeatedParameterSetsReachTheSoftwareEncoder(t *testing.T) {
	cfg := EncoderConfig{Width: 176, Height: 144, FPSNum: 25, FPSDen: 1,
		GOPSize: 100, QP: 27, IntraRefresh: 6, RepeatParameterSets: true}
	repeated, _ := encodeThroughPublicAPI(t, cfg, 24)
	if n := decodeThroughPublicAPI(t, repeated); n != 24 {
		t.Fatalf("decoded %d frames, want 24", n)
	}
	cfg.RepeatParameterSets = false
	plain, _ := encodeThroughPublicAPI(t, cfg, 24)
	if len(repeated) <= len(plain) {
		t.Fatalf("repeating the parameter sets produced %d bytes against %d without", len(repeated), len(plain))
	}
	if n := decodeThroughPublicAPI(t, plain); n != 24 {
		t.Fatalf("decoded %d frames, want 24", n)
	}
	t.Logf("intra refresh over 24 frames: %d bytes plain, %d bytes with repeated parameter sets (+%.1f%%)",
		len(plain), len(repeated), 100*float64(len(repeated)-len(plain))/float64(len(plain)))
}
