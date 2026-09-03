package main

import (
	"fmt"

	. "github.com/mmcloughlin/avo/build"
	. "github.com/mmcloughlin/avo/operand"
	"github.com/mmcloughlin/avo/reg"
)

func main() {
	ConstraintExpr("amd64,!purego")

	for _, size := range [][2]int{
		{16, 16}, {16, 8}, {8, 16}, {8, 8}, {8, 4},
	} {
		genSAD(size[0], size[1])
	}
	for _, size := range [][2]int{
		{4, 8}, {4, 4},
	} {
		genSAD4(size[0], size[1])
	}

	genForward4x4()
	genInverse4x4()
	genQuant4x4()
	genDequantLeft4x4()
	genDequantRight4x4()
	genAddResidual4x4()
	genSATD4x4()
	genSATD4x4AVX2()
	genSATD8x8()
	genSATD8x8AVX2()

	for _, size := range lumaMCSizes {
		genSixTapHoriz(size[0], size[1])
		genSixTapVert(size[0], size[1])
		genSixTapHV(size[0], size[1])
		if size[0] == 16 {
			genSixTapHorizAVX2(size[0], size[1])
			genSixTapVertAVX2(size[0], size[1])
			genSixTapHVAVX2(size[0], size[1])
		}
	}
	for _, size := range chromaMCSizes {
		genBilinearChroma(size[0], size[1])
	}

	Generate()
}

var lumaMCSizes = [][2]int{
	{16, 16}, {16, 8}, {8, 16}, {8, 8}, {8, 4}, {4, 8}, {4, 4},
}

var chromaMCSizes = [][2]int{
	{8, 8}, {8, 4}, {8, 2}, {4, 8}, {4, 4}, {4, 2},
}

func loadRow(width int, addr Mem) reg.VecVirtual {
	v := XMM()
	if width == 16 {
		MOVOU(addr, v)
	} else {
		MOVQ(addr, v)
	}
	return v
}

func genSAD(w, h int) {
	TEXT(fmt.Sprintf("sad%dx%d", w, h), NOSPLIT,
		"func(src []byte, srcStride int, ref []byte, refStride int) int")
	Pragma("noescape")
	Doc("")
	src := Load(Param("src").Base(), GP64())
	srcStride := Load(Param("srcStride"), GP64())
	ref := Load(Param("ref").Base(), GP64())
	refStride := Load(Param("refStride"), GP64())

	acc := XMM()
	PXOR(acc, acc)

	for i := 0; i < h; i++ {
		s := loadRow(w, Mem{Base: src})
		r := loadRow(w, Mem{Base: ref})
		PSADBW(r, s)
		PADDD(s, acc)
		if i != h-1 {
			ADDQ(srcStride, src)
			ADDQ(refStride, ref)
		}
	}

	if w == 16 {
		hi := XMM()
		MOVOU(acc, hi)
		PSRLDQ(Imm(8), hi)
		PADDD(hi, acc)
	}
	out := GP64()
	MOVQ(acc, out)
	Store(out, ReturnIndex(0))
	RET()
}

func genSAD4(w, h int) {
	TEXT(fmt.Sprintf("sad%dx%d", w, h), NOSPLIT,
		"func(src []byte, srcStride int, ref []byte, refStride int) int")
	Pragma("noescape")
	Doc("")
	src := Load(Param("src").Base(), GP64())
	srcStride := Load(Param("srcStride"), GP64())
	ref := Load(Param("ref").Base(), GP64())
	refStride := Load(Param("refStride"), GP64())

	acc := XMM()
	PXOR(acc, acc)

	for i := 0; i < h; i += 2 {
		s := XMM()
		MOVD(Mem{Base: src}, s)
		r := XMM()
		MOVD(Mem{Base: ref}, r)
		ADDQ(srcStride, src)
		ADDQ(refStride, ref)
		s2 := XMM()
		MOVD(Mem{Base: src}, s2)
		r2 := XMM()
		MOVD(Mem{Base: ref}, r2)
		PUNPCKLLQ(s2, s)
		PUNPCKLLQ(r2, r)
		PSADBW(r, s)
		PADDD(s, acc)
		if i+2 != h {
			ADDQ(srcStride, src)
			ADDQ(refStride, ref)
		}
	}

	out := GP64()
	MOVQ(acc, out)
	Store(out, ReturnIndex(0))
	RET()
}

func vAdd(a, b reg.VecVirtual) reg.VecVirtual {
	d := XMM()
	MOVOU(a, d)
	PADDD(b, d)
	return d
}

func vSub(a, b reg.VecVirtual) reg.VecVirtual {
	d := XMM()
	MOVOU(a, d)
	PSUBL(b, d)
	return d
}

func vDouble(a reg.VecVirtual) reg.VecVirtual {
	return vAdd(a, a)
}

func vHalf(a reg.VecVirtual) reg.VecVirtual {
	d := XMM()
	MOVOU(a, d)
	PSRAL(Imm(1), d)
	return d
}

func loadBlockRows(ptr reg.Register) (r0, r1, r2, r3 reg.VecVirtual) {
	r0 = XMM()
	MOVOU(Mem{Base: ptr}, r0)
	r1 = XMM()
	MOVOU(Mem{Base: ptr, Disp: 16}, r1)
	r2 = XMM()
	MOVOU(Mem{Base: ptr, Disp: 32}, r2)
	r3 = XMM()
	MOVOU(Mem{Base: ptr, Disp: 48}, r3)
	return
}

func storeBlockRows(ptr reg.Register, r0, r1, r2, r3 reg.VecVirtual) {
	MOVOU(r0, Mem{Base: ptr})
	MOVOU(r1, Mem{Base: ptr, Disp: 16})
	MOVOU(r2, Mem{Base: ptr, Disp: 32})
	MOVOU(r3, Mem{Base: ptr, Disp: 48})
}

