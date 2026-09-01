package encoder

import (
	"github.com/oops1/go.264/internal/simd"
	"github.com/oops1/go.264/internal/transform"
)

type ScalingMatrix uint8

const (
	ScalingMatrixFlat ScalingMatrix = iota
	ScalingMatrixJVT
)

const (
	scaleIntraY = 0
	scaleIntraU = 1
	scaleIntraV = 2
	scaleInterY = 3
	scaleInterU = 4
	scaleInterV = 5
)

type scale4x4 [6][16]int32

var (
	flatLevel4x4 scale4x4
	flatQuant4x4 scale4x4
)

func init() {
	for m := 0; m < 6; m++ {
		var b transform.Block
		for i := range b {
			b[i] = 1
		}
		transform.Dequant4x4(&b, 24+m, false)
		flatLevel4x4[m] = b

		var q transform.Block
		for i := range q {
			q[i] = 1 << 15
		}
		transform.Quant4x4(&q, m, false)
		flatQuant4x4[m] = q
	}
}

func buildLevelScale4x4(weight [16]uint8) scale4x4 {
	var out scale4x4
	for m := 0; m < 6; m++ {
		for i := 0; i < 16; i++ {
			out[m][i] = flatLevel4x4[m][i] / 16 * int32(weight[i])
		}
	}
	return out
}

func buildQuantScale4x4(weight [16]uint8) scale4x4 {
	var out scale4x4
	for m := 0; m < 6; m++ {
		for i := 0; i < 16; i++ {
			w := int32(weight[i])
			out[m][i] = (flatQuant4x4[m][i]*16 + w/2) / w
		}
	}
	return out
}

func rasterOrder4x4(scan [16]uint8) [16]uint8 {
	var out [16]uint8
	for i := 0; i < 16; i++ {
		out[transform.ZigZagIndex(i)] = scan[i]
	}
	return out
}

func quantRound(qbits uint, intra bool) int32 {
	if intra {
		return int32(1) << qbits / 3
	}
	return int32(1) << qbits / 6
}

func quantValue(v, mf, f int32, qbits uint) int32 {
	if v >= 0 {
		return (v*mf + f) >> qbits
	}
	return -((-v*mf + f) >> qbits)
}

func quant4x4(b *transform.Block, qp int, qs *scale4x4, intra bool) {
	qbits := uint(15 + qp/6)
	mf := qs[qp%6]
	simd.Quant4x4((*[16]int32)(b), &mf, quantRound(qbits, intra), uint32(qbits))
}

func dequant4x4(b *transform.Block, qp int, ls *scale4x4, skipDC bool) {
	row := ls[qp%6]
	var savedDC int32
	if skipDC {
		savedDC = b[0]
	}
	simd.Dequant4x4((*[16]int32)(b), &row, uint32(qp/6))
	if skipDC {
		b[0] = savedDC
	}
}

func quantLumaDC(b *transform.Block, qp int, qs *scale4x4, intra bool) {
	transform.HadamardForwardDC4x4(b)
	mf := qs[qp%6][0]
	qbits := uint(16 + qp/6)
	f := quantRound(qbits, intra)
	for i := range b {
		b[i] = quantValue(b[i], mf, f, qbits)
	}
}

func dequantLumaDC(b *transform.Block, qp int, ls *scale4x4) {
	transform.Hadamard4x4(b)
	v := ls[qp%6][0]
	shift := qp / 6
	if shift >= 6 {
		s := uint(shift - 6)
		for i := range b {
			b[i] = b[i] * v << s
		}
		return
	}
	s := uint(6 - shift)
	round := int32(1) << (s - 1)
	for i := range b {
		b[i] = (b[i]*v + round) >> s
	}
}

func quantChromaDC(b *transform.ChromaDC, qp int, qs *scale4x4, intra bool) {
	transform.Hadamard2x2(b)
	mf := qs[qp%6][0]
	qbits := uint(16 + qp/6)
	f := quantRound(qbits, intra)
	for i := range b {
		b[i] = quantValue(b[i], mf, f, qbits)
	}
}

func dequantChromaDC(b *transform.ChromaDC, qp int, ls *scale4x4) {
	transform.Hadamard2x2(b)
	v := ls[qp%6][0]
	shift := uint(qp / 6)
	for i := range b {
		b[i] = (b[i] * v << shift) >> 5
	}
}

func (e *Encoder) buildScalingTables() {
	list4x4, list8x8 := e.pps.ResolvedScalingLists(e.sps)
	for i := 0; i < 6; i++ {
		w := rasterOrder4x4(list4x4[i])
		e.level4x4[i] = buildLevelScale4x4(w)
		e.quant4x4[i] = buildQuantScale4x4(w)
	}
	for i := 0; i < 2; i++ {
		w := rasterOrder8x8(list8x8[i])
		e.level8x8[i] = transform.BuildLevelScale8x8(w)
		e.quant8x8[i] = transform.BuildQuantScale8x8(w)
	}
}

func (e *Encoder) lumaLevel4x4(intra bool) *scale4x4 {
	if intra {
		return &e.level4x4[scaleIntraY]
	}
	return &e.level4x4[scaleInterY]
}

func (e *Encoder) lumaQuant4x4(intra bool) *scale4x4 {
	if intra {
		return &e.quant4x4[scaleIntraY]
	}
	return &e.quant4x4[scaleInterY]
}

func (e *Encoder) chromaLevel4x4(intra bool, plane int) *scale4x4 {
	if intra {
		return &e.level4x4[scaleIntraU+plane]
	}
	return &e.level4x4[scaleInterU+plane]
}

func (e *Encoder) chromaQuant4x4(intra bool, plane int) *scale4x4 {
	if intra {
		return &e.quant4x4[scaleIntraU+plane]
	}
	return &e.quant4x4[scaleInterU+plane]
}
