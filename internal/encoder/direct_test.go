package encoder

import (
	"fmt"
	"testing"

	"github.com/oops1/go.264/internal/bits"
	"github.com/oops1/go.264/internal/nal"
	"github.com/oops1/go.264/internal/syntax"
)

func directConfig(qp, bframes int, cabac bool, mode DirectMode) Config {
	cfg := bConfig(176, 144, qp, bframes, 100, cabac)
	cfg.DirectMode = mode
	return cfg
}

type encoderSets struct {
	sps *syntax.SPS
	pps *syntax.PPS
}

func (s encoderSets) SPS(uint32) *syntax.SPS { return s.sps }

func (s encoderSets) PPS(uint32) *syntax.PPS { return s.pps }

func directFlagsIn(t *testing.T, stream []byte, sps *syntax.SPS, pps *syntax.PPS) []bool {
	t.Helper()
	var out []bool
	for _, ebsp := range nal.SplitAnnexB(stream) {
		u, err := nal.Parse(ebsp)
		if err != nil {
			t.Fatalf("parsing our own unit: %v", err)
		}
		if u.Header.Type != nal.TypeSliceNonIDR && u.Header.Type != nal.TypeSliceIDR {
			continue
		}
		hdr, _, _, err := syntax.ParseSliceHeader(bits.NewReader(u.RBSP), u.Header,
			encoderSets{sps: sps, pps: pps})
		if err != nil {
			t.Fatalf("parsing our own slice header: %v", err)
		}
		if !hdr.SliceType.IsB() {
			continue
		}
		out = append(out, hdr.DirectSpatialMvPred)
	}
	return out
}

func TestDirectModeIsSpatialByDefault(t *testing.T) {
	cfg := directConfig(28, 2, false, DirectSpatial)
	frames := fadeFrames(t)
	stream, _ := encodeBStream(t, cfg, frames, nil)
	enc, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	flags := directFlagsIn(t, stream, enc.SPS(), enc.PPS())
	if len(flags) == 0 {
		t.Fatal("the stream carries no B slices")
	}
	for i, f := range flags {
		if !f {
			t.Fatalf("B slice %d announced temporal direct without the caller asking", i)
		}
	}
}

func TestTemporalDirectIsAnnouncedInEveryBSlice(t *testing.T) {
	cfg := directConfig(28, 2, false, DirectTemporal)
	frames := fadeFrames(t)
	stream, _ := encodeBStream(t, cfg, frames, nil)
	enc, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	flags := directFlagsIn(t, stream, enc.SPS(), enc.PPS())
	if len(flags) == 0 {
		t.Fatal("the stream carries no B slices")
	}
	for i, f := range flags {
		if f {
			t.Fatalf("B slice %d still announced spatial direct", i)
		}
	}
}

func TestDirectModeRejectsAnUnknownValue(t *testing.T) {
	if _, err := New(directConfig(28, 2, false, DirectTemporal+1)); err == nil {
		t.Fatal("New accepted a DirectMode outside the defined range")
	}
}

func TestTemporalDirectMatchesTheEncoderReconstruction(t *testing.T) {
	sources := []struct {
		name  string
		build func(t *testing.T) [][]byte
	}{
		{"fade", func(t *testing.T) [][]byte { return fadeFrames(t) }},
		{"translation", func(t *testing.T) [][]byte {
			var out [][]byte
			for i := 0; i < 13; i++ {
				out = append(out, movingFrame(176, 144, i))
			}
			return out
		}},
	}
	for _, src := range sources {
		frames := src.build(t)
		for _, bframes := range []int{1, 2, 3} {
			for _, cabac := range []bool{false, true} {
				for _, qp := range []int{0, 20, 34, 51} {
					cfg := directConfig(qp, bframes, cabac, DirectTemporal)
					label := fmt.Sprintf("%s, bframes %d, cabac %v, qp %d", src.name, bframes, cabac, qp)
					stream, recons := encodeBStream(t, cfg, frames, nil)
					assertMatchesReconstruction(t, label, decodeUnits(t, [][]byte{stream}), recons)
				}
			}
		}
	}
}

