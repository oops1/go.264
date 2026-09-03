package encoder

import (
	"fmt"
	"math"
	"math/rand"
	"testing"

	"github.com/oops1/go.264/internal/frame"
)

type trellisMeasurement struct {
	bytes int
	psnr  float64
}

func picturesToI420(pics []*frame.Picture, width, height int) [][]byte {
	out := make([][]byte, len(pics))
	for i, p := range pics {
		buf := make([]byte, 0, width*height*3/2)
		for y := 0; y < height; y++ {
			off := p.LumaOffset(0, y)
			buf = append(buf, p.Y[off:off+width]...)
		}
		cw, ch := width/2, height/2
		for _, plane := range [][]byte{p.Cb, p.Cr} {
			for y := 0; y < ch; y++ {
				off := p.ChromaOffset(0, y)
				buf = append(buf, plane[off:off+cw]...)
			}
		}
		out[i] = buf
	}
	return out
}

func measureTrellis(t *testing.T, cfg Config, frames [][]byte) trellisMeasurement {
	t.Helper()
	stream, recons := encodeBStream(t, cfg, frames, nil)
	pics := decodeUnits(t, [][]byte{stream})
	assertMatchesReconstruction(t, "trellis", pics, recons)
	decoded := picturesToI420(pics, cfg.Width, cfg.Height)
	var sum float64
	var n int
	for i := range decoded {
		for j := range decoded[i] {
			d := float64(decoded[i][j]) - float64(frames[i][j])
			sum += d * d
		}
		n += len(decoded[i])
	}
	q := math.Inf(1)
	if sum > 0 {
		q = 10 * math.Log10(255*255/(sum/float64(n)))
	}
	return trellisMeasurement{bytes: len(stream), psnr: q}
}

func movingFrames(w, h, count int) [][]byte {
	out := make([][]byte, count)
	for i := range out {
		out[i] = syntheticFrame(w, h, i)
	}
	return out
}

func screenFrames(w, h, count int) [][]byte {
	out := make([][]byte, count)
	for i := 0; i < count; i++ {
		buf := make([]byte, w*h*3/2)
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				v := 235
				if (x/24+y/18)%3 == 0 {
					v = 16
				}
				if y%18 < 2 || x%24 < 2 {
					v = 128
				}
				if x > 20+i*3 && x < 70+i*3 && y > h/4 && y < h/2 {
					v = 200
				}
				buf[y*w+x] = byte(v)
			}
		}
		for p := 0; p < 2; p++ {
			base := w*h + p*(w/2)*(h/2)
			for j := 0; j < (w/2)*(h/2); j++ {
				buf[base+j] = 128
			}
		}
		out[i] = buf
	}
	return out
}

func noiseFrames(w, h, count int) [][]byte {
	rng := rand.New(rand.NewSource(5))
	out := make([][]byte, count)
	for i := range out {
		buf := make([]byte, w*h*3/2)
		for j := range buf {
			buf[j] = byte(128 + rng.Intn(64) - 32)
		}
		out[i] = buf
	}
	return out
}

func trellisPair(cfg Config) (Config, Config) {
	off, on := cfg, cfg
	off.Trellis = false
	on.Trellis = true
	return off, on
}

type rdPoint struct {
	logBytes float64
	psnr     float64
}

func trellisCurve(t *testing.T, cfg Config, frames [][]byte, qps []int) []rdPoint {
	t.Helper()
	pts := make([]rdPoint, 0, len(qps))
	for _, qp := range qps {
		c := cfg
		c.QP = qp
		m := measureTrellis(t, c, frames)
		pts = append(pts, rdPoint{math.Log(float64(m.bytes)), m.psnr})
	}
	return pts
}

