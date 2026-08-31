package encoder

import (
	"github.com/oops1/go.264/internal/frame"
	"github.com/oops1/go.264/internal/mc"
)

type partition struct {
	x, y, w, h int
}

func partitionsFor(kind int) []partition {
	switch kind {
	case mbTypeP16x16:
		return []partition{{0, 0, 16, 16}}
	case mbTypeP16x8:
		return []partition{{0, 0, 16, 8}, {0, 8, 16, 8}}
	case mbTypeP8x16:
		return []partition{{0, 0, 8, 16}, {8, 0, 8, 16}}
	}
	return nil
}

type partResult struct {
	mv   [2]int16
	mvd  [2]int16
	ref  int8
	cost int
}

func (s *mbEncoder) refPicture(idx int8) *frame.Picture {
	if int(idx) < 0 || int(idx) >= len(s.e.refs) {
		if len(s.e.refs) == 0 {
			return nil
		}
		return s.e.refs[0]
	}
	return s.e.refs[idx]
}

func (s *mbEncoder) partLimits(px, py, w, h int) (loX, hiX, loY, hiY int) {
	x, y := s.mbx*16+px, s.mby*16+py
	loX = lumaTapBefore - frame.LumaMargin - x
	hiX = s.e.width() + frame.LumaMargin - lumaTapAfter - x - w
	loY = lumaTapBefore - frame.LumaMargin - y
	hiY = s.e.height() + frame.LumaMargin - lumaTapAfter - y - h
	return
}

func (s *mbEncoder) partSAD(ref *frame.Picture, px, py, w, h, ix, iy int) int {
	x, y := s.mbx*16+px, s.mby*16+py
	return sad(s.e.src.Y, s.e.src.StrideY, s.e.src.LumaOffset(x, y),
		ref.Y, ref.StrideY, ref.LumaOffset(x+ix, y+iy), w, h)
}

func (s *mbEncoder) partSubPelCost(ref *frame.Picture, px, py, w, h int, mv, mvp [2]int16, lambda int) int {
	x, y := s.mbx*16+px, s.mby*16+py
	mc.PredictLuma(s.scratch[:], 16, 0, ref.Y, ref.StrideY,
		ref.LumaOffset(x, y), w, h, int(mv[0]), int(mv[1]))
	c := satdBlock(s.e.src.Y, s.e.src.StrideY, s.e.src.LumaOffset(x, y),
		s.scratch[:], 16, 0, w, h)
	return c + mvBitCost(mv, mvp, lambda)
}

func (s *mbEncoder) searchPartition(p partition, partIdx, kind, lambda int) partResult {
	best := partResult{ref: -1}
	for idx := 0; idx < s.numRefs; idx++ {
		r := s.searchPartitionRef(p, partIdx, kind, lambda, int8(idx))
		if s.numRefs > 1 {
			r.cost += lambda * bitsForTE(uint32(idx), uint32(s.numRefs-1))
		}
		if best.ref < 0 || r.cost < best.cost {
			best = r
		}
	}
	return best
}

func bitsForTE(v, max uint32) int {
	if max == 0 {
		return 0
	}
	if max == 1 {
		return 1
	}
	return bitsForUE(v)
}