func TestTemporalDirectMatchesAcrossTheOptionMatrix(t *testing.T) {
	frames := fadeFrames(t)
	for _, slices := range []int{1, 3} {
		for _, t8 := range []bool{false, true} {
			for _, mode := range []WeightedPrediction{
				WeightedPredictionOff, WeightedPredictionExplicit, WeightedPredictionImplicit} {
				cfg := directConfig(28, 2, true, DirectTemporal)
				cfg.Slices = slices
				cfg.Transform8x8 = t8
				cfg.WeightedPrediction = mode
				label := fmt.Sprintf("slices %d, transform8x8 %v, weights %d", slices, t8, mode)
				stream, recons := encodeBStream(t, cfg, frames, nil)
				assertMatchesReconstruction(t, label, decodeUnits(t, [][]byte{stream}), recons)
			}
		}
	}
}

func TestFFmpegDecodesTemporalDirectStreams(t *testing.T) {
	frameSize := 176 * 144 * 3 / 2
	sources := []struct {
		name  string
		build func(t *testing.T) [][]byte
	}{
		{"fade", func(t *testing.T) [][]byte { return fadeFrames(t) }},
		{"translation", func(t *testing.T) [][]byte {
			var out [][]byte
			for i := 0; i < 13; i++ {
				out = append(out, movingFrame(176, 144, i))
			}
			return out
		}},
	}
	for _, src := range sources {
		frames := src.build(t)
		for _, bframes := range []int{1, 3} {
			for _, cabac := range []bool{false, true} {
				for _, qp := range []int{18, 32, 44} {
					cfg := directConfig(qp, bframes, cabac, DirectTemporal)
					label := fmt.Sprintf("%s, bframes %d, cabac %v, qp %d", src.name, bframes, cabac, qp)
					stream, recons := encodeBStream(t, cfg, frames, nil)
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

func TestDirectModesAreMeasuredAgainstEachOther(t *testing.T) {
	sources := []struct {
		name  string
		build func(t *testing.T) [][]byte
	}{
		{"fade", func(t *testing.T) [][]byte { return fadeFrames(t) }},
		{"translation", func(t *testing.T) [][]byte {
			var out [][]byte
			for i := 0; i < 25; i++ {
				out = append(out, movingFrame(176, 144, i))
			}
			return out
		}},
		{"screen", func(t *testing.T) [][]byte {
			var out [][]byte
			for i := 0; i < 25; i++ {
				out = append(out, syntheticFrame(176, 144, i))
			}
			return out
		}},
	}
	for _, src := range sources {
		frames := src.build(t)
		for _, cabac := range []bool{false, true} {
			for _, qp := range []int{22, 30, 38} {
				spatial, _ := encodeBStream(t, directConfig(qp, 2, cabac, DirectSpatial), frames, nil)
				temporal, _ := encodeBStream(t, directConfig(qp, 2, cabac, DirectTemporal), frames, nil)
				t.Logf("%s, cabac %v, qp %d: spatial %d bytes, temporal %d bytes, %+.1f%%",
					src.name, cabac, qp, len(spatial), len(temporal),
					100*float64(len(temporal)-len(spatial))/float64(len(spatial)))
			}
		}
	}
}

func TestTemporalDirectWithTheBufferModelMatches(t *testing.T) {
	frames := fadeFrames(t)
	for _, cabac := range []bool{false, true} {
		cfg := directConfig(26, 2, cabac, DirectTemporal)
		cfg.BitrateKbps = 300
		cfg.VBVBufferKbits = 300
		cfg.VBVMaxrateKbps = 400
		cfg.WeightedPrediction = WeightedPredictionImplicit
		stream, recons := encodeBStream(t, cfg, frames, nil)
		assertMatchesReconstruction(t, fmt.Sprintf("buffer model, cabac %v", cabac),
			decodeUnits(t, [][]byte{stream}), recons)
	}
}
