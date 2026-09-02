package encoder

import (
	"fmt"
	"image"
	"testing"

	"github.com/oops1/go.264/internal/bits"
	"github.com/oops1/go.264/internal/frame"
	"github.com/oops1/go.264/internal/nal"
	"github.com/oops1/go.264/internal/syntax"
)

func weightConfig(qp, bframes int, cabac bool, mode WeightedPrediction) Config {
	return Config{Width: 176, Height: 144, FPSNum: 25, FPSDen: 1, GOPSize: 100, QP: qp,
		RefFrames: 2, BFrames: bframes, CABAC: cabac, WeightedPrediction: mode}
}

func TestWeightedPredictionIsOffByDefault(t *testing.T) {
	enc, err := New(Config{Width: 176, Height: 144, FPSNum: 25, FPSDen: 1, GOPSize: 30, QP: 26})
	if err != nil {
		t.Fatal(err)
	}
	if enc.PPS().WeightedPred {
		t.Error("weighted_pred_flag is set without the caller asking for it")
	}
	if enc.PPS().WeightedBipredIDC != 0 {
		t.Errorf("weighted_bipred_idc = %d without the caller asking for it", enc.PPS().WeightedBipredIDC)
	}
}

func TestWeightedPredictionAnnouncesItselfInTheParameterSets(t *testing.T) {
	cases := []struct {
		mode     WeightedPrediction
		bframes  int
		wantPred bool
		wantIDC  uint32
	}{
		{WeightedPredictionOff, 0, false, 0},
		{WeightedPredictionOff, 2, false, 0},
		{WeightedPredictionExplicit, 0, true, 0},
		{WeightedPredictionExplicit, 2, true, 1},
		{WeightedPredictionImplicit, 0, true, 0},
		{WeightedPredictionImplicit, 2, true, 2},
	}
	for _, c := range cases {
		cfg := weightConfig(26, c.bframes, false, c.mode)
		enc, err := New(cfg)
		if err != nil {
			t.Fatalf("mode %d: %v", c.mode, err)
		}
		if got := enc.PPS().WeightedPred; got != c.wantPred {
			t.Errorf("mode %d bframes %d: weighted_pred_flag = %v, want %v", c.mode, c.bframes, got, c.wantPred)
		}
		if got := enc.PPS().WeightedBipredIDC; got != c.wantIDC {
			t.Errorf("mode %d bframes %d: weighted_bipred_idc = %d, want %d", c.mode, c.bframes, got, c.wantIDC)
		}
		if c.mode != WeightedPredictionOff && enc.SPS().ProfileIDC < 77 {
			t.Errorf("mode %d: profile_idc %d does not allow weighted prediction", c.mode, enc.SPS().ProfileIDC)
		}
	}
}

func TestWeightedPredictionRejectsAnUnknownMode(t *testing.T) {
	_, err := New(weightConfig(26, 0, false, WeightedPredictionImplicit+1))
	if err == nil {
		t.Fatal("New accepted a WeightedPrediction value outside the defined range")
	}
}

func TestFitWeightRecoversAKnownScaleAndOffset(t *testing.T) {
	const n = 4096
	var sums planeSums
	src := make([]byte, n)
	ref := make([]byte, n)
	for i := 0; i < n; i++ {
		v := 20 + (i*37)%200
		ref[i] = byte(v)
		ref[i] = byte(clipSample(v))
		src[i] = byte(clipSample(v/2 + 40))
	}
	accumulateSums(&sums, src, n, 0, ref, n, 0, n, 1)
	w, o := fitWeight(sums, lumaLog2Denom)
	unit := float64(int32(1) << lumaLog2Denom)
	scale := float64(w) / unit
	if scale < 0.45 || scale > 0.55 {
		t.Errorf("recovered scale %.3f, want about 0.5", scale)
	}
	if o < 36 || o > 44 {
		t.Errorf("recovered offset %d, want about 40", o)
	}
	plain, shaped := compareWeighted(src, n, 0, ref, n, 0, n, 1, w, o, lumaLog2Denom)
	if shaped >= plain/4 {
		t.Errorf("weighted error %d against plain %d, the fit did not pay", shaped, plain)
	}
	if !worthSending(plain, shaped, w, o, lumaLog2Denom) {
		t.Error("a fit this good was declined")
	}
}

