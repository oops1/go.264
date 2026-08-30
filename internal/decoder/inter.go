package decoder

import (
	"fmt"

	"github.com/oops1/go264/internal/frame"
	"github.com/oops1/go264/internal/mc"
	"github.com/oops1/go264/internal/transform"
)

type subMbInfo struct {
	numParts int
	w        int
	h        int
}

var subMbTable = [4]subMbInfo{
	{1, 8, 8},
	{2, 8, 4},
	{2, 4, 8},
	{4, 4, 4},
}

type mbPart struct {
	x, y, w, h int
}

func partitionsOf(kind int) []mbPart {
	switch kind {
	case mbTypeP16x16:
		return []mbPart{{0, 0, 16, 16}}
	case mbTypeP16x8:
		return []mbPart{{0, 0, 16, 8}, {0, 8, 16, 8}}
	case mbTypeP8x16:
		return []mbPart{{0, 0, 8, 16}, {8, 0, 8, 16}}
	}
	return nil
}

func (d *sliceDecoder) readRefIdx(maxMinus1 uint32) (int8, error) {
	if maxMinus1 == 0 {
		return 0, nil
	}
	v, err := d.r.ReadTE(maxMinus1)
	if err != nil {
		return 0, err
	}
	if v > maxMinus1 {
		return 0, fmt.Errorf("%w: ref_idx_l0 %d", ErrCorrupt, v)
	}
	return int8(v), nil
}

func (d *sliceDecoder) readMVD() ([2]int16, error) {
	x, err := d.r.ReadSE()
	if err != nil {
		return [2]int16{}, err
	}
	y, err := d.r.ReadSE()
	if err != nil {
		return [2]int16{}, err
	}
	if x < -32768 || x > 32767 || y < -32768 || y > 32767 {
		return [2]int16{}, fmt.Errorf("%w: motion vector difference out of range", ErrCorrupt)
	}
	return [2]int16{int16(x), int16(y)}, nil
}

func (d *sliceDecoder) decodeInterMB(info mbTypeInfo, res *mbResidual) error {
	d.cur.kind = info.kind
	d.cur.intra = false
	maxRef := uint32(d.numRefIdxActive - 1)

	if info.kind == mbTypeP8x8 || info.kind == mbTypeP8x8Ref0 {
		if err := d.decodeP8x8(info, maxRef); err != nil {
			return err
		}
	} else {
		parts := partitionsOf(info.kind)
		refs := make([]int8, len(parts))
		for i := range parts {
			r, err := d.readRefIdx(maxRef)
			if err != nil {
				return err
			}
			refs[i] = r
		}
		for i, p := range parts {
			mvd, err := d.readMVD()
			if err != nil {
				return err
			}
			mvp := d.predictMV(p.x, p.y, p.w, p.h, refs[i], i, info.kind)
			mv := [2]int16{mvp[0] + mvd[0], mvp[1] + mvd[1]}
			d.storeMotion(p.x, p.y, p.w, p.h, mv, refs[i])
		}
	}

	if err := d.parseCBP(false); err != nil {
		return err
	}
	if d.cur.cbpLuma != 0 || d.cur.cbpChroma != 0 {
		if err := d.parseQPDelta(); err != nil {
			return err
		}
		if err := d.readResidual(res, false); err != nil {
			return err
		}
	}
	d.cur.qpY = d.qpY
	d.motionCompensate()
	d.addInterResidual(res)
	return nil
}

func (d *sliceDecoder) decodeP8x8(info mbTypeInfo, maxRef uint32) error {
	var sub [4]subMbInfo
	for i := 0; i < 4; i++ {
		v, err := d.r.ReadUE()
		if err != nil {
			return err
		}
		if v > 3 {
			return fmt.Errorf("%w: sub_mb_type %d", ErrCorrupt, v)
		}
		sub[i] = subMbTable[v]
	}
	var refs [4]int8
	for i := 0; i < 4; i++ {
		if info.kind == mbTypeP8x8Ref0 {
			refs[i] = 0
			continue
		}
		r, err := d.readRefIdx(maxRef)
		if err != nil {
			return err
		}
		refs[i] = r
	}
	for i := 0; i < 4; i++ {
		ox := i % 2 * 8
		oy := i / 2 * 8
		s := sub[i]
		cols := 8 / s.w
		for p := 0; p < s.numParts; p++ {
			px := ox + p%cols*s.w
			py := oy + p/cols*s.h
			mvd, err := d.readMVD()
			if err != nil {
				return err
			}
			mvp := d.predictMV(px, py, s.w, s.h, refs[i], p, mbTypeP8x8)
			mv := [2]int16{mvp[0] + mvd[0], mvp[1] + mvd[1]}
			d.storeMotion(px, py, s.w, s.h, mv, refs[i])
		}
	}
	return nil
}

