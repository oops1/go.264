package encoder

import (
	"github.com/oops1/go.264/internal/mc"
	"github.com/oops1/go.264/internal/syntax"
	"github.com/oops1/go.264/internal/transform"
)

var zscanOf = [4][4]int{
	{0, 1, 4, 5},
	{2, 3, 6, 7},
	{8, 9, 12, 13},
	{10, 11, 14, 15},
}

type motion struct {
	mv  [2]int16
	ref int8
}

func (s *mbEncoder) neighbourMotion(x, y, curZ int) (motion, bool) {
	unavailable := motion{ref: -1}
	switch {
	case x < 0 && y < 0:
		if s.nb.topLeft == nil {
			return unavailable, false
		}
		return motionOf(s.nb.topLeft, blkIdxAt(12, 12)), true
	case x < 0:
		if y >= 16 || s.nb.left == nil {
			return unavailable, false
		}
		return motionOf(s.nb.left, blkIdxAt(12, y&^3)), true
	case y < 0:
		if x < 16 {
			if s.nb.top == nil {
				return unavailable, false
			}
			return motionOf(s.nb.top, blkIdxAt(x&^3, 12)), true
		}
		if s.nb.topRight == nil {
			return unavailable, false
		}
		return motionOf(s.nb.topRight, blkIdxAt(0, 12)), true
	case x >= 16 || y >= 16:
		return unavailable, false
	}
	z := zscanOf[y>>2][x>>2]
	if z >= curZ {
		return unavailable, false
	}
	return motionOf(s.cur, z), true
}

func motionOf(m *mbInfo, blk int) motion {
	return motion{mv: m.MvL0[blk], ref: m.refIdx[blk]}
}

func median(a, b, c int16) int16 {
	if a > b {
		a, b = b, a
	}
	if b > c {
		b = c
	}
	if a > b {
		b = a
	}
	return b
}

func (s *mbEncoder) predictMV(x, y, w int, partIdx, kind int, refIdx int8) [2]int16 {
	curZ := zscanOf[y>>2][x>>2]
	a, okA := s.neighbourMotion(x-1, y, curZ)
	b, okB := s.neighbourMotion(x, y-1, curZ)
	c, okC := s.neighbourMotion(x+w, y-1, curZ)
	if !okC {
		c, okC = s.neighbourMotion(x-1, y-1, curZ)
	}
	if !okB && !okC && okA {
		b, c = a, a
	}
	switch {
	case kind == mbTypeP16x8 && partIdx == 0 && b.ref == refIdx:
		return b.mv
	case kind == mbTypeP16x8 && partIdx == 1 && a.ref == refIdx:
		return a.mv
	case kind == mbTypeP8x16 && partIdx == 0 && a.ref == refIdx:
		return a.mv
	case kind == mbTypeP8x16 && partIdx == 1 && c.ref == refIdx:
		return c.mv
	}
	matches := 0
	var only motion
	for _, n := range [3]motion{a, b, c} {
		if n.ref == refIdx {
			matches++
			only = n
		}
	}
	if matches == 1 {
		return only.mv
	}
	return [2]int16{
		median(a.mv[0], b.mv[0], c.mv[0]),
		median(a.mv[1], b.mv[1], c.mv[1]),
	}
}

func (s *mbEncoder) predictMV16x16() [2]int16 {
	return s.predictMV(0, 0, 16, 0, mbTypeP16x16, 0)
}

func (s *mbEncoder) skipMV() [2]int16 {
	if s.nb.left == nil || s.nb.top == nil {
		return [2]int16{}
	}
	a, _ := s.neighbourMotion(-1, 0, 0)
	b, _ := s.neighbourMotion(0, -1, 0)
	if a.ref == 0 && a.mv == [2]int16{} {
		return [2]int16{}
	}
	if b.ref == 0 && b.mv == [2]int16{} {
		return [2]int16{}
	}
	return s.predictMV16x16()
}

const (
	lumaTapBefore = 2
	lumaTapAfter  = 3
	searchRange   = 24
)

func mvBitCost(mv, mvp [2]int16, lambda int) int {
	return lambda * (bitsForSE(int(mv[0])-int(mvp[0])) + bitsForSE(int(mv[1])-int(mvp[1])))
}

func bitsForSE(v int) int {
	code := uint32(0)
	if v > 0 {
		code = uint32(v)*2 - 1
	} else {
		code = uint32(-v) * 2
	}
	n := 1
	for c := code + 1; c > 1; c >>= 1 {
		n += 2
	}
	return n
}