func (s *mbEncoder) searchPartitionRef(p partition, partIdx, kind, lambda int, refIdx int8) partResult {
	ref := s.refPicture(refIdx)
	mvp := s.predictMV(p.x, p.y, p.w, partIdx, kind, refIdx)
	loX, hiX, loY, hiY := s.partLimits(p.x, p.y, p.w, p.h)
	clampX := func(v int) int {
		if v < loX {
			return loX
		}
		if v > hiX {
			return hiX
		}
		return v
	}
	clampY := func(v int) int {
		if v < loY {
			return loY
		}
		if v > hiY {
			return hiY
		}
		return v
	}

	bestX, bestY := clampX(int(mvp[0])>>2), clampY(int(mvp[1])>>2)
	cost := func(ix, iy int) int {
		return s.partSAD(ref, p.x, p.y, p.w, p.h, ix, iy) +
			mvBitCost([2]int16{int16(ix << 2), int16(iy << 2)}, mvp, lambda)
	}
	best := cost(bestX, bestY)
	if c := cost(0, 0); c < best {
		best, bestX, bestY = c, 0, 0
	}

	for step := 8; step >= 1; step >>= 1 {
		for improved := true; improved; {
			improved = false
			for _, d := range [4][2]int{{0, -1}, {0, 1}, {-1, 0}, {1, 0}} {
				nx := clampX(bestX + d[0]*step)
				ny := clampY(bestY + d[1]*step)
				if nx == bestX && ny == bestY {
					continue
				}
				if absInt(nx) > searchRange || absInt(ny) > searchRange {
					continue
				}
				if c := cost(nx, ny); c < best {
					best, bestX, bestY = c, nx, ny
					improved = true
				}
			}
		}
	}

	mv := [2]int16{int16(bestX << 2), int16(bestY << 2)}
	bestCost := s.partSubPelCost(ref, p.x, p.y, p.w, p.h, mv, mvp, lambda)
	inRange := func(m [2]int16) bool {
		return int(m[0])>>2 >= loX && int(m[0])>>2 <= hiX &&
			int(m[1])>>2 >= loY && int(m[1])>>2 <= hiY
	}
	for _, step := range []int16{2, 1} {
		for improved := true; improved; {
			improved = false
			for _, d := range [8][2]int16{{0, -1}, {0, 1}, {-1, 0}, {1, 0}, {-1, -1}, {1, -1}, {-1, 1}, {1, 1}} {
				cand := [2]int16{mv[0] + d[0]*step, mv[1] + d[1]*step}
				if !inRange(cand) {
					continue
				}
				if c := s.partSubPelCost(ref, p.x, p.y, p.w, p.h, cand, mvp, lambda); c < bestCost {
					bestCost, mv = c, cand
					improved = true
				}
			}
		}
	}
	return partResult{
		mv:   mv,
		mvd:  [2]int16{mv[0] - mvp[0], mv[1] - mvp[1]},
		ref:  refIdx,
		cost: bestCost,
	}
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func (s *mbEncoder) storePartitionMotion(p partition, mv [2]int16, ref int8) {
	pic := s.refPicture(ref)
	for by := p.y; by < p.y+p.h; by += 4 {
		for bx := p.x; bx < p.x+p.w; bx += 4 {
			z := zscanOf[by>>2][bx>>2]
			s.cur.MvL0[z] = mv
			s.cur.refIdx[z] = ref
			s.cur.RefPicL0[z] = pic
		}
	}
}

func (s *mbEncoder) clearMotion() {
	for i := 0; i < 16; i++ {
		s.cur.MvL0[i] = [2]int16{}
		s.cur.refIdx[i] = -1
		s.cur.RefPicL0[i] = nil
	}
}

func (s *mbEncoder) trySplit(kind, lambda int) (int, []partResult) {
	parts := partitionsFor(kind)
	results := make([]partResult, len(parts))
	s.clearMotion()
	total := 0
	for i, p := range parts {
		r := s.searchPartition(p, i, kind, lambda)
		s.storePartitionMotion(p, r.mv, r.ref)
		results[i] = r
		total += r.cost
	}
	return total, results
}

func (s *mbEncoder) compensatePartition(p partition, mv [2]int16, refIdx int8) {
	ref := s.refPicture(refIdx)
	x, y := s.mbx*16+p.x, s.mby*16+p.y
	mc.PredictLuma(s.e.rec.Y, s.e.rec.StrideY, s.e.rec.LumaOffset(x, y),
		ref.Y, ref.StrideY, ref.LumaOffset(x, y), p.w, p.h, int(mv[0]), int(mv[1]))
	cx, cy := x/2, y/2
	cw, ch := p.w/2, p.h/2
	mc.PredictChroma(s.e.rec.Cb, s.e.rec.StrideC, s.e.rec.ChromaOffset(cx, cy),
		ref.Cb, ref.StrideC, ref.ChromaOffset(cx, cy), cw, ch, int(mv[0]), int(mv[1]))
	mc.PredictChroma(s.e.rec.Cr, s.e.rec.StrideC, s.e.rec.ChromaOffset(cx, cy),
		ref.Cr, ref.StrideC, ref.ChromaOffset(cx, cy), cw, ch, int(mv[0]), int(mv[1]))
}

func (s *mbEncoder) compensatePartitions(kind int, results []partResult) {
	for i, p := range partitionsFor(kind) {
		s.compensatePartition(p, results[i].mv, results[i].ref)
	}
}
