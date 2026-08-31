package encoder

import "math"

type rateControl struct {
	enabled    bool
	targetBits float64
	bufferSize float64
	buffer     float64
	baseQP     int
	minQP      int
	maxQP      int

	complexityInter float64
	complexityIntra float64

	framesCoded  int
	bitsProduced float64
}

func newRateControl(cfg Config) *rateControl {
	rc := &rateControl{
		baseQP: cfg.QP,
		minQP:  8,
		maxQP:  50,
	}
	if cfg.BitrateKbps <= 0 {
		return rc
	}
	fps := float64(cfg.FPSNum) / float64(cfg.FPSDen)
	if fps <= 0 {
		fps = 25
	}
	rc.enabled = true
	rc.targetBits = float64(cfg.BitrateKbps) * 1000 / fps
	rc.bufferSize = rc.targetBits * 8
	return rc
}

func (rc *rateControl) intraTarget() float64 { return rc.targetBits * 4 }

func (rc *rateControl) frameQP(idr bool) int {
	if !rc.enabled {
		return rc.baseQP
	}
	complexity := rc.complexityInter
	target := rc.targetBits
	if idr {
		complexity = rc.complexityIntra
		target = rc.intraTarget()
	}
	if complexity <= 0 {
		return rc.baseQP
	}
	target -= rc.buffer * 0.5
	if minimum := rc.targetBits * 0.1; target < minimum {
		target = minimum
	}
	qp := 6 * math.Log2(complexity/target)
	return rc.clamp(int(math.Round(qp)))
}

func (rc *rateControl) clamp(qp int) int {
	if qp < rc.minQP {
		return rc.minQP
	}
	if qp > rc.maxQP {
		return rc.maxQP
	}
	return qp
}

func (rc *rateControl) update(bits, qp int, idr bool) {
	rc.framesCoded++
	rc.bitsProduced += float64(bits)
	if !rc.enabled {
		return
	}
	observed := float64(bits) * math.Exp2(float64(qp)/6)
	if idr {
		rc.complexityIntra = blend(rc.complexityIntra, observed, 0.5)
	} else {
		rc.complexityInter = blend(rc.complexityInter, observed, 0.4)
		if rc.complexityIntra == 0 {
			rc.complexityIntra = observed * 4
		}
	}
	target := rc.targetBits
	if idr {
		target = rc.intraTarget()
	}
	rc.buffer += float64(bits) - target
	if rc.buffer > rc.bufferSize {
		rc.buffer = rc.bufferSize
	}
	if rc.buffer < -rc.bufferSize {
		rc.buffer = -rc.bufferSize
	}
}

func blend(old, observed, weight float64) float64 {
	if old <= 0 {
		return observed
	}
	return old*(1-weight) + observed*weight
}
