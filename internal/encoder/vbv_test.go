package encoder

import (
	"bytes"
	"math"
	"testing"

	"github.com/oops1/go.264/internal/bits"
	"github.com/oops1/go.264/internal/nal"
	"github.com/oops1/go.264/internal/syntax"
)

func vbvConfig(w, h, buffer, maxrate int) Config {
	return Config{Width: w, Height: h, FPSNum: 25, FPSDen: 1, GOPSize: 50, QP: 26,
		RefFrames: 1, VBVBufferKbits: buffer, VBVMaxrateKbps: maxrate}
}

type hrdParameters struct {
	bitRate      float64
	cpbSize      float64
	cbr          bool
	initialDelay float64
	fps          float64
}

func readHRD(t *testing.T, stream []byte) hrdParameters {
	t.Helper()
	var sps *syntax.SPS
	var h hrdParameters
	seen := false
	for _, ebsp := range nal.SplitAnnexB(stream) {
		u, err := nal.Parse(ebsp)
		if err != nil {
			t.Fatalf("parsing our own unit: %v", err)
		}
		switch u.Type {
		case nal.TypeSPS:
			if sps, err = syntax.ParseSPS(u.RBSP); err != nil {
				t.Fatalf("parsing our own SPS: %v", err)
			}
			if !sps.VUI.NalHRDPresent {
				t.Fatal("the sequence parameter set announces no buffer to the decoder")
			}
			hrd := &sps.VUI.NalHRD
			h.bitRate = float64(hrd.BitRateValueMinus1[0]+1) * math.Exp2(6+float64(hrd.BitRateScale))
			h.cpbSize = float64(hrd.CPBSizeValueMinus1[0]+1) * math.Exp2(4+float64(hrd.CPBSizeScale))
			h.cbr = hrd.CBRFlag[0]
			h.fps = float64(sps.VUI.TimeScale) / float64(2*sps.VUI.NumUnitsInTick)
		case nal.TypeSEI:
			if sps == nil || seen {
				continue
			}
			msgs, err := syntax.ParseSEI(u.RBSP, sps, func(uint32) *syntax.SPS { return sps })
			if err != nil {
				t.Fatalf("parsing our own SEI: %v", err)
			}
			for _, m := range msgs {
				if m.BufferingPeriod != nil && len(m.BufferingPeriod.NalHRD) != 0 {
					h.initialDelay = float64(m.BufferingPeriod.NalHRD[0].InitialCPBRemovalDelay)
					seen = true
				}
			}
		}
	}
	if sps == nil {
		t.Fatal("the stream carries no sequence parameter set")
	}
	if !seen {
		t.Fatal("the stream carries no buffering period message")
	}
	return h
}

func accessUnitSizes(t *testing.T, stream []byte) []int {
	t.Helper()
	var out []int
	size := 0
	haveVCL := false
	for _, ebsp := range nal.SplitAnnexB(stream) {
		u, err := nal.Parse(ebsp)
		if err != nil {
			t.Fatalf("parsing our own unit: %v", err)
		}
		first := false
		if u.Type.IsVCL() {
			v, err := bits.NewReader(u.RBSP).ReadUE()
			if err != nil {
				t.Fatalf("reading first_mb_in_slice: %v", err)
			}
			first = v == 0
		}
		trailing := u.Type == nal.TypeFillerData || u.Type == nal.TypeEndOfSequence ||
			u.Type == nal.TypeEndOfStream
		if haveVCL && !trailing && (!u.Type.IsVCL() || first) {
			out = append(out, size)
			size = 0
			haveVCL = false
		}
		size += 4 + len(ebsp)
		if u.Type.IsVCL() {
			haveVCL = true
		}
	}
	if size != 0 {
		out = append(out, size)
	}
	return out
}

type cpbReport struct {
	minFill  float64
	maxFill  float64
	average  float64
	consumed float64
}

func walkCPB(t *testing.T, h hrdParameters, sizes []int) cpbReport {
	t.Helper()
	fill := h.initialDelay / 90000 * h.bitRate
	start := fill
	arrival := h.bitRate / h.fps
	r := cpbReport{minFill: fill, maxFill: fill}
	total := 0.0
	for i, n := range sizes {
		b := float64(n * 8)
		total += b
		if b > fill {
			t.Fatalf("frame %d wanted %.0f bits with only %.0f in the buffer, so the coded picture buffer underflows",
				i, b, fill)
		}
		fill -= b
		if fill < r.minFill {
			r.minFill = fill
		}
		fill += arrival
		if fill > h.cpbSize {
			if h.cbr {
				t.Fatalf("after frame %d the buffer holds %.0f bits against a size of %.0f, so a constant bitrate stream overflows",
					i, fill, h.cpbSize)
			}
			fill = h.cpbSize
		}
		if fill > r.maxFill {
			r.maxFill = fill
		}
	}
	seconds := float64(len(sizes)) / h.fps
	r.average = total / seconds / 1000
	r.consumed = (total - start + fill) / seconds / 1000
	return r
}

