package decoder

const (
	mbTypeINxN = iota
	mbTypeI16x16
	mbTypeIPCM
	mbTypePSkip
	mbTypeP16x16
	mbTypeP16x8
	mbTypeP8x16
	mbTypeP8x8
	mbTypeP8x8Ref0
)

type mbTypeInfo struct {
	kind            int
	intra16PredMode int
	cbpChroma       int
	cbpLuma         int
}

func intraMBType(v uint32) (mbTypeInfo, bool) {
	switch {
	case v == 0:
		return mbTypeInfo{kind: mbTypeINxN}, true
	case v >= 1 && v <= 24:
		idx := int(v) - 1
		info := mbTypeInfo{
			kind:            mbTypeI16x16,
			intra16PredMode: idx % 4,
			cbpChroma:       idx / 4 % 3,
		}
		if idx >= 12 {
			info.cbpLuma = 15
		}
		return info, true
	case v == 25:
		return mbTypeInfo{kind: mbTypeIPCM}, true
	}
	return mbTypeInfo{}, false
}

func interMBType(v uint32) (mbTypeInfo, bool) {
	switch v {
	case 0:
		return mbTypeInfo{kind: mbTypeP16x16}, true
	case 1:
		return mbTypeInfo{kind: mbTypeP16x8}, true
	case 2:
		return mbTypeInfo{kind: mbTypeP8x16}, true
	case 3:
		return mbTypeInfo{kind: mbTypeP8x8}, true
	case 4:
		return mbTypeInfo{kind: mbTypeP8x8Ref0}, true
	}
	return mbTypeInfo{}, false
}

var golombToIntraCBP = [48]uint8{
	47, 31, 15, 0, 23, 27, 29, 30, 7, 11, 13, 14, 39, 43, 45, 46,
	16, 3, 5, 10, 12, 19, 21, 26, 28, 35, 37, 42, 44, 1, 2, 4,
	8, 17, 18, 20, 24, 6, 9, 22, 25, 32, 33, 34, 36, 40, 38, 41,
}

var golombToInterCBP = [48]uint8{
	0, 16, 1, 2, 4, 8, 32, 3, 5, 10, 12, 15, 47, 7, 11, 13,
	14, 6, 9, 31, 35, 37, 42, 44, 33, 34, 36, 40, 39, 43, 45, 46,
	17, 18, 20, 24, 19, 21, 26, 28, 23, 27, 29, 30, 22, 25, 38, 41,
}

var blockX = [16]int{0, 4, 0, 4, 8, 12, 8, 12, 0, 4, 0, 4, 8, 12, 8, 12}

var blockY = [16]int{0, 0, 4, 4, 0, 0, 4, 4, 8, 8, 12, 12, 8, 8, 12, 12}

var chromaBlockX = [4]int{0, 4, 0, 4}

var chromaBlockY = [4]int{0, 0, 4, 4}
