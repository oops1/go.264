package decoder

import (
	"errors"
	"testing"

	"github.com/oops1/go264/internal/bits"
	"github.com/oops1/go264/internal/frame"
	"github.com/oops1/go264/internal/nal"
	"github.com/oops1/go264/internal/syntax"
)

func minimalSPS() *syntax.SPS {
	return &syntax.SPS{
		ProfileIDC:                77,
		LevelIDC:                  30,
		ID:                        0,
		ChromaFormatIDC:           syntax.Chroma420,
		Log2MaxFrameNumMinus4:     0,
		PicOrderCntType:           2,
		MaxNumRefFrames:           1,
		PicWidthInMbsMinus1:       1,
		PicHeightInMapUnitsMinus1: 1,
		FrameMbsOnly:              true,
		Direct8x8Inference:        true,
	}
}

func minimalPPS(spsID uint32, cabac bool) *syntax.PPS {
	return &syntax.PPS{ID: 0, SPSID: spsID, CABAC: cabac}
}

func lookupOne(sps *syntax.SPS) func(uint32) *syntax.SPS {
	return func(id uint32) *syntax.SPS {
		if id == sps.ID {
			return sps
		}
		return nil
	}
}

func mustWriteSPSBytes(t *testing.T, s *syntax.SPS) []byte {
	t.Helper()
	b, err := syntax.WriteSPS(s)
	if err != nil {
		t.Fatalf("WriteSPS: %v", err)
	}
	return b
}

func mustWritePPSBytes(t *testing.T, p *syntax.PPS, lookup func(uint32) *syntax.SPS) []byte {
	t.Helper()
	b, err := syntax.WritePPS(p, lookup)
	if err != nil {
		t.Fatalf("WritePPS: %v", err)
	}
	return b
}

func mustSliceBytes(t *testing.T, h *syntax.SliceHeader, sps *syntax.SPS, pps *syntax.PPS) []byte {
	t.Helper()
	w := bits.NewWriter()
	if err := syntax.WriteSliceHeader(w, h, sps, pps); err != nil {
		t.Fatalf("WriteSliceHeader: %v", err)
	}
	w.WriteRBSPTrailingBits()
	if err := w.Err(); err != nil {
		t.Fatalf("writer: %v", err)
	}
	return w.Bytes()
}

func annexBUnit(typ nal.Type, refIDC uint8, rbsp []byte) []byte {
	return nal.AppendAnnexB(nil, nal.Unit{Header: nal.Header{RefIDC: refIDC, Type: typ}, RBSP: rbsp}, true)
}

func decodeWithFlush(d *Decoder, data []byte) ([]*frame.Picture, error) {
	pics, err := d.Decode(data)
	if err != nil {
		return pics, err
	}
	rest, err := d.Flush()
	return append(pics, rest...), err
}

func TestCheckSupportedRejections(t *testing.T) {
	base := func() *syntax.SPS {
		return &syntax.SPS{ChromaFormatIDC: syntax.Chroma420, FrameMbsOnly: true}
	}

	cases := []struct {
		name string
		mod  func(*syntax.SPS)
	}{
		{"chroma format not 4:2:0", func(s *syntax.SPS) { s.ChromaFormatIDC = syntax.Chroma422 }},
		{"luma bit depth above 8", func(s *syntax.SPS) { s.BitDepthLumaMinus8 = 2 }},
		{"chroma bit depth above 8", func(s *syntax.SPS) { s.BitDepthChromaMinus8 = 2 }},
		{"field or MBAFF coding", func(s *syntax.SPS) { s.FrameMbsOnly = false }},
		{"sequence scaling matrices", func(s *syntax.SPS) { s.SeqScalingMatrixPresent = true }},
	}

	d := New()
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			sps := base()
			c.mod(sps)
			err := d.checkSupported(sps)
			if !errors.Is(err, ErrUnsupported) {
				t.Fatalf("checkSupported() = %v, want ErrUnsupported", err)
			}
		})
	}

	t.Run("fully supported SPS passes", func(t *testing.T) {
		if err := d.checkSupported(base()); err != nil {
			t.Fatalf("checkSupported() = %v, want nil", err)
		}
	})
}

func TestDecodeCABACPPSIsUnsupported(t *testing.T) {
	sps := minimalSPS()
	spsBytes := mustWriteSPSBytes(t, sps)
	pps := minimalPPS(sps.ID, true)
	ppsBytes := mustWritePPSBytes(t, pps, lookupOne(sps))

	var data []byte
	data = append(data, annexBUnit(nal.TypeSPS, 3, spsBytes)...)
	data = append(data, annexBUnit(nal.TypePPS, 3, ppsBytes)...)

	d := New()
	_, err := decodeWithFlush(d, data)
	if !errors.Is(err, ErrUnsupported) {
		t.Fatalf("Decode() = %v, want ErrUnsupported", err)
	}
}