func transpose4x4(r0, r1, r2, r3 reg.VecVirtual) (c0, c1, c2, c3 reg.VecVirtual) {
	t0 := XMM()
	MOVOU(r0, t0)
	PUNPCKLLQ(r1, t0)
	t1 := XMM()
	MOVOU(r0, t1)
	PUNPCKHLQ(r1, t1)
	t2 := XMM()
	MOVOU(r2, t2)
	PUNPCKLLQ(r3, t2)
	t3 := XMM()
	MOVOU(r2, t3)
	PUNPCKHLQ(r3, t3)

	c0 = XMM()
	MOVOU(t0, c0)
	PUNPCKLQDQ(t2, c0)
	c1 = XMM()
	MOVOU(t0, c1)
	PUNPCKHQDQ(t2, c1)
	c2 = XMM()
	MOVOU(t1, c2)
	PUNPCKLQDQ(t3, c2)
	c3 = XMM()
	MOVOU(t1, c3)
	PUNPCKHQDQ(t3, c3)
	return
}

func genForward4x4() {
	TEXT("forward4x4", NOSPLIT, "func(b *[16]int32)")
	Pragma("noescape")
	Doc("")
	ptr := Load(Param("b"), GP64())

	r0, r1, r2, r3 := loadBlockRows(ptr)
	c0, c1, c2, c3 := transpose4x4(r0, r1, r2, r3)

	o0, o1, o2, o3 := forwardButterfly(c0, c1, c2, c3)
	m0, m1, m2, m3 := transpose4x4(o0, o1, o2, o3)
	f0, f1, f2, f3 := forwardButterfly(m0, m1, m2, m3)

	storeBlockRows(ptr, f0, f1, f2, f3)
	RET()
}

func forwardButterfly(s0, s1, s2, s3 reg.VecVirtual) (o0, o1, o2, o3 reg.VecVirtual) {
	t0 := vAdd(s0, s3)
	t1 := vAdd(s1, s2)
	t2 := vSub(s1, s2)
	t3 := vSub(s0, s3)

	o0 = vAdd(t0, t1)
	o1 = vAdd(vDouble(t3), t2)
	o2 = vSub(t0, t1)
	o3 = vSub(t3, vDouble(t2))
	return
}

func genInverse4x4() {
	TEXT("inverse4x4", NOSPLIT, "func(b *[16]int32)")
	Pragma("noescape")
	Doc("")
	ptr := Load(Param("b"), GP64())

	r0, r1, r2, r3 := loadBlockRows(ptr)
	c0, c1, c2, c3 := transpose4x4(r0, r1, r2, r3)

	o0, o1, o2, o3 := inverseRowButterfly(c0, c1, c2, c3)
	m0, m1, m2, m3 := transpose4x4(o0, o1, o2, o3)
	f0, f1, f2, f3 := inverseColumnButterfly(m0, m1, m2, m3)

	storeBlockRows(ptr, f0, f1, f2, f3)
	RET()
}

func inverseRowButterfly(s0, s1, s2, s3 reg.VecVirtual) (o0, o1, o2, o3 reg.VecVirtual) {
	t0 := vAdd(s0, s2)
	t1 := vSub(s0, s2)
	t2 := vSub(vHalf(s1), s3)
	t3 := vAdd(s1, vHalf(s3))

	o0 = vAdd(t0, t3)
	o1 = vAdd(t1, t2)
	o2 = vSub(t1, t2)
	o3 = vSub(t0, t3)
	return
}

func broadcastImm32(v uint32) reg.VecVirtual {
	g := GP32()
	MOVL(U32(v), g)
	x := XMM()
	MOVD(g, x)
	PSHUFD(Imm(0), x, x)
	return x
}

func inverseColumnButterfly(s0, s1, s2, s3 reg.VecVirtual) (o0, o1, o2, o3 reg.VecVirtual) {
	t0 := vAdd(s0, s2)
	t1 := vSub(s0, s2)
	t2 := vSub(vHalf(s1), s3)
	t3 := vAdd(s1, vHalf(s3))

	round := broadcastImm32(32)
	round4x4 := func(v reg.VecVirtual) reg.VecVirtual {
		d := XMM()
		MOVOU(v, d)
		PADDD(round, d)
		PSRAL(Imm(6), d)
		return d
	}

	o0 = round4x4(vAdd(t0, t3))
	o1 = round4x4(vAdd(t1, t2))
	o2 = round4x4(vSub(t1, t2))
	o3 = round4x4(vSub(t0, t3))
	return
}

func genQuant4x4() {
	TEXT("quant4x4", NOSPLIT, "func(b *[16]int32, mf *[16]int32, f int32, qbits uint64)")
	Pragma("noescape")
	Doc("")
	bPtr := Load(Param("b"), GP64())
	mfPtr := Load(Param("mf"), GP64())
	fGP := Load(Param("f"), GP32())
	qbitsGP := Load(Param("qbits"), GP64())

	fVec := XMM()
	MOVD(fGP, fVec)
	PSHUFD(Imm(0), fVec, fVec)

	countVec := XMM()
	MOVQ(qbitsGP, countVec)

	for i := 0; i < 4; i++ {
		disp := i * 16
		bv := XMM()
		MOVOU(Mem{Base: bPtr, Disp: disp}, bv)
		mfv := XMM()
		MOVOU(Mem{Base: mfPtr, Disp: disp}, mfv)

		absv := XMM()
		PABSD(bv, absv)
		PMULLD(mfv, absv)
		PADDD(fVec, absv)
		PSRAL(countVec, absv)
		PSIGND(bv, absv)

		MOVOU(absv, Mem{Base: bPtr, Disp: disp})
	}
	RET()
}

