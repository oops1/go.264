//go:build linux && (amd64 || arm64)

package nvenc

import (
	"testing"

	"github.com/oops1/go.264/internal/hwaccel"
	"github.com/oops1/go.264/internal/testutil/sliceqp"
)

func TestRateControlTakesTheBitrateOrElseTheQuantiser(t *testing.T) {
	cases := []struct {
		p           hwaccel.EncoderParams
		bitrate, qp int
	}{
		{hwaccel.EncoderParams{QP: 30}, 0, 30},
		{hwaccel.EncoderParams{QP: 30, BitrateKbps: 1500}, 1500000, 0},
		{hwaccel.EncoderParams{}, 0, hwaccel.DefaultQP},
		{hwaccel.EncoderParams{QP: 60}, 0, 51},
	}
	for _, c := range cases {
		if bitrate, qp := rateControl(c.p); bitrate != c.bitrate || qp != c.qp {
			t.Errorf("%+v: bitrate %d QP %d, want %d and %d", c.p, bitrate, qp, c.bitrate, c.qp)
		}
	}
}

func TestTheAdapterHoldsAConstantQuantiser(t *testing.T) {
	requireAdapter(t)
	sizes := map[int]int{}
	for _, qp := range []int{16, 22, 38, 46} {
		cfg := probeConfig()
		cfg.BitrateBitsPerSecond = 0
		cfg.QP = qp
		enc, err := Open(cfg)
		if err != nil {
			t.Fatalf("QP %d: %v", qp, err)
		}
		var stream []byte
		for i := 0; i < probeFrames; i++ {
			out, err := enc.Encode(movingPattern(i))
			if err != nil {
				closeWithin(t, enc)
				t.Fatalf("QP %d frame %d: %v", qp, i, err)
			}
			stream = append(stream, out...)
		}
		stream = append(stream, drainWithin(t, enc)...)
		closeWithin(t, enc)
		slices, err := sliceqp.Read(stream)
		if err != nil {
			t.Fatalf("QP %d: %v", qp, err)
		}
		if len(slices) == 0 {
			t.Fatalf("QP %d: no slices", qp)
		}
		for i, s := range slices {
			if s.QP != qp {
				t.Fatalf("slice %d (IDR %v) is coded at QP %d, asked for %d", i, s.IDR, s.QP, qp)
			}
		}
		sizes[qp] = len(stream)
		t.Logf("QP %d: %d slices, %d bytes", qp, len(slices), len(stream))
	}
	if sizes[16] <= sizes[22] || sizes[22] <= sizes[38] || sizes[38] <= sizes[46] {
		t.Fatalf("stream sizes by quantiser %v do not fall as the quantiser rises", sizes)
	}
}