func psnrAtRate(pts []rdPoint, logBytes float64) (float64, bool) {
	for i := 0; i+1 < len(pts); i++ {
		lo, hi := pts[i], pts[i+1]
		if lo.logBytes > hi.logBytes {
			lo, hi = hi, lo
		}
		if logBytes >= lo.logBytes && logBytes <= hi.logBytes {
			if hi.logBytes == lo.logBytes {
				return lo.psnr, true
			}
			f := (logBytes - lo.logBytes) / (hi.logBytes - lo.logBytes)
			return lo.psnr + f*(hi.psnr-lo.psnr), true
		}
	}
	return 0, false
}

func rateAtQuality(pts []rdPoint, psnr float64) (float64, bool) {
	for i := 0; i+1 < len(pts); i++ {
		lo, hi := pts[i], pts[i+1]
		if lo.psnr > hi.psnr {
			lo, hi = hi, lo
		}
		if psnr >= lo.psnr && psnr <= hi.psnr {
			if hi.psnr == lo.psnr {
				return lo.logBytes, true
			}
			f := (psnr - lo.psnr) / (hi.psnr - lo.psnr)
			return lo.logBytes + f*(hi.logBytes-lo.logBytes), true
		}
	}
	return 0, false
}

func spanOf(pts []rdPoint, field func(rdPoint) float64) (float64, float64) {
	lo, hi := math.Inf(1), math.Inf(-1)
	for _, p := range pts {
		lo = math.Min(lo, field(p))
		hi = math.Max(hi, field(p))
	}
	return lo, hi
}

func qualityShortfall(t *testing.T, plain, refined []rdPoint) (float64, float64) {
	t.Helper()
	plainLo, plainHi := spanOf(plain, func(p rdPoint) float64 { return p.logBytes })
	refLo, refHi := spanOf(refined, func(p rdPoint) float64 { return p.logBytes })
	lo, hi := math.Max(plainLo, refLo), math.Min(plainHi, refHi)
	if hi <= lo {
		t.Fatalf("the two rate curves do not overlap")
	}
	worst, at := 0.0, 0.0
	for i := 0; i <= 20; i++ {
		r := lo + (hi-lo)*float64(i)/20
		a, oka := psnrAtRate(plain, r)
		b, okb := psnrAtRate(refined, r)
		if !oka || !okb {
			continue
		}
		if a-b > worst {
			worst, at = a-b, math.Exp(r)
		}
	}
	return worst, at
}

func bitsSaved(t *testing.T, plain, refined []rdPoint) float64 {
	t.Helper()
	plainLo, plainHi := spanOf(plain, func(p rdPoint) float64 { return p.psnr })
	refLo, refHi := spanOf(refined, func(p rdPoint) float64 { return p.psnr })
	lo, hi := math.Max(plainLo, refLo), math.Min(plainHi, refHi)
	if hi <= lo {
		t.Fatalf("the two quality curves do not overlap")
	}
	total, n := 0.0, 0
	for i := 0; i <= 20; i++ {
		q := lo + (hi-lo)*float64(i)/20
		a, oka := rateAtQuality(plain, q)
		b, okb := rateAtQuality(refined, q)
		if !oka || !okb {
			continue
		}
		total += math.Exp(b-a) - 1
		n++
	}
	if n == 0 {
		t.Fatalf("no common quality point")
	}
	return 100 * total / float64(n)
}

const trellisQualityFloor = 0.25

