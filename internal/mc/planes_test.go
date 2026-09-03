package mc

import (
	"math/rand"
	"testing"
)

func TestPlanePredictionMatchesTheDirectFilter(t *testing.T) {
	const stride, rows = 96, 80
	r := rand.New(rand.NewSource(3))
	src := make([]byte, stride*rows)
	for i := range src {
		src[i] = byte(r.Intn(256))
	}
	var p Planes
	p.Build(src, stride, rows)
	if !p.Ready() {
		t.Fatal("the planes were not built")
	}

	sizes := [][2]int{{16, 16}, {16, 8}, {8, 16}, {8, 8}, {8, 4}, {4, 8}, {4, 4}, {2, 2}, {1, 1}}
	checked := 0
	for _, size := range sizes {
		w, h := size[0], size[1]
		for mvy := -12; mvy <= 12; mvy++ {
			for mvx := -12; mvx <= 12; mvx++ {
				for _, base := range []int{20*stride + 20, 30*stride + 41, 44*stride + 7} {
					want := make([]byte, stride*rows)
					got := make([]byte, stride*rows)
					PredictLuma(want, stride, base, src, stride, base, w, h, mvx, mvy)
					PredictLumaPlanes(got, stride, base, src, stride, base, &p, w, h, mvx, mvy)
					checked++
					for y := 0; y < h; y++ {
						for x := 0; x < w; x++ {
							i := base + y*stride + x
							if want[i] != got[i] {
								t.Fatalf("%dx%d block at mv (%d,%d) offset %d: sample (%d,%d) is %d, the direct filter gives %d",
									w, h, mvx, mvy, base, x, y, got[i], want[i])
							}
						}
					}
				}
			}
		}
	}
	t.Logf("%d block predictions match the direct filter byte for byte", checked)
}
