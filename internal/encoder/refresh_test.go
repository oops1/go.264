package encoder

import (
	"bytes"
	"testing"

	"github.com/oops1/go.264/internal/frame"
	"github.com/oops1/go.264/internal/nal"
	"github.com/oops1/go.264/internal/syntax"
)

func workload(full int) int {
	switch {
	case raceDetector:
		return full/12 + 4
	case testing.Short():
		return full/3 + 4
	}
	return full
}

func skipUnderRace(t *testing.T) {
	t.Helper()
	if raceDetector {
		t.Skip("the race detector has no concurrency to find here and the encode is too slow")
	}
}

func trim[T any](s []T, n int) []T {
	if raceDetector && len(s) > n {
		return s[:n]
	}
	return s
}

func refreshConfig(qp, period int) Config {
	return Config{Width: 176, Height: 144, FPSNum: 25, FPSDen: 1, GOPSize: 12, QP: qp,
		RefFrames: 1, IntraRefresh: period}
}

func encodeUnits(t *testing.T, cfg Config, frames [][]byte) ([][]byte, []*frame.Picture) {
	t.Helper()
	enc, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	var units [][]byte
	var recons []*frame.Picture
	for i, f := range frames {
		pkt, err := enc.Encode(f)
		if err != nil {
			t.Fatalf("frame %d: Encode: %v", i, err)
		}
		units = append(units, pkt)
		recons = append(recons, snapshotOf(cfg, enc.Reconstruction()))
	}
	return units, recons
}

func screenFrame(w, h, t int) []byte {
	buf := make([]byte, w*h*3/2)
	cursor := (t * 7) % (w - 24)
	scroll := (t * 3) % 64
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			v := 235
			if (x/6+(y+scroll)/11)%5 == 0 {
				v = 24
			}
			if y%13 < 2 && x > w/4 && x < 3*w/4 {
				v = 60
			}
			if x >= cursor && x < cursor+18 && y >= h/2-9 && y < h/2+9 {
				v = 140
			}
			buf[y*w+x] = byte(v)
		}
	}
	cw, ch := w/2, h/2
	for i := 0; i < 2; i++ {
		base := w*h + i*cw*ch
		for y := 0; y < ch; y++ {
			for x := 0; x < cw; x++ {
				v := 128
				if 2*x >= cursor && 2*x < cursor+18 && 2*y >= h/2-9 && 2*y < h/2+9 {
					v = 90 + i*70
				}
				buf[base+y*cw+x] = byte(v)
			}
		}
	}
	return buf
}

func screenSequence(w, h, n int) [][]byte {
	out := make([][]byte, n)
	for i := range out {
		out[i] = screenFrame(w, h, i)
	}
	return out
}

func panningFrame(w, h, t int) []byte {
	buf := make([]byte, w*h*3/2)
	shift := t * 20
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			u := x + shift
			v := 40 + (u*u/53+y*y/37+u/3+y*5)%180
			if (u/9+y/7)%4 == 0 {
				v = 250 - (u+y)%60
			}
			buf[y*w+x] = byte(v)
		}
	}
	cw, ch := w/2, h/2
	for i := 0; i < 2; i++ {
		base := w*h + i*cw*ch
		for y := 0; y < ch; y++ {
			for x := 0; x < cw; x++ {
				u := 2*x + shift
				buf[base+y*cw+x] = byte(64 + (u/5+y/3+i*23)%128)
			}
		}
	}
	return buf
}

func panningSequence(w, h, n int) [][]byte {
	out := make([][]byte, n)
	for i := range out {
		out[i] = panningFrame(w, h, i)
	}
	return out
}

func TestIntraRefreshOffLeavesTheStreamUntouched(t *testing.T) {
	cfg := refreshConfig(27, 0)
	frames := screenSequence(cfg.Width, cfg.Height, 10)
	before, _ := encodeUnits(t, cfg, frames)

	cfg.IntraRefresh = 0
	after, _ := encodeUnits(t, cfg, frames)
	for i := range before {
		if !bytes.Equal(before[i], after[i]) {
			t.Fatalf("frame %d differs although intra refresh is off", i)
		}
	}
}

