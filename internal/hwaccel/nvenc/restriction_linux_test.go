//go:build linux && (amd64 || arm64)

package nvenc

import (
	"testing"

	"github.com/oops1/go.264/internal/nal"
	"github.com/oops1/go.264/internal/syntax"
)

func TestTheDriverDeclaresThatNothingIsReordered(t *testing.T) {
	requireAdapter(t)
	stream, name := encodeProbeStream(t)
	seen := 0
	for _, ebsp := range nal.SplitAnnexB(stream) {
		u, err := nal.Parse(ebsp)
		if err != nil {
			t.Fatal(err)
		}
		if u.Header.Type != nal.TypeSPS {
			continue
		}
		sps, err := syntax.ParseSPS(u.RBSP)
		if err != nil {
			t.Fatalf("%s wrote a sequence we cannot read: %v", name, err)
		}
		seen++
		v := sps.VUI
		if !sps.VUIPresent || !v.BitstreamRestriction {
			t.Fatalf("%s wrote no bitstream restriction, so a decoder holds %d frames back", name, sps.MaxDpbFrames())
		}
		if v.MaxNumReorderFrames != 0 {
			t.Fatalf("%s declares %d frames of reordering for I and P pictures", name, v.MaxNumReorderFrames)
		}
		if v.MaxDecFrameBuffering < sps.MaxNumRefFrames {
			t.Fatalf("%s declares a buffer of %d frames for %d references", name, v.MaxDecFrameBuffering, sps.MaxNumRefFrames)
		}
		t.Logf("%s: max_num_ref_frames %d, max_dec_frame_buffering %d, picture order count type %d",
			name, sps.MaxNumRefFrames, v.MaxDecFrameBuffering, sps.PicOrderCntType)
	}
	if seen == 0 {
		t.Fatalf("%s wrote no sequence parameter set", name)
	}
}
