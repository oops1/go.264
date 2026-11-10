//go:build linux && (amd64 || arm64)

package nvenc

import "github.com/oops1/go.264/internal/hwaccel"

const backendName = "nvenc"

func Backend() hwaccel.Backend {
	return hwaccel.Backend{Name: backendName, ProbeEncode: probeEncode}
}

type registeredEncoder struct {
	enc *Encoder
}

func rateControl(p hwaccel.EncoderParams) (bitrate, qp int) {
	if p.BitrateKbps > 0 {
		return p.BitrateKbps * 1000, 0
	}
	return 0, hwaccel.ConstantQP(p.QP)
}

func probeEncode(p hwaccel.EncoderParams) (hwaccel.Encoder, bool) {
	if !Available() {
		return nil, false
	}
	num, den := p.FPSNum, p.FPSDen
	if num <= 0 || den <= 0 {
		num, den = 30, 1
	}
	bitrate, qp := rateControl(p)
	enc, err := Open(Config{
		Width:                p.Width,
		Height:               p.Height,
		FPSNum:               num,
		FPSDen:               den,
		BitrateBitsPerSecond: bitrate,
		QP:                   qp,
		GOPLength:            p.GOPSize,
		Profile:              ProfileMain,
	})
	if err != nil {
		return nil, false
	}
	return &registeredEncoder{enc: enc}, true
}

func (r *registeredEncoder) Encode(i420 []byte) ([]byte, error) { return r.enc.Encode(i420) }

func (r *registeredEncoder) Drain() ([]byte, error) { return r.enc.Drain() }

func (r *registeredEncoder) Close() error { return r.enc.Close() }
