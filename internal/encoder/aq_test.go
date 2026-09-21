package encoder

import (
	"bytes"
	"errors"
	"image"
	"math"
	"testing"
)

func adaptiveQuantConfig(w, h int, strength float64) Config {
	return Config{Width: w, Height: h, FPSNum: 25, FPSDen: 1, GOPSize: 250, QP: 26,
		RefFrames: 1, CABAC: true, AQMode: AQVariance, AQStrength: strength}
}

func TestAdaptiveQuantRejectsContradictoryConfigurations(t *testing.T) {
	cases := []struct {
		name string
		edit func(c *Config)
	}{
		{"a mode off the scale", func(c *Config) { c.AQMode = 2 }},
		{"a strength without a mode", func(c *Config) { c.AQStrength = 1 }},
		{"a negative strength", func(c *Config) { c.AQMode = AQVariance; c.AQStrength = -1 }},
		{"a strength past the scale", func(c *Config) { c.AQMode = AQVariance; c.AQStrength = 5 }},
	}
	for _, tc := range cases {
		cfg := Config{Width: 176, Height: 144, FPSNum: 25, FPSDen: 1, GOPSize: 8, QP: 26, RefFrames: 1}
		tc.edit(&cfg)
		_, err := New(cfg)
		if err == nil {
			t.Errorf("%s was accepted", tc.name)
			continue
		}
		if !errors.Is(err, ErrConfig) {
			t.Errorf("%s was refused with %v, which is not a configuration error", tc.name, err)
		}
	}
}

func TestAdaptiveQuantIsOffByDefault(t *testing.T) {
	cfg := Config{Width: 176, Height: 144, FPSNum: 25, FPSDen: 1, GOPSize: 8, QP: 26, RefFrames: 1}
	if err := cfg.validate(); err != nil {
		t.Fatal(err)
	}
	if cfg.AQMode != AQOff {
		t.Fatalf("adaptive quantisation defaulted to mode %d; it loses 1.0 to 1.6 dB on scrolling text and stays off until that is fixed",
			cfg.AQMode)
	}
}

func TestAdaptiveQuantFillsInTheDefaultStrength(t *testing.T) {
	cfg := adaptiveQuantConfig(176, 144, 0)
	if err := cfg.validate(); err != nil {
		t.Fatal(err)
	}
	if cfg.AQStrength != aqStrengthDefault {
		t.Fatalf("AQStrength defaulted to %g, want %g", cfg.AQStrength, aqStrengthDefault)
	}
}

func TestAdaptiveQuantOffLeavesTheStreamUntouched(t *testing.T) {
	const w, h = 320, 240
	frames := screencastClips()[3].frames(w, h, workload(16))
	cfg := Config{Width: w, Height: h, FPSNum: 25, FPSDen: 1, GOPSize: 8, QP: 27, RefFrames: 1}
	before, _ := encodeUnits(t, cfg, frames)
	cfg.AQMode = AQOff
	after, _ := encodeUnits(t, cfg, frames)
	for i := range before {
		if !bytes.Equal(before[i], after[i]) {
			t.Fatalf("frame %d differs although adaptive quantisation is off", i)
		}
	}
}

func TestAdaptiveQuantSpendsFinerQuantisersOnDetail(t *testing.T) {
	const w, h = 320, 240
	cfg := adaptiveQuantConfig(w, h, 1)
	enc, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	enc.loadSourceInto(enc.src, screencastClips()[2].frames(w, h, 1)[0])
	hints := enc.adaptiveQuant(nil)
	if hints == nil || hints.qpOffset == nil {
		t.Fatal("adaptive quantisation produced no per-macroblock offsets")
	}
	var flat, busy, all []int
	for mby := 0; mby < enc.heightMBs; mby++ {
		for mbx := 0; mbx < enc.widthMBs; mbx++ {
			offset := int(hints.qpOffset[mby*enc.widthMBs+mbx])
			all = append(all, offset)
			if lumaEnergy(enc.src, mbx, mby) < 256 {
				flat = append(flat, offset)
			} else {
				busy = append(busy, offset)
			}
		}
	}
	if len(flat) == 0 || len(busy) == 0 {
		t.Fatalf("the picture has %d flat and %d detailed macroblocks, so there is nothing to compare",
			len(flat), len(busy))
	}
	t.Logf("flat macroblocks were moved %+.2f quantiser steps and detailed ones %+.2f, %+.2f over the picture",
		mean(flat), mean(busy), mean(all))
	if mean(flat) <= mean(busy) {
		t.Fatalf("flat macroblocks were moved %+.2f steps and detailed ones %+.2f; the finer quantiser is meant to go to the detail",
			mean(flat), mean(busy))
	}
	if math.Abs(mean(all)) > 2 {
		t.Fatalf("over the whole picture the quantiser moved %+.2f steps; the offsets are taken against the picture's own mean and should very nearly cancel",
			mean(all))
	}
}

