package encoder

import (
	"math"
	"math/rand"
	"testing"

	"github.com/oops1/go.264/internal/transform"
)

func randomResidual4x4(rng *rand.Rand, spread int) transform.Block {
	var b transform.Block
	for i := range b {
		b[i] = int32(rng.Intn(2*spread+1) - spread)
	}
	return b
}

func modelDistortion4x4(coef *transform.Block, levels *[16]int32, start, qp int, qs, ls *scale4x4) float64 {
	var t trellisBlock
	n := 16 - start
	t.prepare(n, uint(15+qp/6), 0)
	mf := &qs[qp%6]
	scale := &ls[qp%6]
	for i := 0; i < n; i++ {
		p := transform.ZigZagIndex(i + start)
		t.set(i, int64(coef[p]), mf[p], scale[p], invGain4x4[p], trellisScale4x4)
	}
	total := 0.0
	for i := 0; i < n; i++ {
		total += t.distortion(i, levels[i+start])
	}
	return total
}

func trueDistortion4x4(residual, rec *transform.Block) float64 {
	total := 0.0
	for i := range residual {
		d := float64(residual[i] - rec[i])
		total += d * d
	}
	return total
}

func TestTrellisDistortionMatchesPixelError4x4(t *testing.T) {
	rng := rand.New(rand.NewSource(9))
	for _, matrix := range []ScalingMatrix{ScalingMatrixFlat, ScalingMatrixJVT} {
		e, err := New(Config{Width: 32, Height: 32, QP: 26, ScalingMatrix: matrix})
		if err != nil {
			t.Fatal(err)
		}
		for _, intra := range []bool{true, false} {
			qs, ls := e.lumaQuant4x4(intra), e.lumaLevel4x4(intra)
			for qp := 0; qp <= 51; qp++ {
				var model, truth float64
				for n := 0; n < 64; n++ {
					residual := randomResidual4x4(rng, 96)
					coef := residual
					transform.Forward4x4(&coef)
					orig := coef
					quant4x4(&coef, qp, qs, intra)
					var scan [16]int32
					transform.BlockToScan(&scan, &coef)
					model += modelDistortion4x4(&orig, &scan, 0, qp, qs, ls)

					rec := coef
					dequant4x4(&rec, qp, ls, false)
					transform.Inverse4x4(&rec)
					truth += trueDistortion4x4(&residual, &rec)
				}
				assertModelAgrees(t, "luma4x4", matrix, intra, qp, model, truth)
			}
		}
	}
}

func assertModelAgrees(t *testing.T, label string, matrix ScalingMatrix, intra bool, qp int, model, truth float64) {
	t.Helper()
	const floor = 4096
	if truth < floor && model < floor {
		return
	}
	ratio := model / truth
	if ratio < 0.9 || ratio > 1.1 {
		t.Errorf("%s matrix %d intra %v qp %d: the model predicts %.0f but the reconstruction loses %.0f, a ratio of %.3f",
			label, matrix, intra, qp, model, truth, ratio)
	}
}

func modelDistortion8x8(coef *transform.Block8x8, levels *[64]int32, qp int,
	qs *transform.QuantScale8x8, ls *transform.LevelScale8x8) float64 {
	var t trellisBlock
	t.prepare(64, uint(16+qp/6), 0)
	mf := &qs[qp%6]
	scale := &ls[qp%6]
	for i := 0; i < 64; i++ {
		p := int(transform.ZigZagScan8x8[i])
		t.set(i, int64(coef[p]), mf[p], scale[p], invGain8x8[p], trellisScale8x8)
	}
	total := 0.0
	for i := 0; i < 64; i++ {
		total += t.distortion(i, levels[i])
	}
	return total
}

func TestTrellisDistortionMatchesPixelError8x8(t *testing.T) {
	rng := rand.New(rand.NewSource(11))
	for _, matrix := range []ScalingMatrix{ScalingMatrixFlat, ScalingMatrixJVT} {
		e, err := New(Config{Width: 32, Height: 32, QP: 26, Transform8x8: true, ScalingMatrix: matrix})
		if err != nil {
			t.Fatal(err)
		}
		for _, intra := range []bool{true, false} {
			qs, ls := e.quantScale8x8(intra), e.levelScale8x8(intra)
			for qp := 0; qp <= 51; qp++ {
				var model, truth float64
				for n := 0; n < 32; n++ {
					var residual transform.Block8x8
					for i := range residual {
						residual[i] = int32(rng.Intn(193) - 96)
					}
					coef := residual
					transform.Forward8x8(&coef)
					orig := coef
					transform.Quant8x8(&coef, qp, qs, intra)
					var scan [64]int32
					block8x8ToScan(&scan, &coef)
					model += modelDistortion8x8(&orig, &scan, qp, qs, ls)

					rec := coef
					transform.Dequant8x8(&rec, qp, ls)
					transform.Inverse8x8(&rec)
					for i := range residual {
						d := float64(residual[i] - rec[i])
						truth += d * d
					}
				}
				assertModelAgrees(t, "luma8x8", matrix, intra, qp, model, truth)
			}
		}
	}
}

