package cabac

const (
	CatIntra16x16DC = 0
	CatIntra16x16AC = 1
	CatLuma4x4      = 2
	CatChromaDC     = 3
	CatChromaAC     = 4
)

const (
	offCodedBlockFlag  = 85
	offSignificant     = 105
	offLastSignificant = 166
	offAbsLevel        = 227
)

var (
	catOffsetCodedBlockFlag = [5]int{0, 4, 8, 12, 16}
	catOffsetSignificant    = [5]int{0, 15, 29, 44, 47}
	catOffsetAbsLevel       = [5]int{0, 10, 20, 30, 39}
	maxCoeffForCat          = [5]int{16, 15, 16, 4, 15}
)

func MaxCoeff(cat int) int { return maxCoeffForCat[cat] }

func (d *Decoder) CodedBlockFlag(cat, condTermA, condTermB int) bool {
	inc := condTermA + 2*condTermB
	return d.DecodeDecision(offCodedBlockFlag+catOffsetCodedBlockFlag[cat]+inc) == 1
}

func significanceInc(cat, idx, numC8x8 int) int {
	if cat == CatChromaDC {
		v := idx / numC8x8
		if v > 2 {
			v = 2
		}
		return v
	}
	return idx
}

func (d *Decoder) absLevelInc(cat, binIdx, numGt1, numEq1 int) int {
	base := offAbsLevel + catOffsetAbsLevel[cat]
	if binIdx == 0 {
		if numGt1 != 0 {
			return base
		}
		v := 1 + numEq1
		if v > 4 {
			v = 4
		}
		return base + v
	}
	limit := 4
	if cat == CatChromaDC {
		limit = 3
	}
	v := numGt1
	if v > limit {
		v = limit
	}
	return base + 5 + v
}

func (d *Decoder) absLevelMinus1(cat, numGt1, numEq1 int) int {
	prefix := 0
	for prefix < 14 {
		if d.DecodeDecision(d.absLevelInc(cat, prefix, numGt1, numEq1)) == 0 {
			break
		}
		prefix++
	}
	if prefix < 14 {
		return prefix
	}
	value := 14
	k := 0
	for d.DecodeBypass() == 1 {
		value += 1 << uint(k)
		k++
		if k > 24 {
			break
		}
	}
	for k--; k >= 0; k-- {
		value += int(d.DecodeBypass()) << uint(k)
	}
	return value
}

func (d *Decoder) ResidualBlock(coeffs []int32, cat, condTermA, condTermB, numC8x8 int) int {
	for i := range coeffs {
		coeffs[i] = 0
	}
	if !d.CodedBlockFlag(cat, condTermA, condTermB) {
		return 0
	}
	maxNumCoeff := len(coeffs)

	var significant [16]bool
	numCoeff := maxNumCoeff
	for i := 0; i < numCoeff-1; i++ {
		sigInc := significanceInc(cat, i, numC8x8)
		if d.DecodeDecision(offSignificant+catOffsetSignificant[cat]+sigInc) == 1 {
			significant[i] = true
			if d.DecodeDecision(offLastSignificant+catOffsetSignificant[cat]+sigInc) == 1 {
				numCoeff = i + 1
			}
		}
	}
	significant[numCoeff-1] = true

	numGt1, numEq1 := 0, 0
	total := 0
	for i := numCoeff - 1; i >= 0; i-- {
		if !significant[i] {
			continue
		}
		level := d.absLevelMinus1(cat, numGt1, numEq1) + 1
		if level > 1 {
			numGt1++
		} else {
			numEq1++
		}
		if d.DecodeBypass() == 1 {
			coeffs[i] = int32(-level)
		} else {
			coeffs[i] = int32(level)
		}
		total++
	}
	return total
}