func genDequantLeft4x4() {
	TEXT("dequantLeft4x4", NOSPLIT, "func(b *[16]int32, scale *[16]int32, shift uint64)")
	Pragma("noescape")
	Doc("")
	bPtr := Load(Param("b"), GP64())
	scalePtr := Load(Param("scale"), GP64())
	shiftGP := Load(Param("shift"), GP64())

	countVec := XMM()
	MOVQ(shiftGP, countVec)

	for i := 0; i < 4; i++ {
		disp := i * 16
		bv := XMM()
		MOVOU(Mem{Base: bPtr, Disp: disp}, bv)
		sv := XMM()
		MOVOU(Mem{Base: scalePtr, Disp: disp}, sv)

		PMULLD(sv, bv)
		PSLLL(countVec, bv)

		MOVOU(bv, Mem{Base: bPtr, Disp: disp})
	}
	RET()
}

func genDequantRight4x4() {
	TEXT("dequantRight4x4", NOSPLIT, "func(b *[16]int32, scale *[16]int32, shift uint64, round int32)")
	Pragma("noescape")
	Doc("")
	bPtr := Load(Param("b"), GP64())
	scalePtr := Load(Param("scale"), GP64())
	shiftGP := Load(Param("shift"), GP64())
	roundGP := Load(Param("round"), GP32())

	countVec := XMM()
	MOVQ(shiftGP, countVec)

	roundVec := XMM()
	MOVD(roundGP, roundVec)
	PSHUFD(Imm(0), roundVec, roundVec)

	for i := 0; i < 4; i++ {
		disp := i * 16
		bv := XMM()
		MOVOU(Mem{Base: bPtr, Disp: disp}, bv)
		sv := XMM()
		MOVOU(Mem{Base: scalePtr, Disp: disp}, sv)

		PMULLD(sv, bv)
		PADDD(roundVec, bv)
		PSRAL(countVec, bv)

		MOVOU(bv, Mem{Base: bPtr, Disp: disp})
	}
	RET()
}

func genAddResidual4x4() {
	TEXT("addResidual4x4", NOSPLIT, "func(plane []byte, stride int, b *[16]int32)")
	Pragma("noescape")
	Doc("")
	planePtr := Load(Param("plane").Base(), GP64())
	strideGP := Load(Param("stride"), GP64())
	bPtr := Load(Param("b"), GP64())

	for y := 0; y < 4; y++ {
		wide := XMM()
		PMOVZXBD(Mem{Base: planePtr}, wide)

		res := XMM()
		MOVOU(Mem{Base: bPtr, Disp: y * 16}, res)
		PADDD(res, wide)

		words := XMM()
		MOVOU(wide, words)
		PACKSSLW(words, words)

		bytes := XMM()
		MOVOU(words, bytes)
		PACKUSWB(bytes, bytes)

		out := GP32()
		MOVD(bytes, out)
		MOVL(out, Mem{Base: planePtr})

		if y != 3 {
			ADDQ(strideGP, planePtr)
		}
	}
	RET()
}

func loadDiffRow4(srcPtr, refPtr reg.Register) reg.VecVirtual {
	s := XMM()
	PMOVZXBD(Mem{Base: srcPtr}, s)
	r := XMM()
	PMOVZXBD(Mem{Base: refPtr}, r)
	return vSub(s, r)
}

func hadamardButterfly(s0, s1, s2, s3 reg.VecVirtual) (o0, o1, o2, o3 reg.VecVirtual) {
	t0 := vAdd(s0, s1)
	t1 := vSub(s0, s1)
	t2 := vAdd(s2, s3)
	t3 := vSub(s2, s3)

	o0 = vAdd(t0, t2)
	o1 = vAdd(t1, t3)
	o2 = vSub(t0, t2)
	o3 = vSub(t1, t3)
	return
}

func absDWord(v reg.VecVirtual) reg.VecVirtual {
	a := XMM()
	PABSD(v, a)
	return a
}

func horizontalSum4(v reg.VecVirtual) reg.VecVirtual {
	acc := XMM()
	MOVOU(v, acc)
	hi := XMM()
	MOVOU(acc, hi)
	PSRLDQ(Imm(8), hi)
	PADDD(hi, acc)
	hi2 := XMM()
	MOVOU(acc, hi2)
	PSRLDQ(Imm(4), hi2)
	PADDD(hi2, acc)
	return acc
}

func satd4x4RawSSE(srcPtr, srcStride, refPtr, refStride reg.Register) reg.Register {
	r0 := loadDiffRow4(srcPtr, refPtr)
	ADDQ(srcStride, srcPtr)
	ADDQ(refStride, refPtr)
	r1 := loadDiffRow4(srcPtr, refPtr)
	ADDQ(srcStride, srcPtr)
	ADDQ(refStride, refPtr)
	r2 := loadDiffRow4(srcPtr, refPtr)
	ADDQ(srcStride, srcPtr)
	ADDQ(refStride, refPtr)
	r3 := loadDiffRow4(srcPtr, refPtr)

	c0, c1, c2, c3 := transpose4x4(r0, r1, r2, r3)
	o0, o1, o2, o3 := hadamardButterfly(c0, c1, c2, c3)
	m0, m1, m2, m3 := transpose4x4(o0, o1, o2, o3)
	f0, f1, f2, f3 := hadamardButterfly(m0, m1, m2, m3)

	a0 := absDWord(f0)
	a1 := absDWord(f1)
	a2 := absDWord(f2)
	a3 := absDWord(f3)

	acc := vAdd(vAdd(a0, a1), vAdd(a2, a3))
	sum := horizontalSum4(acc)

	out := GP64()
	sumGP := out.As32()
	MOVD(sum, sumGP)
	ADDL(Imm(1), sumGP)
	SARL(Imm(1), sumGP)
	return out
}

