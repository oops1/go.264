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
	case w == 4 && h == 8:
		return sad4x8(src, srcStride, ref, refStride)
	case w == 4 && h == 4:
		return sad4x4(src, srcStride, ref, refStride)
	}
	return sadGeneric(src, srcStride, ref, refStride, w, h)
}

func Accelerated() bool { return hasSSE41 }

func Forward4x4(b *[16]int32) {
	forward4x4(b)
}

func Inverse4x4(b *[16]int32) {
	inverse4x4(b)
}

func Quant4x4(b *[16]int32, mf *[16]int32, f int32, qbits uint32) {
	if !hasSSE41 {
		quant4x4Generic(b, mf, f, qbits)
		return
	}
	quant4x4(b, mf, f, uint64(qbits))
}

func Dequant4x4(b *[16]int32, scale *[16]int32, shift uint32) {
	if !hasSSE41 {
		dequant4x4Generic(b, scale, shift)
		return
	}
	if shift >= 4 {
		dequantLeft4x4(b, scale, uint64(shift-4))
		return
	}
	s := uint64(4 - shift)
	round := int32(1) << (s - 1)
	dequantRight4x4(b, scale, s, round)
}

func AddResidual4x4(plane []byte, stride, offset int, b *[16]int32) {
	if !hasSSE41 || !addResidualFits(plane, stride, offset) {
		addResidual4x4Generic(plane, stride, offset, b)
		return
	}
	addResidual4x4(plane[offset:], stride, b)
}

func satd4x4Dispatch(src []byte, srcStride int, ref []byte, refStride int) int {
	if !hasSSE41 || !spanFits(src, srcStride, 4, 4) || !spanFits(ref, refStride, 4, 4) {
		return satd4x4Generic(src, srcStride, ref, refStride)
	}
	return satd4x4(src, srcStride, ref, refStride)
}

type sixTapFn func(dst []byte, dstStride int, src []byte, srcStride int)

func sixTapHorizKernel(w, h int) sixTapFn {
	if !hasSSE41 {
		return nil
	}
	if hasAVX2 {
		switch {
		case w == 16 && h == 16:
			return sixTapHoriz16x16AVX2
		case w == 16 && h == 8:
			return sixTapHoriz16x8AVX2
		}
	}
	switch {
	case w == 16 && h == 16:
		return sixTapHoriz16x16
	case w == 16 && h == 8:
		return sixTapHoriz16x8
	case w == 8 && h == 16:
		return sixTapHoriz8x16
	case w == 8 && h == 8:
		return sixTapHoriz8x8
	case w == 8 && h == 4:
		return sixTapHoriz8x4
	case w == 4 && h == 8:
		return sixTapHoriz4x8
	case w == 4 && h == 4:
		return sixTapHoriz4x4
	}
	return nil
}

func sixTapVertKernel(w, h int) sixTapFn {
	if !hasSSE41 {
		return nil
	}
	if hasAVX2 {
		switch {
		case w == 16 && h == 16:
			return sixTapVert16x16AVX2
		case w == 16 && h == 8:
			return sixTapVert16x8AVX2
		}
	}
	switch {
	case w == 16 && h == 16:
		return sixTapVert16x16
	case w == 16 && h == 8:
		return sixTapVert16x8
	case w == 8 && h == 16:
		return sixTapVert8x16
	case w == 8 && h == 8:
		return sixTapVert8x8
	case w == 8 && h == 4:
		return sixTapVert8x4
	case w == 4 && h == 8:
		return sixTapVert4x8
	case w == 4 && h == 4:
		return sixTapVert4x4
	}
	return nil
}

func sixTapHVKernel(w, h int) sixTapFn {
	if !hasSSE41 {
		return nil
	}
	if hasAVX2 {
		switch {
		case w == 16 && h == 16:
			return sixTapHV16x16AVX2
		case w == 16 && h == 8:
			return sixTapHV16x8AVX2
		}
	}
	switch {
	case w == 16 && h == 16:
		return sixTapHV16x16
	case w == 16 && h == 8:
		return sixTapHV16x8
	case w == 8 && h == 16:
		return sixTapHV8x16
	case w == 8 && h == 8:
		return sixTapHV8x8
	case w == 8 && h == 4:
		return sixTapHV8x4
	case w == 4 && h == 8:
		return sixTapHV4x8
	case w == 4 && h == 4:
		return sixTapHV4x4
	}
	return nil
}

func sixTapHorizDispatch(dst []byte, dstStride, dstOff int, src []byte, srcStride, srcOff, w, h int) {
	if spanFitsMargin(src, srcStride, srcOff, w, h, 2, 3, 0, 0) && spanFits(dst[dstOff:], dstStride, w, h) {
		if fn := sixTapHorizKernel(w, h); fn != nil {
			fn(dst[dstOff:], dstStride, src[srcOff:], srcStride)
			return
		}
	}
	sixTapHorizGeneric(dst, dstStride, dstOff, src, srcStride, srcOff, w, h)
}

func sixTapVertDispatch(dst []byte, dstStride, dstOff int, src []byte, srcStride, srcOff, w, h int) {
	if spanFitsMargin(src, srcStride, srcOff, w, h, 0, 0, 2, 3) && spanFits(dst[dstOff:], dstStride, w, h) {
		if fn := sixTapVertKernel(w, h); fn != nil {
			fn(dst[dstOff:], dstStride, src[srcOff:], srcStride)
			return
		}
	}
	sixTapVertGeneric(dst, dstStride, dstOff, src, srcStride, srcOff, w, h)
}

func sixTapHVDispatch(dst []byte, dstStride, dstOff int, src []byte, srcStride, srcOff, w, h int) {
	if spanFitsMargin(src, srcStride, srcOff, w, h, 2, 3, 2, 3) && spanFits(dst[dstOff:], dstStride, w, h) {
		if fn := sixTapHVKernel(w, h); fn != nil {
			fn(dst[dstOff:], dstStride, src[srcOff:], srcStride)
			return
		}
	}
	sixTapHVGeneric(dst, dstStride, dstOff, src, srcStride, srcOff, w, h)
}

type bilinearFn func(dst []byte, dstStride int, src []byte, srcStride int, xFrac, yFrac int32)

func bilinearChromaKernel(w, h int) bilinearFn {
	if !hasSSE41 {
		return nil
	}
	switch {
	case w == 8 && h == 8:
		return bilinearChroma8x8
	case w == 8 && h == 4:
		return bilinearChroma8x4
	case w == 8 && h == 2:
		return bilinearChroma8x2
	case w == 4 && h == 8:
		return bilinearChroma4x8
	case w == 4 && h == 4:
		return bilinearChroma4x4
	case w == 4 && h == 2:
		return bilinearChroma4x2
	}
	return nil
}

func bilinearChromaDispatch(dst []byte, dstStride, dstOff int, src []byte, srcStride, srcOff, w, h, xFrac, yFrac int) {
	if spanFitsMargin(src, srcStride, srcOff, w, h, 0, 1, 0, 1) && spanFits(dst[dstOff:], dstStride, w, h) {
		if fn := bilinearChromaKernel(w, h); fn != nil {
			fn(dst[dstOff:], dstStride, src[srcOff:], srcStride, int32(xFrac), int32(yFrac))
			return
		}
	}
	bilinearChromaGeneric(dst, dstStride, dstOff, src, srcStride, srcOff, w, h, xFrac, yFrac)
}
