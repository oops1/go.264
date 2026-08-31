package decoder

import "github.com/oops1/go.264/internal/loopfilter"

type motion struct {
	mv  [2]int16
	ref int8
}

var zscanOf = [4][4]int{
	{0, 1, 4, 5},
	{2, 3, 6, 7},
	{8, 9, 12, 13},
	{10, 11, 14, 15},
}

func blockMotion(m *mbState, blk int) motion {
	return motion{mv: m.MvL0[blk], ref: m.refIdxL0[blk]}
}

func (d *sliceDecoder) neighbourBlock(x, y, curZ int) (*mbState, int) {
	switch {
	case x < 0 && y < 0:
		return d.nb.topLeft, loopfilter.BlkIdxAt(12, 12)
	case x < 0:
		if y >= 16 {
			return nil, 0
		}
		return d.nb.left, loopfilter.BlkIdxAt(12, y&^3)
	case y < 0:
		if x < 16 {
			return d.nb.top, loopfilter.BlkIdxAt(x&^3, 12)
		}
		return d.nb.topRight, loopfilter.BlkIdxAt(0, 12)
	case x >= 16 || y >= 16:
		return nil, 0
	}
	z := zscanOf[y>>2][x>>2]
	if z >= curZ {
		return nil, 0
	}
	return d.cur, z
}

func (d *sliceDecoder) neighbourMotion(x, y, curZ int) (motion, bool) {
	m, blk := d.neighbourBlock(x, y, curZ)
	if m == nil {
		return motion{ref: -1}, false
	}
	return blockMotion(m, blk), true
}

func (d *sliceDecoder) neighbourMVD(x, y, curZ int) [2]uint8 {
	m, blk := d.neighbourBlock(x, y, curZ)
	if m == nil || m.Intra {
		return [2]uint8{}
	}
	return m.mvdL0[blk]
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

func (d *sliceDecoder) predictMV(x, y, w, h int, refIdx int8, partIdx int, kind int) [2]int16 {
	curZ := zscanOf[y>>2][x>>2]
	a, okA := d.neighbourMotion(x-1, y, curZ)
	b, okB := d.neighbourMotion(x, y-1, curZ)
	c, okC := d.neighbourMotion(x+w, y-1, curZ)
	if !okC {
		c, okC = d.neighbourMotion(x-1, y-1, curZ)
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
	if a.ref == refIdx {
		matches++
		only = a
	}
	if b.ref == refIdx {
		matches++
		only = b
	}
	if c.ref == refIdx {
		matches++
		only = c
	}
	if matches == 1 {
		return only.mv
	}
	return [2]int16{
		median(a.mv[0], b.mv[0], c.mv[0]),
		median(a.mv[1], b.mv[1], c.mv[1]),
	}
}

func (d *sliceDecoder) skipMV() [2]int16 {
	if d.nb.left == nil || d.nb.top == nil {
		return [2]int16{}
	}
	a, _ := d.neighbourMotion(-1, 0, 0)
	b, _ := d.neighbourMotion(0, -1, 0)
	if a.ref == 0 && a.mv == [2]int16{} {
		return [2]int16{}
	}
	if b.ref == 0 && b.mv == [2]int16{} {
		return [2]int16{}
	}
	return d.predictMV(0, 0, 16, 16, 0, 0, mbTypeP16x16)
}

func absClip70(v int16) uint8 {
	if v < 0 {
		v = -v
	}
	if v > 70 {
		return 70
	}
	return uint8(v)
}

func (d *sliceDecoder) storeMVD(x, y, w, h int, mvd [2]int16) {
	packed := [2]uint8{absClip70(mvd[0]), absClip70(mvd[1])}
	for by := y; by < y+h; by += 4 {
		for bx := x; bx < x+w; bx += 4 {
			d.cur.mvdL0[zscanOf[by>>2][bx>>2]] = packed
		}
	}
}

func (d *sliceDecoder) storeRefIdx(x, y, w, h int, ref int8) {
	for by := y; by < y+h; by += 4 {
		for bx := x; bx < x+w; bx += 4 {
			d.cur.refIdxL0[zscanOf[by>>2][bx>>2]] = ref
		}
	}
}

func (d *sliceDecoder) storeMotion(x, y, w, h int, mv [2]int16, ref int8) {
	pic := d.refPicture(ref)
	for by := y; by < y+h; by += 4 {
		for bx := x; bx < x+w; bx += 4 {
			z := zscanOf[by>>2][bx>>2]
			d.cur.MvL0[z] = mv
			d.cur.refIdxL0[z] = ref
			d.cur.RefPicL0[z] = pic
		}
	}
}
