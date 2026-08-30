package simd

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func sadGeneric(src []byte, srcStride int, ref []byte, refStride, w, h int) int {
	total := 0
	for y := 0; y < h; y++ {
		a := src[y*srcStride:]
		b := ref[y*refStride:]
		for x := 0; x < w; x++ {
			total += absInt(int(a[x]) - int(b[x]))
		}
	}
	return total
}

func SAD(src []byte, srcStride, srcOff int, ref []byte, refStride, refOff, w, h int) int {
	return sadDispatch(src[srcOff:], srcStride, ref[refOff:], refStride, w, h)
}

func SADGeneric(src []byte, srcStride, srcOff int, ref []byte, refStride, refOff, w, h int) int {
	return sadGeneric(src[srcOff:], srcStride, ref[refOff:], refStride, w, h)
}

func spanFits(b []byte, stride, w, h int) bool {
	if h == 0 {
		return true
	}
	return len(b) >= (h-1)*stride+w
}