func nalTypesIn(t *testing.T, pkt []byte) []nal.Type {
	t.Helper()
	var out []nal.Type
	for _, ebsp := range nal.SplitAnnexB(pkt) {
		u, err := nal.Parse(ebsp)
		if err != nil {
			t.Fatalf("parsing our own unit: %v", err)
		}
		out = append(out, u.Type)
	}
	return out
}

func TestIntraRefreshSendsNoKeyFrameAfterTheFirst(t *testing.T) {
	cfg := refreshConfig(27, 5)
	units, _ := encodeUnits(t, cfg, screenSequence(cfg.Width, cfg.Height, 21))
	for i, pkt := range units {
		for _, ty := range nalTypesIn(t, pkt) {
			if ty == nal.TypeSliceIDR && i != 0 {
				t.Fatalf("frame %d carries an IDR although intra refresh should have replaced it", i)
			}
		}
	}
}

func recoveryPointsIn(t *testing.T, pkt []byte, sps *syntax.SPS) []uint32 {
	t.Helper()
	var out []uint32
	for _, ebsp := range nal.SplitAnnexB(pkt) {
		u, err := nal.Parse(ebsp)
		if err != nil {
			t.Fatalf("parsing our own unit: %v", err)
		}
		if u.Type != nal.TypeSEI {
			continue
		}
		msgs, err := syntax.ParseSEI(u.RBSP, sps, func(uint32) *syntax.SPS { return sps })
		if err != nil {
			t.Fatalf("parsing our own SEI: %v", err)
		}
		for _, m := range msgs {
			if m.RecoveryPoint != nil {
				out = append(out, m.RecoveryPoint.RecoveryFrameCnt)
			}
		}
	}
	return out
}

