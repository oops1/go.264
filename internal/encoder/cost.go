package encoder

import "math"

var lambdaTable [52]int

func init() {
	for qp := 0; qp < 52; qp++ {
		v := int(math.Round(math.Pow(2, float64(qp-12)/6)))
		if v < 1 {
			v = 1
		}
		lambdaTable[qp] = v
	}
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func sad(src []byte, srcStride, srcOff int, ref []byte, refStride, refOff, w, h int) int {
	total := 0
	for y := 0; y < h; y++ {
		a := src[srcOff+y*srcStride:]
		b := ref[refOff+y*refStride:]
		for x := 0; x < w; x++ {
			total += abs(int(a[x]) - int(b[x]))
		}
	}
	return total
}

func satd4x4(src []byte, srcStride, srcOff int, ref []byte, refStride, refOff int) int {
	var d [16]int
	for y := 0; y < 4; y++ {
		a := src[srcOff+y*srcStride:]
		b := ref[refOff+y*refStride:]
		for x := 0; x < 4; x++ {
			d[y*4+x] = int(a[x]) - int(b[x])
		}
	}
	for i := 0; i < 16; i += 4 {
		s0, s1, s2, s3 := d[i], d[i+1], d[i+2], d[i+3]
		t0 := s0 + s1
		t1 := s0 - s1
		t2 := s2 + s3
		t3 := s2 - s3
		d[i] = t0 + t2
		d[i+1] = t1 + t3
		d[i+2] = t0 - t2
		d[i+3] = t1 - t3
	}
	total := 0
	for i := 0; i < 4; i++ {
		s0, s1, s2, s3 := d[i], d[i+4], d[i+8], d[i+12]
		t0 := s0 + s1
		t1 := s0 - s1
		t2 := s2 + s3
		t3 := s2 - s3
		total += abs(t0+t2) + abs(t1+t3) + abs(t0-t2) + abs(t1-t3)
	}
	return (total + 1) >> 1
}

func satdBlock(src []byte, srcStride, srcOff int, ref []byte, refStride, refOff, w, h int) int {
	total := 0
	for y := 0; y < h; y += 4 {
		for x := 0; x < w; x += 4 {
			total += satd4x4(src, srcStride, srcOff+y*srcStride+x, ref, refStride, refOff+y*refStride+x)
		}
	}
	return total
}