func TestFitWeightDeclinesWhenTheReferenceAlreadyMatches(t *testing.T) {
	const n = 4096
	var sums planeSums
	buf := make([]byte, n)
	for i := 0; i < n; i++ {
		buf[i] = byte(30 + (i*17)%180)
	}
	accumulateSums(&sums, buf, n, 0, buf, n, 0, n, 1)
	w, o := fitWeight(sums, lumaLog2Denom)
	plain, shaped := compareWeighted(buf, n, 0, buf, n, 0, n, 1, w, o, lumaLog2Denom)
	if worthSending(plain, shaped, w, o, lumaLog2Denom) {
		t.Errorf("an identical reference was given weight %d offset %d", w, o)
	}
}

func weightedRunLabel(mode WeightedPrediction, bframes int, cabac bool, qp int) string {
	return fmt.Sprintf("mode %d, bframes %d, cabac %v, qp %d", mode, bframes, cabac, qp)
}

func encodeWeighted(t *testing.T, cfg Config, frames [][]byte) ([]byte, []*frame.Picture) {
	t.Helper()
	return encodeBStream(t, cfg, frames, nil)
}

func TestWeightedPredictionMatchesTheEncoderReconstruction(t *testing.T) {
	frames := fadeFrames(t)
	for _, mode := range []WeightedPrediction{WeightedPredictionExplicit, WeightedPredictionImplicit} {
		for _, bframes := range []int{0, 2} {
			for _, cabac := range []bool{false, true} {
				for _, qp := range []int{0, 18, 30, 44, 51} {
					cfg := weightConfig(qp, bframes, cabac, mode)
					label := weightedRunLabel(mode, bframes, cabac, qp)
					stream, recons := encodeWeighted(t, cfg, frames)
					assertMatchesReconstruction(t, label, decodeUnits(t, [][]byte{stream}), recons)
				}
			}
		}
	}
}

func TestWeightedPredictionMatchesAcrossTheOptionMatrix(t *testing.T) {
	frames := fadeFrames(t)
	for _, mode := range []WeightedPrediction{WeightedPredictionExplicit, WeightedPredictionImplicit} {
		for _, slices := range []int{1, 3} {
			for _, t8 := range []bool{false, true} {
				cfg := weightConfig(28, 2, true, mode)
				cfg.Slices = slices
				cfg.Transform8x8 = t8
				label := fmt.Sprintf("mode %d, slices %d, transform8x8 %v", mode, slices, t8)
				stream, recons := encodeWeighted(t, cfg, frames)
				assertMatchesReconstruction(t, label, decodeUnits(t, [][]byte{stream}), recons)
			}
		}
	}
}

func TestWeightedPredictionWithIntraRefreshMatches(t *testing.T) {
	frames := fadeFrames(t)
	for _, cabac := range []bool{false, true} {
		cfg := weightConfig(26, 0, cabac, WeightedPredictionExplicit)
		cfg.IntraRefresh = 4
		cfg.RefFrames = 1
		stream, recons := encodeWeighted(t, cfg, frames)
		assertMatchesReconstruction(t, fmt.Sprintf("intra refresh, cabac %v", cabac),
			decodeUnits(t, [][]byte{stream}), recons)
	}
}

func TestWeightedPredictionWithTheBufferModelMatches(t *testing.T) {
	frames := fadeFrames(t)
	cfg := weightConfig(26, 0, true, WeightedPredictionExplicit)
	cfg.BitrateKbps = 300
	cfg.VBVBufferKbits = 300
	cfg.VBVMaxrateKbps = 400
	stream, recons := encodeWeighted(t, cfg, frames)
	assertMatchesReconstruction(t, "buffer model", decodeUnits(t, [][]byte{stream}), recons)
}

func TestFFmpegDecodesWeightedStreams(t *testing.T) {
	frames := fadeFrames(t)
	frameSize := 176 * 144 * 3 / 2
	for _, mode := range []WeightedPrediction{WeightedPredictionExplicit, WeightedPredictionImplicit} {
		for _, bframes := range []int{0, 2} {
			for _, cabac := range []bool{false, true} {
				for _, qp := range []int{16, 30, 42} {
					cfg := weightConfig(qp, bframes, cabac, mode)
					label := weightedRunLabel(mode, bframes, cabac, qp)
					stream, recons := encodeWeighted(t, cfg, frames)
					ref := decodeWithFFmpeg(t, stream)
					if len(ref) != frameSize*len(recons) {
						t.Fatalf("%s: ffmpeg produced %d bytes for %d pictures", label, len(ref), len(recons))
					}
					for i := range recons {
						got := make([]byte, recons[i].Size())
						recons[i].CopyOut(got)
						want := ref[i*frameSize : (i+1)*frameSize]
						for j := range got {
							if got[j] != want[j] {
								t.Fatalf("%s frame %d: ffmpeg and our reconstruction disagree at sample %d, ffmpeg %d ours %d",
									label, i, j, want[j], got[j])
							}
						}
					}
				}
			}
		}
	}
}

