//go:build amd64 && !purego

package simd

func sadDispatch(src []byte, srcStride int, ref []byte, refStride, w, h int) int {
	if !spanFits(src, srcStride, w, h) || !spanFits(ref, refStride, w, h) {
		return sadGeneric(src, srcStride, ref, refStride, w, h)
	}
	switch {
	case w == 16 && h == 16:
		return sad16x16(src, srcStride, ref, refStride)
	case w == 16 && h == 8:
		return sad16x8(src, srcStride, ref, refStride)
	case w == 8 && h == 16:
		return sad8x16(src, srcStride, ref, refStride)
	case w == 8 && h == 8:
		return sad8x8(src, srcStride, ref, refStride)
	case w == 8 && h == 4:
		return sad8x4(src, srcStride, ref, refStride)
	}
	return sadGeneric(src, srcStride, ref, refStride, w, h)
}

func Accelerated() bool { return true }
