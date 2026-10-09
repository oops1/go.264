package encoder

import (
	"fmt"
	"testing"

	"github.com/oops1/go.264/internal/bits"
	"github.com/oops1/go.264/internal/nal"
	"github.com/oops1/go.264/internal/syntax"
)

type codedPicture struct {
	idr       bool
	reference bool
	poc       int
}

func codedPicturesOf(t *testing.T, stream []byte, sps *syntax.SPS, pps *syntax.PPS) []codedPicture {
	t.Helper()
	var out []codedPicture
	maxLsb := 1 << (sps.Log2MaxPicOrderCntLsbMinus4 + 4)
	prevMsb, prevLsb := 0, 0
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
		if hdr.FirstMBInSlice != 0 {
			continue
		}
		p := codedPicture{idr: hdr.IDR, reference: u.Header.RefIDC != 0}
		if p.idr {
			prevMsb, prevLsb = 0, 0
		}
		switch sps.PicOrderCntType {
		case 0:
			lsb := int(hdr.PicOrderCntLsb)
			msb := prevMsb
			switch {
			case lsb < prevLsb && prevLsb-lsb >= maxLsb/2:
				msb += maxLsb
			case lsb > prevLsb && lsb-prevLsb > maxLsb/2:
				msb -= maxLsb
			}
			p.poc = msb + lsb
			if p.reference {
				prevMsb, prevLsb = msb, lsb
			}
		case 2:
			p.poc = 2 * len(out)
		default:
			t.Fatalf("picture order count type %d is not one the encoder writes", sps.PicOrderCntType)
		}
		out = append(out, p)
	}
	return out
}

func reorderDepthOf(pics []codedPicture) int {
	depth, start := 0, 0
	for i, p := range pics {
		if p.idr {
			start = i
		}
		n := 0
		for _, q := range pics[start:i] {
			if q.poc > p.poc {
				n++
			}
		}
		depth = max(depth, n)
	}
	return depth
}

type dpbSlot struct {
	poc     int
	ref     bool
	waiting bool
}

func outputOrderHolds(pics []codedPicture, maxRefs, capacity, reorder int) error {
	var dpb []*dpbSlot
	last, started := 0, false
	emit := func(poc int) error {
		if started && poc < last {
			return fmt.Errorf("picture %d left the buffer after picture %d", poc, last)
		}
		started, last = true, poc
		return nil
	}
	waiting := func() int {
		n := 0
		for _, s := range dpb {
			if s.waiting {
				n++
			}
		}
		return n
	}
	bump := func() error {
		best := -1
		for i, s := range dpb {
			if s.waiting && (best < 0 || s.poc < dpb[best].poc) {
				best = i
			}
		}
		if best < 0 {
			return fmt.Errorf("the buffer of %d frames is full of references", capacity)
		}
		if err := emit(dpb[best].poc); err != nil {
			return err
		}
		dpb[best].waiting = false
		if !dpb[best].ref {
			dpb = append(dpb[:best], dpb[best+1:]...)
		}
		return nil
	}
	for _, p := range pics {
		if p.idr {
			for waiting() > 0 {
				if err := bump(); err != nil {
					return err
				}
			}
			dpb, started = nil, false
		}
		if p.reference && !p.idr {
			refs := 0
			for _, s := range dpb {
				if s.ref {
					refs++
				}
			}
			if refs >= maxRefs {
				for i, s := range dpb {
					if s.ref {
						s.ref = false
						if !s.waiting {
							dpb = append(dpb[:i], dpb[i+1:]...)
						}
						break
					}
				}
			}
		}
		direct := false
		for len(dpb) >= capacity && !direct {
			if !p.reference {
				direct = true
				for _, s := range dpb {
					if s.waiting && s.poc < p.poc {
						direct = false
					}
				}
				if direct {
					if err := emit(p.poc); err != nil {
						return err
					}
					continue
				}
			}
			if err := bump(); err != nil {
				return err
			}
		}
		if direct {
			continue
		}
		dpb = append(dpb, &dpbSlot{poc: p.poc, ref: p.reference, waiting: true})
		for waiting() > reorder {
			if err := bump(); err != nil {
				return err
			}
		}
	}
	for waiting() > 0 {
		if err := bump(); err != nil {
			return err
		}
	}
	return nil
}