func (d *sliceDecoder) decodePSkip() error {
	d.cur.kind = mbTypePSkip
	d.cur.intra = false
	d.cur.cbpLuma = 0
	d.cur.cbpChroma = 0
	d.cur.qpY = d.qpY
	mv := d.skipMV()
	d.storeMotion(0, 0, 16, 16, mv, 0)
	d.motionCompensate()
	return nil
}

func (d *sliceDecoder) refPicture(idx int8) *frame.Picture {
	if int(idx) >= len(d.refList) || idx < 0 {
		if len(d.refList) == 0 {
			return nil
		}
		return d.refList[0]
	}
	return d.refList[idx]
}

func clampComponent(v, lo, hi, fracBits int) int {
	frac := v & (1<<uint(fracBits) - 1)
	iv := v >> uint(fracBits)
	if iv < lo {
		iv = lo
	}
	if iv > hi {
		iv = hi
	}
	return iv<<uint(fracBits) | frac
}

const (
	lumaTapBefore  = 2
	lumaTapAfter   = 3
	chromaTapAfter = 1
)

func (d *sliceDecoder) motionCompensate() {
	baseX, baseY := d.mbx*16, d.mby*16
	for _, seg := range motionSegments(d.cur) {
		ref := d.refPicture(seg.ref)
		if ref == nil {
			continue
		}
		x := baseX + seg.x
		y := baseY + seg.y

		mvx := clampComponent(int(seg.mv[0]),
			lumaTapBefore-frame.LumaMargin-x,
			ref.Width+frame.LumaMargin-lumaTapAfter-x-seg.w, 2)
		mvy := clampComponent(int(seg.mv[1]),
			lumaTapBefore-frame.LumaMargin-y,
			ref.Height+frame.LumaMargin-lumaTapAfter-y-seg.h, 2)
		mc.PredictLuma(d.pic.Y, d.pic.StrideY, d.pic.LumaOffset(x, y),
			ref.Y, ref.StrideY, ref.LumaOffset(x, y),
			seg.w, seg.h, mvx, mvy)

		cx, cy := x/2, y/2
		cw, ch := seg.w/2, seg.h/2
		cmvx := clampComponent(int(seg.mv[0]),
			-frame.ChromaMargin-cx,
			ref.Width/2+frame.ChromaMargin-chromaTapAfter-cx-cw, 3)
		cmvy := clampComponent(int(seg.mv[1]),
			-frame.ChromaMargin-cy,
			ref.Height/2+frame.ChromaMargin-chromaTapAfter-cy-ch, 3)
		mc.PredictChroma(d.pic.Cb, d.pic.StrideC, d.pic.ChromaOffset(cx, cy),
			ref.Cb, ref.StrideC, ref.ChromaOffset(cx, cy), cw, ch, cmvx, cmvy)
		mc.PredictChroma(d.pic.Cr, d.pic.StrideC, d.pic.ChromaOffset(cx, cy),
			ref.Cr, ref.StrideC, ref.ChromaOffset(cx, cy), cw, ch, cmvx, cmvy)
	}
}

type motionSegment struct {
	x, y, w, h int
	mv         [2]int16
	ref        int8
}

func motionSegments(m *mbState) []motionSegment {
	segs := make([]motionSegment, 0, 16)
	var used [16]bool
	for by := 0; by < 16; by += 4 {
		for bx := 0; bx < 16; bx += 4 {
			z := zscanOf[by>>2][bx>>2]
			if used[z] {
				continue
			}
			mv := m.mvL0[z]
			ref := m.refIdxL0[z]
			w := 4
			for bx+w < 16 {
				nz := zscanOf[by>>2][(bx+w)>>2]
				if used[nz] || m.mvL0[nz] != mv || m.refIdxL0[nz] != ref {
					break
				}
				w += 4
			}
			h := 4
			for by+h < 16 {
				same := true
				for i := 0; i < w; i += 4 {
					nz := zscanOf[(by+h)>>2][(bx+i)>>2]
					if used[nz] || m.mvL0[nz] != mv || m.refIdxL0[nz] != ref {
						same = false
						break
					}
				}
				if !same {
					break
				}
				h += 4
			}
			for yy := by; yy < by+h; yy += 4 {
				for xx := bx; xx < bx+w; xx += 4 {
					used[zscanOf[yy>>2][xx>>2]] = true
				}
			}
			segs = append(segs, motionSegment{x: bx, y: by, w: w, h: h, mv: mv, ref: ref})
		}
	}
	return segs
}

func (d *sliceDecoder) addInterResidual(res *mbResidual) {
	baseX, baseY := d.mbx*16, d.mby*16
	for blk := 0; blk < 16; blk++ {
		if d.cur.nzY[blk] == 0 {
			continue
		}
		transform.Dequant4x4(&res.luma[blk], d.qpY, false)
		transform.Inverse4x4(&res.luma[blk])
		transform.AddResidual4x4(d.pic.Y, d.pic.StrideY,
			d.pic.LumaOffset(baseX+blockX[blk], baseY+blockY[blk]), &res.luma[blk])
	}
	d.addChromaResidual(res)
}
