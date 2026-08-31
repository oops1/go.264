package decoder

import (
	"fmt"

	"github.com/oops1/go.264/internal/cabac"
	"github.com/oops1/go.264/internal/loopfilter"
	"github.com/oops1/go.264/internal/transform"
)

func (d *sliceDecoder) neighbourCBP(m *mbState) int {
	if m == nil {
		return cabac.CBPUnavailable
	}
	if m.IPCM {
		return cabac.CBPPCM
	}
	return m.cbpLuma | m.cbpChroma<<4
}

func (d *sliceDecoder) intraMBTypeInc() int {
	inc := 0
	if m := d.nb.left; m != nil && m.Intra && m.kind != mbTypeINxN {
		inc++
	}
	if m := d.nb.top; m != nil && m.Intra && m.kind != mbTypeINxN {
		inc++
	}
	return inc
}

func (d *sliceDecoder) chromaPredInc() int {
	inc := 0
	if m := d.nb.left; m != nil && m.Intra && m.chromaMode != 0 {
		inc++
	}
	if m := d.nb.top; m != nil && m.Intra && m.chromaMode != 0 {
		inc++
	}
	return inc
}

func (d *sliceDecoder) cbfCondTerm(m *mbState, present bool, flag bool) int {
	if m == nil {
		if d.cur.Intra {
			return 1
		}
		return 0
	}
	if m.IPCM {
		return 1
	}
	if !present {
		return 0
	}
	if flag {
		return 1
	}
	return 0
}

func (d *sliceDecoder) lumaCBFContext(blk int) (int, int) {
	x, y := blockX[blk], blockY[blk]
	var a, b int
	if x > 0 {
		a = d.cbfCondTerm(d.cur, true, d.cur.NzY[loopfilter.BlkIdxAt(x-4, y)] != 0)
	} else {
		m := d.nb.left
		a = d.cbfCondTerm(m, m != nil && m.hasLumaResidual(), m != nil && m.NzY[loopfilter.BlkIdxAt(12, y)] != 0)
	}
	if y > 0 {
		b = d.cbfCondTerm(d.cur, true, d.cur.NzY[loopfilter.BlkIdxAt(x, y-4)] != 0)
	} else {
		m := d.nb.top
		b = d.cbfCondTerm(m, m != nil && m.hasLumaResidual(), m != nil && m.NzY[loopfilter.BlkIdxAt(x, 12)] != 0)
	}
	return a, b
}

func (m *mbState) hasLumaResidual() bool {
	return m.kind != mbTypePSkip
}

func (d *sliceDecoder) lumaDCCBFContext() (int, int) {
	a := d.cbfCondTerm(d.nb.left, d.nb.left != nil && d.nb.left.kind == mbTypeI16x16,
		d.nb.left != nil && d.nb.left.cbfLumaDC)
	b := d.cbfCondTerm(d.nb.top, d.nb.top != nil && d.nb.top.kind == mbTypeI16x16,
		d.nb.top != nil && d.nb.top.cbfLumaDC)
	return a, b
}

func (d *sliceDecoder) chromaDCCBFContext(plane int) (int, int) {
	pick := func(m *mbState) (bool, bool) {
		if m == nil {
			return false, false
		}
		return m.cbpChroma != 0, m.cbfChromaDC[plane]
	}
	pa, fa := pick(d.nb.left)
	pb, fb := pick(d.nb.top)
	return d.cbfCondTerm(d.nb.left, pa, fa), d.cbfCondTerm(d.nb.top, pb, fb)
}

func (d *sliceDecoder) chromaACCBFContext(plane, blk int) (int, int) {
	nz := func(m *mbState, idx int) bool {
		if plane == 0 {
			return m.nzCb[idx] != 0
		}
		return m.nzCr[idx] != 0
	}
	x, y := chromaBlockX[blk], chromaBlockY[blk]
	var a, b int
	if x > 0 {
		a = d.cbfCondTerm(d.cur, true, nz(d.cur, blk-1))
	} else {
		m := d.nb.left
		a = d.cbfCondTerm(m, m != nil && m.cbpChroma == 2, m != nil && nz(m, blk+1))
	}
	if y > 0 {
		b = d.cbfCondTerm(d.cur, true, nz(d.cur, blk-2))
	} else {
		m := d.nb.top
		b = d.cbfCondTerm(m, m != nil && m.cbpChroma == 2, m != nil && nz(m, blk+2))
	}
	return a, b
}

func (d *sliceDecoder) cabacIntraPredModes() {
	for blk := 0; blk < 16; blk++ {
		pred := d.predIntra4x4Mode(blk)
		d.cur.intra4Modes[blk] = int8(d.cb.Intra4x4PredMode(pred))
	}
}

