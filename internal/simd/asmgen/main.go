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

	Generate()
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