func TestDecodeSliceReferencingAbsentPPS(t *testing.T) {
	sps := minimalSPS()
	spsBytes := mustWriteSPSBytes(t, sps)
	pps := minimalPPS(sps.ID, false)

	hdr := &syntax.SliceHeader{
		SliceType: syntax.SliceI,
		PPSID:     pps.ID,
		IDR:       true,
		NalRefIDC: 1,
	}
	sliceRBSP := mustSliceBytes(t, hdr, sps, pps)

	var data []byte
	data = append(data, annexBUnit(nal.TypeSPS, 3, spsBytes)...)
	data = append(data, annexBUnit(nal.TypeSliceIDR, 1, sliceRBSP)...)

	d := New()
	_, err := decodeWithFlush(d, data)
	if !errors.Is(err, syntax.ErrMissingPPS) {
		t.Fatalf("Decode() = %v, want syntax.ErrMissingPPS", err)
	}
}

func TestDecodeFirstSliceIsPWithNoReference(t *testing.T) {
	sps := minimalSPS()
	pps := minimalPPS(sps.ID, false)
	spsBytes := mustWriteSPSBytes(t, sps)
	ppsBytes := mustWritePPSBytes(t, pps, lookupOne(sps))

	hdr := &syntax.SliceHeader{SliceType: syntax.SliceP, PPSID: pps.ID, NalRefIDC: 1}
	sliceRBSP := mustSliceBytes(t, hdr, sps, pps)

	var data []byte
	data = append(data, annexBUnit(nal.TypeSPS, 3, spsBytes)...)
	data = append(data, annexBUnit(nal.TypePPS, 3, ppsBytes)...)
	data = append(data, annexBUnit(nal.TypeSliceNonIDR, 1, sliceRBSP)...)

	d := New()
	_, err := decodeWithFlush(d, data)
	if !errors.Is(err, ErrCorrupt) {
		t.Fatalf("Decode() = %v, want ErrCorrupt (P slice with an empty reference list)", err)
	}
}

func TestDecodeMalformedNALHeaderPropagatesError(t *testing.T) {
	data := []byte{0, 0, 1, 0x80, 0x00, 0, 0, 1}

	d := New()
	_, err := d.Decode(data)
	if !errors.Is(err, nal.ErrForbiddenBit) {
		t.Fatalf("Decode() = %v, want nal.ErrForbiddenBit", err)
	}
}

func TestDecodeUnsupportedSPSPropagatesError(t *testing.T) {
	sps := minimalSPS()
	sps.FrameMbsOnly = false // field/MBAFF coding: rejected by checkSupported
	spsBytes := mustWriteSPSBytes(t, sps)

	d := New()
	_, err := decodeWithFlush(d, annexBUnit(nal.TypeSPS, 3, spsBytes))
	if !errors.Is(err, ErrUnsupported) {
		t.Fatalf("Decode() = %v, want ErrUnsupported", err)
	}
}

func TestDecodeMalformedPPSPropagatesParseError(t *testing.T) {
	var data []byte
	data = append(data, annexBUnit(nal.TypePPS, 3, nil)...)
	data = append(data, annexBUnit(nal.TypeAccessUnitDelim, 3, []byte{0xF0})...)

	d := New()
	_, err := d.Decode(data)
	if err == nil {
		t.Fatalf("Decode() = nil, want a parse error from the malformed PPS")
	}
}

func TestDecodeSlicePicScalingMatrixIsUnsupported(t *testing.T) {
	sps := minimalSPS()
	pps := minimalPPS(sps.ID, false)
	pps.HasExtension = true
	pps.PicScalingMatrixPresent = true
	spsBytes := mustWriteSPSBytes(t, sps)
	ppsBytes := mustWritePPSBytes(t, pps, lookupOne(sps))

	hdr := &syntax.SliceHeader{SliceType: syntax.SliceI, PPSID: pps.ID, IDR: true, NalRefIDC: 1}
	sliceRBSP := mustSliceBytes(t, hdr, sps, pps)

	var data []byte
	data = append(data, annexBUnit(nal.TypeSPS, 3, spsBytes)...)
	data = append(data, annexBUnit(nal.TypePPS, 3, ppsBytes)...)
	data = append(data, annexBUnit(nal.TypeSliceIDR, 1, sliceRBSP)...)

	d := New()
	_, err := decodeWithFlush(d, data)
	if !errors.Is(err, ErrUnsupported) {
		t.Fatalf("Decode() = %v, want ErrUnsupported (picture scaling matrices)", err)
	}
}

