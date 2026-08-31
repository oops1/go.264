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

func Forward4x4(b *[16]int32) {
	forward4x4(b)
}

func Inverse4x4(b *[16]int32) {
	inverse4x4(b)
}

func Quant4x4(b *[16]int32, mf *[16]int32, f int32, qbits uint32) {
	quant4x4(b, mf, f, uint64(qbits))
}

func Dequant4x4(b *[16]int32, scale *[16]int32, shift uint32) {
	if shift >= 4 {
		dequantLeft4x4(b, scale, uint64(shift-4))
		return
	}
	s := uint64(4 - shift)
	round := int32(1) << (s - 1)
	dequantRight4x4(b, scale, s, round)
}

func AddResidual4x4(plane []byte, stride, offset int, b *[16]int32) {
	if !addResidualFits(plane, stride, offset) {
		addResidual4x4Generic(plane, stride, offset, b)
		return
	}
	addResidual4x4(plane[offset:], stride, b)
}
