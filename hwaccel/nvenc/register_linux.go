package nvenc

import "github.com/oops1/go.264/internal/hwaccel"

const backendName = "nvenc"

func init() {
	hwaccel.Register(hwaccel.Backend{
		Name:        backendName,
		ProbeEncode: probeEncode,
	})
}

type registeredEncoder struct {
	enc *Encoder
}

func defaultBitrate(p hwaccel.EncoderParams) int {
	if p.BitrateKbps > 0 {
		return p.BitrateKbps * 1000
	}
	num, den := p.FPSNum, p.FPSDen
	if num <= 0 || den <= 0 {
		num, den = 30, 1
	}
	rate := p.Width * p.Height * num / den / 10
	if rate < 100000 {
		rate = 100000
	}
	return rate
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
		Width:                p.Width,
		Height:               p.Height,
		FPSNum:               num,
		FPSDen:               den,
		BitrateBitsPerSecond: defaultBitrate(p),
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
