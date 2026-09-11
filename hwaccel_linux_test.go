//go:build linux && (amd64 || arm64) && !go264_nohwaccel

package go264

import (
	"slices"
	"testing"

	"github.com/oops1/go.264/internal/nal"
)

func TestTheLinuxBackendsNeedNoImportOfTheirOwn(t *testing.T) {
	if got, want := Backends(), []string{"cpu", "nvenc", "vaapi"}; !slices.Equal(got, want) {
		t.Fatalf("Backends() = %v, want %v: NVENC is tried before VA-API, and both come with the codec", got, want)
	}
}

func TestThePublicEncoderUsesTheHardwareThisMachineHas(t *testing.T) {
	const w, h, frames = 176, 144, 12
	enc, err := NewEncoder(EncoderConfig{Width: w, Height: h, FPSNum: 25, FPSDen: 1, GOPSize: 5, QP: 26})
	if err != nil {
		t.Fatal(err)
	}
	defer enc.Close()
	if enc.Backend() == "cpu" {
		t.Skip("no hardware encoder on this machine accepted the configuration")
	}
	var stream []byte
	for i := 0; i < frames; i++ {
		pkt, err := enc.Encode(pattern(w, h, i))
		if err != nil {
			t.Fatalf("%s, frame %d: %v", enc.Backend(), i, err)
		}
		if countUnits(pkt, nal.TypeSliceIDR) > 0 && (countUnits(pkt, nal.TypeSPS) == 0 || countUnits(pkt, nal.TypePPS) == 0) {
			t.Fatalf("%s, frame %d: an IDR left the encoder without parameter sets", enc.Backend(), i)
		}
		stream = append(stream, pkt...)
	}
	tail, err := enc.Flush()
	if err != nil {
		t.Fatalf("%s: Flush: %v", enc.Backend(), err)
	}
	stream = append(stream, tail...)
	d := NewDecoderWithConfig(DecoderConfig{ForceSoftware: true})
	defer d.Close()
	pics, err := d.Decode(stream)
	if err != nil {
		t.Fatalf("our decoder refused what %s wrote: %v", enc.Backend(), err)
	}
	rest, err := d.Flush()
	if err != nil {
		t.Fatalf("our decoder refused to flush what %s wrote: %v", enc.Backend(), err)
	}
	pics = append(pics, rest...)
	if len(pics) != frames {
		t.Fatalf("%s: %d frames in, %d pictures out", enc.Backend(), frames, len(pics))
	}
	for i, p := range pics {
		if p.Width != w || p.Height != h {
			t.Fatalf("%s: picture %d is %dx%d", enc.Backend(), i, p.Width, p.Height)
		}
	}
	t.Logf("%s: %d frames, %d bytes, all read back by our decoder", enc.Backend(), frames, len(stream))
}
