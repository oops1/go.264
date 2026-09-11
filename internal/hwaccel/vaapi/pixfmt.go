package vaapi

func I420Size(w, h int) int {
	if w <= 0 || h <= 0 || w%2 != 0 || h%2 != 0 {
		return 0
	}
	return w*h + 2*(w/2)*(h/2)
}

func NV12PlaneSize(strideY, strideC, h int) int {
	if strideY <= 0 || strideC <= 0 || h <= 0 || h%2 != 0 {
		return 0
	}
	return strideY*h + strideC*(h/2)
}

func I420ToNV12(dstY []byte, dstStrideY int, dstUV []byte, dstStrideUV int, src []byte, w, h int) {
	if I420Size(w, h) == 0 || dstStrideY < w || dstStrideUV < w {
		return
	}
	if len(src) < I420Size(w, h) {
		return
	}
	if len(dstY) < dstStrideY*h {
		return
	}
	cw, ch := w/2, h/2
	if len(dstUV) < dstStrideUV*ch {
		return
	}
	for y := 0; y < h; y++ {
		copy(dstY[y*dstStrideY:y*dstStrideY+w], src[y*w:(y+1)*w])
	}
	cb := src[w*h:]
	cr := src[w*h+cw*ch:]
	for y := 0; y < ch; y++ {
		row := dstUV[y*dstStrideUV:]
		for x := 0; x < cw; x++ {
			row[2*x] = cb[y*cw+x]
			row[2*x+1] = cr[y*cw+x]
		}
	}
}
