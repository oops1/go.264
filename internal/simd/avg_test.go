package simd

import (
	"math/rand"
	"testing"
)

func TestAvgBytesMatchesTheReference(t *testing.T) {
	r := rand.New(rand.NewSource(11))
	sizes := [][2]int{{16, 16}, {16, 8}, {8, 16}, {8, 8}, {8, 4}, {4, 8}, {4, 4}, {2, 2}, {1, 1}, {12, 3}}
	for _, size := range sizes {
		w, h := size[0], size[1]
		for round := 0; round < 200; round++ {
			const stride = 48
			a := make([]byte, stride*(h+4))
			b := make([]byte, stride*(h+4))
			for i := range a {
				a[i] = byte(r.Intn(256))
				b[i] = byte(r.Intn(256))
			}
			aOff := stride + r.Intn(stride-w)
			bOff := stride + r.Intn(stride-w)
			want := make([]byte, stride*(h+4))
			got := make([]byte, stride*(h+4))
			avgBytesGeneric(want, stride, stride, a, stride, aOff, b, stride, bOff, w, h)
			AvgBytes(got, stride, stride, a, stride, aOff, b, stride, bOff, w, h)
			for i := range want {
				if want[i] != got[i] {
					t.Fatalf("%dx%d round %d: byte %d is %d, want %d", w, h, round, i, got[i], want[i])
				}
			}
		}
	}
}
