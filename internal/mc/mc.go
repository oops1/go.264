package mc

func clip1(v int) int {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return v
}

func hFilter1(src []byte, off int) int {
	return int(src[off-2]) -
		5*int(src[off-1]) +
		20*int(src[off]) +
		20*int(src[off+1]) -
		5*int(src[off+2]) +
		int(src[off+3])
}

func vFilter1(src []byte, stride, off int) int {
	return int(src[off-2*stride]) -
		5*int(src[off-stride]) +
		20*int(src[off]) +
		20*int(src[off+stride]) -
		5*int(src[off+2*stride]) +
		int(src[off+3*stride])
}

func halfB(src []byte, off int) int {
	return clip1((hFilter1(src, off) + 16) >> 5)
}

func halfH(src []byte, stride, off int) int {
	return clip1((vFilter1(src, stride, off) + 16) >> 5)
}

func halfJ(src []byte, stride, off int) int {
	cc := hFilter1(src, off-2*stride)
	dd := hFilter1(src, off-stride)
	h1 := hFilter1(src, off)
	m1 := hFilter1(src, off+stride)
	ee := hFilter1(src, off+2*stride)
	ff := hFilter1(src, off+3*stride)
	j1 := cc - 5*dd + 20*h1 + 20*m1 - 5*ee + ff
	return clip1((j1 + 512) >> 10)
}

func lumaVal(src []byte, stride, off, xFrac, yFrac int) int {
	switch {
	case xFrac == 0 && yFrac == 0:
		return int(src[off])
	case xFrac == 2 && yFrac == 0:
		return halfB(src, off)
	case xFrac == 0 && yFrac == 2:
		return halfH(src, stride, off)
	case xFrac == 2 && yFrac == 2:
		return halfJ(src, stride, off)
	case yFrac == 0:
		b := halfB(src, off)
		if xFrac == 1 {
			return (int(src[off]) + b + 1) >> 1
		}
		return (int(src[off+1]) + b + 1) >> 1
	case xFrac == 0:
		h := halfH(src, stride, off)
		if yFrac == 1 {
			return (int(src[off]) + h + 1) >> 1
		}
		return (int(src[off+stride]) + h + 1) >> 1
	case xFrac == 2:
		j := halfJ(src, stride, off)
		b := halfB(src, off)
		if yFrac == 1 {
			return (b + j + 1) >> 1
		}
		s := halfB(src, off+stride)
		return (j + s + 1) >> 1
	case yFrac == 2:
		j := halfJ(src, stride, off)
		h := halfH(src, stride, off)
		if xFrac == 1 {
			return (h + j + 1) >> 1
		}
		m := halfH(src, stride, off+1)
		return (j + m + 1) >> 1
	default:
		b := halfB(src, off)
		h := halfH(src, stride, off)
		m := halfH(src, stride, off+1)
		s := halfB(src, off+stride)
		switch {
		case xFrac == 1 && yFrac == 1:
			return (b + h + 1) >> 1
		case xFrac == 3 && yFrac == 1:
			return (b + m + 1) >> 1
		case xFrac == 1 && yFrac == 3:
			return (h + s + 1) >> 1
		default:
			return (m + s + 1) >> 1
		}
	}
}

func PredictLuma(dst []byte, dstStride, dstOff int, src []byte, srcStride, srcOff int, w, h, mvx, mvy int) {
	srcOff += (mvy>>2)*srcStride + (mvx >> 2)
	xFrac := mvx & 3
	yFrac := mvy & 3
	for y := 0; y < h; y++ {
		so := srcOff + y*srcStride
		do := dstOff + y*dstStride
		for x := 0; x < w; x++ {
			dst[do+x] = byte(lumaVal(src, srcStride, so+x, xFrac, yFrac))
		}
	}
}

func PredictChroma(dst []byte, dstStride, dstOff int, src []byte, srcStride, srcOff int, w, h, mvx, mvy int) {
	srcOff += (mvy>>3)*srcStride + (mvx >> 3)
	xFrac := mvx & 7
	yFrac := mvy & 7
	for y := 0; y < h; y++ {
		so := srcOff + y*srcStride
		do := dstOff + y*dstStride
		for x := 0; x < w; x++ {
			p := so + x
			a := int(src[p])
			b := int(src[p+1])
			c := int(src[p+srcStride])
			d := int(src[p+srcStride+1])
			val := ((8-xFrac)*(8-yFrac)*a + xFrac*(8-yFrac)*b + (8-xFrac)*yFrac*c + xFrac*yFrac*d + 32) >> 6
			dst[do+x] = byte(val)
		}
	}
}

func Average(dst []byte, dstStride, dstOff int, a []byte, aStride, aOff int, b []byte, bStride, bOff int, w, h int) {
	for y := 0; y < h; y++ {
		da := dstOff + y*dstStride
		aa := aOff + y*aStride
		bb := bOff + y*bStride
		for x := 0; x < w; x++ {
			dst[da+x] = byte((int(a[aa+x]) + int(b[bb+x]) + 1) >> 1)
		}
	}
}
