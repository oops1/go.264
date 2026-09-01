package encoder

var bSubTypeOf [4][4]int

func init() {
	for i := range bSubTypeOf {
		for j := range bSubTypeOf[i] {
			bSubTypeOf[i][j] = -1
		}
	}
	for t, info := range bSubTypes {
		if info.direct {
			continue
		}
		for k, shape := range subMbShapes {
			if shape.numParts == info.numParts && shape.w == info.w && shape.h == info.h {
				bSubTypeOf[k][info.pred] = t
			}
		}
	}
}

type bSubResult struct {
	subType int
	pred    uint8
	parts   []bPartResult
	cost    int
}

type bMotionSnapshot struct {
	mv  [2][16][2]int16
	ref [2][16]int8
}

func (s *mbEncoder) snapshotBMotion() bMotionSnapshot {
	var m bMotionSnapshot
	m.mv[0] = s.cur.MvL0
	m.mv[1] = s.cur.MvL1
	m.ref[0] = s.cur.refIdx
	m.ref[1] = s.cur.refIdxL1
	return m
}

func (s *mbEncoder) restoreBMotion(m bMotionSnapshot, x, y, w, h int) {
	for by := y; by < y+h; by += 4 {
		for bx := x; bx < x+w; bx += 4 {
			z := zscanOf[by>>2][bx>>2]
			for list := 0; list < 2; list++ {
				s.storeMotionIn(list, bx, by, 4, 4, m.mv[list][z], m.ref[list][z])
			}
		}
	}
}

func (s *mbEncoder) directSubCost(idx, lambda int) int {
	ox, oy := idx%2*8, idx/2*8
	x, y := s.mbx*16+ox, s.mby*16+oy
	c := satdBlock(s.e.src.Y, s.e.src.StrideY, s.e.src.LumaOffset(x, y),
		s.e.rec.Y, s.e.rec.StrideY, s.e.rec.LumaOffset(x, y), 8, 8)
	return c + lambda*bitsForUE(0)
}

func (s *mbEncoder) searchSubShape(list, shape, idx, lambda int) []partResult {
	ox, oy := idx%2*8, idx/2*8
	parts := subPartitionsOf(subMbShapes[shape], ox, oy)
	out := make([]partResult, len(parts))
	for i, p := range parts {
		r := s.searchPartitionRef(list, p, i, mbTypeB8x8, lambda, 0)
		s.storeMotionIn(list, p.x, p.y, p.w, p.h, r.mv, 0)
		out[i] = r
	}
	return out
}

func (s *mbEncoder) searchBSubMB(idx, lambda int, allowDirect bool, direct bMotionSnapshot) bSubResult {
	best := bSubResult{subType: -1}
	if allowDirect {
		best = bSubResult{subType: 0, cost: s.directSubCost(idx, lambda)}
	}
	ox, oy := idx%2*8, idx/2*8

	for shape := range subMbShapes {
		var uni [2][]partResult
		for list := 0; list < 2; list++ {
			uni[list] = s.searchSubShape(list, shape, idx, lambda)
		}
		parts := subPartitionsOf(subMbShapes[shape], ox, oy)
		for _, pred := range [3]uint8{predL0, predL1, predBi} {
			t := bSubTypeOf[shape][pred]
			if t < 0 {
				continue
			}
			cost := lambda * bitsForUE(uint32(t))
			results := make([]bPartResult, len(parts))
			for i, p := range parts {
				r := bPartResult{pred: pred, ref: [2]int8{-1, -1}}
				switch pred {
				case predL0:
					r.mv[0], r.mvd[0], r.ref[0] = uni[0][i].mv, uni[0][i].mvd, 0
					cost += uni[0][i].cost
				case predL1:
					r.mv[1], r.mvd[1], r.ref[1] = uni[1][i].mv, uni[1][i].mvd, 0
					cost += uni[1][i].cost
				default:
					r.mv = [2][2]int16{uni[0][i].mv, uni[1][i].mv}
					r.mvd = [2][2]int16{uni[0][i].mvd, uni[1][i].mvd}
					r.ref = [2]int8{0, 0}
					cost += s.biSATD(p, r.mv)
					for list := 0; list < 2; list++ {
						cost += lambda * (bitsForSE(int(r.mvd[list][0])) + bitsForSE(int(r.mvd[list][1])))
					}
				}
				results[i] = r
			}
			if best.subType < 0 || cost < best.cost {
				best = bSubResult{subType: t, pred: pred, parts: results, cost: cost}
			}
		}
	}

	if best.subType == 0 {
		s.restoreBMotion(direct, ox, oy, 8, 8)
		return best
	}
	for list := 0; list < 2; list++ {
		if best.pred&(1<<uint(list)) == 0 {
			s.storeMotionIn(list, ox, oy, 8, 8, [2]int16{}, -1)
			continue
		}
		for i, p := range subPartitionsOf(subMbShapes[subShapeOf(best.subType)], ox, oy) {
			s.storeMotionIn(list, p.x, p.y, p.w, p.h, best.parts[i].mv[list], 0)
			s.storeMVDIn(list, p.x, p.y, p.w, p.h, best.parts[i].mvd[list])
		}
	}
	return best
}