func (s *mbEncoder) motionCompensateMB(mv [2]int16) {
	ref := s.refPicture(0)
	x, y := s.mbx*16, s.mby*16
	mc.PredictLuma(s.e.rec.Y, s.e.rec.StrideY, s.e.rec.LumaOffset(x, y),
		ref.Y, ref.StrideY, ref.LumaOffset(x, y), 16, 16, int(mv[0]), int(mv[1]))
	cx, cy := x/2, y/2
	mc.PredictChroma(s.e.rec.Cb, s.e.rec.StrideC, s.e.rec.ChromaOffset(cx, cy),
		ref.Cb, ref.StrideC, ref.ChromaOffset(cx, cy), 8, 8, int(mv[0]), int(mv[1]))
	mc.PredictChroma(s.e.rec.Cr, s.e.rec.StrideC, s.e.rec.ChromaOffset(cx, cy),
		ref.Cr, ref.StrideC, ref.ChromaOffset(cx, cy), 8, 8, int(mv[0]), int(mv[1]))
}

func (s *mbEncoder) quantiseInterLuma() bool {
	var block transform.Block
	any := false
	for blk := 0; blk < 16; blk++ {
		off := s.lumaOffset(blk)
		srcOff := s.e.src.LumaOffset(s.mbx*16+blockX[blk], s.mby*16+blockY[blk])
		transform.Residual4x4(&block, s.e.src.Y, s.e.src.StrideY, srcOff, s.e.rec.Y, s.e.rec.StrideY, off)
		transform.Forward4x4(&block)
		transform.Quant4x4(&block, s.qpY, false)
		var scan [16]int32
		transform.BlockToScan(&scan, &block)
		s.lumaScan[blk] = scan
		n := countNonZero(scan[:])
		s.cur.NzY[blk] = uint8(n)
		if n != 0 {
			any = true
		}
	}
	s.cur.cbpLuma = 0
	for i8 := 0; i8 < 4; i8++ {
		for i4 := 0; i4 < 4; i4++ {
			if s.cur.NzY[i8*4+i4] != 0 {
				s.cur.cbpLuma |= 1 << uint(i8)
				break
			}
		}
	}
	return any
}

func (s *mbEncoder) reconstructInterLuma() {
	for blk := 0; blk < 16; blk++ {
		if s.cur.NzY[blk] == 0 {
			continue
		}
		var b transform.Block
		transform.ScanToBlock(&b, &s.lumaScan[blk])
		transform.Dequant4x4(&b, s.qpY, false)
		transform.Inverse4x4(&b)
		transform.AddResidual4x4(s.e.rec.Y, s.e.rec.StrideY, s.lumaOffset(blk), &b)
	}
}

func (s *mbEncoder) quantiseInterChroma() {
	offsets := [2]int32{s.e.pps.ChromaQPIndexOffset, s.e.pps.SecondChromaQPIndexOffset}
	planes := [2][]byte{s.e.rec.Cb, s.e.rec.Cr}
	srcPlanes := [2][]byte{s.e.src.Cb, s.e.src.Cr}
	anyDC, anyAC := false, false
	for plane := 0; plane < 2; plane++ {
		qpc := syntax.ChromaQP(s.qpY, int(offsets[plane]))
		var dc transform.ChromaDC
		var blocks [4]transform.Block
		for blk := 0; blk < 4; blk++ {
			off := s.chromaOffset(blk)
			srcOff := s.e.src.ChromaOffset(s.mbx*8+chromaBlockX[blk], s.mby*8+chromaBlockY[blk])
			transform.Residual4x4(&blocks[blk], srcPlanes[plane], s.e.src.StrideC, srcOff,
				planes[plane], s.e.rec.StrideC, off)
			transform.Forward4x4(&blocks[blk])
			dc[blk] = blocks[blk][0]
		}
		transform.QuantChromaDC(&dc, qpc, false)
		s.chromaDC[plane] = dc
		for i := 0; i < 4; i++ {
			if dc[i] != 0 {
				anyDC = true
			}
		}
		for blk := 0; blk < 4; blk++ {
			blocks[blk][0] = 0
			transform.Quant4x4(&blocks[blk], qpc, false)
			blocks[blk][0] = 0
			var scan [16]int32
			transform.BlockToScan(&scan, &blocks[blk])
			s.chromaScan[plane][blk] = scan
			n := countNonZero(scan[1:])
			if plane == 0 {
				s.cur.nzCb[blk] = uint8(n)
			} else {
				s.cur.nzCr[blk] = uint8(n)
			}
			if n != 0 {
				anyAC = true
			}
		}
	}
	switch {
	case anyAC:
		s.cur.cbpChroma = 2
	case anyDC:
		s.cur.cbpChroma = 1
	default:
		s.cur.cbpChroma = 0
	}
	if s.cur.cbpChroma < 2 {
		for plane := 0; plane < 2; plane++ {
			for blk := 0; blk < 4; blk++ {
				for i := 1; i < 16; i++ {
					s.chromaScan[plane][blk][i] = 0
				}
			}
		}
		s.cur.nzCb = [4]uint8{}
		s.cur.nzCr = [4]uint8{}
	}
}

