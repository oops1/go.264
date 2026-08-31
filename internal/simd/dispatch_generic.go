//go:build !amd64 || purego

package simd

func sadDispatch(src []byte, srcStride int, ref []byte, refStride, w, h int) int {
	return sadGeneric(src, srcStride, ref, refStride, w, h)
}

func Accelerated() bool { return false }

func Forward4x4(b *[16]int32) {
	forward4x4Generic(b)
}

func Inverse4x4(b *[16]int32) {
	inverse4x4Generic(b)
}

func Quant4x4(b *[16]int32, mf *[16]int32, f int32, qbits uint32) {
	quant4x4Generic(b, mf, f, qbits)
}

func Dequant4x4(b *[16]int32, scale *[16]int32, shift uint32) {
	dequant4x4Generic(b, scale, shift)
}

func AddResidual4x4(plane []byte, stride, offset int, b *[16]int32) {
	addResidual4x4Generic(plane, stride, offset, b)
}