func TestWeightedPredictionSavesBitsOnAFade(t *testing.T) {
	frames := fadeFrames(t)
	for _, cabac := range []bool{false, true} {
		for _, qp := range []int{22, 30, 38} {
			plain, _ := encodeWeighted(t, weightConfig(qp, 0, cabac, WeightedPredictionOff), frames)
			shaped, _ := encodeWeighted(t, weightConfig(qp, 0, cabac, WeightedPredictionExplicit), frames)
			saving := 100 * float64(len(plain)-len(shaped)) / float64(len(plain))
			t.Logf("fade, P only, cabac %v, qp %d: unweighted %d bytes, explicit %d bytes, %.1f%% saved",
				cabac, qp, len(plain), len(shaped), saving)
			if len(shaped) >= len(plain) {
				t.Errorf("cabac %v, qp %d: explicit weights cost %d bytes against %d unweighted",
					cabac, qp, len(shaped), len(plain))
			}
		}
	}
}

func TestWeightedPredictionOnAFadeWithBPictures(t *testing.T) {
	frames := fadeFrames(t)
	for _, cabac := range []bool{false, true} {
		for _, qp := range []int{22, 30, 38} {
			var sizes [3]int
			for i, mode := range []WeightedPrediction{
				WeightedPredictionOff, WeightedPredictionExplicit, WeightedPredictionImplicit} {
				stream, _ := encodeWeighted(t, weightConfig(qp, 2, cabac, mode), frames)
				sizes[i] = len(stream)
			}
			t.Logf("fade, two B pictures, cabac %v, qp %d: off %d, explicit %d (%.1f%%), implicit %d (%.1f%%)",
				cabac, qp, sizes[0], sizes[1],
				100*float64(sizes[0]-sizes[1])/float64(sizes[0]),
				sizes[2], 100*float64(sizes[0]-sizes[2])/float64(sizes[0]))
		}
	}
}

func TestWeightedPredictionCostsLittleOnOrdinaryContent(t *testing.T) {
	var frames [][]byte
	for i := 0; i < 12; i++ {
		frames = append(frames, movingFrame(176, 144, i))
	}
	for _, cabac := range []bool{false, true} {
		for _, qp := range []int{22, 30, 38} {
			plain, _ := encodeWeighted(t, weightConfig(qp, 0, cabac, WeightedPredictionOff), frames)
			shaped, _ := encodeWeighted(t, weightConfig(qp, 0, cabac, WeightedPredictionExplicit), frames)
			overhead := len(shaped) - len(plain)
			t.Logf("rigid translation, cabac %v, qp %d: unweighted %d bytes, explicit %d bytes, %+d bytes over %d pictures (%+.2f%%)",
				cabac, qp, len(plain), len(shaped), overhead, len(frames),
				100*float64(overhead)/float64(len(plain)))
			if overhead > len(frames) {
				t.Errorf("cabac %v, qp %d: weights that were all declined cost %d bytes over %d pictures",
					cabac, qp, overhead, len(frames))
			}
		}
	}
}

func assertFFmpegMatchesRecons(t *testing.T, label string, stream []byte, recons []*frame.Picture, w, h int) {
	t.Helper()
	ref := decodeWithFFmpeg(t, stream)
	frameSize := w * h * 3 / 2
	if len(ref) != frameSize*len(recons) {
		t.Fatalf("%s: ffmpeg produced %d bytes for %d pictures", label, len(ref), len(recons))
	}
	for i := range recons {
		got := make([]byte, recons[i].Size())
		recons[i].CopyOut(got)
		want := ref[i*frameSize : (i+1)*frameSize]
		for j := range got {
			if got[j] != want[j] {
				t.Fatalf("%s frame %d: ffmpeg and our reconstruction disagree at sample %d, ffmpeg %d ours %d",
					label, i, j, want[j], got[j])
			}
		}
	}
}

