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

	genForward4x4()
	genInverse4x4()
	genQuant4x4()
	genDequantLeft4x4()
	genDequantRight4x4()
	genAddResidual4x4()
	genSATD4x4()

	for _, size := range lumaMCSizes {
		genSixTapHoriz(size[0], size[1])
		genSixTapVert(size[0], size[1])
		genSixTapHV(size[0], size[1])
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

func genSATD4x4() {
	TEXT("satd4x4", NOSPLIT, "func(src []byte, srcStride int, ref []byte, refStride int) int")
	Pragma("noescape")
	Doc("")
	src := Load(Param("src").Base(), GP64())
	srcStride := Load(Param("srcStride"), GP64())
	ref := Load(Param("ref").Base(), GP64())
	refStride := Load(Param("refStride"), GP64())

	r0 := loadDiffRow4(src, ref)
	ADDQ(srcStride, src)
	ADDQ(refStride, ref)
	r1 := loadDiffRow4(src, ref)
	ADDQ(srcStride, src)
	ADDQ(refStride, ref)
	r2 := loadDiffRow4(src, ref)
	ADDQ(srcStride, src)
	ADDQ(refStride, ref)
	r3 := loadDiffRow4(src, ref)

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

	Store(out, ReturnIndex(0))
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

func shiftLeftDWImm(v reg.VecVirtual, n int) reg.VecVirtual {
	d := XMM()
	MOVOU(v, d)
	PSLLL(Imm(uint64(n)), d)
	return d
}

func mul5DW(v reg.VecVirtual) reg.VecVirtual {
	return vAdd(shiftLeftDWImm(v, 2), v)
}

func mul20DW(v reg.VecVirtual) reg.VecVirtual {
	return vAdd(shiftLeftDWImm(v, 4), shiftLeftDWImm(v, 2))
}

func combine6Tap(t0, t1, t2, t3, t4, t5 reg.VecVirtual) reg.VecVirtual {
	a := vSub(t0, mul5DW(t1))
	b := vAdd(mul20DW(t2), mul20DW(t3))
	c := vAdd(a, b)
	d := vSub(c, mul5DW(t4))
	return vAdd(d, t5)
}

func hTapRaw6(ptr reg.Register, x int) reg.VecVirtual {
	t0 := loadDW4At(ptr, x-2)
	t1 := loadDW4At(ptr, x-1)
	t2 := loadDW4At(ptr, x)
	t3 := loadDW4At(ptr, x+1)
	t4 := loadDW4At(ptr, x+2)
	t5 := loadDW4At(ptr, x+3)
	return combine6Tap(t0, t1, t2, t3, t4, t5)
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

	for y := 0; y < h; y++ {
		for x := 0; x < w; x += 4 {
			raw := hTapRaw6(src, x)
			rounded := clipRoundDW(raw, 16, 5)
			bytes := packClip4(rounded)
			storeBytes4(bytes, dst, x)
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

	m2, m1, z0, p1, p2, p3 := sixRowPointers(src, srcStride)

	for y := 0; y < h; y++ {
		for x := 0; x < w; x += 4 {
			t0 := loadDW4At(m2, x)
			t1 := loadDW4At(m1, x)
			t2 := loadDW4At(z0, x)
			t3 := loadDW4At(p1, x)
			t4 := loadDW4At(p2, x)
			t5 := loadDW4At(p3, x)
			raw := combine6Tap(t0, t1, t2, t3, t4, t5)
			rounded := clipRoundDW(raw, 16, 5)
			bytes := packClip4(rounded)
			storeBytes4(bytes, dst, x)
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

	m2, m1, z0, p1, p2, p3 := sixRowPointers(src, srcStride)

	for y := 0; y < h; y++ {
		for x := 0; x < w; x += 4 {
			rm2 := hTapRaw6(m2, x)
			rm1 := hTapRaw6(m1, x)
			r0 := hTapRaw6(z0, x)
			rp1 := hTapRaw6(p1, x)
			rp2 := hTapRaw6(p2, x)
			rp3 := hTapRaw6(p3, x)
			raw2 := combine6Tap(rm2, rm1, r0, rp1, rp2, rp3)
			rounded := clipRoundDW(raw2, 512, 10)
			bytes := packClip4(rounded)
			storeBytes4(bytes, dst, x)
		}
		if y != h-1 {
			advanceSixRowPointers(srcStride, m2, m1, z0, p1, p2, p3)
			ADDQ(dstStride, dst)
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
