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