func TestTrellisDistortionMatchesPixelErrorLumaDC(t *testing.T) {
	rng := rand.New(rand.NewSource(13))
	for _, matrix := range []ScalingMatrix{ScalingMatrixFlat, ScalingMatrixJVT} {
		e, err := New(Config{Width: 32, Height: 32, QP: 26, ScalingMatrix: matrix})
		if err != nil {
			t.Fatal(err)
		}
		qs, ls := e.lumaQuant4x4(true), e.lumaLevel4x4(true)
		for qp := 0; qp <= 51; qp++ {
			var model, truth float64
			for n := 0; n < 64; n++ {
				var dc transform.Block
				for i := range dc {
					dc[i] = int32(rng.Intn(2001) - 1000)
				}
				want := dc
				transform.HadamardForwardDC4x4(&dc)
				orig := dc
				quantDCLevels(dc[:], qp, qs, true)
				var t2 trellisBlock
				t2.prepare(16, uint(16+qp/6), 0)
				for i := 0; i < 16; i++ {
					t2.set(i, int64(orig[i]), qs[qp%6][0], ls[qp%6][0], 16*invGain4x4[0], trellisScaleDC)
				}
				for i := 0; i < 16; i++ {
					model += t2.distortion(i, dc[i])
				}
				rec := dc
				dequantLumaDC(&rec, qp, ls)
				for i := range rec {
					d := float64(rec[i]) - 4*float64(want[i])
					truth += d * d * invGain4x4[0]
				}
			}
			assertModelAgrees(t, "lumaDC", matrix, true, qp, model, truth)
		}
	}
}

func TestTrellisDistortionMatchesPixelErrorChromaDC(t *testing.T) {
	rng := rand.New(rand.NewSource(17))
	for _, matrix := range []ScalingMatrix{ScalingMatrixFlat, ScalingMatrixJVT} {
		e, err := New(Config{Width: 32, Height: 32, QP: 26, ScalingMatrix: matrix})
		if err != nil {
			t.Fatal(err)
		}
		for _, intra := range []bool{true, false} {
			qs, ls := e.chromaQuant4x4(intra, 0), e.chromaLevel4x4(intra, 0)
			for qp := 0; qp <= 39; qp++ {
				var model, truth float64
				for n := 0; n < 64; n++ {
					var dc transform.ChromaDC
					for i := range dc {
						dc[i] = int32(rng.Intn(2001) - 1000)
					}
					want := dc
					transform.Hadamard2x2(&dc)
					orig := dc
					quantDCLevels(dc[:], qp, qs, intra)
					var t2 trellisBlock
					t2.prepare(4, uint(16+qp/6), 0)
					for i := 0; i < 4; i++ {
						t2.set(i, int64(orig[i]), qs[qp%6][0], ls[qp%6][0], 4*invGain4x4[0], trellisScaleCDC)
					}
					for i := 0; i < 4; i++ {
						model += t2.distortion(i, dc[i])
					}
					rec := dc
					dequantChromaDC(&rec, qp, ls)
					for i := range rec {
						d := float64(rec[i]) - 4*float64(want[i])
						truth += d * d * invGain4x4[0]
					}
				}
				assertModelAgrees(t, "chromaDC", matrix, intra, qp, model, truth)
			}
		}
	}
}

func TestInverseTransformGains(t *testing.T) {
	want4 := [16]float64{}
	n := [4]float64{4, 2.5, 4, 2.5}
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			want4[y*4+x] = n[x] * n[y] / 4096
		}
	}
	for p := 0; p < 16; p++ {
		if math.Abs(invGain4x4[p]-want4[p]) > 1e-9 {
			t.Errorf("4x4 gain at %d is %g, want %g", p, invGain4x4[p], want4[p])
		}
	}
	for p := 0; p < 64; p++ {
		if invGain8x8[p] <= 0 {
			t.Errorf("8x8 gain at %d is %g", p, invGain8x8[p])
		}
	}
}