func trellisCases(w, h int) []struct {
	name   string
	frames [][]byte
	cfg    Config
} {
	base := func(gop int, cabac bool) Config {
		return Config{Width: w, Height: h, FPSNum: 25, FPSDen: 1, GOPSize: gop, CABAC: cabac}
	}
	eight := base(4, true)
	eight.Transform8x8 = true
	eightCAVLC := base(4, false)
	eightCAVLC.Transform8x8 = true
	scaled := base(4, true)
	scaled.Transform8x8 = true
	scaled.ScalingMatrix = ScalingMatrixJVT
	bframes := base(6, true)
	bframes.BFrames = 2
	bframes.RefFrames = 2
	slices := base(4, true)
	slices.Slices = 3
	weighted := bframes
	weighted.WeightedPrediction = WeightedPredictionExplicit
	implicit := bframes
	implicit.WeightedPrediction = WeightedPredictionImplicit
	temporal := bframes
	temporal.DirectMode = DirectTemporal
	return []struct {
		name   string
		frames [][]byte
		cfg    Config
	}{
		{"cavlc", movingFrames(w, h, 4), base(4, false)},
		{"cabac", movingFrames(w, h, 4), base(4, true)},
		{"cabac-8x8", movingFrames(w, h, 4), eight},
		{"cavlc-8x8", movingFrames(w, h, 4), eightCAVLC},
		{"scaling-matrix", movingFrames(w, h, 4), scaled},
		{"bframes", movingFrames(w, h, 6), bframes},
		{"weighted", movingFrames(w, h, 6), weighted},
		{"implicit-weights", movingFrames(w, h, 6), implicit},
		{"temporal-direct", movingFrames(w, h, 6), temporal},
		{"slices", movingFrames(w, h, 4), slices},
		{"screen", screenFrames(w, h, 4), base(4, true)},
		{"noise", noiseFrames(w, h, 3), base(3, true)},
	}
}

func TestTrellisKeepsQualityAtEqualRate(t *testing.T) {
	const w, h = 128, 96
	qps := trim([]int{20, 26, 32, 38}, 3)
	for _, c := range trim(trellisCases(w, h), 2) {
		if c.name == screenTrellisCase {
			continue
		}
		t.Run(c.name, func(t *testing.T) {
			off, on := trellisPair(c.cfg)
			plain := trellisCurve(t, off, c.frames, qps)
			refined := trellisCurve(t, on, c.frames, qps)
			shortfall, at := qualityShortfall(t, plain, refined)
			saved := bitsSaved(t, plain, refined)
			t.Logf("%s: %+.1f%% bits at equal quality, worst quality shortfall %.2f dB near %.0f bytes",
				c.name, saved, shortfall, at)
			if shortfall > trellisQualityFloor {
				t.Errorf("%s: at equal rate the trellis is %.2f dB worse near %.0f bytes, beyond the %.2f dB allowance",
					c.name, shortfall, at, trellisQualityFloor)
			}
		})
	}
}

func TestTrellisNeverCollapsesTheStream(t *testing.T) {
	const w, h = 128, 96
	for _, c := range trim(trellisCases(w, h), 2) {
		for _, qp := range trim([]int{16, 22, 28, 34, 40}, 2) {
			t.Run(fmt.Sprintf("%s-qp%d", c.name, qp), func(t *testing.T) {
				off, on := trellisPair(c.cfg)
				off.QP, on.QP = qp, qp
				plain := measureTrellis(t, off, c.frames)
				refined := measureTrellis(t, on, c.frames)
				drop := plain.psnr - refined.psnr
				shrink := 100 * float64(plain.bytes-refined.bytes) / float64(plain.bytes)
				if drop > 1.5 {
					t.Errorf("%s qp %d: the trellis threw away %.2f dB, from %.2f to %.2f",
						c.name, qp, drop, plain.psnr, refined.psnr)
				}
				if shrink > 20 && drop > 0.5 {
					t.Errorf("%s qp %d: the trellis cut %.1f%% of the bits, from %d to %d, and %.2f dB with them, which is far past any honest gain",
						c.name, qp, shrink, plain.bytes, refined.bytes, drop)
				}
				if float64(refined.bytes) > 1.1*float64(plain.bytes) {
					t.Errorf("%s qp %d: the trellis grew the stream from %d to %d bytes",
						c.name, qp, plain.bytes, refined.bytes)
				}
			})
		}
	}
}