func TestDecodeSliceWeightedPredIsUnsupported(t *testing.T) {
	sps := minimalSPS()
	pps := minimalPPS(sps.ID, false)
	pps.WeightedPred = true
	spsBytes := mustWriteSPSBytes(t, sps)
	ppsBytes := mustWritePPSBytes(t, pps, lookupOne(sps))

	hdr := &syntax.SliceHeader{SliceType: syntax.SliceP, PPSID: pps.ID, NalRefIDC: 1}
	hdr.PredWeight.L0 = make([]syntax.WeightEntry, 1)
	sliceRBSP := mustSliceBytes(t, hdr, sps, pps)

	var data []byte
	data = append(data, annexBUnit(nal.TypeSPS, 3, spsBytes)...)
	data = append(data, annexBUnit(nal.TypePPS, 3, ppsBytes)...)
	data = append(data, annexBUnit(nal.TypeSliceNonIDR, 1, sliceRBSP)...)

	d := New()
	_, err := decodeWithFlush(d, data)
	if !errors.Is(err, ErrUnsupported) {
		t.Fatalf("Decode() = %v, want ErrUnsupported (weighted prediction)", err)
	}
}

func TestDecodeContinuationSliceWithNoCurrentPictureErrors(t *testing.T) {
	sps := minimalSPS()
	pps := minimalPPS(sps.ID, false)
	spsBytes := mustWriteSPSBytes(t, sps)
	ppsBytes := mustWritePPSBytes(t, pps, lookupOne(sps))

	hdr := &syntax.SliceHeader{FirstMBInSlice: 5, SliceType: syntax.SliceI, PPSID: pps.ID, IDR: true, NalRefIDC: 1}
	sliceRBSP := mustSliceBytes(t, hdr, sps, pps)

	var data []byte
	data = append(data, annexBUnit(nal.TypeSPS, 3, spsBytes)...)
	data = append(data, annexBUnit(nal.TypePPS, 3, ppsBytes)...)
	data = append(data, annexBUnit(nal.TypeSliceIDR, 1, sliceRBSP)...)

	d := New()
	_, err := decodeWithFlush(d, data)
	if !errors.Is(err, ErrNoParameters) {
		t.Fatalf("Decode() = %v, want ErrNoParameters", err)
	}
}

func TestRefPictureOutOfRangeAndEmpty(t *testing.T) {
	sd := &sliceDecoder{}
	if p := sd.refPicture(0); p != nil {
		t.Fatalf("empty reference list: want nil, got %v", p)
	}

	p0 := frame.NewPicture(1, 1)
	p1 := frame.NewPicture(1, 1)
	sd.refList = []*frame.Picture{p0, p1}

	if got := sd.refPicture(5); got != p0 {
		t.Fatalf("out-of-range index: want fallback to refList[0]")
	}
	if got := sd.refPicture(-1); got != p0 {
		t.Fatalf("negative index: want fallback to refList[0]")
	}
	if got := sd.refPicture(1); got != p1 {
		t.Fatalf("in-range index: want refList[1]")
	}
}

func TestReadRefIdxRejectsOutOfRange(t *testing.T) {
	w := bits.NewWriter()
	w.WriteUE(5) // ref_idx_l0 = 5, but max_minus1 = 2
	w.WriteRBSPTrailingBits()
	if err := w.Err(); err != nil {
		t.Fatalf("writer: %v", err)
	}
	sd := &sliceDecoder{r: bits.NewReader(w.Bytes())}

	_, err := sd.readRefIdx(2)
	if !errors.Is(err, ErrCorrupt) {
		t.Fatalf("readRefIdx() = %v, want ErrCorrupt", err)
	}
}

func TestReadMVDRejectsOutOfRange(t *testing.T) {
	cases := []struct {
		name string
		x, y int32
	}{
		{"x too large", 40000, 0},
		{"y too small", 0, -40000},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			w := bits.NewWriter()
			w.WriteSE(c.x)
			w.WriteSE(c.y)
			w.WriteRBSPTrailingBits()
			if err := w.Err(); err != nil {
				t.Fatalf("writer: %v", err)
			}
			sd := &sliceDecoder{r: bits.NewReader(w.Bytes())}

			_, err := sd.readMVD()
			if !errors.Is(err, ErrCorrupt) {
				t.Fatalf("readMVD() = %v, want ErrCorrupt", err)
			}
		})
	}
}
