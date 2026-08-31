package cabac

const (
	offMBTypeI          = 3
	offMBQPDelta        = 60
	offIntraChromaPred  = 64
	offPrevIntraPred    = 68
	offRemIntraPred     = 69
	offCodedBlockPatLum = 73
	offCodedBlockPatChr = 77
)

const (
	CBPUnavailable = 0x0F
	CBPPCM         = 0x2F
)

func (d *Decoder) IntraMBType(inc int) int {
	if d.DecodeDecision(offMBTypeI+inc) == 0 {
		return 0
	}
	if d.DecodeTerminate() == 1 {
		return 25
	}
	mbType := 1
	if d.DecodeDecision(offMBTypeI+3) == 1 {
		mbType += 12
	}
	if d.DecodeDecision(offMBTypeI+4) == 1 {
		mbType += 4 + 4*int(d.DecodeDecision(offMBTypeI+5))
	}
	mbType += 2 * int(d.DecodeDecision(offMBTypeI+6))
	mbType += int(d.DecodeDecision(offMBTypeI + 7))
	return mbType
}

func (d *Decoder) IntraChromaPredMode(inc int) int {
	if d.DecodeDecision(offIntraChromaPred+inc) == 0 {
		return 0
	}
	if d.DecodeDecision(offIntraChromaPred+3) == 0 {
		return 1
	}
	if d.DecodeDecision(offIntraChromaPred+3) == 0 {
		return 2
	}
	return 3
}

func (d *Decoder) Intra4x4PredMode(predMode int) int {
	if d.DecodeDecision(offPrevIntraPred) == 1 {
		return predMode
	}
	mode := int(d.DecodeDecision(offRemIntraPred))
	mode += 2 * int(d.DecodeDecision(offRemIntraPred))
	mode += 4 * int(d.DecodeDecision(offRemIntraPred))
	if mode >= predMode {
		mode++
	}
	return mode
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func (d *Decoder) CodedBlockPatternLuma(leftCBP, topCBP int) int {
	cbp := 0
	ctx := boolToInt(leftCBP&0x02 == 0) + 2*boolToInt(topCBP&0x04 == 0)
	cbp |= int(d.DecodeDecision(offCodedBlockPatLum + ctx))
	ctx = boolToInt(cbp&0x01 == 0) + 2*boolToInt(topCBP&0x08 == 0)
	cbp |= int(d.DecodeDecision(offCodedBlockPatLum+ctx)) << 1
	ctx = boolToInt(leftCBP&0x08 == 0) + 2*boolToInt(cbp&0x01 == 0)
	cbp |= int(d.DecodeDecision(offCodedBlockPatLum+ctx)) << 2
	ctx = boolToInt(cbp&0x04 == 0) + 2*boolToInt(cbp&0x02 == 0)
	cbp |= int(d.DecodeDecision(offCodedBlockPatLum+ctx)) << 3
	return cbp
}

func (d *Decoder) CodedBlockPatternChroma(leftCBP, topCBP int) int {
	a := leftCBP >> 4 & 3
	b := topCBP >> 4 & 3
	ctx := 0
	if a > 0 {
		ctx++
	}
	if b > 0 {
		ctx += 2
	}
	if d.DecodeDecision(offCodedBlockPatChr+ctx) == 0 {
		return 0
	}
	ctx = 4
	if a == 2 {
		ctx++
	}
	if b == 2 {
		ctx += 2
	}
	return 1 + int(d.DecodeDecision(offCodedBlockPatChr+ctx))
}

func (d *Decoder) MBQPDelta(prevNonZero bool) int {
	if d.DecodeDecision(offMBQPDelta+boolToInt(prevNonZero)) == 0 {
		return 0
	}
	val := 1
	ctx := 2
	for d.DecodeDecision(offMBQPDelta+ctx) == 1 {
		ctx = 3
		val++
		if val > 102 {
			break
		}
	}
	if val&1 == 1 {
		return (val + 1) >> 1
	}
	return -((val + 1) >> 1)
}

func (d *Decoder) EndOfSlice() bool { return d.DecodeTerminate() == 1 }