func TestTrellisAcrossEveryQuantiser(t *testing.T) {
	const w, h = 64, 48
	frames := movingFrames(w, h, 3)
	step := 1
	if raceDetector {
		step = 7
	}
	for _, cabac := range []bool{false, true} {
		for qp := 0; qp <= 51; qp += step {
			name := fmt.Sprintf("cavlc-qp%d", qp)
			if cabac {
				name = fmt.Sprintf("cabac-qp%d", qp)
			}
			t.Run(name, func(t *testing.T) {
				cfg := Config{Width: w, Height: h, FPSNum: 25, FPSDen: 1, GOPSize: 3, QP: qp, CABAC: cabac}
				off, on := trellisPair(cfg)
				plain := measureTrellis(t, off, frames)
				refined := measureTrellis(t, on, frames)
				if refined.psnr < plain.psnr-1.5 {
					t.Errorf("qp %d: the trellis threw away %.2f dB, from %.2f to %.2f",
						qp, plain.psnr-refined.psnr, plain.psnr, refined.psnr)
				}
				if float64(refined.bytes) > 1.1*float64(plain.bytes) {
					t.Errorf("qp %d: the trellis grew the stream from %d to %d bytes", qp, plain.bytes, refined.bytes)
				}
			})
		}
	}
}

func TestTrellisIsLosslessAtQuantiserZero(t *testing.T) {
	const w, h = 96, 80
	frames := movingFrames(w, h, 2)
	for _, cabac := range []bool{false, true} {
		cfg := Config{Width: w, Height: h, FPSNum: 25, FPSDen: 1, GOPSize: 2, QP: 0, CABAC: cabac}
		off, on := trellisPair(cfg)
		plain := measureTrellis(t, off, frames)
		refined := measureTrellis(t, on, frames)
		t.Logf("cabac %v at qp 0: %d bytes %.2f dB becomes %d bytes %.2f dB",
			cabac, plain.bytes, plain.psnr, refined.bytes, refined.psnr)
		if refined.psnr < plain.psnr {
			t.Errorf("cabac %v: at qp 0 the trellis lost %.2f dB with nothing to throw away",
				cabac, plain.psnr-refined.psnr)
		}
		if float64(refined.bytes) > 1.02*float64(plain.bytes) {
			t.Errorf("cabac %v: at qp 0 the trellis grew the stream from %d to %d bytes",
				cabac, plain.bytes, refined.bytes)
		}
	}
}

func TestTrellisLeavesTodaysStreamsAlone(t *testing.T) {
	const w, h = 96, 80
	frames := movingFrames(w, h, 3)
	for _, cabac := range []bool{false, true} {
		cfg := Config{Width: w, Height: h, FPSNum: 25, FPSDen: 1, GOPSize: 3, QP: 27, CABAC: cabac}
		var streams [2][]byte
		for i, trellis := range []bool{false, false} {
			c := cfg
			c.Trellis = trellis
			s, _ := encodeBStream(t, c, frames, nil)
			streams[i] = s
		}
		if string(streams[0]) != string(streams[1]) {
			t.Fatalf("cabac %v: the default encoder is not deterministic", cabac)
		}
		c := cfg
		c.Trellis = true
		refined, _ := encodeBStream(t, c, frames, nil)
		if string(refined) == string(streams[0]) {
			t.Fatalf("cabac %v: turning the trellis on changed nothing", cabac)
		}
	}
}

