package decoder

import (
	"github.com/oops1/go264/internal/deblock"
	"github.com/oops1/go264/internal/frame"
	"github.com/oops1/go264/internal/syntax"
)

func (m *mbState) filterQPY() int {
	if m.ipcm {
		return 0
	}
	return m.qpY
}

func lumaNonZero(m *mbState, blk int) bool {
	if m.ipcm {
		return true
	}
	return m.nzY[blk] != 0
}

func edgeStrength(p, q *mbState, pBlk, qBlk int, mbEdge bool) uint8 {
	if p.intra || q.intra {
		if mbEdge {
			return 4
		}
		return 3
	}
	if lumaNonZero(p, pBlk) || lumaNonZero(q, qBlk) {
		return 2
	}
	if p.refPicL0[pBlk] != q.refPicL0[qBlk] {
		return 1
	}
	dx := int(p.mvL0[pBlk][0]) - int(q.mvL0[qBlk][0])
	dy := int(p.mvL0[pBlk][1]) - int(q.mvL0[qBlk][1])
	if dx <= -4 || dx >= 4 || dy <= -4 || dy >= 4 {
		return 1
	}
	return 0
}

type edgeContext struct {
	grid *mbGrid
	pic  *frame.Picture
}

func (d *Decoder) filterPictureOn(pic *frame.Picture) {
	if d.grid == nil || pic == nil {
		return
	}
	ctx := edgeContext{grid: d.grid, pic: pic}
	for mby := 0; mby < d.grid.heightMBs; mby++ {
		for mbx := 0; mbx < d.grid.widthMBs; mbx++ {
			ctx.filterMB(mbx, mby)
		}
	}
}

func (c *edgeContext) neighbourFor(m *mbState, mbx, mby, dx, dy int) *mbState {
	n := c.grid.at(mbx+dx, mby+dy)
	if n == nil || !n.decoded {
		return nil
	}
	if m.disableDeblock == 2 && n.sliceID != m.sliceID {
		return nil
	}
	return n
}

func (c *edgeContext) filterMB(mbx, mby int) {
	cur := c.grid.at(mbx, mby)
	if !cur.decoded || cur.disableDeblock == 1 {
		return
	}
	left := c.neighbourFor(cur, mbx, mby, -1, 0)
	top := c.neighbourFor(cur, mbx, mby, 0, -1)

	for e := 0; e < 4; e++ {
		c.verticalEdge(cur, left, mbx, mby, e)
	}
	for e := 0; e < 4; e++ {
		c.horizontalEdge(cur, top, mbx, mby, e)
	}
}

func (c *edgeContext) verticalEdge(cur, left *mbState, mbx, mby, e int) {
	p := cur
	mbEdge := e == 0
	if mbEdge {
		if left == nil {
			return
		}
		p = left
	}
	x := e * 4
	var bs [4]uint8
	any := false
	for g := 0; g < 4; g++ {
		y := g * 4
		qBlk := blkIdxAt(x, y)
		var pBlk int
		if mbEdge {
			pBlk = blkIdxAt(12, y)
		} else {
			pBlk = blkIdxAt(x-4, y)
		}
		bs[g] = edgeStrength(p, cur, pBlk, qBlk, mbEdge)
		if bs[g] != 0 {
			any = true
		}
	}
	if !any {
		return
	}
	qpAv := deblock.AverageQP(p.filterQPY(), cur.filterQPY())
	ia := deblock.IndexA(qpAv, cur.alphaOffset)
	ib := deblock.IndexB(qpAv, cur.betaOffset)
	deblock.FilterLumaEdgeVertical(c.pic.Y, c.pic.StrideY,
		c.pic.LumaOffset(mbx*16+x, mby*16), bs, ia, ib)

	if e != 0 && e != 2 {
		return
	}
	cx := mbx*8 + e*2
	offsets := cur.chromaQPOffset
	planes := [2][]byte{c.pic.Cb, c.pic.Cr}
	for plane := 0; plane < 2; plane++ {
		qpc := deblock.AverageQP(
			syntax.ChromaQP(p.filterQPY(), offsets[plane]),
			syntax.ChromaQP(cur.filterQPY(), offsets[plane]))
		deblock.FilterChromaEdgeVertical(planes[plane], c.pic.StrideC,
			c.pic.ChromaOffset(cx, mby*8), bs,
			deblock.IndexA(qpc, cur.alphaOffset), deblock.IndexB(qpc, cur.betaOffset))
	}
}

func (c *edgeContext) horizontalEdge(cur, top *mbState, mbx, mby, e int) {
	p := cur
	mbEdge := e == 0
	if mbEdge {
		if top == nil {
			return
		}
		p = top
	}
	y := e * 4
	var bs [4]uint8
	any := false
	for g := 0; g < 4; g++ {
		x := g * 4
		qBlk := blkIdxAt(x, y)
		var pBlk int
		if mbEdge {
			pBlk = blkIdxAt(x, 12)
		} else {
			pBlk = blkIdxAt(x, y-4)
		}
		bs[g] = edgeStrength(p, cur, pBlk, qBlk, mbEdge)
		if bs[g] != 0 {
			any = true
		}
	}
	if !any {
		return
	}
	qpAv := deblock.AverageQP(p.filterQPY(), cur.filterQPY())
	deblock.FilterLumaEdgeHorizontal(c.pic.Y, c.pic.StrideY,
		c.pic.LumaOffset(mbx*16, mby*16+y), bs,
		deblock.IndexA(qpAv, cur.alphaOffset), deblock.IndexB(qpAv, cur.betaOffset))

	if e != 0 && e != 2 {
		return
	}
	cy := mby*8 + e*2
	offsets := cur.chromaQPOffset
	planes := [2][]byte{c.pic.Cb, c.pic.Cr}
	for plane := 0; plane < 2; plane++ {
		qpc := deblock.AverageQP(
			syntax.ChromaQP(p.filterQPY(), offsets[plane]),
			syntax.ChromaQP(cur.filterQPY(), offsets[plane]))
		deblock.FilterChromaEdgeHorizontal(planes[plane], c.pic.StrideC,
			c.pic.ChromaOffset(mbx*8, cy), bs,
			deblock.IndexA(qpc, cur.alphaOffset), deblock.IndexB(qpc, cur.betaOffset))
	}
}
