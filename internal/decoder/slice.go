package decoder

import (
	"fmt"

	"github.com/oops1/go264/internal/bits"
	"github.com/oops1/go264/internal/syntax"
)

type sliceDecoder struct {
	r    *bits.Reader
	sps  *syntax.SPS
	pps  *syntax.PPS
	hdr  *syntax.SliceHeader
	pic  *Picture
	grid *mbGrid

	cur *mbState
	nb  neighbours

	mbx int
	mby int

	qpY         int
	sliceID     int
	constrained bool
}

func (d *sliceDecoder) totalMBs() int { return d.grid.widthMBs * d.grid.heightMBs }

func (d *sliceDecoder) run() error {
	d.qpY = d.hdr.QPY(d.pps)
	d.constrained = d.pps.ConstrainedIntraPred
	var res mbResidual

	mbAddr := int(d.hdr.FirstMBInSlice)
	if mbAddr >= d.totalMBs() {
		return fmt.Errorf("%w: first_mb_in_slice %d beyond the picture", ErrCorrupt, mbAddr)
	}
	for {
		d.mbx = mbAddr % d.grid.widthMBs
		d.mby = mbAddr / d.grid.widthMBs
		d.cur = d.grid.at(d.mbx, d.mby)
		*d.cur = mbState{
			sliceID:        d.sliceID,
			qpY:            d.qpY,
			chromaQPOffset: [2]int{int(d.pps.ChromaQPIndexOffset), int(d.pps.SecondChromaQPIndexOffset)},
			disableDeblock: d.hdr.DisableDeblockingFilterIDC,
			alphaOffset:    int(d.hdr.SliceAlphaC0OffsetDiv2) * 2,
			betaOffset:     int(d.hdr.SliceBetaOffsetDiv2) * 2,
		}
		d.nb = d.grid.around(d.mbx, d.mby, d.sliceID)
		res.reset()

		if err := d.decodeMacroblock(&res); err != nil {
			return err
		}
		d.cur.decoded = true

		mbAddr++
		if !d.r.MoreRBSPData() {
			return nil
		}
		if mbAddr >= d.totalMBs() {
			return fmt.Errorf("%w: slice extends past the last macroblock", ErrCorrupt)
		}
	}
}

func (d *sliceDecoder) decodeMacroblock(res *mbResidual) error {
	if !d.hdr.SliceType.IsI() {
		return fmt.Errorf("%w: %s slices", ErrUnsupported, d.hdr.SliceType)
	}
	v, err := d.r.ReadUE()
	if err != nil {
		return err
	}
	info, ok := intraMBType(v)
	if !ok {
		return fmt.Errorf("%w: mb_type %d in an I slice", ErrCorrupt, v)
	}
	return d.decodeIntraMB(info, res)
}
