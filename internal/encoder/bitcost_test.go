package encoder

import "testing"

func referenceBitsForUE(v uint32) int {
	n := 1
	for c := uint64(v) + 1; c > 1; c >>= 1 {
		n += 2
	}
	return n
}

func referenceBitsForSE(v int) int {
	code := uint32(0)
	if v > 0 {
		code = uint32(v)*2 - 1
	} else {
		code = uint32(-v) * 2
	}
	n := 1
	for c := code + 1; c > 1; c >>= 1 {
		n += 2
	}
	return n
}

func TestBitCountsMatchTheCountingLoops(t *testing.T) {
	for _, v := range []uint32{0, 1, 2, 3, 6, 7, 8, 1 << 15, 1<<31 - 1, 1 << 31, 0xFFFFFFFE, 0xFFFFFFFF} {
		if got, want := bitsForUE(v), referenceBitsForUE(v); got != want {
			t.Fatalf("bitsForUE(%d) = %d, want %d", v, got, want)
		}
	}
	for v := uint32(0); v < 1<<16; v++ {
		if got, want := bitsForUE(v), referenceBitsForUE(v); got != want {
			t.Fatalf("bitsForUE(%d) = %d, want %d", v, got, want)
		}
	}
	for v := -70000; v <= 70000; v++ {
		if got, want := bitsForSE(v), referenceBitsForSE(v); got != want {
			t.Fatalf("bitsForSE(%d) = %d, want %d", v, got, want)
		}
	}
}