func mean(v []int) float64 {
	sum := 0
	for _, x := range v {
		sum += x
	}
	return float64(sum) / float64(len(v))
}

func TestAdaptiveQuantReachesTheBitstream(t *testing.T) {
	const w, h = 320, 240
	cfg := adaptiveQuantConfig(w, h, 2)
	enc, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	frames := screencastClips()[4].frames(w, h, 3)
	for i, f := range frames {
		if _, err := enc.Encode(f); err != nil {
			t.Fatalf("frame %d: %v", i, err)
		}
	}
	seen := map[int]bool{}
	for i := range enc.grid {
		seen[enc.grid[i].QPY] = true
	}
	if len(seen) < 2 {
		t.Fatalf("every macroblock was coded at the same quantiser, so adaptive quantisation never reached the stream")
	}
	t.Logf("the last picture was coded with %d different quantisers", len(seen))
}

func TestAdaptiveQuantIsDeterministic(t *testing.T) {
	const w, h = 320, 240
	frames := screencastClips()[4].frames(w, h, workload(16))
	cfg := adaptiveQuantConfig(w, h, 1)
	first, _ := encodeUnits(t, cfg, frames)
	second, _ := encodeUnits(t, cfg, frames)
	for i := range first {
		if !bytes.Equal(first[i], second[i]) {
			t.Fatalf("frame %d differs between two runs of the same input", i)
		}
	}
}

func TestAdaptiveQuantStreamsSurviveOurOwnDecoder(t *testing.T) {
	const w, h = 176, 144
	cases := []struct {
		name string
		edit func(c *Config)
	}{
		{"cabac", func(c *Config) {}},
		{"cavlc", func(c *Config) { c.CABAC = false }},
		{"slices", func(c *Config) { c.Slices = 3 }},
		{"transform 8x8", func(c *Config) { c.Transform8x8 = true }},
		{"short gop", func(c *Config) { c.GOPSize = 5 }},
		{"intra refresh", func(c *Config) { c.IntraRefresh = 7; c.GOPSize = 1000 }},
		{"at full strength", func(c *Config) { c.AQStrength = 4 }},
		{"under a rate factor", func(c *Config) { c.RateFactor = 24; c.QP = 0 }},
		{"under a ceiling", func(c *Config) { c.VBVBufferKbits = 200; c.VBVMaxrateKbps = 700 }},
	}
	count := workload(30)
	for _, tc := range trim(cases, 3) {
		cfg := adaptiveQuantConfig(w, h, 1)
		tc.edit(&cfg)
		var frames [][]byte
		for _, c := range screencastClips() {
			frames = append(frames, c.frames(w, h, count/len(screencastClips())+1)...)
		}
		frames = frames[:count]
		units, recons := encodeUnits(t, cfg, frames)
		assertMatchesReconstruction(t, tc.name, decodeUnits(t, units), recons)
	}
}

func TestAdaptiveQuantWithBPicturesSurvivesOurOwnDecoder(t *testing.T) {
	const w, h = 176, 144
	cfg := adaptiveQuantConfig(w, h, 1)
	cfg.BFrames = 2
	cfg.RefFrames = 2
	cfg.GOPSize = 12
	frames := screencastClips()[2].frames(w, h, workload(24))
	stream, recons := encodeBStream(t, cfg, frames, nil)
	assertMatchesReconstruction(t, "b pictures", decodeUnits(t, [][]byte{stream}), recons)
}

func TestAdaptiveQuantComposesWithRegionHints(t *testing.T) {
	const w, h = 176, 144
	cfg := adaptiveQuantConfig(w, h, 2)
	frames := screencastClips()[2].frames(w, h, workload(16))
	hints := make([]Hints, len(frames))
	for i := range hints {
		hints[i] = Hints{Regions: []Region{
			{Rect: image.Rect(0, 0, w/2, h), Kind: RegionText},
			{Rect: image.Rect(w/2, 0, w, h), Kind: RegionImage},
		}}
	}
	units, recons := encodeWithHints(t, cfg, frames, hints)
	assertMatchesReconstruction(t, "regions", decodeUnits(t, units), recons)
}