func subShapeOf(subType int) int {
	info := bSubTypes[subType]
	for k, shape := range subMbShapes {
		if shape.numParts == info.numParts && shape.w == info.w && shape.h == info.h {
			return k
		}
	}
	return 0
}

func (s *mbEncoder) runB8x8Pass(lambda int, allowDirect bool) []bSubResult {
	s.clearMotion()
	var direct bMotionSnapshot
	if allowDirect {
		s.spatialDirect(0, 0, 16, 16)
		direct = s.snapshotBMotion()
		s.compensateB()
	}
	subs := make([]bSubResult, 4)
	for i := 0; i < 4; i++ {
		subs[i] = s.searchBSubMB(i, lambda, allowDirect, direct)
	}
	return subs
}

func anyDirectSub(subs []bSubResult) bool {
	for _, sub := range subs {
		if sub.subType == 0 {
			return true
		}
	}
	return false
}

func (s *mbEncoder) searchB8x8(lambda int) bCandidate {
	subs := s.runB8x8Pass(lambda, true)
	if !anyDirectSub(subs) {
		subs = s.runB8x8Pass(lambda, false)
	}
	return bCandidate{typeIdx: 22, kind: mbTypeB8x8, subs: subs}
}

func (s *mbEncoder) applyB8x8(c bCandidate) {
	s.clearMotion()
	if anyDirectSub(c.subs) {
		s.spatialDirect(0, 0, 16, 16)
		for i, sub := range c.subs {
			if sub.subType == 0 {
				s.markDirect(i%2*8, i/2*8, 8, 8)
			}
		}
	}
	for list := 0; list < 2; list++ {
		for i, sub := range c.subs {
			if sub.subType == 0 {
				continue
			}
			ox, oy := i%2*8, i/2*8
			if sub.pred&(1<<uint(list)) == 0 {
				s.storeRefIdxIn(list, ox, oy, 8, 8, -1)
				continue
			}
			s.storeRefIdxIn(list, ox, oy, 8, 8, sub.parts[0].ref[list])
		}
	}
	for list := 0; list < 2; list++ {
		for i, sub := range c.subs {
			ox, oy := i%2*8, i/2*8
			if sub.subType == 0 {
				continue
			}
			if sub.pred&(1<<uint(list)) == 0 {
				s.storeMotionIn(list, ox, oy, 8, 8, [2]int16{}, -1)
				continue
			}
			for j, p := range subPartitionsOf(subMbShapes[subShapeOf(sub.subType)], ox, oy) {
				s.storeMVDIn(list, p.x, p.y, p.w, p.h, sub.parts[j].mvd[list])
				s.storeMotionIn(list, p.x, p.y, p.w, p.h, sub.parts[j].mv[list], sub.parts[j].ref[list])
			}
		}
	}
}

func (s *mbEncoder) writeB8x8() {
	for _, sub := range s.bsubs {
		s.w.WriteUE(uint32(sub.subType))
	}
	for list := 0; list < 2; list++ {
		for _, sub := range s.bsubs {
			if sub.subType == 0 || sub.pred&(1<<uint(list)) == 0 {
				continue
			}
			s.writeRefIdxIn(list, sub.parts[0].ref[list])
		}
	}
	for list := 0; list < 2; list++ {
		for i, sub := range s.bsubs {
			if sub.subType == 0 || sub.pred&(1<<uint(list)) == 0 {
				continue
			}
			for j := range subPartitionsOf(subMbShapes[subShapeOf(sub.subType)], i%2*8, i/2*8) {
				s.w.WriteSE(int32(sub.parts[j].mvd[list][0]))
				s.w.WriteSE(int32(sub.parts[j].mvd[list][1]))
			}
		}
	}
}

func (s *mbEncoder) writeB8x8CABAC() {
	for _, sub := range s.bsubs {
		s.cb.SubMBTypeB(sub.subType)
	}
	for list := 0; list < 2; list++ {
		for i, sub := range s.bsubs {
			if sub.subType == 0 || sub.pred&(1<<uint(list)) == 0 {
				continue
			}
			s.writeRefIdxCABACIn(list, i%2*8, i/2*8, sub.parts[0].ref[list])
		}
	}
	for list := 0; list < 2; list++ {
		for i, sub := range s.bsubs {
			if sub.subType == 0 || sub.pred&(1<<uint(list)) == 0 {
				continue
			}
			for j, p := range subPartitionsOf(subMbShapes[subShapeOf(sub.subType)], i%2*8, i/2*8) {
				s.writeMVDCABACIn(list, p.x, p.y, sub.parts[j].mvd[list])
			}
		}
	}
}
