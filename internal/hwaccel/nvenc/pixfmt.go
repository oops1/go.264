package nvenc

func I420Size(w, h int) int {
	if w <= 0 || h <= 0 || w%2 != 0 || h%2 != 0 {
		return 0
	}
	return w*h + 2*(w/2)*(h/2)
}

func NV12Size(stride, h int) int {
	if stride <= 0 || h <= 0 || h%2 != 0 {
		return 0
	}
	return stride*h + stride*(h/2)
}

func I420ToNV12(dst []byte, dstStride int, src []byte, w, h int) {
	if I420Size(w, h) == 0 || dstStride < w {
		return
	}
	if len(src) < I420Size(w, h) || len(dst) < NV12Size(dstStride, h) {
		return
	}
	for y := 0; y < h; y++ {
		copy(dst[y*dstStride:y*dstStride+w], src[y*w:(y+1)*w])
	}
	cw, ch := w/2, h/2
	cb := src[w*h:]
	cr := src[w*h+cw*ch:]
	base := dstStride * h
	for y := 0; y < ch; y++ {
		row := dst[base+y*dstStride:]
		for x := 0; x < cw; x++ {
			row[2*x] = cb[y*cw+x]
			row[2*x+1] = cr[y*cw+x]
		}
	}
}

func NV12ToI420(dst []byte, src []byte, srcStride, w, h int) {
	if I420Size(w, h) == 0 || srcStride < w {
		return
	}
	if len(dst) < I420Size(w, h) || len(src) < NV12Size(srcStride, h) {
		return
	}
	for y := 0; y < h; y++ {
		copy(dst[y*w:(y+1)*w], src[y*srcStride:y*srcStride+w])
	}
	cw, ch := w/2, h/2
	cb := dst[w*h:]
	cr := dst[w*h+cw*ch:]
	base := srcStride * h
	for y := 0; y < ch; y++ {
		row := src[base+y*srcStride:]
		for x := 0; x < cw; x++ {
			cb[y*cw+x] = row[2*x]
			cr[y*cw+x] = row[2*x+1]
		}
	}
}
