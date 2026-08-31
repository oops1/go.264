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

func addResidualFits(plane []byte, stride, offset int) bool {
	if offset < 0 {
		return false
	}
	return len(plane)-offset >= 3*stride+4
}

func forward4x4Generic(b *[16]int32) {
	for i := 0; i < 16; i += 4 {
		s0, s1, s2, s3 := b[i], b[i+1], b[i+2], b[i+3]
		t0 := s0 + s3
		t1 := s1 + s2
		t2 := s1 - s2
		t3 := s0 - s3
		b[i] = t0 + t1
		b[i+1] = t3*2 + t2
		b[i+2] = t0 - t1
		b[i+3] = t3 - t2*2
	}
	for i := 0; i < 4; i++ {
		s0, s1, s2, s3 := b[i], b[i+4], b[i+8], b[i+12]
		t0 := s0 + s3
		t1 := s1 + s2
		t2 := s1 - s2
		t3 := s0 - s3
		b[i] = t0 + t1
		b[i+4] = t3*2 + t2
		b[i+8] = t0 - t1
		b[i+12] = t3 - t2*2
	}
}

func inverse4x4Generic(b *[16]int32) {
	for i := 0; i < 16; i += 4 {
		s0, s1, s2, s3 := b[i], b[i+1], b[i+2], b[i+3]
		t0 := s0 + s2
		t1 := s0 - s2
		t2 := s1>>1 - s3
		t3 := s1 + s3>>1
		b[i] = t0 + t3
		b[i+1] = t1 + t2
		b[i+2] = t1 - t2
		b[i+3] = t0 - t3
	}
	for i := 0; i < 4; i++ {
		s0, s1, s2, s3 := b[i], b[i+4], b[i+8], b[i+12]
		t0 := s0 + s2
		t1 := s0 - s2
		t2 := s1>>1 - s3
		t3 := s1 + s3>>1
		b[i] = (t0 + t3 + 32) >> 6
		b[i+4] = (t1 + t2 + 32) >> 6
		b[i+8] = (t1 - t2 + 32) >> 6
		b[i+12] = (t0 - t3 + 32) >> 6
	}
}

func quant4x4Generic(b *[16]int32, mf *[16]int32, f int32, qbits uint32) {
	qb := uint(qbits)
	for i := range b {
		v := b[i]
		if v >= 0 {
			b[i] = (v*mf[i] + f) >> qb
		} else {
			b[i] = -((-v*mf[i] + f) >> qb)
		}
	}
}

func dequant4x4Generic(b *[16]int32, scale *[16]int32, shift uint32) {
	if shift >= 4 {
		s := uint(shift - 4)
		for i := range b {
			b[i] = b[i] * scale[i] << s
		}
		return
	}
	s := uint(4 - shift)
	round := int32(1) << (s - 1)
	for i := range b {
		b[i] = (b[i]*scale[i] + round) >> s
	}
}

func clip1(v int32) byte {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return byte(v)
}

func addResidual4x4Generic(plane []byte, stride, offset int, b *[16]int32) {
	for y := 0; y < 4; y++ {
		row := plane[offset+y*stride:]
		row[0] = clip1(int32(row[0]) + b[y*4])
		row[1] = clip1(int32(row[1]) + b[y*4+1])
		row[2] = clip1(int32(row[2]) + b[y*4+2])
		row[3] = clip1(int32(row[3]) + b[y*4+3])
	}
}