func TestFFmpegAgreesAcrossTheNewOptionMatrix(t *testing.T) {
	frames := fadeFrames(t)
	for _, mode := range []WeightedPrediction{WeightedPredictionExplicit, WeightedPredictionImplicit} {
		for _, direct := range []DirectMode{DirectSpatial, DirectTemporal} {
			for _, slices := range []int{1, 3} {
				for _, t8 := range []bool{false, true} {
					for _, cabac := range []bool{false, true} {
						cfg := weightConfig(29, 2, cabac, mode)
						cfg.DirectMode = direct
						cfg.Slices = slices
						cfg.Transform8x8 = t8
						label := fmt.Sprintf("weights %d, direct %d, slices %d, transform8x8 %v, cabac %v",
							mode, direct, slices, t8, cabac)
						stream, recons := encodeWeighted(t, cfg, frames)
						assertFFmpegMatchesRecons(t, label, stream, recons, cfg.Width, cfg.Height)
					}
				}
			}
		}
	}
}

func TestFFmpegAgreesOnWeightsWithIntraRefreshAndHints(t *testing.T) {
	frames := fadeFrames(t)
	hints := make([]Hints, len(frames))
	for i := range hints {
		hints[i] = Hints{Regions: []Region{
			{Rect: image.Rect(0, 0, 88, 72), Kind: RegionText},
			{Rect: image.Rect(88, 72, 176, 144), Kind: RegionImage},
		}}
	}
	for _, cabac := range []bool{false, true} {
		cfg := weightConfig(27, 0, cabac, WeightedPredictionExplicit)
		cfg.RefFrames = 1
		cfg.IntraRefresh = 5
		stream, recons := encodeBStream(t, cfg, frames, hints)
		assertFFmpegMatchesRecons(t, fmt.Sprintf("intra refresh with hints, cabac %v", cabac),
			stream, recons, cfg.Width, cfg.Height)
	}
}

type fixedSets struct {
	sps *syntax.SPS
	pps *syntax.PPS
}

func (f fixedSets) SPS(uint32) *syntax.SPS { return f.sps }
func (f fixedSets) PPS(uint32) *syntax.PPS { return f.pps }

func TestExplicitBiWeightsObeyTheConformanceBound(t *testing.T) {
	frames := fadeFrames(t)
	for _, qp := range []int{22, 30, 38, 44, 51} {
		cfg := Config{Width: 176, Height: 144, FPSNum: 25, FPSDen: 1, GOPSize: 100, QP: qp,
			RefFrames: 2, BFrames: 2, WeightedPrediction: WeightedPredictionExplicit}
		enc, err := New(cfg)
		if err != nil {
			t.Fatal(err)
		}
		if enc.PPS().WeightedBipredIDC != 1 {
			t.Fatal("explicit weighting with B pictures did not announce weighted_bipred_idc 1")
		}
		var stream []byte
		for _, f := range frames {
			pkt, err := enc.Encode(f)
			if err != nil {
				t.Fatal(err)
			}
			stream = append(stream, pkt...)
		}
		rest, err := enc.Flush()
		if err != nil {
			t.Fatal(err)
		}
		stream = append(stream, rest...)

		sets := fixedSets{sps: enc.SPS(), pps: enc.PPS()}
		checked, worst := 0, int32(0)
		for _, ebsp := range nal.SplitAnnexB(stream) {
			u, err := nal.Parse(ebsp)
			if err != nil {
				t.Fatal(err)
			}
			if u.Header.Type != nal.TypeSliceNonIDR {
				continue
			}
			hdr, _, _, err := syntax.ParseSliceHeader(bits.NewReader(u.RBSP), u.Header, sets)
			if err != nil {
				t.Fatalf("qp %d: parsing our own slice header: %v", qp, err)
			}
			if !hdr.SliceType.IsB() {
				continue
			}
			for c := 0; c < 3; c++ {
				denom := hdr.PredWeight.LumaLog2WeightDenom
				if c > 0 {
					denom = hdr.PredWeight.ChromaLog2WeightDenom
				}
				limit := biWeightLimit(denom)
				for i := range hdr.PredWeight.L0 {
					for j := range hdr.PredWeight.L1 {
						sum := componentWeight(&hdr.PredWeight.L0[i], c) +
							componentWeight(&hdr.PredWeight.L1[j], c)
						checked++
						if sum > worst {
							worst = sum
						}
						if sum < -128 || sum > limit {
							t.Fatalf("qp %d: a bi-predicted weight pair in the stream sums to %d, outside the -128..%d that equation 8-298 allows",
								qp, sum, limit)
						}
					}
				}
			}
		}
		if checked == 0 {
			t.Fatalf("qp %d: the stream carried no bi-predictive weight table", qp)
		}
		t.Logf("qp %2d: %d weight pairs checked, largest sum %d", qp, checked, worst)
	}
}