func (d *sliceDecoder) cabacResidual(res *mbResidual, i16 bool) {
	if i16 {
		var scan [16]int32
		a, b := d.lumaDCCBFContext()
		n := d.cb.ResidualBlock(scan[:], cabac.CatIntra16x16DC, a, b, 1)
		d.cur.cbfLumaDC = n != 0
		transform.ScanToBlock(&res.lumaDC, &scan)
	}
	for i8 := 0; i8 < 4; i8++ {
		if d.cur.cbpLuma&(1<<uint(i8)) == 0 {
			for i4 := 0; i4 < 4; i4++ {
				d.cur.NzY[i8*4+i4] = 0
			}
			continue
		}
		for i4 := 0; i4 < 4; i4++ {
			blk := i8*4 + i4
			var scan [16]int32
			a, b := d.lumaCBFContext(blk)
			cat := cabac.CatLuma4x4
			start := 0
			if i16 {
				cat = cabac.CatIntra16x16AC
				start = 1
			}
			n := d.cb.ResidualBlock(scan[start:], cat, a, b, 1)
			d.cur.NzY[blk] = uint8(n)
			transform.ScanToBlock(&res.luma[blk], &scan)
		}
	}
	if d.cur.cbpChroma == 0 {
		return
	}
	for plane := 0; plane < 2; plane++ {
		var dc [4]int32
		a, b := d.chromaDCCBFContext(plane)
		n := d.cb.ResidualBlock(dc[:], cabac.CatChromaDC, a, b, 1)
		d.cur.cbfChromaDC[plane] = n != 0
		copy(res.chromaDC[plane][:], dc[:])
	}
	if d.cur.cbpChroma < 2 {
		return
	}
	for plane := 0; plane < 2; plane++ {
		for blk := 0; blk < 4; blk++ {
			var scan [16]int32
			a, b := d.chromaACCBFContext(plane, blk)
			n := d.cb.ResidualBlock(scan[1:], cabac.CatChromaAC, a, b, 1)
			if plane == 0 {
				d.cur.nzCb[blk] = uint8(n)
			} else {
				d.cur.nzCr[blk] = uint8(n)
			}
			transform.ScanToBlock(&res.chromaAC[plane][blk], &scan)
		}
	}
}

func (d *sliceDecoder) cabacQPDelta() error {
	delta := d.cb.MBQPDelta(d.prevQPDeltaNonZero)
	d.prevQPDeltaNonZero = delta != 0
	if delta < -26 || delta > 25 {
		return fmt.Errorf("%w: mb_qp_delta %d", ErrCorrupt, delta)
	}
	d.qpY = (d.qpY + delta + 52) % 52
	return nil
}

func (d *sliceDecoder) decodeIntraMBCABAC(res *mbResidual) error {
	v := d.cb.IntraMBType(d.intraMBTypeInc())
	info, ok := intraMBType(uint32(v))
	if !ok {
		return fmt.Errorf("%w: mb_type %d in a CABAC I slice", ErrCorrupt, v)
	}
	d.cur.Intra = true
	d.cur.kind = info.kind

	switch info.kind {
	case mbTypeIPCM:
		return fmt.Errorf("%w: I_PCM under CABAC", ErrUnsupported)

	case mbTypeINxN:
		if d.pps.Transform8x8Mode {
			return fmt.Errorf("%w: 8x8 transform", ErrUnsupported)
		}
		d.cabacIntraPredModes()
		d.cur.chromaMode = int8(d.cb.IntraChromaPredMode(d.chromaPredInc()))
		cbp := d.cb.CodedBlockPatternLuma(d.neighbourCBP(d.nb.left), d.neighbourCBP(d.nb.top))
		d.cur.cbpLuma = cbp
		d.cur.cbpChroma = d.cb.CodedBlockPatternChroma(d.neighbourCBP(d.nb.left), d.neighbourCBP(d.nb.top))
		if d.cur.cbpLuma != 0 || d.cur.cbpChroma != 0 {
			if err := d.cabacQPDelta(); err != nil {
				return err
			}
			d.cabacResidual(res, false)
		} else {
			d.prevQPDeltaNonZero = false
		}
		d.cur.QPY = d.qpY
		d.reconstructIntra4x4(res)
		d.reconstructChroma(res)
		return nil

	case mbTypeI16x16:
		d.cur.intra16Mode = int8(info.intra16PredMode)
		d.cur.cbpLuma = info.cbpLuma
		d.cur.cbpChroma = info.cbpChroma
		d.cur.chromaMode = int8(d.cb.IntraChromaPredMode(d.chromaPredInc()))
		if err := d.cabacQPDelta(); err != nil {
			return err
		}
		d.cabacResidual(res, true)
		d.cur.QPY = d.qpY
		d.reconstructIntra16x16(res)
		d.reconstructChroma(res)
		return nil
	}
	return fmt.Errorf("%w: macroblock kind %d", ErrCorrupt, info.kind)
}

func (d *sliceDecoder) runCABAC() error {
	if err := d.cb.Init(d.r, d.qpY, d.hdr.SliceType.IsI(), d.hdr.CABACInitIDC); err != nil {
		return err
	}
	var res mbResidual
	mbAddr := int(d.hdr.FirstMBInSlice)
	if mbAddr >= d.totalMBs() {
		return fmt.Errorf("%w: first_mb_in_slice %d beyond the picture", ErrCorrupt, mbAddr)
	}
	if !d.hdr.SliceType.IsI() {
		return fmt.Errorf("%w: CABAC %s slices", ErrUnsupported, d.hdr.SliceType)
	}
	for {
		d.startMB(mbAddr)
		res.reset()
		if err := d.decodeIntraMBCABAC(&res); err != nil {
			return err
		}
		d.cur.Decoded = true
		if d.cb.EndOfSlice() {
			return nil
		}
		mbAddr++
		if mbAddr >= d.totalMBs() {
			return fmt.Errorf("%w: slice extends past the last macroblock", ErrCorrupt)
		}
	}
}
