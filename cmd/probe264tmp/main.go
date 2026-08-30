package main

import (
	"fmt"
	"math/rand"

	"github.com/oops1/go264/internal/transform"
)

func absI32(v int32) int32 {
	if v < 0 {
		return -v
	}
	return v
}

func probeACMode(seed int64, intra bool, iters int) [52]int32 {
	rng := rand.New(rand.NewSource(seed))
	var maxErrs [52]int32
	for qp := 0; qp < 52; qp++ {
		var maxErr int32
		for n := 0; n < iters; n++ {
			var orig transform.Block
			for i := range orig {
				orig[i] = int32(rng.Intn(511) - 255)
			}
			b := orig
			transform.Forward4x4(&b)
			transform.Quant4x4(&b, qp, intra)
			transform.Dequant4x4(&b, qp, false)
			transform.Inverse4x4(&b)
			for i := range b {
				e := absI32(b[i] - orig[i])
				if e > maxErr {
					maxErr = e
				}
			}
		}
		maxErrs[qp] = maxErr
	}
	return maxErrs
}

func probeLumaDCMode(seed int64, iters int) [52]int64 {
	rng := rand.New(rand.NewSource(seed))
	var res [52]int64
	for qp := 0; qp < 52; qp++ {
		var maxErr int64
		for n := 0; n < iters; n++ {
			var orig transform.Block
			for i := range orig {
				orig[i] = int32(rng.Intn(4096) - 2048)
			}
			bi := orig
			transform.QuantLumaDC(&bi, qp, true)
			transform.DequantLumaDC(&bi, qp)
			for i := range bi {
				target := int64(orig[i]) * 8
				e := target - int64(bi[i])
				if e < 0 {
					e = -e
				}
				if e > maxErr {
					maxErr = e
				}
			}
			be := orig
			transform.QuantLumaDC(&be, qp, false)
			transform.DequantLumaDC(&be, qp)
			for i := range be {
				target := int64(orig[i]) * 8
				e := target - int64(be[i])
				if e < 0 {
					e = -e
				}
				if e > maxErr {
					maxErr = e
				}
			}
		}
		res[qp] = maxErr
	}
	return res
}

func probeChromaDCMode(seed int64, iters int) [52]int64 {
	rng := rand.New(rand.NewSource(seed))
	var res [52]int64
	for qp := 0; qp < 52; qp++ {
		var maxErr int64
		for n := 0; n < iters; n++ {
			var orig transform.ChromaDC
			for i := range orig {
				orig[i] = int32(rng.Intn(4096) - 2048)
			}
			bi := orig
			transform.QuantChromaDC(&bi, qp, true)
			transform.DequantChromaDC(&bi, qp)
			for i := range bi {
				target := int64(orig[i]) * 4
				e := target - int64(bi[i])
				if e < 0 {
					e = -e
				}
				if e > maxErr {
					maxErr = e
				}
			}
			be := orig
			transform.QuantChromaDC(&be, qp, false)
			transform.DequantChromaDC(&be, qp)
			for i := range be {
				target := int64(orig[i]) * 4
				e := target - int64(be[i])
				if e < 0 {
					e = -e
				}
				if e > maxErr {
					maxErr = e
				}
			}
		}
		res[qp] = maxErr
	}
	return res
}

func probeMonotonic(seed int64, blocks int) (violations int) {
	rng := rand.New(rand.NewSource(seed))
	for n := 0; n < blocks; n++ {
		var orig transform.Block
		for i := range orig {
			orig[i] = int32(rng.Intn(511) - 255)
		}
		prevCount := 17
		for qp := 0; qp < 52; qp++ {
			b := orig
			transform.Forward4x4(&b)
			transform.Quant4x4(&b, qp, true)
			cnt := 0
			for i := range b {
				if b[i] != 0 {
					cnt++
				}
			}
			if cnt > prevCount {
				violations++
			}
			prevCount = cnt
		}
	}
	return
}

func probeQP0(seed int64, iters int) int32 {
	rng := rand.New(rand.NewSource(seed))
	var maxErr int32
	for n := 0; n < iters; n++ {
		var orig transform.Block
		for i := range orig {
			orig[i] = int32(rng.Intn(511) - 255)
		}
		b := orig
		transform.Forward4x4(&b)
		transform.Quant4x4(&b, 0, true)
		transform.Dequant4x4(&b, 0, false)
		transform.Inverse4x4(&b)
		for i := range b {
			e := absI32(b[i] - orig[i])
			if e > maxErr {
				maxErr = e
			}
		}
	}
	return maxErr
}

func main() {
	intra := probeACMode(111, true, 8000)
	inter := probeACMode(222, false, 8000)
	fmt.Println("AC intra:", intra)
	fmt.Println("AC inter:", inter)

	lumaDC := probeLumaDCMode(333, 3000)
	fmt.Println("LumaDC:", lumaDC)

	chromaDC := probeChromaDCMode(444, 3000)
	fmt.Println("ChromaDC:", chromaDC)

	v := probeMonotonic(555, 200)
	fmt.Println("monotonic violations over 200 blocks:", v)

	q0 := probeQP0(666, 20000)
	fmt.Println("qp0 max err over 20000:", q0)
}