func genSATD4x4() {
	TEXT("satd4x4", NOSPLIT, "func(src []byte, srcStride int, ref []byte, refStride int) int")
	Pragma("noescape")
	Doc("")
	src := Load(Param("src").Base(), GP64())
	srcStride := Load(Param("srcStride"), GP64())
	ref := Load(Param("ref").Base(), GP64())
	refStride := Load(Param("refStride"), GP64())

	out := satd4x4RawSSE(src, srcStride, ref, refStride)

	Store(out, ReturnIndex(0))
	RET()
}

func blockOffsetPtr(base, stride4 reg.Register, xOff, yOff int) reg.Register {
	p := GP64()
	MOVQ(base, p)
	if xOff != 0 {
		ADDQ(Imm(uint64(xOff)), p)
	}
	if yOff != 0 {
		ADDQ(stride4, p)
	}
	return p
}

var satd8x8Blocks = [][2]int{{0, 0}, {4, 0}, {0, 4}, {4, 4}}

func genSATD8x8() {
	TEXT("satd8x8", NOSPLIT, "func(src []byte, srcStride int, ref []byte, refStride int) int")
	Pragma("noescape")
	Doc("")
	srcBase := Load(Param("src").Base(), GP64())
	srcStride := Load(Param("srcStride"), GP64())
	refBase := Load(Param("ref").Base(), GP64())
	refStride := Load(Param("refStride"), GP64())

	srcStride4 := GP64()
	MOVQ(srcStride, srcStride4)
	SHLQ(Imm(2), srcStride4)
	refStride4 := GP64()
	MOVQ(refStride, refStride4)
	SHLQ(Imm(2), refStride4)

	total := GP64()
	for i, blk := range satd8x8Blocks {
		srcPtr := blockOffsetPtr(srcBase, srcStride4, blk[0], blk[1])
		refPtr := blockOffsetPtr(refBase, refStride4, blk[0], blk[1])
		res := satd4x4RawSSE(srcPtr, srcStride, refPtr, refStride)
		if i == 0 {
			MOVQ(res, total)
		} else {
			ADDQ(res, total)
		}
	}

	Store(total, ReturnIndex(0))
	RET()
}

func packFourRows(ptr, stride reg.Register) reg.VecVirtual {
	r := make([]reg.VecVirtual, 4)
	p := GP64()
	MOVQ(ptr, p)
	for i := 0; i < 4; i++ {
		v := XMM()
		VMOVD(Mem{Base: p}, v)
		r[i] = v
		if i != 3 {
			ADDQ(stride, p)
		}
	}
	lo := XMM()
	VPUNPCKLDQ(r[1], r[0], lo)
	hi := XMM()
	VPUNPCKLDQ(r[3], r[2], hi)
	all := XMM()
	VPUNPCKLQDQ(hi, lo, all)
	return all
}

func butterflyBlend(v, swapped reg.VecVirtual, blend func(a, b, d reg.VecVirtual)) reg.VecVirtual {
	sum := YMM()
	VPADDW(swapped, v, sum)
	diff := YMM()
	VPSUBW(swapped, v, diff)
	d := YMM()
	blend(sum, diff, d)
	return d
}

func avx2SatdOnes() reg.VecVirtual {
	oneGP := GP32()
	MOVL(U32(1|1<<16), oneGP)
	oneX := XMM()
	VMOVD(oneGP, oneX)
	ones := YMM()
	VPBROADCASTD(oneX, ones)
	return ones
}

func satd4x4RawAVX2(srcPtr, srcStride, refPtr, refStride reg.Register, ones reg.VecVirtual) reg.VecVirtual {
	sBytes := packFourRows(srcPtr, srcStride)
	rBytes := packFourRows(refPtr, refStride)

	sw := YMM()
	VPMOVZXBW(sBytes, sw)
	rw := YMM()
	VPMOVZXBW(rBytes, rw)
	d := YMM()
	VPSUBW(rw, sw, d)

	t := YMM()
	VPSHUFLW(Imm(0xB1), d, t)
	VPSHUFHW(Imm(0xB1), t, t)
	stage1 := butterflyBlend(d, t, func(a, b, dst reg.VecVirtual) {
		VPBLENDW(Imm(0xAA), b, a, dst)
	})

	t2 := YMM()
	VPSHUFD(Imm(0xB1), stage1, t2)
	stage2 := butterflyBlend(stage1, t2, func(a, b, dst reg.VecVirtual) {
		VPBLENDD(Imm(0xAA), b, a, dst)
	})

	t3 := YMM()
	VPSHUFD(Imm(0x4E), stage2, t3)
	stage3 := butterflyBlend(stage2, t3, func(a, b, dst reg.VecVirtual) {
		VPBLENDD(Imm(0xCC), b, a, dst)
	})

	t4 := YMM()
	VPERMQ(Imm(0x4E), stage3, t4)
	sum := YMM()
	VPADDW(t4, stage3, sum)
	diff := YMM()
	VPSUBW(t4, stage3, diff)

	aSum := YMM()
	VPABSW(sum, aSum)
	aDiff := YMM()
	VPABSW(diff, aDiff)
	tot := YMM()
	VPADDW(aDiff, aSum, tot)

	wide := YMM()
	VPMADDWD(ones, tot, wide)

	hi128 := XMM()
	VEXTRACTI128(Imm(1), wide, hi128)
	acc := XMM()
	VPADDD(hi128, wide.AsX(), acc)
	sh := XMM()
	VPSHUFD(Imm(0x0E), acc, sh)
	VPADDD(sh, acc, acc)
	VPSHUFD(Imm(0x01), acc, sh)
	VPADDD(sh, acc, acc)
	return acc
}

