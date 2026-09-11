//go:build linux && (amd64 || arm64)

package vaapi

import (
	"github.com/oops1/go.264/internal/nal"
	"github.com/oops1/go.264/internal/syntax"
)

const constraintSet1 = 0x40

func (e *Encoder) sequenceParameterSet() *syntax.SPS {
	sps := &syntax.SPS{
		ProfileIDC:                  profileIDC(e.profile),
		LevelIDC:                    e.levelIDC,
		ChromaFormatIDC:             1,
		Log2MaxFrameNumMinus4:       log2MaxFrameNumM4,
		PicOrderCntType:             0,
		Log2MaxPicOrderCntLsbMinus4: log2MaxPicOrderCntLsbM4,
		MaxNumRefFrames:             maxNumRefFrames,
		PicWidthInMbsMinus1:         uint32(e.mbWidth - 1),
		PicHeightInMapUnitsMinus1:   uint32(e.mbHeight - 1),
		FrameMbsOnly:                true,
		Direct8x8Inference:          true,
	}
	if e.baseline {
		sps.ConstraintSet = constraintSet1
	}
	if e.alignedWidth != e.width || e.alignedHeight != e.height {
		sps.FrameCropping = true
		sps.FrameCropRightOffset = uint32(e.alignedWidth-e.width) / 2
		sps.FrameCropBottomOffset = uint32(e.alignedHeight-e.height) / 2
	}
	return sps
}

func (e *Encoder) pictureParameterSet() *syntax.PPS {
	return &syntax.PPS{
		CABAC:                          !e.baseline,
		PicInitQPMinus26:               int32(e.qp) - 26,
		DeblockingFilterControlPresent: true,
	}
}

func (e *Encoder) parameterSets() ([]byte, error) {
	if e.headers != nil {
		return e.headers, nil
	}
	sps := e.sequenceParameterSet()
	spsRBSP, err := syntax.WriteSPS(sps)
	if err != nil {
		return nil, err
	}
	ppsRBSP, err := syntax.WritePPS(e.pictureParameterSet(), func(uint32) *syntax.SPS { return sps })
	if err != nil {
		return nil, err
	}
	out := nal.AppendAnnexB(nil, nal.Unit{Header: nal.Header{RefIDC: 3, Type: nal.TypeSPS}, RBSP: spsRBSP}, true)
	out = nal.AppendAnnexB(out, nal.Unit{Header: nal.Header{RefIDC: 3, Type: nal.TypePPS}, RBSP: ppsRBSP}, true)
	e.headers = out
	return out, nil
}

func carriesParameterSets(pkt []byte) bool {
	sps, pps := false, false
	for _, u := range nal.SplitAnnexB(pkt) {
		if len(u) == 0 {
			continue
		}
		switch nal.Type(u[0] & 0x1f) {
		case nal.TypeSPS:
			sps = true
		case nal.TypePPS:
			pps = true
		}
	}
	return sps && pps
}

func (e *Encoder) withParameterSets(pkt []byte, isIDR bool) ([]byte, error) {
	if !isIDR || carriesParameterSets(pkt) {
		return pkt, nil
	}
	hdrs, err := e.parameterSets()
	if err != nil {
		return nil, err
	}
	out := make([]byte, 0, len(hdrs)+len(pkt))
	out = append(out, hdrs...)
	return append(out, pkt...), nil
}
