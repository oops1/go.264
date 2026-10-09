package go264

import (
	"bytes"
	"testing"

	"github.com/oops1/go.264/internal/nal"
	"github.com/oops1/go.264/internal/syntax"
)

func cpuPackets(t *testing.T, w, h, n int) [][]byte {
	t.Helper()
	enc, err := NewEncoder(EncoderConfig{Width: w, Height: h, FPSNum: 25, FPSDen: 1, GOPSize: 8, QP: 26,
		ForceSoftware: true})
	if err != nil {
		t.Fatal(err)
	}
	defer enc.Close()
	var out [][]byte
	for i := 0; i < n; i++ {
		pkt, err := enc.Encode(pattern(w, h, i))
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, pkt)
	}
	return out
}

func spsIn(t *testing.T, pkt []byte) *syntax.SPS {
	t.Helper()
	for _, ebsp := range nal.SplitAnnexB(pkt) {
		u, err := nal.Parse(ebsp)
		if err != nil {
			t.Fatal(err)
		}
		if u.Header.Type != nal.TypeSPS {
			continue
		}
		sps, err := syntax.ParseSPS(u.RBSP)
		if err != nil {
			t.Fatal(err)
		}
		return sps
	}
	return nil
}

func withSPS(t *testing.T, pkt []byte, edit func(s *syntax.SPS) []byte) []byte {
	t.Helper()
	var out []byte
	for _, ebsp := range nal.SplitAnnexB(pkt) {
		out = append(out, 0, 0, 0, 1)
		u, err := nal.Parse(ebsp)
		if err != nil {
			t.Fatal(err)
		}
		if u.Header.Type != nal.TypeSPS {
			out = append(out, ebsp...)
			continue
		}
		sps, err := syntax.ParseSPS(u.RBSP)
		if err != nil {
			t.Fatal(err)
		}
		out = nal.Escape(append(out, u.Header.Byte()), edit(sps))
	}
	return out
}

func unrestricted(t *testing.T, pocType uint32) func(s *syntax.SPS) []byte {
	return func(s *syntax.SPS) []byte {
		s.VUI.BitstreamRestriction = false
		s.VUI.MotionVectorsOverPicBoundary = false
		s.VUI.MaxBytesPerPicDenom, s.VUI.MaxBitsPerMBDenom = 0, 0
		s.VUI.Log2MaxMvLengthHorizontal, s.VUI.Log2MaxMvLengthVertical = 0, 0
		s.VUI.MaxNumReorderFrames, s.VUI.MaxDecFrameBuffering = 0, 0
		s.PicOrderCntType = pocType
		rbsp, err := syntax.WriteSPS(s)
		if err != nil {
			t.Fatal(err)
		}
		return rbsp
	}
}

func outsideSPS(t *testing.T, pkt []byte) []byte {
	t.Helper()
	var out []byte
	for _, ebsp := range nal.SplitAnnexB(pkt) {
		if nal.Type(ebsp[0]&0x1f) != nal.TypeSPS {
			out = append(out, ebsp...)
		}
	}
	return out
}

func TestAHardwareSequenceWithoutReorderingSaysSo(t *testing.T) {
	packets := cpuPackets(t, 176, 144, 10)
	driver := withSPS(t, packets[0], unrestricted(t, 2))
	if s := spsIn(t, driver); s.VUI.BitstreamRestriction {
		t.Fatal("the stand-in for a driver's sequence still carries a restriction")
	}

	e := &Encoder{}
	out, err := e.fromHardware(driver, nil)
	if err != nil {
		t.Fatal(err)
	}
	s := spsIn(t, out)
	if s == nil || !s.VUIPresent || !s.VUI.BitstreamRestriction {
		t.Fatal("the hardware sequence still leaves a decoder to assume reordering")
	}
	if s.VUI.MaxNumReorderFrames != 0 || s.VUI.MaxDecFrameBuffering != s.MaxNumRefFrames {
		t.Fatalf("max_num_reorder_frames %d, max_dec_frame_buffering %d, want 0 and %d",
			s.VUI.MaxNumReorderFrames, s.VUI.MaxDecFrameBuffering, s.MaxNumRefFrames)
	}
	if !s.VUI.TimingInfoPresent {
		t.Fatal("the rewrite dropped the driver's timing information")
	}
	if !bytes.Equal(outsideSPS(t, out), outsideSPS(t, driver)) {
		t.Fatal("the rewrite touched more than the sequence parameter set")
	}
	if !bytes.Equal(outsideSPS(t, out), outsideSPS(t, packets[0])) {
		t.Fatal("the stand-in packet does not carry the encoder's own pictures")
	}

	later := withSPS(t, packets[8], unrestricted(t, 2))
	later = annexBWithout(later, nal.TypeSPS, nal.TypePPS)
	if countUnits(later, nal.TypeSliceIDR) == 0 {
		t.Fatal("frame 8 is not an IDR, so the repeated sequence is not exercised")
	}
	again, err := e.fromHardware(later, nil)
	if err != nil {
		t.Fatal(err)
	}
	if s := spsIn(t, again); s == nil || !s.VUI.BitstreamRestriction {
		t.Fatal("the sequence put in front of a later IDR is the driver's, not the restricted one")
	}
}

func TestAHardwareSequenceIsLeftAloneWhenItMayReorder(t *testing.T) {
	packets := cpuPackets(t, 176, 144, 1)
	cases := map[string][]byte{
		"picture order count type 0": withSPS(t, packets[0], unrestricted(t, 0)),
		"a restriction of its own": withSPS(t, packets[0], func(s *syntax.SPS) []byte {
			s.RestrictBitstream(1)
			rbsp, err := syntax.WriteSPS(s)
			if err != nil {
				t.Fatal(err)
			}
			return rbsp
		}),
		"syntax we would not write back": withSPS(t, packets[0], func(s *syntax.SPS) []byte {
			rbsp := unrestricted(t, 2)(s)
			return append(rbsp[:len(rbsp):len(rbsp)], 0x80)
		}),
	}
	for name, pkt := range cases {
		if s := spsIn(t, pkt); s == nil {
			t.Fatalf("%s: the stand-in packet carries no readable sequence", name)
		}
		e := &Encoder{}
		out, err := e.fromHardware(pkt, nil)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(out, pkt) {
			t.Errorf("%s: the hardware packet was rewritten", name)
		}
	}
}

func framesOutAfterEachPacket(t *testing.T, packets [][]byte) []int {
	t.Helper()
	d := NewDecoderWithConfig(DecoderConfig{ForceSoftware: true})
	defer d.Close()
	var counts []int
	total := 0
	for _, p := range packets {
		frames, err := d.Decode(p)
		if err != nil {
			t.Fatal(err)
		}
		total += len(frames)
		counts = append(counts, total)
	}
	return counts
}

func TestAFrameLeavesTheDecoderWithoutWaitingForTheBuffer(t *testing.T) {
	packets := cpuPackets(t, 176, 144, 12)
	counts := framesOutAfterEachPacket(t, packets)
	for i, n := range counts {
		if n < i-1 {
			t.Fatalf("after %d packets the decoder has given back %d frames: it is holding frames for reordering that never comes",
				i+1, n)
		}
	}

	old := make([][]byte, len(packets))
	for i, p := range packets {
		old[i] = withSPS(t, p, unrestricted(t, 2))
	}
	held := framesOutAfterEachPacket(t, old)
	if held[len(held)-1] >= counts[len(counts)-1] {
		t.Fatalf("without the restriction the decoder gave back %d frames, with it %d: the test does not show the delay it guards against",
			held[len(held)-1], counts[len(counts)-1])
	}
}