func genSATD4x4AVX2() {
	TEXT("satd4x4AVX2", NOSPLIT, "func(src []byte, srcStride int, ref []byte, refStride int) int")
	Pragma("noescape")
	Doc("")
	src := Load(Param("src").Base(), GP64())
	srcStride := Load(Param("srcStride"), GP64())
	ref := Load(Param("ref").Base(), GP64())
	refStride := Load(Param("refStride"), GP64())

	ones := avx2SatdOnes()
	acc := satd4x4RawAVX2(src, srcStride, ref, refStride, ones)

	out := GP64()
	sumGP := out.As32()
	VMOVD(acc, sumGP)
	VZEROUPPER()
	SHRL(Imm(1), sumGP)
	ADDL(Imm(1), sumGP)
	SARL(Imm(1), sumGP)

	Store(out, ReturnIndex(0))
	RET()
}

func genSATD8x8AVX2() {
	TEXT("satd8x8AVX2", NOSPLIT, "func(src []byte, srcStride int, ref []byte, refStride int) int")
	Pragma("noescape")
	Doc("")
	srcBase := Load(Param("src").Base(), GP64())
	srcStride := Load(Param("srcStride"), GP64())
	refBase := Load(Param("ref").Base(), GP64())
	refStride := Load(Param("refStride"), GP64())

	srcStride4 := GP64()
	MOVQ(srcStride, srcStride4)
	SHLQ(Imm(2), srcStride4)
	refStride4 := GP64()
	MOVQ(refStride, refStride4)
	SHLQ(Imm(2), refStride4)

	ones := avx2SatdOnes()

	accs := make([]reg.VecVirtual, len(satd8x8Blocks))
	for i, blk := range satd8x8Blocks {
		srcPtr := blockOffsetPtr(srcBase, srcStride4, blk[0], blk[1])
		refPtr := blockOffsetPtr(refBase, refStride4, blk[0], blk[1])
		accs[i] = satd4x4RawAVX2(srcPtr, srcStride, refPtr, refStride, ones)
	}

	total := GP64()
	for i, acc := range accs {
		g := GP64()
		gLow := g.As32()
		VMOVD(acc, gLow)
		SHRL(Imm(1), gLow)
		ADDL(Imm(1), gLow)
		SARL(Imm(1), gLow)
		if i == 0 {
			MOVQ(g, total)
		} else {
			ADDQ(g, total)
		}
	}
	VZEROUPPER()

	Store(total, ReturnIndex(0))
	RET()
}

func loadDW4At(ptr reg.Register, disp int) reg.VecVirtual {
	v := XMM()
	if disp == 0 {
		PMOVZXBD(Mem{Base: ptr}, v)
	} else {
		PMOVZXBD(Mem{Base: ptr, Disp: disp}, v)
	}
	return v
}

func memAt(p reg.Register, d int) Mem {
	if d == 0 {
		return Mem{Base: p}
	}
	return Mem{Base: p, Disp: d}
}

func wAdd(a, b reg.VecVirtual) reg.VecVirtual {
	d := XMM()
	MOVOU(a, d)
	PADDW(b, d)
	return d
}

func wSub(a, b reg.VecVirtual) reg.VecVirtual {
	d := XMM()
	MOVOU(a, d)
	PSUBW(b, d)
	return d
}

func wMul(a, b reg.VecVirtual) reg.VecVirtual {
	d := XMM()
	MOVOU(a, d)
	PMULLW(b, d)
	return d
}

func dAdd(a, b reg.VecVirtual) reg.VecVirtual {
	d := XMM()
	MOVOU(a, d)
	PADDD(b, d)
	return d
}

type tapLoader func(k int) reg.VecVirtual

func hTapLoader(ptr reg.Register, x, n int) tapLoader {
	if n >= 8 {
		return func(k int) reg.VecVirtual {
			v := XMM()
			PMOVZXBW(memAt(ptr, x-2+k), v)
			return v
		}
	}
	a := XMM()
	PMOVZXBW(memAt(ptr, x-2), a)
	b := XMM()
	PMOVZXBW(memAt(ptr, x-1), b)
	return func(k int) reg.VecVirtual {
		v := XMM()
		if k == 5 {
			MOVOU(b, v)
			PSRLDQ(Imm(8), v)
			return v
		}
		MOVOU(a, v)
		if k != 0 {
			PSRLDQ(Imm(uint64(2*k)), v)
		}
		return v
	}
}

func vTapLoader(rows []reg.Register, x, n int) tapLoader {
	return func(k int) reg.VecVirtual {
		v := XMM()
		if n >= 8 {
			PMOVZXBW(memAt(rows[k], x), v)
			return v
		}
		MOVD(memAt(rows[k], x), v)
		PMOVZXBW(v, v)
		return v
	}
}

func tapSum6W(load tapLoader, c20, c5 reg.VecVirtual) reg.VecVirtual {
	s := wAdd(load(0), load(5))
	s = wAdd(s, wMul(wAdd(load(2), load(3)), c20))
	return wSub(s, wMul(wAdd(load(1), load(4)), c5))
}

func roundPackW(v, round reg.VecVirtual, sh int) reg.VecVirtual {
	d := XMM()
	MOVOU(v, d)
	PADDW(round, d)
	PSRAW(Imm(uint64(sh)), d)
	p := XMM()
	MOVOU(d, p)
	PACKUSWB(p, p)
	return p
}

func storeBytesN(v reg.VecVirtual, ptr reg.Register, disp, n int) {
	if n >= 8 {
		MOVQ(v, memAt(ptr, disp))
		return
	}
	storeBytes4(v, ptr, disp)
}