func TestFFmpegDecodesAdaptiveQuantStreams(t *testing.T) {
	skipUnderRace(t)
	for _, cabac := range []bool{false, true} {
		const w, h = 176, 144
		cfg := adaptiveQuantConfig(w, h, 2)
		cfg.CABAC = cabac
		cfg.GOPSize = 12
		var frames [][]byte
		for _, c := range trim(screencastClips(), 2) {
			frames = append(frames, c.frames(w, h, 6)...)
		}
		units, _ := encodeUnits(t, cfg, frames)
		assertFFmpegAgrees(t, "adaptive quantisation", cfg, units, decodeUnits(t, units))
	}
}

func TestAdaptiveQuantIsStillWrongOnScrollingText(t *testing.T) {
	skipUnlessMeasuring(t)
	const n = 40
	frames := screencastClips()[2].frames(sessionWidth, sessionHeight, n)
	base := Config{Width: sessionWidth, Height: sessionHeight, FPSNum: 25, FPSDen: 1,
		GOPSize: 250, RefFrames: 1, CABAC: true}
	qps := []int{18, 22, 26, 30}
	plain := fixedQuantiserCurve(t, base, frames, append([]int{10, 14}, qps...))
	for i, p := range plain {
		t.Logf("flat quantiser point %d: %.0f kbit/s %.2f dB", i, p.kbps, p.mean)
	}
	aq := base
	aq.AQMode = AQVariance
	aq.AQStrength = 1
	worst, at, sum := 0.0, 0.0, 0.0
	for _, qp := range qps {
		cfg := aq
		cfg.QP = qp
		got := measureQuality(t, cfg, frames)
		want, _ := qualityAtRate(t, plain, got.kbps)
		t.Logf("qp %d: adaptive quantisation coded %.0f kbit/s at %.2f dB where a flat quantiser at the same rate gives %.2f dB",
			qp, got.kbps, got.mean, want)
		sum += got.mean - want
		if d := got.mean - want; d < worst {
			worst, at = d, got.kbps
		}
	}
	average := sum / float64(len(qps))
	t.Logf("across the curve adaptive quantisation is %+.2f dB on average at equal rate, worst %+.2f dB near %.0f kbit/s",
		average, worst, at)
	if average > 0 {
		t.Fatalf("adaptive quantisation is now %+.2f dB at equal rate on scrolling text, better than the 1.0 to 1.6 dB loss this records. If that is real, put the new figure here and consider turning the mode on by default",
			average)
	}
	if average < -3 {
		t.Fatalf("adaptive quantisation is %+.2f dB at equal rate on scrolling text, further below the 1.0 to 1.6 dB loss this records; something made a known defect deeper",
			average)
	}
}

func TestAdaptiveQuantOffsetsStayInsideTheLegalDelta(t *testing.T) {
	const w, h = 320, 240
	cfg := adaptiveQuantConfig(w, h, 4)
	enc, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	frames := screencastClips()[4].frames(w, h, 6)
	for i, f := range frames {
		if _, err := enc.Encode(f); err != nil {
			t.Fatalf("frame %d: %v", i, err)
		}
		lo, hi := 51, 0
		for mby := 0; mby < enc.heightMBs; mby++ {
			for mbx := 0; mbx < enc.widthMBs; mbx++ {
				qp := enc.at(mbx, mby).QPY
				lo, hi = min(lo, qp), max(hi, qp)
			}
		}
		if hi-lo > 25 {
			t.Fatalf("frame %d holds quantisers from %d to %d, so two neighbours could need an mb_qp_delta of %d, past the %d the syntax allows",
				i, lo, hi, hi-lo, 25)
		}
	}
}

func TestAdaptiveQuantEnergyIsTheVariance(t *testing.T) {
	const w, h = 64, 48
	cfg := adaptiveQuantConfig(w, h, 1)
	enc, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	flat := flatFrame(w, h, 128, 128, 128)
	if _, err := enc.Encode(flat); err != nil {
		t.Fatal(err)
	}
	if e := lumaEnergy(enc.src, 0, 0); e != 0 {
		t.Fatalf("a flat macroblock has energy %g, want none", e)
	}
	checker := make([]byte, len(flat))
	copy(checker, flat)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if (x/4+y/4)%2 == 0 {
				checker[y*w+x] = 0
			} else {
				checker[y*w+x] = 255
			}
		}
	}
	if _, err := enc.Encode(checker); err != nil {
		t.Fatal(err)
	}
	want := 255.0 * 255.0 / 4 * 256
	if e := lumaEnergy(enc.src, 0, 0); math.Abs(e-want) > 1 {
		t.Fatalf("a half black half white macroblock has energy %g, want %g", e, want)
	}
}