func (s *mbEncoder) reconstructInterChroma() {
	offsets := [2]int32{s.e.pps.ChromaQPIndexOffset, s.e.pps.SecondChromaQPIndexOffset}
	planes := [2][]byte{s.e.rec.Cb, s.e.rec.Cr}
	for plane := 0; plane < 2; plane++ {
		qpc := syntax.ChromaQP(s.qpY, int(offsets[plane]))
		dc := s.chromaDC[plane]
		if s.cur.cbpChroma == 0 {
			dc = transform.ChromaDC{}
		}
		transform.DequantChromaDC(&dc, qpc)
		for blk := 0; blk < 4; blk++ {
			var b transform.Block
			transform.ScanToBlock(&b, &s.chromaScan[plane][blk])
			b[0] = dc[blk]
			transform.Dequant4x4(&b, qpc, true)
			transform.Inverse4x4(&b)
			transform.AddResidual4x4(planes[plane], s.e.rec.StrideC, s.chromaOffset(blk), &b)
		}
	}
}

func (s *mbEncoder) setMotion(mv [2]int16) {
	pic := s.refPicture(0)
	for i := 0; i < 16; i++ {
		s.cur.MvL0[i] = mv
		s.cur.refIdx[i] = 0
		s.cur.RefPicL0[i] = pic
	}
}

func (s *mbEncoder) encodeInterMB() (bool, error) {
	lambda := lambdaTable[s.qpY]

	skipMV := s.skipMV()
	s.motionCompensateMB(skipMV)
	skipHasLuma := s.quantiseInterLuma()
	s.quantiseInterChroma()
	if !skipHasLuma && s.cur.cbpChroma == 0 {
		s.cur.kind = mbTypePSkip
		s.cur.Intra = false
		s.cur.cbpLuma = 0
		s.setMotion(skipMV)
		s.reconstructInterChroma()
		return true, nil
	}

	bestKind := mbTypeP16x16
	interCost, bestParts := s.trySplit(mbTypeP16x16, lambda)
	var bestSubs []subResult
	for _, kind := range []int{mbTypeP16x8, mbTypeP8x16} {
		cost, parts := s.trySplit(kind, lambda)
		if cost+lambda*20 < interCost {
			interCost, bestParts, bestKind = cost, parts, kind
		}
	}
	if cost, subs := s.trySplit8x8(lambda); cost+lambda*20 < interCost {
		interCost, bestSubs, bestKind = cost, subs, mbTypeP8x8
	}
	intraCost, intraMode := s.searchIntra16x16()
	if intraCost+lambda*16 < interCost {
		s.reset()
		if err := s.encodeIntraMBInP(intraMode); err != nil {
			return false, err
		}
		return false, nil
	}

	s.reset()
	s.cur.kind = bestKind
	s.cur.Intra = false
	s.parts = bestParts
	s.subs = bestSubs
	if bestKind == mbTypeP8x8 {
		s.applySubMotion(bestSubs)
		s.compensateSubMBs(bestSubs)
	} else {
		s.clearMotion()
		for i, p := range partitionsFor(bestKind) {
			s.storePartitionMotion(p, bestParts[i].mv, bestParts[i].ref)
		}
		s.compensatePartitions(bestKind, bestParts)
	}
	s.quantiseInterLuma()
	s.quantiseInterChroma()
	s.reconstructInterLuma()
	s.reconstructInterChroma()

	s.w.WriteUE(s.pendingSkipRun)
	s.pendingSkipRun = 0
	if err := s.writeInterMB(); err != nil {
		return false, err
	}
	return false, nil
}

func (s *mbEncoder) encodeIntraMBInP(mode16 int) error {
	s.w.WriteUE(s.pendingSkipRun)
	s.pendingSkipRun = 0
	s.clearMotion()
	_ = mode16
	return s.encodeIntraMB(5)
}

func (s *mbEncoder) writeInterMB() error {
	switch s.cur.kind {
	case mbTypeP16x8:
		s.w.WriteUE(1)
	case mbTypeP8x16:
		s.w.WriteUE(2)
	case mbTypeP8x8:
		s.w.WriteUE(3)
	default:
		s.w.WriteUE(0)
	}
	if s.cur.kind == mbTypeP8x8 {
		s.writeSubMBs()
	} else {
		for _, r := range s.parts {
			s.writeRefIdx(r.ref)
		}
		for _, r := range s.parts {
			s.w.WriteSE(int32(r.mvd[0]))
			s.w.WriteSE(int32(r.mvd[1]))
		}
	}
	cbp := uint8(s.cur.cbpLuma | s.cur.cbpChroma<<4)
	s.w.WriteUE(interCBPToGolomb[cbp])
	if cbp != 0 {
		s.w.WriteSE(0)
		if err := s.writeResidual(false); err != nil {
			return err
		}
	}
	return s.w.Err()
}

func (s *mbEncoder) writeRefIdx(ref int8) {
	if s.numRefs <= 1 {
		return
	}
	s.w.WriteTE(uint32(ref), uint32(s.numRefs-1))
}