func hvPairSum(win []reg.VecVirtual, c1m5, c2020, cm51 reg.VecVirtual, high bool) reg.VecVirtual {
	mk := func(a, b, coef reg.VecVirtual) reg.VecVirtual {
		d := XMM()
		MOVOU(a, d)
		if high {
			PUNPCKHWL(b, d)
		} else {
			PUNPCKLWL(b, d)
		}
		PMADDWL(coef, d)
		return d
	}
	s := dAdd(mk(win[0], win[1], c1m5), mk(win[2], win[3], c2020))
	return dAdd(s, mk(win[4], win[5], cm51))
}

func hvRoundPack(lo, hi, round reg.VecVirtual) reg.VecVirtual {
	rl := XMM()
	MOVOU(lo, rl)
	PADDD(round, rl)
	PSRAL(Imm(10), rl)
	rh := XMM()
	MOVOU(hi, rh)
	PADDD(round, rh)
	PSRAL(Imm(10), rh)
	PACKSSLW(rh, rl)
	PACKUSWB(rl, rl)
	return rl
}

const (
	packed20  = 20 | 20<<16
	packed5   = 5 | 5<<16
	packed16  = 16 | 16<<16
	coef1m5   = 1 | 0xFFFB<<16
	coef2020  = 20 | 20<<16
	coefm51   = 0xFFFB | 1<<16
	roundHV32 = 512
)

func stripWidth(w int) int {
	if w > 8 {
		return 8
	}
	return w
}

func ybroadcastImm32(v uint32) reg.VecVirtual {
	g := GP32()
	MOVL(U32(v), g)
	x := XMM()
	VMOVD(g, x)
	y := YMM()
	VPBROADCASTD(x, y)
	return y
}

func yAddW(a, b reg.VecVirtual) reg.VecVirtual {
	d := YMM()
	VPADDW(a, b, d)
	return d
}

func ySubW(a, b reg.VecVirtual) reg.VecVirtual {
	d := YMM()
	VPSUBW(b, a, d)
	return d
}

func yMulW(a, b reg.VecVirtual) reg.VecVirtual {
	d := YMM()
	VPMULLW(a, b, d)
	return d
}

func yAddD(a, b reg.VecVirtual) reg.VecVirtual {
	d := YMM()
	VPADDD(a, b, d)
	return d
}

func yHTapLoader(ptr reg.Register, x int) tapLoader {
	return func(k int) reg.VecVirtual {
		v := YMM()
		VPMOVZXBW(memAt(ptr, x-2+k), v)
		return v
	}
}

func yVTapLoader(rows []reg.Register, x int) tapLoader {
	return func(k int) reg.VecVirtual {
		v := YMM()
		VPMOVZXBW(memAt(rows[k], x), v)
		return v
	}
}

func yTapSum6W(load tapLoader, c20, c5 reg.VecVirtual) reg.VecVirtual {
	s := yAddW(load(0), load(5))
	s = yAddW(s, yMulW(yAddW(load(2), load(3)), c20))
	return ySubW(s, yMulW(yAddW(load(1), load(4)), c5))
}

func yStore16(v reg.VecVirtual, ptr reg.Register, disp int) {
	p := YMM()
	VPACKUSWB(v, v, p)
	VPERMQ(Imm(8), p, p)
	VMOVDQU(p.AsX(), memAt(ptr, disp))
}

func yRoundPackStore(v, round reg.VecVirtual, sh int, ptr reg.Register, disp int) {
	d := YMM()
	VPADDW(round, v, d)
	VPSRAW(Imm(uint64(sh)), d, d)
	yStore16(d, ptr, disp)
}

func yHVPairSum(win []reg.VecVirtual, c1m5, c2020, cm51 reg.VecVirtual, high bool) reg.VecVirtual {
	mk := func(a, b, coef reg.VecVirtual) reg.VecVirtual {
		d := YMM()
		if high {
			VPUNPCKHWD(b, a, d)
		} else {
			VPUNPCKLWD(b, a, d)
		}
		e := YMM()
		VPMADDWD(coef, d, e)
		return e
	}
	s := yAddD(mk(win[0], win[1], c1m5), mk(win[2], win[3], c2020))
	return yAddD(s, mk(win[4], win[5], cm51))
}

func yHVRoundPackStore(lo, hi, round reg.VecVirtual, ptr reg.Register, disp int) {
	rl := YMM()
	VPADDD(round, lo, rl)
	VPSRAD(Imm(10), rl, rl)
	rh := YMM()
	VPADDD(round, hi, rh)
	VPSRAD(Imm(10), rh, rh)
	w := YMM()
	VPACKSSDW(rh, rl, w)
	yStore16(w, ptr, disp)
}

func genSixTapHorizAVX2(w, h int) {
	name := fmt.Sprintf("sixTapHoriz%dx%dAVX2", w, h)
	TEXT(name, NOSPLIT, "func(dst []byte, dstStride int, src []byte, srcStride int)")
	Pragma("noescape")
	Doc("")
	dst := Load(Param("dst").Base(), GP64())
	dstStride := Load(Param("dstStride"), GP64())
	src := Load(Param("src").Base(), GP64())
	srcStride := Load(Param("srcStride"), GP64())

	c20 := ybroadcastImm32(packed20)
	c5 := ybroadcastImm32(packed5)
	c16 := ybroadcastImm32(packed16)

	for y := 0; y < h; y++ {
		raw := yTapSum6W(yHTapLoader(src, 0), c20, c5)
		yRoundPackStore(raw, c16, 5, dst, 0)
		if y != h-1 {
			ADDQ(srcStride, src)
			ADDQ(dstStride, dst)
		}
	}
	VZEROUPPER()
	RET()
}