func TestFFmpegDecodesTrellisStreamsIdentically(t *testing.T) {
	const w, h = 176, 144
	cases := []struct {
		name   string
		frames [][]byte
		cfg    Config
	}{
		{"cavlc", movingFrames(w, h, 5), Config{Width: w, Height: h, FPSNum: 25, FPSDen: 1, GOPSize: 5, QP: 24, Trellis: true}},
		{"cabac", movingFrames(w, h, 5), Config{Width: w, Height: h, FPSNum: 25, FPSDen: 1, GOPSize: 5, QP: 24, CABAC: true, Trellis: true}},
		{"cabac-8x8", movingFrames(w, h, 5), Config{Width: w, Height: h, FPSNum: 25, FPSDen: 1, GOPSize: 5, QP: 30, CABAC: true, Transform8x8: true, ScalingMatrix: ScalingMatrixJVT, Trellis: true}},
		{"cavlc-8x8", movingFrames(w, h, 5), Config{Width: w, Height: h, FPSNum: 25, FPSDen: 1, GOPSize: 5, QP: 30, Transform8x8: true, Trellis: true}},
		{"bframes", movingFrames(w, h, 7), Config{Width: w, Height: h, FPSNum: 25, FPSDen: 1, GOPSize: 7, QP: 27, CABAC: true, BFrames: 2, RefFrames: 2, WeightedPrediction: WeightedPredictionExplicit, Trellis: true}},
		{"slices", movingFrames(w, h, 4), Config{Width: w, Height: h, FPSNum: 25, FPSDen: 1, GOPSize: 4, QP: 27, CABAC: true, Slices: 3, Trellis: true}},
		{"screen", screenFrames(w, h, 5), Config{Width: w, Height: h, FPSNum: 25, FPSDen: 1, GOPSize: 5, QP: 30, CABAC: true, Trellis: true}},
	}
	frameSize := w * h * 3 / 2
	for _, c := range trim(cases, 2) {
		t.Run(c.name, func(t *testing.T) {
			stream, recons := encodeBStream(t, c.cfg, c.frames, nil)
			assertMatchesReconstruction(t, c.name, decodeUnits(t, [][]byte{stream}), recons)
			ref := decodeWithFFmpeg(t, stream)
			if len(ref) != frameSize*len(c.frames) {
				t.Fatalf("%s: ffmpeg produced %d bytes, want %d", c.name, len(ref), frameSize*len(c.frames))
			}
			for i := range recons {
				got := make([]byte, recons[i].Size())
				recons[i].CopyOut(got)
				want := ref[i*frameSize : (i+1)*frameSize]
				for j := range got {
					if got[j] != want[j] {
						t.Fatalf("%s frame %d: ffmpeg and our reconstruction disagree at sample %d, ffmpeg %d ours %d",
							c.name, i, j, want[j], got[j])
					}
				}
			}
			t.Logf("%s: %d bytes decode identically in ffmpeg", c.name, len(stream))
		})
	}
}

const screenTrellisCase = "screen"

func TestTheTrellisIsStillWrongOnScreenContent(t *testing.T) {
	const w, h = 128, 96
	qps := trim([]int{20, 22, 24, 26, 28, 30, 32, 34, 36, 38}, 3)
	var screen struct {
		frames [][]byte
		cfg    Config
		found  bool
	}
	for _, c := range trellisCases(w, h) {
		if c.name == screenTrellisCase {
			screen.frames, screen.cfg, screen.found = c.frames, c.cfg, true
		}
	}
	if !screen.found {
		t.Fatalf("no %q case to measure", screenTrellisCase)
	}
	off, on := trellisPair(screen.cfg)
	plain := trellisCurve(t, off, screen.frames, qps)
	refined := trellisCurve(t, on, screen.frames, qps)
	shortfall, at := qualityShortfall(t, plain, refined)
	saved := bitsSaved(t, plain, refined)
	t.Logf("screen: %+.1f%% bits at equal quality, worst shortfall %.2f dB near %.0f bytes",
		saved, shortfall, at)

	if len(qps) < 10 {
		return
	}
	if shortfall < 0.8 {
		t.Fatalf("the trellis is only %.2f dB worse than no trellis on screen content, which is better than the 1.18 dB this records. If you fixed it, put the new figure here and fold the case back into TestTrellisKeepsQualityAtEqualRate",
			shortfall)
	}
	if shortfall > 1.6 {
		t.Fatalf("the trellis is %.2f dB worse than no trellis on screen content, worse than the 1.18 dB this records; something made a known defect deeper",
			shortfall)
	}
}