func flattenUnits(units [][]byte) []byte {
	var out []byte
	for _, u := range units {
		out = append(out, u...)
	}
	return out
}

func TestVBVOffLeavesTheStreamUntouched(t *testing.T) {
	cfg := Config{Width: 176, Height: 144, FPSNum: 25, FPSDen: 1, GOPSize: 8, QP: 27, RefFrames: 1}
	frames := screenSequence(cfg.Width, cfg.Height, 8)
	before, _ := encodeUnits(t, cfg, frames)
	cfg.VBVBufferKbits = 0
	cfg.VBVMaxrateKbps = 0
	after, _ := encodeUnits(t, cfg, frames)
	for i := range before {
		if !bytes.Equal(before[i], after[i]) {
			t.Fatalf("frame %d differs although the buffer model is off", i)
		}
	}
}

func TestVBVRejectsHalfConfigurations(t *testing.T) {
	cases := []struct {
		name string
		edit func(c *Config)
	}{
		{"buffer without maxrate", func(c *Config) { c.VBVBufferKbits = 500 }},
		{"maxrate without buffer", func(c *Config) { c.VBVMaxrateKbps = 500 }},
		{"cbr without a buffer", func(c *Config) { c.CBR = true }},
		{"negative buffer", func(c *Config) { c.VBVBufferKbits = -1; c.VBVMaxrateKbps = 100 }},
		{"bitrate above maxrate", func(c *Config) {
			c.VBVBufferKbits = 500
			c.VBVMaxrateKbps = 400
			c.BitrateKbps = 900
		}},
	}
	for _, tc := range cases {
		cfg := Config{Width: 176, Height: 144, FPSNum: 25, FPSDen: 1, GOPSize: 8, QP: 26, RefFrames: 1}
		tc.edit(&cfg)
		if _, err := New(cfg); err == nil {
			t.Fatalf("%s was accepted", tc.name)
		}
	}
}

func TestVBVAnnouncesWhatItPromises(t *testing.T) {
	cfg := vbvConfig(320, 240, 600, 1500)
	units, _ := encodeUnits(t, cfg, screenSequence(cfg.Width, cfg.Height, 12))
	h := readHRD(t, flattenUnits(units))
	if h.bitRate < 1500*1000 || h.bitRate > 1500*1000+64 {
		t.Fatalf("the stream announces %.0f bit/s for a 1500 kbit/s request", h.bitRate)
	}
	if h.cpbSize < 600*1000 || h.cpbSize > 600*1000+16 {
		t.Fatalf("the stream announces a %.0f bit buffer for a 600 kbit request", h.cpbSize)
	}
	if h.cbr {
		t.Fatal("the stream claims constant bitrate although it was not asked for")
	}
	if h.fps != 25 {
		t.Fatalf("the stream announces %v frames per second", h.fps)
	}
}

func TestVBVKeepsTheBufferInsideItsBounds(t *testing.T) {
	cases := []struct {
		name   string
		buffer int
		rate   int
		edit   func(c *Config)
	}{
		{"small buffer", 200, 800, func(c *Config) {}},
		{"one second", 1000, 1000, func(c *Config) {}},
		{"tight", 120, 400, func(c *Config) {}},
		{"cabac", 400, 1200, func(c *Config) { c.CABAC = true }},
		{"slices", 400, 1200, func(c *Config) { c.Slices = 4 }},
		{"b pictures", 500, 1500, func(c *Config) { c.BFrames = 2; c.CABAC = true }},
		{"transform 8x8", 400, 1200, func(c *Config) { c.Transform8x8 = true; c.CABAC = true }},
		{"intra refresh", 400, 1200, func(c *Config) { c.IntraRefresh = 10; c.GOPSize = 1000 }},
		{"constant bitrate", 400, 1000, func(c *Config) { c.CBR = true }},
		{"constant bitrate cabac", 250, 700, func(c *Config) { c.CBR = true; c.CABAC = true }},
	}
	count := workload(150)
	for _, tc := range trim(cases, 2) {
		cfg := vbvConfig(320, 240, tc.buffer, tc.rate)
		tc.edit(&cfg)
		frames := make([][]byte, count)
		for i := range frames {
			if i%37 < 18 {
				frames[i] = screenFrame(cfg.Width, cfg.Height, i)
			} else {
				frames[i] = panningFrame(cfg.Width, cfg.Height, i)
			}
		}
		var stream []byte
		if cfg.BFrames > 0 {
			stream, _ = encodeBStream(t, cfg, frames, nil)
		} else {
			units, _ := encodeUnits(t, cfg, frames)
			stream = flattenUnits(units)
		}
		h := readHRD(t, stream)
		sizes := accessUnitSizes(t, stream)
		if len(sizes) != count {
			t.Fatalf("%s: the stream holds %d access units for %d frames", tc.name, len(sizes), count)
		}
		r := walkCPB(t, h, sizes)
		t.Logf("%s: asked %d kbit/s with a %d kbit buffer, coded %.0f kbit/s, drew %.0f kbit/s from the channel, buffer between %.0f%% and %.0f%% full",
			tc.name, tc.rate, tc.buffer, r.average, r.consumed, 100*r.minFill/h.cpbSize, 100*r.maxFill/h.cpbSize)
		if r.consumed > float64(tc.rate)*1.005 {
			t.Fatalf("%s: drew %.0f kbit/s from a %d kbit/s channel", tc.name, r.consumed, tc.rate)
		}
	}
}

