package vaapi

import "github.com/oops1/go.264/internal/hwaccel"

const backendName = "vaapi"

func init() {
	hwaccel.Register(hwaccel.Backend{
		Name:        backendName,
		ProbeEncode: probeEncode,
	})
}

type registeredEncoder struct {
	enc *Encoder
}

func probeEncode(p hwaccel.EncoderParams) (hwaccel.Encoder, bool) {
	if !Available() {
		return nil, false
	}
	num, den := p.FPSNum, p.FPSDen
	if num <= 0 || den <= 0 {
		num, den = 30, 1
	}
	enc, err := Open(Config{
		Width:     p.Width,
		Height:    p.Height,
		FPSNum:    num,
		FPSDen:    den,
		GOPLength: p.GOPSize,
		QP:        p.QP,
	})
	if err != nil {
		return nil, false
	}
	return &registeredEncoder{enc: enc}, true
}

func (r *registeredEncoder) Encode(i420 []byte) ([]byte, error) { return r.enc.Encode(i420) }

func (r *registeredEncoder) Drain() ([]byte, error) { return r.enc.Drain() }

func (r *registeredEncoder) Close() error { return r.enc.Close() }