func declaredRestriction(t *testing.T, enc *Encoder) *syntax.SPS {
	t.Helper()
	hdrs, err := enc.Headers()
	if err != nil {
		t.Fatal(err)
	}
	for _, ebsp := range nal.SplitAnnexB(hdrs) {
		u, err := nal.Parse(ebsp)
		if err != nil {
			t.Fatal(err)
		}
		if u.Header.Type != nal.TypeSPS {
			continue
		}
		sps, err := syntax.ParseSPS(u.RBSP)
		if err != nil {
			t.Fatalf("reading back our own sequence parameter set: %v", err)
		}
		if !sps.VUIPresent || !sps.VUI.BitstreamRestriction {
			t.Fatal("the sequence parameter set carries no bitstream restriction")
		}
		return sps
	}
	t.Fatal("the headers carry no sequence parameter set")
	return nil
}

func TestTheSequenceDeclaresNoReorderingWithoutBFrames(t *testing.T) {
	cases := []Config{
		{Width: 176, Height: 144, RefFrames: 1},
		{Width: 176, Height: 144, RefFrames: 4},
		{Width: 176, Height: 144, RefFrames: 1, LongTermReferences: 2},
		{Width: 176, Height: 144, RefFrames: 1, CABAC: true, Transform8x8: true},
		{Width: 176, Height: 144, RefFrames: 1, BitrateKbps: 300, VBVBufferKbits: 300, VBVMaxrateKbps: 400},
		{Width: 1920, Height: 1200, RefFrames: 1},
		{Width: 1920, Height: 1080, RefFrames: 1},
		{Width: 3440, Height: 1440, RefFrames: 1},
	}
	for _, cfg := range cases {
		cfg.FPSNum, cfg.FPSDen, cfg.GOPSize, cfg.QP = 30, 1, 30, 26
		enc, err := New(cfg)
		if err != nil {
			t.Fatalf("%dx%d: %v", cfg.Width, cfg.Height, err)
		}
		sps := declaredRestriction(t, enc)
		v := sps.VUI
		name := fmt.Sprintf("%dx%d, %d+%d references", cfg.Width, cfg.Height, cfg.RefFrames, cfg.LongTermReferences)
		if v.MaxNumReorderFrames != 0 {
			t.Errorf("%s: max_num_reorder_frames %d, want 0", name, v.MaxNumReorderFrames)
		}
		if int(v.MaxDecFrameBuffering) != enc.refCap() || v.MaxDecFrameBuffering != sps.MaxNumRefFrames {
			t.Errorf("%s: max_dec_frame_buffering %d, max_num_ref_frames %d, want both %d",
				name, v.MaxDecFrameBuffering, sps.MaxNumRefFrames, enc.refCap())
		}
		if int(v.MaxDecFrameBuffering) > sps.MaxDpbFrames() {
			t.Errorf("%s: max_dec_frame_buffering %d exceeds the level's %d frames",
				name, v.MaxDecFrameBuffering, sps.MaxDpbFrames())
		}
		if !v.MotionVectorsOverPicBoundary || v.MaxBytesPerPicDenom != 2 || v.MaxBitsPerMBDenom != 1 ||
			v.Log2MaxMvLengthHorizontal != 15 || v.Log2MaxMvLengthVertical != 15 {
			t.Errorf("%s: the restriction tightens limits the stream had without it: %+v", name, v)
		}
		if sps.MaxNumReorder() != 0 {
			t.Errorf("%s: our own decoder would hold back %d frames", name, sps.MaxNumReorder())
		}
	}
}

func TestTheDeclaredReorderingCoversTheBFrames(t *testing.T) {
	for _, b := range []int{1, 2, 3, 7} {
		for _, refs := range []int{2, 4} {
			cfg := bConfig(176, 144, 26, b, 30, b%2 == 0)
			cfg.RefFrames = refs
			enc, err := New(cfg)
			if err != nil {
				t.Fatal(err)
			}
			sps := declaredRestriction(t, enc)
			v := sps.VUI
			if v.MaxNumReorderFrames < 1 {
				t.Errorf("%d B frames, %d references: max_num_reorder_frames %d, want at least 1", b, refs, v.MaxNumReorderFrames)
			}
			if v.MaxDecFrameBuffering < sps.MaxNumRefFrames || v.MaxDecFrameBuffering < v.MaxNumReorderFrames {
				t.Errorf("%d B frames, %d references: max_dec_frame_buffering %d below max_num_ref_frames %d or the reordering %d",
					b, refs, v.MaxDecFrameBuffering, sps.MaxNumRefFrames, v.MaxNumReorderFrames)
			}
		}
	}
}