func genSixTapVertAVX2(w, h int) {
	name := fmt.Sprintf("sixTapVert%dx%dAVX2", w, h)
	TEXT(name, NOSPLIT, "func(dst []byte, dstStride int, src []byte, srcStride int)")
	Pragma("noescape")
	Doc("")
	dst := Load(Param("dst").Base(), GP64())
	dstStride := Load(Param("dstStride"), GP64())
	src := Load(Param("src").Base(), GP64())
	srcStride := Load(Param("srcStride"), GP64())

	c20 := ybroadcastImm32(packed20)
	c5 := ybroadcastImm32(packed5)
	c16 := ybroadcastImm32(packed16)

	m2, m1, z0, p1, p2, p3 := sixRowPointers(src, srcStride)
	rows := []reg.Register{m2, m1, z0, p1, p2, p3}

	for y := 0; y < h; y++ {
		raw := yTapSum6W(yVTapLoader(rows, 0), c20, c5)
		yRoundPackStore(raw, c16, 5, dst, 0)
		if y != h-1 {
			advanceSixRowPointers(srcStride, m2, m1, z0, p1, p2, p3)
			ADDQ(dstStride, dst)
		}
	}
	VZEROUPPER()
	RET()
}

func genSixTapHVAVX2(w, h int) {
	name := fmt.Sprintf("sixTapHV%dx%dAVX2", w, h)
	TEXT(name, NOSPLIT, "func(dst []byte, dstStride int, src []byte, srcStride int)")
	Pragma("noescape")
	Doc("")
	dst := Load(Param("dst").Base(), GP64())
	dstStride := Load(Param("dstStride"), GP64())
	src := Load(Param("src").Base(), GP64())
	srcStride := Load(Param("srcStride"), GP64())

	c20 := ybroadcastImm32(packed20)
	c5 := ybroadcastImm32(packed5)
	c1m5 := ybroadcastImm32(coef1m5)
	c2020 := ybroadcastImm32(coef2020)
	cm51 := ybroadcastImm32(coefm51)
	c512 := ybroadcastImm32(roundHV32)

	p := GP64()
	MOVQ(src, p)
	SUBQ(srcStride, p)
	SUBQ(srcStride, p)

	win := make([]reg.VecVirtual, 6)
	for k := 0; k < 6; k++ {
		win[k] = yTapSum6W(yHTapLoader(p, 0), c20, c5)
		if k != 5 {
			ADDQ(srcStride, p)
		}
	}
	for y := 0; y < h; y++ {
		lo := yHVPairSum(win, c1m5, c2020, cm51, false)
		hi := yHVPairSum(win, c1m5, c2020, cm51, true)
		yHVRoundPackStore(lo, hi, c512, dst, 0)
		if y != h-1 {
			ADDQ(srcStride, p)
			copy(win, win[1:])
			win[5] = yTapSum6W(yHTapLoader(p, 0), c20, c5)
			ADDQ(dstStride, dst)
		}
	}
	VZEROUPPER()
	RET()
}

func clipRoundDW(raw reg.VecVirtual, round int32, shiftN int) reg.VecVirtual {
	v := XMM()
	MOVOU(raw, v)
	roundVec := broadcastImm32(uint32(round))
	PADDD(roundVec, v)
	PSRAL(Imm(uint64(shiftN)), v)
	return v
}

func packClip4(v reg.VecVirtual) reg.VecVirtual {
	words := XMM()
	MOVOU(v, words)
	PACKSSLW(words, words)
	bytes := XMM()
	MOVOU(words, bytes)
	PACKUSWB(bytes, bytes)
	return bytes
}

func storeBytes4(v reg.VecVirtual, ptr reg.Register, disp int) {
	out := GP32()
	MOVD(v, out)
	if disp == 0 {
		MOVL(out, Mem{Base: ptr})
	} else {
		MOVL(out, Mem{Base: ptr, Disp: disp})
	}
}

func genSixTapHoriz(w, h int) {
	name := fmt.Sprintf("sixTapHoriz%dx%d", w, h)
	TEXT(name, NOSPLIT, "func(dst []byte, dstStride int, src []byte, srcStride int)")
	Pragma("noescape")
	Doc("")
	dst := Load(Param("dst").Base(), GP64())
	dstStride := Load(Param("dstStride"), GP64())
	src := Load(Param("src").Base(), GP64())
	srcStride := Load(Param("srcStride"), GP64())

	c20 := broadcastImm32(packed20)
	c5 := broadcastImm32(packed5)
	c16 := broadcastImm32(packed16)
	n := stripWidth(w)

	for y := 0; y < h; y++ {
		for x := 0; x < w; x += n {
			raw := tapSum6W(hTapLoader(src, x, n), c20, c5)
			storeBytesN(roundPackW(raw, c16, 5), dst, x, n)
		}
		if y != h-1 {
			ADDQ(srcStride, src)
			ADDQ(dstStride, dst)
		}
	}
	RET()
}

func sixRowPointers(base reg.Register, stride reg.Register) (m2, m1, z0, p1, p2, p3 reg.Register) {
	m2 = GP64()
	MOVQ(base, m2)
	SUBQ(stride, m2)
	SUBQ(stride, m2)
	m1 = GP64()
	MOVQ(base, m1)
	SUBQ(stride, m1)
	z0 = base
	p1 = GP64()
	MOVQ(base, p1)
	ADDQ(stride, p1)
	p2 = GP64()
	MOVQ(base, p2)
	ADDQ(stride, p2)
	ADDQ(stride, p2)
	p3 = GP64()
	MOVQ(base, p3)
	ADDQ(stride, p3)
	ADDQ(stride, p3)
	ADDQ(stride, p3)
	return
}

func advanceSixRowPointers(stride reg.Register, ptrs ...reg.Register) {
	for _, p := range ptrs {
		ADDQ(stride, p)
	}
}

