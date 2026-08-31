package encoder

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
	ref     int8
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
			s.cur.RefPicL0[i] = s.refPicture(s.cur.refIdx[i])
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

	for refIdx := 0; refIdx < s.numRefs; refIdx++ {
		refCost := 0
		if s.numRefs > 1 {
			refCost = lambda * bitsForTE(uint32(refIdx), uint32(s.numRefs-1))
		}
		for subType, shape := range subMbShapes {
			s.restoreMotion(entry)
			parts := subPartitionsOf(shape, ox, oy)
			results := make([]partResult, len(parts))
			cost := lambda*bitsForUE(uint32(subType)) + refCost
			for i, p := range parts {
				r := s.searchPartitionRef(p, i, mbTypeP8x8, lambda, int8(refIdx))
				s.storePartitionMotion(p, r.mv, r.ref)
				results[i] = r
				cost += r.cost
			}
			if best.subType < 0 || cost < bestCost {
				bestCost = cost
				best = subResult{subType: subType, ref: int8(refIdx), parts: results}
			}
		}
	}

	s.restoreMotion(entry)
	for i, p := range subPartitionsOf(subMbShapes[best.subType], ox, oy) {
		s.storePartitionMotion(p, best.parts[i].mv, best.ref)
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
			s.storePartitionMotion(p, sub.parts[j].mv, sub.ref)
		}
	}
}

func (s *mbEncoder) compensateSubMBs(subs []subResult) {
	for i, sub := range subs {
		ox, oy := i%2*8, i/2*8
		for j, p := range subPartitionsOf(subMbShapes[sub.subType], ox, oy) {
			s.compensatePartition(p, sub.parts[j].mv, sub.ref)
		}
	}
}

func (s *mbEncoder) writeSubMBs() {
	for _, sub := range s.subs {
		s.w.WriteUE(uint32(sub.subType))
	}
	for _, sub := range s.subs {
		s.writeRefIdx(sub.ref)
	}
	for _, sub := range s.subs {
		for _, r := range sub.parts {
			s.w.WriteSE(int32(r.mvd[0]))
			s.w.WriteSE(int32(r.mvd[1]))
		}
	}
}
