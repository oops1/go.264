package encoder

import "image"

type RegionKind uint8

const (
	RegionUnknown RegionKind = iota
	RegionFill
	RegionText
	RegionImage
)

type MotionSearch uint8

const (
	MotionSearchFull MotionSearch = iota
	MotionSearchZero
)

type Region struct {
	Rect image.Rectangle
	Kind RegionKind
}

type Hints struct {
	Changed []image.Rectangle
	Regions []Region
}

var regionQPOffset = [4]int{
	RegionUnknown: 0,
	RegionFill:    -2,
	RegionText:    -6,
	RegionImage:   +2,
}

func (k RegionKind) qpOffset() int {
	if int(k) >= len(regionQPOffset) {
		return 0
	}
	return regionQPOffset[k]
}

type frameHints struct {
	changedMB []bool
	qpOffset  []int8
	widthMBs  int
	heightMBs int
}

func (e *Encoder) prepareHints(h Hints) *frameHints {
	if h.Changed == nil && len(h.Regions) == 0 {
		return nil
	}
	f := &frameHints{widthMBs: e.widthMBs, heightMBs: e.heightMBs}

	if h.Changed != nil {
		f.changedMB = make([]bool, e.widthMBs*e.heightMBs)
		for _, r := range h.Changed {
			markRect(f.changedMB, e.widthMBs, e.heightMBs, r)
		}
	}
	if len(h.Regions) != 0 {
		f.qpOffset = make([]int8, e.widthMBs*e.heightMBs)
		for _, region := range h.Regions {
			offset := int8(region.Kind.qpOffset())
			applyRegion(f.qpOffset, e.widthMBs, e.heightMBs, region.Rect, offset)
		}
	}
	return f
}

func clampMBRange(lo, hi, limit int) (int, int) {
	if lo < 0 {
		lo = 0
	}
	if hi > limit {
		hi = limit
	}
	return lo, hi
}

func rectToMacroblocks(widthMBs, heightMBs int, r image.Rectangle) (x0, y0, x1, y1 int, ok bool) {
	if r.Empty() {
		return 0, 0, 0, 0, false
	}
	x0, x1 = clampMBRange(r.Min.X/16, (r.Max.X+15)/16, widthMBs)
	y0, y1 = clampMBRange(r.Min.Y/16, (r.Max.Y+15)/16, heightMBs)
	if x0 >= x1 || y0 >= y1 {
		return 0, 0, 0, 0, false
	}
	return x0, y0, x1, y1, true
}

func markRect(dst []bool, widthMBs, heightMBs int, r image.Rectangle) {
	x0, y0, x1, y1, ok := rectToMacroblocks(widthMBs, heightMBs, r)
	if !ok {
		return
	}
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			dst[y*widthMBs+x] = true
		}
	}
}

func applyRegion(dst []int8, widthMBs, heightMBs int, r image.Rectangle, offset int8) {
	x0, y0, x1, y1, ok := rectToMacroblocks(widthMBs, heightMBs, r)
	if !ok {
		return
	}
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			dst[y*widthMBs+x] = offset
		}
	}
}

func (f *frameHints) unchanged(mbx, mby int) bool {
	if f == nil || f.changedMB == nil {
		return false
	}
	return !f.changedMB[mby*f.widthMBs+mbx]
}

func (f *frameHints) qpDelta(mbx, mby int) int {
	if f == nil || f.qpOffset == nil {
		return 0
	}
	return int(f.qpOffset[mby*f.widthMBs+mbx])
}