func TestConstantBitrateStaysNearTheRequest(t *testing.T) {
	skipUnderRace(t)
	count := workload(200)
	cfg := vbvConfig(320, 240, 400, 1000)
	cfg.CBR = true
	frames := make([][]byte, count)
	for i := range frames {
		frames[i] = screenFrame(cfg.Width, cfg.Height, i)
	}
	units, _ := encodeUnits(t, cfg, frames)
	stream := flattenUnits(units)
	h := readHRD(t, stream)
	if !h.cbr {
		t.Fatal("a constant bitrate stream does not set cbr_flag")
	}
	r := walkCPB(t, h, accessUnitSizes(t, stream))
	t.Logf("constant bitrate: asked 1000 kbit/s, coded %.0f kbit/s, drew %.0f kbit/s from the channel, buffer between %.0f%% and %.0f%% full",
		r.average, r.consumed, 100*r.minFill/h.cpbSize, 100*r.maxFill/h.cpbSize)
	if r.consumed < 995 || r.consumed > 1005 {
		t.Fatalf("a constant bitrate run at 1000 kbit/s drew %.0f kbit/s from the channel", r.consumed)
	}
	if 100*r.maxFill/h.cpbSize < 50 {
		t.Fatalf("the constant bitrate buffer never rose above %.0f%% full", 100*r.maxFill/h.cpbSize)
	}
}

func TestVBVEmitsFillerWhenTheStreamIsTooSmall(t *testing.T) {
	cfg := vbvConfig(176, 144, 300, 900)
	cfg.CBR = true
	frames := make([][]byte, 60)
	still := flatFrame(cfg.Width, cfg.Height, 128, 128, 128)
	for i := range frames {
		frames[i] = still
	}
	units, _ := encodeUnits(t, cfg, frames)
	filler := 0
	for _, u := range units {
		for _, ty := range nalTypesIn(t, u) {
			if ty == nal.TypeFillerData {
				filler++
			}
		}
	}
	if filler == 0 {
		t.Fatal("a still picture at constant bitrate produced no filler, so the buffer would overflow")
	}
	stream := flattenUnits(units)
	h := readHRD(t, stream)
	walkCPB(t, h, accessUnitSizes(t, stream))
	t.Logf("a still picture at 900 kbit/s needed filler in %d of %d frames", filler, len(units))
}

func TestVBVReconstructionSurvivesTheDecoder(t *testing.T) {
	cases := []struct {
		name string
		edit func(c *Config)
	}{
		{"variable bitrate", func(c *Config) {}},
		{"cabac", func(c *Config) { c.CABAC = true }},
		{"slices", func(c *Config) { c.Slices = 3 }},
		{"constant bitrate", func(c *Config) { c.CBR = true }},
		{"constant bitrate cabac", func(c *Config) { c.CBR = true; c.CABAC = true }},
		{"transform 8x8", func(c *Config) { c.Transform8x8 = true; c.CABAC = true }},
		{"intra refresh", func(c *Config) { c.IntraRefresh = 7; c.GOPSize = 1000 }},
		{"dropped pictures", func(c *Config) { c.VBVBufferKbits = 60; c.VBVMaxrateKbps = 200 }},
	}
	count := workload(60)
	for _, tc := range trim(cases, 3) {
		cfg := vbvConfig(176, 144, 200, 700)
		tc.edit(&cfg)
		frames := make([][]byte, count)
		for i := range frames {
			if i%23 < 11 {
				frames[i] = screenFrame(cfg.Width, cfg.Height, i)
			} else {
				frames[i] = panningFrame(cfg.Width, cfg.Height, i)
			}
		}
		units, recons := encodeUnits(t, cfg, frames)
		assertMatchesReconstruction(t, tc.name, decodeUnits(t, units), recons)
	}
}

func TestFFmpegDecodesVBVStreams(t *testing.T) {
	skipUnderRace(t)
	for _, cbr := range []bool{false, true} {
		for _, cabac := range []bool{false, true} {
			cfg := vbvConfig(176, 144, 300, 900)
			cfg.CBR = cbr
			cfg.CABAC = cabac
			frames := screenSequence(cfg.Width, cfg.Height, 30)
			units, _ := encodeUnits(t, cfg, frames)
			assertFFmpegAgrees(t, "buffer model", cfg, units, decodeUnits(t, units))
		}
	}
}