func genSixTapVert(w, h int) {
	name := fmt.Sprintf("sixTapVert%dx%d", w, h)
	TEXT(name, NOSPLIT, "func(dst []byte, dstStride int, src []byte, srcStride int)")
	Pragma("noescape")
	Doc("")
	dst := Load(Param("dst").Base(), GP64())
	dstStride := Load(Param("dstStride"), GP64())
	src := Load(Param("src").Base(), GP64())
	srcStride := Load(Param("srcStride"), GP64())

	c20 := broadcastImm32(packed20)
	c5 := broadcastImm32(packed5)
	c16 := broadcastImm32(packed16)
	n := stripWidth(w)

	m2, m1, z0, p1, p2, p3 := sixRowPointers(src, srcStride)
	rows := []reg.Register{m2, m1, z0, p1, p2, p3}

	for y := 0; y < h; y++ {
		for x := 0; x < w; x += n {
			raw := tapSum6W(vTapLoader(rows, x, n), c20, c5)
			storeBytesN(roundPackW(raw, c16, 5), dst, x, n)
		}
		if y != h-1 {
			advanceSixRowPointers(srcStride, m2, m1, z0, p1, p2, p3)
			ADDQ(dstStride, dst)
		}
	}
	RET()
}

func genSixTapHV(w, h int) {
	name := fmt.Sprintf("sixTapHV%dx%d", w, h)
	TEXT(name, NOSPLIT, "func(dst []byte, dstStride int, src []byte, srcStride int)")
	Pragma("noescape")
	Doc("")
	dst := Load(Param("dst").Base(), GP64())
	dstStride := Load(Param("dstStride"), GP64())
	src := Load(Param("src").Base(), GP64())
	srcStride := Load(Param("srcStride"), GP64())

	c20 := broadcastImm32(packed20)
	c5 := broadcastImm32(packed5)
	c1m5 := broadcastImm32(coef1m5)
	c2020 := broadcastImm32(coef2020)
	cm51 := broadcastImm32(coefm51)
	c512 := broadcastImm32(roundHV32)
	n := stripWidth(w)

	for x := 0; x < w; x += n {
		p := GP64()
		MOVQ(src, p)
		SUBQ(srcStride, p)
		SUBQ(srcStride, p)
		dp := GP64()
		MOVQ(dst, dp)

		win := make([]reg.VecVirtual, 6)
		for k := 0; k < 6; k++ {
			win[k] = tapSum6W(hTapLoader(p, x, n), c20, c5)
			if k != 5 {
				ADDQ(srcStride, p)
			}
		}
		for y := 0; y < h; y++ {
			lo := hvPairSum(win, c1m5, c2020, cm51, false)
			hi := lo
			if n >= 8 {
				hi = hvPairSum(win, c1m5, c2020, cm51, true)
			}
			storeBytesN(hvRoundPack(lo, hi, c512), dp, x, n)
			if y != h-1 {
				ADDQ(srcStride, p)
				copy(win, win[1:])
				win[5] = tapSum6W(hTapLoader(p, x, n), c20, c5)
				ADDQ(dstStride, dp)
			}
		}
	}
	RET()
}

func mulDW(a, b reg.VecVirtual) reg.VecVirtual {
	d := XMM()
	MOVOU(a, d)
	PMULLD(b, d)
	return d
}

func genBilinearChroma(w, h int) {
	name := fmt.Sprintf("bilinearChroma%dx%d", w, h)
	TEXT(name, NOSPLIT, "func(dst []byte, dstStride int, src []byte, srcStride int, xFrac, yFrac int32)")
	Pragma("noescape")
	Doc("")
	dst := Load(Param("dst").Base(), GP64())
	dstStride := Load(Param("dstStride"), GP64())
	src := Load(Param("src").Base(), GP64())
	srcStride := Load(Param("srcStride"), GP64())
	xF := Load(Param("xFrac"), GP32())
	yF := Load(Param("yFrac"), GP32())

	invX := GP32()
	MOVL(U32(8), invX)
	SUBL(xF, invX)
	invY := GP32()
	MOVL(U32(8), invY)
	SUBL(yF, invY)

	w00 := GP32()
	MOVL(invX, w00)
	IMULL(invY, w00)
	w10 := GP32()
	MOVL(xF, w10)
	IMULL(invY, w10)
	w01 := GP32()
	MOVL(invX, w01)
	IMULL(yF, w01)
	w11 := GP32()
	MOVL(xF, w11)
	IMULL(yF, w11)

	w00v := broadcastGP32(w00)
	w10v := broadcastGP32(w10)
	w01v := broadcastGP32(w01)
	w11v := broadcastGP32(w11)

	srcNext := GP64()
	MOVQ(src, srcNext)
	ADDQ(srcStride, srcNext)

	for y := 0; y < h; y++ {
		for x := 0; x < w; x += 4 {
			a := loadDW4At(src, x)
			b := loadDW4At(src, x+1)
			c := loadDW4At(srcNext, x)
			d := loadDW4At(srcNext, x+1)
			sum := vAdd(vAdd(mulDW(a, w00v), mulDW(b, w10v)), vAdd(mulDW(c, w01v), mulDW(d, w11v)))
			rounded := clipRoundDW(sum, 32, 6)
			bytes := packClip4(rounded)
			storeBytes4(bytes, dst, x)
		}
		if y != h-1 {
			ADDQ(srcStride, src)
			ADDQ(srcStride, srcNext)
			ADDQ(dstStride, dst)
		}
	}
	RET()
}

func broadcastGP32(g reg.Register) reg.VecVirtual {
	x := XMM()
	MOVD(g, x)
	PSHUFD(Imm(0), x, x)
	return x
}
