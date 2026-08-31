package encoder

import "github.com/oops1/go.264/internal/mc"

type subMbShape struct {
	numParts int
	w        int
	h        int
}

var subMbShapes = [4]subMbShape{
	{1, 8, 8},
	{2, 8, 4},
	{2, 4, 8},
	{4, 4, 4},
}

type subResult struct {
	subType int
	parts   []partResult
}

type motionSnapshot struct {
	mv  [16][2]int16
	ref [16]int8
}

func (s *mbEncoder) snapshotMotion() motionSnapshot {
	return motionSnapshot{mv: s.cur.MvL0, ref: s.cur.refIdx}
}

func (s *mbEncoder) restoreMotion(m motionSnapshot) {
	s.cur.MvL0 = m.mv
	s.cur.refIdx = m.ref
	for i := 0; i < 16; i++ {
		if s.cur.refIdx[i] < 0 {
			s.cur.RefPicL0[i] = nil
		} else {
			s.cur.RefPicL0[i] = s.e.ref
		}
	}
}

func bitsForUE(v uint32) int {
	n := 1
	for c := uint64(v) + 1; c > 1; c >>= 1 {
		n += 2
	}
	return n
}

func subPartitionsOf(shape subMbShape, ox, oy int) []partition {
	cols := 8 / shape.w
	out := make([]partition, shape.numParts)
	for p := 0; p < shape.numParts; p++ {
		out[p] = partition{
			x: ox + p%cols*shape.w,
			y: oy + p/cols*shape.h,
			w: shape.w,
			h: shape.h,
		}
	}
	return out
}

func (s *mbEncoder) searchSubMB(ox, oy, lambda int) subResult {
	best := subResult{subType: -1}
	bestCost := 0
	entry := s.snapshotMotion()

	for subType, shape := range subMbShapes {
		s.restoreMotion(entry)
		parts := subPartitionsOf(shape, ox, oy)
		results := make([]partResult, len(parts))
		cost := lambda * bitsForUE(uint32(subType))
		for i, p := range parts {
			r := s.searchPartition(p, i, mbTypeP8x8, lambda)
			s.storePartitionMotion(p, r.mv)
			results[i] = r
			cost += r.cost
		}
		if best.subType < 0 || cost < bestCost {
			bestCost = cost
			best = subResult{subType: subType, parts: results}
		}
	}

	s.restoreMotion(entry)
	for i, p := range subPartitionsOf(subMbShapes[best.subType], ox, oy) {
		s.storePartitionMotion(p, best.parts[i].mv)
	}
	return best
}

func (s *mbEncoder) trySplit8x8(lambda int) (int, []subResult) {
	s.clearMotion()
	subs := make([]subResult, 4)
	total := lambda * bitsForUE(3)
	for i := 0; i < 4; i++ {
		ox, oy := i%2*8, i/2*8
		subs[i] = s.searchSubMB(ox, oy, lambda)
		for _, r := range subs[i].parts {
			total += r.cost
		}
		total += lambda * bitsForUE(uint32(subs[i].subType))
	}
	return total, subs
}

func (s *mbEncoder) applySubMotion(subs []subResult) {
	s.clearMotion()
	for i, sub := range subs {
		ox, oy := i%2*8, i/2*8
		for j, p := range subPartitionsOf(subMbShapes[sub.subType], ox, oy) {
			s.storePartitionMotion(p, sub.parts[j].mv)
		}
	}
}

func (s *mbEncoder) compensateSubMBs(subs []subResult) {
	ref := s.e.ref
	for i, sub := range subs {
		ox, oy := i%2*8, i/2*8
		for j, p := range subPartitionsOf(subMbShapes[sub.subType], ox, oy) {
			mv := sub.parts[j].mv
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
	}
}

func (s *mbEncoder) writeSubMBs() {
	for _, sub := range s.subs {
		s.w.WriteUE(uint32(sub.subType))
	}
	for _, sub := range s.subs {
		for _, r := range sub.parts {
			s.w.WriteSE(int32(r.mvd[0]))
			s.w.WriteSE(int32(r.mvd[1]))
		}
	}
}
