//go:build !amd64 || purego

package simd

func sadDispatch(src []byte, srcStride int, ref []byte, refStride, w, h int) int {
	return sadGeneric(src, srcStride, ref, refStride, w, h)
}

func Accelerated() bool { return false }
