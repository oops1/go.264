//go:build linux && (amd64 || arm64)

package vaapi

import (
	"bytes"
	"testing"

	"github.com/oops1/go.264/internal/nal"
	"github.com/oops1/go.264/internal/syntax"
)

func headerTestEncoder(profile Profile, baseline bool, width, height int) *Encoder {
	aw, ah := mbAlign(width), mbAlign(height)
	return &Encoder{profile: profile, baseline: baseline, levelIDC: 31, qp: 22,
		width: width, height: height, alignedWidth: aw, alignedHeight: ah,
		mbWidth: aw / 16, mbHeight: ah / 16}
}

func parsedParameterSets(t *testing.T, hdrs []byte) (*syntax.SPS, *syntax.PPS) {
	t.Helper()
	var sps *syntax.SPS
	var pps *syntax.PPS
	for _, ebsp := range nal.SplitAnnexB(hdrs) {
		u, err := nal.Parse(ebsp)
		if err != nil {
			t.Fatal(err)
		}
		switch u.Header.Type {
		case nal.TypeSPS:
			if sps, err = syntax.ParseSPS(u.RBSP); err != nil {
				t.Fatal(err)
			}
		case nal.TypePPS:
			if pps, err = syntax.ParsePPS(u.RBSP, func(uint32) *syntax.SPS { return sps }); err != nil {
				t.Fatal(err)
			}
		}
	}
	if sps == nil || pps == nil {
		t.Fatal("the headers do not hold both parameter sets")
	}
	return sps, pps
}

func TestTheParameterSetsSayWhatTheDriverIsTold(t *testing.T) {
	for _, c := range []struct {
		name     string
		profile  Profile
		baseline bool
		w, h     int
		idc      uint8
	}{
		{"main, macroblock aligned", ProfileH264Main, false, 1008, 800, syntax.ProfileMain},
		{"high, cropped", ProfileH264High, false, 1920, 1080, syntax.ProfileHigh},
		{"constrained baseline", ProfileH264ConstrainedBaseline, true, 640, 368, syntax.ProfileBaseline},
	} {
		t.Run(c.name, func(t *testing.T) {
			e := headerTestEncoder(c.profile, c.baseline, c.w, c.h)
			hdrs, err := e.parameterSets()
			if err != nil {
				t.Fatal(err)
			}
			sps, pps := parsedParameterSets(t, hdrs)
			if sps.ProfileIDC != c.idc || sps.LevelIDC != e.levelIDC {
				t.Fatalf("profile %d level %d, want %d and %d", sps.ProfileIDC, sps.LevelIDC, c.idc, e.levelIDC)
			}
			if c.baseline && sps.ConstraintSet&constraintSet1 == 0 {
				t.Fatal("constrained baseline without constraint_set1_flag")
			}
			if sps.Log2MaxFrameNumMinus4 != log2MaxFrameNumM4 || sps.PicOrderCntType != 0 ||
				sps.Log2MaxPicOrderCntLsbMinus4 != log2MaxPicOrderCntLsbM4 {
				t.Fatal("the frame number or picture order count widths differ from the sequence buffer")
			}
			if sps.MaxNumRefFrames != maxNumRefFrames || !sps.FrameMbsOnly || !sps.Direct8x8Inference {
				t.Fatal("the reference count or frame flags differ from the sequence buffer")
			}
			if sps.CroppedWidth() != c.w || sps.CroppedHeight() != c.h {
				t.Fatalf("the sequence describes %dx%d, want %dx%d", sps.CroppedWidth(), sps.CroppedHeight(), c.w, c.h)
			}
			if pps.CABAC == c.baseline {
				t.Fatalf("entropy coding CABAC=%v for baseline=%v, which is not what the picture buffer says", pps.CABAC, c.baseline)
			}
			if pps.PicInitQPMinus26 != int32(e.qp)-26 || !pps.DeblockingFilterControlPresent {
				t.Fatal("the initial quantiser or deblocking control differs from the picture buffer")
			}
			if pps.NumRefIdxL0DefaultActiveMinus1 != 0 || pps.WeightedPred || pps.Transform8x8Mode {
				t.Fatal("the picture parameter set claims a tool the picture buffer does not enable")
			}
		})
	}
}

func TestParameterSetsGoInFrontOfAnIDRThatHasNone(t *testing.T) {
	e := headerTestEncoder(ProfileH264Main, false, 176, 144)
	hdrs, _ := e.parameterSets()
	idr := []byte{0, 0, 0, 1, 0x65, 0x88, 0x84}
	got, err := e.withParameterSets(idr, true)
	if err != nil || !bytes.Equal(got, append(append([]byte(nil), hdrs...), idr...)) {
		t.Fatalf("an IDR without parameter sets came back as % x", got)
	}
	p := []byte{0, 0, 0, 1, 0x41, 0x9a}
	if got, _ := e.withParameterSets(p, false); !bytes.Equal(got, p) {
		t.Fatal("a P slice was given parameter sets")
	}
	withOwn := append(append([]byte(nil), hdrs...), idr...)
	if got, _ := e.withParameterSets(withOwn, true); !bytes.Equal(got, withOwn) {
		t.Fatal("an IDR that already carried parameter sets was given a second pair")
	}
}