func TestIntraRefreshAnnouncesEverySweep(t *testing.T) {
	const period = 5
	cfg := refreshConfig(27, period)
	enc, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	frames := screenSequence(cfg.Width, cfg.Height, 21)
	var starts []int
	for i, f := range frames {
		pkt, err := enc.Encode(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, cnt := range recoveryPointsIn(t, pkt, enc.SPS()) {
			if cnt != period-1 {
				t.Fatalf("frame %d announced recovery_frame_cnt %d, want %d", i, cnt, period-1)
			}
			starts = append(starts, i)
		}
	}
	want := []int{1, 6, 11, 16}
	if len(starts) != len(want) {
		t.Fatalf("recovery points at %v, want %v", starts, want)
	}
	for i := range want {
		if starts[i] != want[i] {
			t.Fatalf("recovery points at %v, want %v", starts, want)
		}
	}
}

func TestIntraRefreshReconstructionSurvivesTheDecoder(t *testing.T) {
	for _, cabac := range []bool{false, true} {
		qps := []int{0, 18, 30, 44, 51}
		if testing.Short() {
			qps = []int{0, 30, 51}
		}
		for _, qp := range trim(qps, 1) {
			for _, period := range []int{3, 7} {
				cfg := refreshConfig(qp, period)
				cfg.CABAC = cabac
				frames := screenSequence(cfg.Width, cfg.Height, 16)
				units, recons := encodeUnits(t, cfg, frames)
				assertMatchesReconstruction(t, "intra refresh", decodeUnits(t, units), recons)
			}
		}
	}
}

func TestIntraRefreshReconstructionAcrossFeatures(t *testing.T) {
	base := refreshConfig(28, 6)
	cases := []struct {
		name string
		edit func(c *Config)
	}{
		{"slices", func(c *Config) { c.Slices = 3 }},
		{"slices and cabac", func(c *Config) { c.Slices = 4; c.CABAC = true }},
		{"transform 8x8", func(c *Config) { c.Transform8x8 = true; c.CABAC = true }},
		{"transform 8x8 cavlc", func(c *Config) { c.Transform8x8 = true }},
		{"multiple references", func(c *Config) { c.RefFrames = 3 }},
		{"zero motion", func(c *Config) { c.MotionSearch = MotionSearchZero }},
		{"exhaustive", func(c *Config) { c.ModeDecision = ModeDecisionExhaustive }},
		{"jvt scaling", func(c *Config) { c.ScalingMatrix = ScalingMatrixJVT; c.Transform8x8 = true }},
	}
	for _, tc := range trim(cases, 2) {
		cfg := base
		tc.edit(&cfg)
		frames := screenSequence(cfg.Width, cfg.Height, 15)
		units, recons := encodeUnits(t, cfg, frames)
		assertMatchesReconstruction(t, tc.name, decodeUnits(t, units), recons)
	}
}

func TestIntraRefreshRejectsBPictures(t *testing.T) {
	cfg := refreshConfig(26, 5)
	cfg.BFrames = 2
	if _, err := New(cfg); err == nil {
		t.Fatal("intra refresh with B pictures was accepted")
	}
}

func joinStream(t *testing.T, cfg Config, units [][]byte, at int) []byte {
	t.Helper()
	stray, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	garbage := flatFrame(cfg.Width, cfg.Height, 17, 200, 40)
	head, err := stray.Encode(garbage)
	if err != nil {
		t.Fatal(err)
	}
	out := append([]byte(nil), head...)
	for _, u := range units[at:] {
		out = append(out, u...)
	}
	return out
}

func planeDiff(a, b *frame.Picture) int {
	got := make([]byte, a.Size())
	a.CopyOut(got)
	want := make([]byte, b.Size())
	b.CopyOut(want)
	n := 0
	for i := range got {
		if got[i] != want[i] {
			n++
		}
	}
	return n
}

func sweepStartAtOrAfter(frameIdx, period int) int {
	if frameIdx <= 1 {
		return 1
	}
	k := (frameIdx - 1 + period - 1) / period
	return 1 + k*period
}

var convergenceSources = []struct {
	name string
	make func(w, h, n int) [][]byte
}{
	{"screen", screenSequence},
	{"panning", panningSequence},
}

var convergenceVariants = []struct {
	name string
	edit func(c *Config)
}{
	{"cavlc", func(c *Config) {}},
	{"cabac", func(c *Config) { c.CABAC = true }},
	{"slices", func(c *Config) { c.Slices = 3 }},
	{"transform 8x8", func(c *Config) { c.CABAC = true; c.Transform8x8 = true }},
	{"three references", func(c *Config) { c.RefFrames = 3 }},
	{"deblocking off", func(c *Config) { c.Deblocking = DeblockingOff }},
}

func TestIntraRefreshConvergesWithoutAKeyFrame(t *testing.T) {
	for _, src := range trim(convergenceSources, 1) {
		for _, variant := range trim(convergenceVariants, 1) {
			t.Run(src.name+" "+variant.name, func(t *testing.T) {
				convergenceRun(t, src.make, variant.edit)
			})
		}
	}
}

func convergenceRun(t *testing.T, source func(w, h, n int) [][]byte, edit func(*Config)) {
	t.Helper()
	{
		for _, period := range trim([]int{4, 6, 11}, 1) {
			cfg := refreshConfig(26, period)
			edit(&cfg)
			const count = 40
			frames := source(cfg.Width, cfg.Height, count)
			units, _ := encodeUnits(t, cfg, frames)
			full := decodeUnits(t, units)
			if len(full) != count {
				t.Fatalf("decoded %d pictures, want %d", len(full), count)
			}

			for _, join := range []int{3, 7, 12, 19} {
				pics := decodeUnits(t, [][]byte{joinStream(t, cfg, units, join)})
				if len(pics) != count-join+1 {
					t.Fatalf("period %d, joining at %d: decoded %d pictures, want %d",
						period, join, len(pics), count-join+1)
				}
				pics = pics[1:]

				start := sweepStartAtOrAfter(join, period)
				converged := start + period - 1
				if converged >= count {
					t.Fatalf("period %d, joining at %d: the sequence is too short to converge", period, join)
				}
				dirty := false
				for i, p := range pics {
					at := join + i
					d := planeDiff(p, full[at])
					switch {
					case at >= converged && d != 0:
						t.Fatalf("period %d, joining at %d: frame %d still differs in %d samples although the sweep that began at %d finished at %d",
							period, join, at, d, start, converged)
					case at < converged && d != 0:
						dirty = true
					}
				}
				if !dirty {
					t.Fatalf("period %d, joining at %d: the late decoder matched from the first frame, so the test proves nothing",
						period, join)
				}
			}
			t.Logf("period %d: a decoder joining anywhere is exact by the end of the next sweep", period)
		}
	}
}

func TestIntraRefreshConvergesInOneSweepFromARecoveryPoint(t *testing.T) {
	skipUnderRace(t)
	for _, src := range trim(convergenceSources, 1) {
		for _, period := range trim([]int{5, 9}, 1) {
			recoveryPointRun(t, src.name, src.make, period)
		}
	}
}

func recoveryPointRun(t *testing.T, name string, source func(w, h, n int) [][]byte, period int) {
	t.Helper()
	{
		cfg := refreshConfig(26, period)
		const count = 40
		frames := source(cfg.Width, cfg.Height, count)
		units, _ := encodeUnits(t, cfg, frames)
		full := decodeUnits(t, units)

		for _, start := range []int{1 + period, 1 + 2*period} {
			pics := decodeUnits(t, [][]byte{joinStream(t, cfg, units, start)})[1:]
			for i, p := range pics {
				at := start + i
				if at < start+period-1 {
					continue
				}
				if d := planeDiff(p, full[at]); d != 0 {
					t.Fatalf("%s, period %d, recovery point at %d: frame %d differs in %d samples after a full sweep",
						name, period, start, at, d)
				}
			}
			t.Logf("%s, period %d: joining at the recovery point %d is exact %d frames later", name, period, start, period-1)
		}
	}
}

func streamCost(units [][]byte) (total, peak int) {
	for i, u := range units {
		total += len(u)
		if i != 0 && len(u) > peak {
			peak = len(u)
		}
	}
	return
}

func TestIntraRefreshCostAgainstPeriodicKeyFrames(t *testing.T) {
	if raceDetector {
		t.Skip("the cost measurement is too slow under the race detector")
	}
	count := 120
	periods := []int{12, 30, 60}
	if testing.Short() {
		count, periods = 48, []int{12}
	}
	base := Config{Width: 320, Height: 240, FPSNum: 25, FPSDen: 1, QP: 26, RefFrames: 1}
	frames := screenSequence(base.Width, base.Height, count)

	for _, period := range periods {
		refresh := base
		refresh.GOPSize = 10000
		refresh.IntraRefresh = period
		refreshUnits, _ := encodeUnits(t, refresh, frames)

		keyed := base
		keyed.GOPSize = period
		keyedUnits, _ := encodeUnits(t, keyed, frames)

		rTotal, rPeak := streamCost(refreshUnits)
		kTotal, kPeak := streamCost(keyedUnits)
		t.Logf("period %d: intra refresh %d bytes total, largest frame after the first %d bytes",
			period, rTotal, rPeak)
		t.Logf("period %d: IDR every %d      %d bytes total, largest frame after the first %d bytes",
			period, period, kTotal, kPeak)
		t.Logf("period %d: size %+.1f%%, peak frame %+.1f%%", period,
			100*float64(rTotal-kTotal)/float64(kTotal),
			100*float64(rPeak-kPeak)/float64(kPeak))
		if rPeak >= kPeak {
			t.Fatalf("period %d: intra refresh peaked at %d bytes against %d for periodic key frames, so it spread nothing",
				period, rPeak, kPeak)
		}
	}
}