func TestTheStreamReordersNoDeeperThanItDeclares(t *testing.T) {
	cases := []struct {
		name   string
		cfg    Config
		forced map[int]bool
	}{
		{"p only", Config{RefFrames: 1, GOPSize: 12}, nil},
		{"p only, three references", Config{RefFrames: 3, GOPSize: 12}, nil},
		{"one b frame", Config{RefFrames: 2, BFrames: 1, GOPSize: 12}, nil},
		{"two b frames", Config{RefFrames: 2, BFrames: 2, GOPSize: 12, CABAC: true}, nil},
		{"three b frames, forced key frames", Config{RefFrames: 3, BFrames: 3, GOPSize: 30},
			map[int]bool{6: true, 13: true}},
		{"seven b frames, short gop", Config{RefFrames: 2, BFrames: 7, GOPSize: 10}, nil},
	}
	for _, c := range cases {
		cfg := c.cfg
		cfg.Width, cfg.Height, cfg.FPSNum, cfg.FPSDen, cfg.QP = 176, 144, 25, 1, 28
		enc, err := New(cfg)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		sps := declaredRestriction(t, enc)
		var stream []byte
		const frames = 29
		for i := 0; i < frames; i++ {
			if c.forced[i] {
				enc.ForceKeyFrame()
			}
			pkt, err := enc.Encode(movingFrame(cfg.Width, cfg.Height, i))
			if err != nil {
				t.Fatalf("%s, frame %d: %v", c.name, i, err)
			}
			stream = append(stream, pkt...)
		}
		tail, err := enc.Flush()
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		stream = append(stream, tail...)

		pics := codedPicturesOf(t, stream, enc.SPS(), enc.PPS())
		if len(pics) != frames {
			t.Fatalf("%s: %d coded pictures for %d frames", c.name, len(pics), frames)
		}
		depth := reorderDepthOf(pics)
		declared := int(sps.VUI.MaxNumReorderFrames)
		if depth > declared {
			t.Errorf("%s: the stream reorders %d frames deep, the sequence declares %d", c.name, depth, declared)
		}
		if cfg.BFrames > 0 && depth != declared {
			t.Errorf("%s: the stream reorders %d frames deep, the sequence declares %d; a decoder waits longer than it has to",
				c.name, depth, declared)
		}
		if err := outputOrderHolds(pics, int(sps.MaxNumRefFrames), int(sps.VUI.MaxDecFrameBuffering), declared); err != nil {
			t.Errorf("%s: a decoder holding %d frames and reordering %d: %v",
				c.name, sps.VUI.MaxDecFrameBuffering, declared, err)
		}
		if err := outputOrderHolds(pics, int(sps.MaxNumRefFrames), int(sps.VUI.MaxDecFrameBuffering), int(sps.VUI.MaxDecFrameBuffering)); err != nil {
			t.Errorf("%s: a decoder holding %d frames and bumping only when full: %v",
				c.name, sps.VUI.MaxDecFrameBuffering, err)
		}
	}
}

func TestTheOutputCheckCatchesAnUnderstatedReordering(t *testing.T) {
	pics := []codedPicture{
		{idr: true, reference: true, poc: 0},
		{reference: true, poc: 6},
		{reference: true, poc: 4},
		{reference: false, poc: 2},
	}
	if got := reorderDepthOf(pics); got != 2 {
		t.Fatalf("reorderDepthOf = %d, want 2", got)
	}
	if err := outputOrderHolds(pics, 3, 3, 1); err == nil {
		t.Fatal("a reordering of one frame was accepted for a stream that needs two")
	}
	if err := outputOrderHolds(pics, 3, 3, 2); err != nil {
		t.Fatalf("the reordering the stream needs was refused: %v", err)
	}
	if err := outputOrderHolds(pics, 3, 2, 2); err == nil {
		t.Fatal("a buffer of two frames was accepted for three references")
	}
	bframe := []codedPicture{
		{idr: true, reference: true, poc: 0},
		{reference: true, poc: 4},
		{reference: false, poc: 2},
		{reference: true, poc: 8},
		{reference: false, poc: 6},
	}
	if err := outputOrderHolds(bframe, 2, 2, 2); err != nil {
		t.Fatalf("a full buffer did not let a B frame out ahead of its anchor: %v", err)
	}
}
