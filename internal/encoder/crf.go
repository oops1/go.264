package encoder

import (
	"math"

	"github.com/oops1/go.264/internal/syntax"
)

func qp2qscale(qp float64) float64 { return 0.85 * math.Exp2((qp-12)/6) }

func qscale2qp(qscale float64) float64 { return 12 + 6*math.Log2(qscale/0.85) }

const (
	crfBaseComplexityPerMB  = 210
	crfBaseComplexityPerMBB = 350

	crfQCompDefault   = 0.8
	crfIPRatioDefault = 1.4
	crfPBRatioDefault = 1.3

	crfQPFloorDrop = 6.0
	crfQPStep      = 4.0
)

type constantQuality struct {
	baseComplexity  float64
	rateFactorConst float64
	exponent        float64
	ipRatio         float64
	pbRatio         float64
	floor           float64
	step            float64
	last            [5]float64
}

func newConstantQuality(cfg Config) *constantQuality {
	if cfg.RateFactor <= 0 {
		return nil
	}
	perMB := float64(crfBaseComplexityPerMB)
	if cfg.BFrames > 0 {
		perMB = crfBaseComplexityPerMBB
	}
	base := float64(((cfg.Width+15)/16)*((cfg.Height+15)/16)) * perMB
	exponent := 1 - cfg.QComp
	c := &constantQuality{
		baseComplexity:  base,
		rateFactorConst: math.Pow(base, exponent) / qp2qscale(cfg.RateFactor),
		exponent:        exponent,
		ipRatio:         cfg.IPRatio,
		pbRatio:         cfg.PBRatio,
		floor:           cfg.RateFactor - crfQPFloorDrop,
		step:            crfQPStep,
	}
	for i := range c.last {
		c.last[i] = -1
	}
	return c
}

func (c *constantQuality) qp(complexity float64, t syntax.SliceType) float64 {
	if !(complexity > 0) || math.IsInf(complexity, 0) {
		complexity = c.baseComplexity
	}
	qscale := math.Pow(complexity, c.exponent) / c.rateFactorConst
	switch {
	case t.IsI():
		qscale /= c.ipRatio
	case t.IsB():
		qscale *= c.pbRatio
	}
	qp := qscale2qp(qscale)
	if qp < c.floor {
		qp = c.floor
	}
	if last := c.last[t.Base()]; last >= 0 {
		if qp > last+c.step {
			qp = last + c.step
		}
		if qp < last-c.step {
			qp = last - c.step
		}
	}
	return qp
}

func (c *constantQuality) coded(qp int, t syntax.SliceType) {
	c.last[t.Base()] = float64(qp)
}
