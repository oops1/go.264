package encoder

import (
	"bytes"
	"errors"
	"math"
	"testing"

	"github.com/oops1/go.264/internal/frame"
	"github.com/oops1/go.264/internal/syntax"
)

func constantQualityConfig(w, h int, rateFactor float64) Config {
	return Config{Width: w, Height: h, FPSNum: 25, FPSDen: 1, GOPSize: 250,
		RefFrames: 1, CABAC: true, RateFactor: rateFactor}
}

func TestConstantQualityRejectsContradictoryConfigurations(t *testing.T) {
	cases := []struct {
		name string
		edit func(c *Config)
	}{
		{"a rate factor and a bitrate", func(c *Config) { c.RateFactor = 23; c.BitrateKbps = 2000 }},
		{"a rate factor at constant bitrate", func(c *Config) {
			c.RateFactor = 23
			c.VBVBufferKbits = 500
			c.VBVMaxrateKbps = 1000
			c.CBR = true
		}},
		{"a negative rate factor", func(c *Config) { c.RateFactor = -1 }},
		{"a rate factor past the scale", func(c *Config) { c.RateFactor = 52 }},
		{"a curve past one", func(c *Config) { c.RateFactor = 23; c.QComp = 1.5 }},
		{"a negative curve", func(c *Config) { c.RateFactor = 23; c.QComp = -0.1 }},
		{"an I ratio below one", func(c *Config) { c.RateFactor = 23; c.IPRatio = 0.5 }},
		{"a B ratio below one", func(c *Config) { c.RateFactor = 23; c.PBRatio = 0.5 }},
		{"a curve without a rate factor", func(c *Config) { c.QComp = 0.6 }},
		{"ratios without a rate factor", func(c *Config) { c.IPRatio = 1.4 }},
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

func TestConstantQualityAcceptsARateFactorUnderACeiling(t *testing.T) {
	cfg := constantQualityConfig(176, 144, 23)
	cfg.VBVBufferKbits = 500
	cfg.VBVMaxrateKbps = 1000
	if _, err := New(cfg); err != nil {
		t.Fatalf("constant quality under a bitrate ceiling was refused: %v", err)
	}
}

func TestConstantQualityFillsInTheDefaults(t *testing.T) {
	cfg := constantQualityConfig(176, 144, 23)
	if err := cfg.validate(); err != nil {
		t.Fatal(err)
	}
	if cfg.QComp != crfQCompDefault {
		t.Errorf("QComp defaulted to %g, want %g", cfg.QComp, crfQCompDefault)
	}
	if cfg.IPRatio != crfIPRatioDefault {
		t.Errorf("IPRatio defaulted to %g, want %g", cfg.IPRatio, crfIPRatioDefault)
	}
	if cfg.PBRatio != crfPBRatioDefault {
		t.Errorf("PBRatio defaulted to %g, want %g", cfg.PBRatio, crfPBRatioDefault)
	}
}

func TestQuantiserScaleRoundTrips(t *testing.T) {
	for qp := 0; qp <= 51; qp++ {
		if back := qscale2qp(qp2qscale(float64(qp))); math.Abs(back-float64(qp)) > 1e-9 {
			t.Fatalf("qp %d became %g on the way back from the quantiser scale", qp, back)
		}
	}
	if q := qp2qscale(12); math.Abs(q-0.85) > 1e-12 {
		t.Fatalf("qp 12 sits at qscale %g, want 0.85", q)
	}
	if q := qp2qscale(18) / qp2qscale(12); math.Abs(q-2) > 1e-12 {
		t.Fatalf("six quantiser steps multiply the scale by %g, want 2", q)
	}
}

func TestConstantQualityCurvePassesThroughTheRateFactor(t *testing.T) {
	cfg := constantQualityConfig(1920, 1080, 23)
	if err := cfg.validate(); err != nil {
		t.Fatal(err)
	}
	c := newConstantQuality(cfg)
	if got := c.qp(c.baseComplexity, syntax.SliceP); math.Abs(got-23) > 1e-9 {
		t.Fatalf("at the base complexity a P picture asks for qp %g, want the rate factor 23", got)
	}
	for _, f := range []float64{0.25, 0.5, 2, 4} {
		want := 23 + 6*(1-cfg.QComp)*math.Log2(f)
		if want < c.floor {
			want = c.floor
		}
		c.last[syntax.SliceP] = -1
		if got := c.qp(c.baseComplexity*f, syntax.SliceP); math.Abs(got-want) > 1e-9 {
			t.Fatalf("at %g times the base complexity the curve asks for qp %g, want %g", f, got, want)
		}
	}
}

func TestConstantQualityHoldsTheQuantiserStillAtQCompOne(t *testing.T) {
	cfg := constantQualityConfig(1920, 1080, 23)
	cfg.QComp = 1
	if err := cfg.validate(); err != nil {
		t.Fatal(err)
	}
	c := newConstantQuality(cfg)
	for _, f := range []float64{0.01, 1, 100} {
		c.last[syntax.SliceP] = -1
		if got := c.qp(c.baseComplexity*f, syntax.SliceP); math.Abs(got-23) > 1e-9 {
			t.Fatalf("at a curve of one and %g times the base complexity the quantiser moved to %g", f, got)
		}
	}
}

func TestConstantQualityFavoursIPicturesAndPunishesBPictures(t *testing.T) {
	cfg := constantQualityConfig(1920, 1080, 30)
	cfg.BFrames = 2
	if err := cfg.validate(); err != nil {
		t.Fatal(err)
	}
	c := newConstantQuality(cfg)
	p := c.qp(c.baseComplexity, syntax.SliceP)
	c.last[syntax.SliceI] = -1
	i := c.qp(c.baseComplexity, syntax.SliceI)
	c.last[syntax.SliceB] = -1
	b := c.qp(c.baseComplexity, syntax.SliceB)
	if want := p - 6*math.Log2(cfg.IPRatio); math.Abs(i-want) > 1e-9 {
		t.Fatalf("an I picture asks for qp %g against %g for a P picture, want %g", i, p, want)
	}
	if want := p + 6*math.Log2(cfg.PBRatio); math.Abs(b-want) > 1e-9 {
		t.Fatalf("a B picture asks for qp %g against %g for a P picture, want %g", b, p, want)
	}
}

func TestConstantQualityWillNotWalkTheQuantiserOffTheScale(t *testing.T) {
	cfg := constantQualityConfig(1920, 1080, 23)
	if err := cfg.validate(); err != nil {
		t.Fatal(err)
	}
	c := newConstantQuality(cfg)
	if got := c.qp(1, syntax.SliceP); got != 23-crfQPFloorDrop {
		t.Fatalf("a picture with almost no complexity asks for qp %g, want the floor %g", got, 23-crfQPFloorDrop)
	}
	c.coded(23, syntax.SliceP)
	if got := c.qp(c.baseComplexity*1024, syntax.SliceP); got != 23+crfQPStep {
		t.Fatalf("a thousandfold jump in complexity moved the quantiser to %g, want one step of %g", got, crfQPStep)
	}
	c.coded(40, syntax.SliceP)
	if got := c.qp(1, syntax.SliceP); got != 40-crfQPStep {
		t.Fatalf("a collapse in complexity moved the quantiser to %g, want one step below 40", got)
	}
}

func TestConstantQualityIsDeterministic(t *testing.T) {
	const w, h = 320, 240
	frames := screencastClips()[3].frames(w, h, workload(24))
	cfg := constantQualityConfig(w, h, 23)
	first, _ := encodeUnits(t, cfg, frames)
	second, _ := encodeUnits(t, cfg, frames)
	for i := range first {
		if !bytes.Equal(first[i], second[i]) {
			t.Fatalf("frame %d differs between two runs of the same input", i)
		}
	}
}

func TestConstantQualityOffLeavesTheStreamUntouched(t *testing.T) {
	const w, h = 320, 240
	frames := screencastClips()[0].frames(w, h, workload(16))
	cfg := Config{Width: w, Height: h, FPSNum: 25, FPSDen: 1, GOPSize: 8, QP: 27, RefFrames: 1}
	before, _ := encodeUnits(t, cfg, frames)
	cfg.RateFactor = 0
	after, _ := encodeUnits(t, cfg, frames)
	for i := range before {
		if !bytes.Equal(before[i], after[i]) {
			t.Fatalf("frame %d differs although no rate factor was asked for", i)
		}
	}
}

func TestConstantQualityMovesTheQuantiserWithTheScene(t *testing.T) {
	const w, h = 320, 240
	frames := append(screencastClips()[0].frames(w, h, 20), screencastClips()[4].frames(w, h, 20)...)
	cfg := constantQualityConfig(w, h, 23)
	enc, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	var quiet, busy int
	for i, f := range frames {
		if _, err := enc.Encode(f); err != nil {
			t.Fatalf("frame %d: %v", i, err)
		}
		switch {
		case i == 19:
			quiet = enc.grid[0].QPY
		case i == len(frames)-1:
			busy = enc.grid[0].QPY
		}
	}
	t.Logf("the quiet scene settled at qp %d and the busy one at qp %d", quiet, busy)
	if busy <= quiet {
		t.Fatalf("a busy scene was given qp %d against qp %d for a still desktop, so the curve is not following the picture",
			busy, quiet)
	}
}

func TestConstantQualityStreamsSurviveOurOwnDecoder(t *testing.T) {
	const w, h = 176, 144
	cases := []struct {
		name string
		edit func(c *Config)
	}{
		{"cabac", func(c *Config) {}},
		{"cavlc", func(c *Config) { c.CABAC = false }},
		{"slices", func(c *Config) { c.Slices = 3 }},
		{"transform 8x8", func(c *Config) { c.Transform8x8 = true }},
		{"under a ceiling", func(c *Config) { c.VBVBufferKbits = 200; c.VBVMaxrateKbps = 700 }},
		{"short gop", func(c *Config) { c.GOPSize = 6 }},
	}
	count := workload(40)
	for _, tc := range trim(cases, 2) {
		cfg := constantQualityConfig(w, h, 26)
		tc.edit(&cfg)
		frames := make([][]byte, 0, count)
		for _, c := range screencastClips() {
			frames = append(frames, c.frames(w, h, count/len(screencastClips())+1)...)
		}
		frames = frames[:count]
		units, recons := encodeUnits(t, cfg, frames)
		assertMatchesReconstruction(t, tc.name, decodeUnits(t, units), recons)
	}
}

func TestConstantQualityWithBPicturesSurvivesOurOwnDecoder(t *testing.T) {
	const w, h = 176, 144
	cfg := constantQualityConfig(w, h, 26)
	cfg.BFrames = 2
	cfg.RefFrames = 2
	cfg.GOPSize = 12
	frames := screencastClips()[2].frames(w, h, workload(36))
	stream, recons := encodeBStream(t, cfg, frames, nil)
	assertMatchesReconstruction(t, "b pictures", decodeUnits(t, [][]byte{stream}), recons)
}

func TestFFmpegDecodesConstantQualityStreams(t *testing.T) {
	skipUnderRace(t)
	const w, h = 176, 144
	for _, cabac := range []bool{false, true} {
		cfg := constantQualityConfig(w, h, 24)
		cfg.CABAC = cabac
		cfg.GOPSize = 15
		var frames [][]byte
		for _, c := range trim(screencastClips(), 2) {
			frames = append(frames, c.frames(w, h, 6)...)
		}
		units, _ := encodeUnits(t, cfg, frames)
		assertFFmpegAgrees(t, "constant quality", cfg, units, decodeUnits(t, units))
	}
}

func TestConstantQualityUnderACeilingKeepsTheBufferInsideItsBounds(t *testing.T) {
	cases := []struct {
		name   string
		buffer int
		rate   int
		rf     float64
	}{
		{"a generous ceiling", 600, 1500, 26},
		{"a ceiling that bites", 200, 500, 20},
		{"a tight buffer", 120, 400, 23},
	}
	count := workload(120)
	for _, tc := range trim(cases, 2) {
		cfg := constantQualityConfig(320, 240, tc.rf)
		cfg.VBVBufferKbits = tc.buffer
		cfg.VBVMaxrateKbps = tc.rate
		frames := make([][]byte, 0, count)
		for len(frames) < count {
			for _, c := range screencastClips() {
				frames = append(frames, c.frames(cfg.Width, cfg.Height, 20)...)
			}
		}
		frames = frames[:count]
		units, _ := encodeUnits(t, cfg, frames)
		stream := flattenUnits(units)
		h := readHRD(t, stream)
		sizes := accessUnitSizes(t, stream)
		if len(sizes) != count {
			t.Fatalf("%s: the stream holds %d access units for %d frames", tc.name, len(sizes), count)
		}
		r := walkCPB(t, h, sizes)
		t.Logf("%s: rate factor %g under %d kbit/s with a %d kbit buffer coded %.0f kbit/s, drew %.0f kbit/s, buffer between %.0f%% and %.0f%% full",
			tc.name, tc.rf, tc.rate, tc.buffer, r.average, r.consumed,
			100*r.minFill/h.cpbSize, 100*r.maxFill/h.cpbSize)
		if r.consumed > float64(tc.rate)*1.005 {
			t.Fatalf("%s: drew %.0f kbit/s from a %d kbit/s channel", tc.name, r.consumed, tc.rate)
		}
	}
}

type qualityRun struct {
	kbps   float64
	mean   float64
	spread float64
	qpLo   int
	qpHi   int
	frames []float64
}

func ssimDecibels(v float64) float64 {
	if v >= 1 {
		return 100
	}
	return -10 * math.Log10(1-v)
}

func measureQuality(t *testing.T, cfg Config, frames [][]byte) qualityRun {
	t.Helper()
	enc, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	var recons []*frame.Picture
	enc.onPicture = func(_ int, p *frame.Picture) {
		recons = append(recons, snapshotOf(cfg, p))
	}
	run := qualityRun{qpLo: 51}
	bits := 0
	for i, f := range frames {
		pkt, err := enc.Encode(f)
		if err != nil {
			t.Fatalf("frame %d: Encode: %v", i, err)
		}
		bits += len(pkt) * 8
		q := enc.grid[0].QPY
		run.qpLo, run.qpHi = min(run.qpLo, q), max(run.qpHi, q)
	}
	tail, err := enc.Flush()
	if err != nil {
		t.Fatalf("Flush: %v", err)
	}
	bits += len(tail) * 8
	if len(recons) != len(frames) {
		t.Fatalf("the encoder reconstructed %d of %d frames", len(recons), len(frames))
	}
	luma := make([]byte, cfg.Width*cfg.Height)
	for i, p := range recons {
		for y := 0; y < cfg.Height; y++ {
			copy(luma[y*cfg.Width:(y+1)*cfg.Width], p.Y[p.LumaOffset(0, y):])
		}
		run.frames = append(run.frames,
			ssimDecibels(ssimLuma(luma, frames[i][:cfg.Width*cfg.Height], cfg.Width, cfg.Height)))
	}
	run.mean, run.spread = meanAndDeviation(run.frames)
	run.kbps = float64(bits) / float64(len(frames)) *
		float64(cfg.FPSNum) / float64(cfg.FPSDen) / 1000
	return run
}

func qualityAtRate(t *testing.T, curve []qualityRun, kbps float64) (mean, spread float64) {
	t.Helper()
	for i := 0; i+1 < len(curve); i++ {
		lo, hi := curve[i], curve[i+1]
		if lo.kbps > hi.kbps {
			lo, hi = hi, lo
		}
		if kbps < lo.kbps || kbps > hi.kbps {
			continue
		}
		f := (math.Log(kbps) - math.Log(lo.kbps)) / (math.Log(hi.kbps) - math.Log(lo.kbps))
		return lo.mean + f*(hi.mean-lo.mean), lo.spread + f*(hi.spread-lo.spread)
	}
	t.Fatalf("%.0f kbit/s falls outside the fixed quantiser curve", kbps)
	return 0, 0
}

func screencastSession(w, h, perScene int) [][]byte {
	var out [][]byte
	for _, c := range screencastClips() {
		out = append(out, c.frames(w, h, perScene)...)
	}
	return out
}

func skipUnlessMeasuring(t *testing.T) {
	t.Helper()
	if raceDetector {
		t.Skip("a whole 1920x1080 session is far too slow under the race detector, which has no concurrency to find here")
	}
	if testing.Short() {
		t.Skip("the comparison only means something over a whole 1920x1080 session, which is not a short test")
	}
}

const (
	sessionWidth    = 1920
	sessionHeight   = 1080
	sessionPerScene = 30
	sessionFactor   = 23
)

func fixedQuantiserCurve(t *testing.T, base Config, frames [][]byte, qps []int) []qualityRun {
	t.Helper()
	curve := make([]qualityRun, 0, len(qps))
	for _, qp := range qps {
		cfg := base
		cfg.QP = qp
		curve = append(curve, measureQuality(t, cfg, frames))
	}
	return curve
}

func TestConstantQualityAgainstAFixedQuantiserOnAMixedSession(t *testing.T) {
	skipUnlessMeasuring(t)
	frames := screencastSession(sessionWidth, sessionHeight, sessionPerScene)
	base := Config{Width: sessionWidth, Height: sessionHeight, FPSNum: 25, FPSDen: 1,
		GOPSize: 250, RefFrames: 1, CABAC: true}
	crf := base
	crf.RateFactor = sessionFactor
	got := measureQuality(t, crf, frames)
	curve := fixedQuantiserCurve(t, base, frames, []int{19, 23, 27})
	mean, spread := qualityAtRate(t, curve, got.kbps)
	t.Logf("a session of %d frames at rate factor %g coded %.0f kbit/s at %.2f dB, varying %.3f dB from frame to frame, with the quantiser between %d and %d; a fixed quantiser at the same rate gives %.2f dB varying %.3f dB",
		len(frames), float64(sessionFactor), got.kbps, got.mean, got.spread,
		got.qpLo, got.qpHi, mean, spread)

	if got.mean < mean+0.5 {
		t.Errorf("at %.0f kbit/s constant quality reached %.2f dB against %.2f dB for a fixed quantiser, which is not the gain this mode exists for",
			got.kbps, got.mean, mean)
	}

	if got.spread < spread-0.3 {
		t.Errorf("constant quality now varies %.3f dB against %.3f dB for a fixed quantiser at the same rate. This test records the opposite: a compressed complexity curve gives a busy scene a coarser quantiser than a quiet one, so across scenes of different complexity the spread widens rather than narrows. If something changed that, put the new figures here and say what did it",
			got.spread, spread)
	}
	if got.spread > spread+1.5 {
		t.Errorf("constant quality varies %.3f dB against %.3f dB for a fixed quantiser, a wider gap than the 0.7 dB a curve of %g measured; the quantiser is wandering further than it should",
			got.spread, spread, crfQCompDefault)
	}
}

func TestConstantQualityNarrowsTheSwingAcrossSceneChanges(t *testing.T) {
	skipUnlessMeasuring(t)
	const n = 60
	frames := screencastClips()[5].frames(sessionWidth, sessionHeight, n)
	base := Config{Width: sessionWidth, Height: sessionHeight, FPSNum: 25, FPSDen: 1,
		GOPSize: 250, RefFrames: 1, CABAC: true}
	crf := base
	crf.RateFactor = sessionFactor
	got := measureQuality(t, crf, frames)
	curve := fixedQuantiserCurve(t, base, frames, []int{14, 18, 22})
	mean, spread := qualityAtRate(t, curve, got.kbps)
	t.Logf("on a clip that keeps switching windows constant quality coded %.0f kbit/s at %.2f dB varying %.3f dB from frame to frame; a fixed quantiser at the same rate gives %.2f dB varying %.3f dB",
		got.kbps, got.mean, got.spread, mean, spread)
	if got.spread > spread {
		t.Fatalf("on a clip whose scenes keep changing constant quality varies %.3f dB against %.3f dB for a fixed quantiser, so it is no longer steadying the one case it steadies",
			got.spread, spread)
	}
}
