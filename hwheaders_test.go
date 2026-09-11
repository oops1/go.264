package go264

import (
	"bytes"
	"testing"

	"github.com/oops1/go.264/internal/nal"
)

func annexBWithout(pkt []byte, drop ...nal.Type) []byte {
	var out []byte
	for _, u := range nal.SplitAnnexB(pkt) {
		if len(u) == 0 {
			continue
		}
		skip := false
		for _, d := range drop {
			if nal.Type(u[0]&0x1f) == d {
				skip = true
			}
		}
		if !skip {
			out = append(out, 0, 0, 0, 1)
			out = append(out, u...)
		}
	}
	return out
}

func countUnits(pkt []byte, want nal.Type) int {
	n := 0
	for _, u := range nal.SplitAnnexB(pkt) {
		if len(u) > 0 && nal.Type(u[0]&0x1f) == want {
			n++
		}
	}
	return n
}

func decodedPictures(stream []byte) int {
	d := NewDecoderWithConfig(DecoderConfig{ForceSoftware: true})
	defer d.Close()
	pics, _ := d.Decode(stream)
	rest, _ := d.Flush()
	return len(pics) + len(rest)
}

func TestAHardwareStreamCarriesParameterSetsAtEveryIDR(t *testing.T) {
	const w, h = 176, 144
	cpu, err := NewEncoder(EncoderConfig{Width: w, Height: h, FPSNum: 25, FPSDen: 1, GOPSize: 4, QP: 26,
		ForceSoftware: true})
	if err != nil {
		t.Fatal(err)
	}
	defer cpu.Close()
	var packets [][]byte
	for i := 0; i < 8; i++ {
		pkt, err := cpu.Encode(pattern(w, h, i))
		if err != nil {
			t.Fatal(err)
		}
		packets = append(packets, pkt)
	}
	if countUnits(packets[4], nal.TypeSliceIDR) == 0 {
		t.Fatal("frame 4 is not an IDR, so this test proves nothing")
	}
	packets[4] = annexBWithout(packets[4], nal.TypeSPS, nal.TypePPS)

	joinedLate := bytes.Join(packets[4:], nil)
	if n := decodedPictures(joinedLate); n != 0 {
		t.Fatalf("a decoder joining at an IDR with no parameter sets decoded %d pictures, so the stripped stream is not the broken case", n)
	}

	e := &Encoder{}
	var wrapped [][]byte
	for _, p := range packets {
		out, err := e.fromHardware(p, nil)
		if err != nil {
			t.Fatal(err)
		}
		wrapped = append(wrapped, out)
	}
	if countUnits(wrapped[4], nal.TypeSPS) != 1 || countUnits(wrapped[4], nal.TypePPS) != 1 {
		t.Fatal("the IDR that arrived without parameter sets did not get them")
	}
	if n := decodedPictures(bytes.Join(wrapped[4:], nil)); n != 4 {
		t.Fatalf("a decoder joining at the second IDR decoded %d pictures, want 4", n)
	}
	for _, i := range []int{0, 1, 2, 3, 5, 6, 7} {
		if !bytes.Equal(wrapped[i], packets[i]) {
			t.Fatalf("packet %d already carried what it needed and was changed anyway", i)
		}
	}
}

func TestAnIDRBeforeAnyParameterSetsIsLeftAlone(t *testing.T) {
	idr := []byte{0, 0, 0, 1, 0x65, 0x88, 0x84}
	out, err := (&Encoder{}).fromHardware(idr, nil)
	if err != nil || !bytes.Equal(out, idr) {
		t.Fatalf("fromHardware = % x, %v; with nothing remembered there is nothing to add", out, err)
	}
}

func TestRepeatedParameterSetsAloneDoNotDemandTheProcessor(t *testing.T) {
	cfg := EncoderConfig{Width: 176, Height: 144, FPSNum: 25, FPSDen: 1, GOPSize: 30, QP: 22,
		RepeatParameterSets: true}
	if cfg.needsSoftware() {
		t.Fatal("RepeatParameterSets on its own keeps the encoder on the processor, although every IDR already carries the parameter sets and without intra refresh there is nothing else to repeat them at")
	}
	cfg.IntraRefresh = 6
	if !cfg.needsSoftware() {
		t.Fatal("intra refresh no longer keeps the encoder on the processor")
	}
}

type forcingEncoder struct {
	forced int
}

func (f *forcingEncoder) Encode(i420 []byte) ([]byte, error) { return nil, nil }
func (f *forcingEncoder) Drain() ([]byte, error)             { return nil, nil }
func (f *forcingEncoder) Close() error                       { return nil }
func (f *forcingEncoder) ForceKeyFrame()                     { f.forced++ }

type silentEncoder struct{}

func (silentEncoder) Encode(i420 []byte) ([]byte, error) { return nil, nil }
func (silentEncoder) Drain() ([]byte, error)             { return nil, nil }
func (silentEncoder) Close() error                       { return nil }

func TestForceKeyFrameReachesAHardwareEncoderThatCanDoIt(t *testing.T) {
	hw := &forcingEncoder{}
	e := &Encoder{hw: hw, backend: "fake"}
	e.ForceKeyFrame()
	if hw.forced != 1 {
		t.Fatalf("the hardware encoder was asked for a key frame %d times, want 1", hw.forced)
	}
	(&Encoder{hw: silentEncoder{}, backend: "fake"}).ForceKeyFrame()
}
