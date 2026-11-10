package go264

import (
	"bytes"
	"errors"
	"testing"

	"github.com/oops1/go.264/internal/decoder"
	"github.com/oops1/go.264/internal/hwaccel"
)

type stubHardwareDecoder struct {
	dec    *decoder.Decoder
	fed    []byte
	closed bool
}

func (s *stubHardwareDecoder) pictures(f []*Frame) []*hwaccel.Picture {
	var out []*hwaccel.Picture
	for _, fr := range f {
		out = append(out, &hwaccel.Picture{Y: fr.Y, Cb: fr.Cb, Cr: fr.Cr,
			StrideY: fr.StrideY, StrideC: fr.StrideC, Width: fr.Width, Height: fr.Height})
	}
	return out
}

func (s *stubHardwareDecoder) Decode(b []byte) ([]*hwaccel.Picture, error) {
	s.fed = append(s.fed, b...)
	pics, err := s.dec.Decode(b)
	return s.pictures((&Decoder{}).convert(pics)), err
}

func (s *stubHardwareDecoder) Flush() ([]*hwaccel.Picture, error) {
	pics, err := s.dec.Flush()
	return s.pictures((&Decoder{}).convert(pics)), err
}

func (s *stubHardwareDecoder) Close() error {
	s.closed = true
	return nil
}

type stubHardware struct {
	refuse  bool
	opened  []hwaccel.DecoderParams
	decoder *stubHardwareDecoder
}

func (s *stubHardware) open(p hwaccel.DecoderParams) (hwaccel.Decoder, string, bool) {
	s.opened = append(s.opened, p)
	if s.refuse {
		return nil, "", false
	}
	s.decoder = &stubHardwareDecoder{dec: decoder.New()}
	return s.decoder, "stub", true
}

func decodeInChunks(t *testing.T, d *Decoder, chunks [][]byte) ([][]byte, error) {
	t.Helper()
	var out [][]byte
	var errs []error
	for _, c := range chunks {
		frames, err := d.Decode(c)
		errs = append(errs, err)
		for _, f := range frames {
			out = append(out, f.AppendI420(nil))
		}
	}
	frames, err := d.Flush()
	errs = append(errs, err)
	for _, f := range frames {
		out = append(out, f.AppendI420(nil))
	}
	return out, errors.Join(errs...)
}

func accessUnits(t *testing.T, w, h, n, gop int) [][]byte {
	t.Helper()
	enc, err := NewEncoder(EncoderConfig{Width: w, Height: h, FPSNum: 25, FPSDen: 1, GOPSize: gop, QP: 26, ForceSoftware: true})
	if err != nil {
		t.Fatal(err)
	}
	defer enc.Close()
	var units [][]byte
	for i := 0; i < n; i++ {
		pkt, err := enc.Encode(pattern(w, h, i))
		if err != nil {
			t.Fatal(err)
		}
		units = append(units, append([]byte(nil), pkt...))
	}
	return units
}

func inFixedChunks(stream []byte, size int) [][]byte {
	var out [][]byte
	for i := 0; i < len(stream); i += size {
		out = append(out, stream[i:min(i+size, len(stream))])
	}
	return out
}

func TestHandOverToHardwareLosesNoPicture(t *testing.T) {
	const w, h, n, gop = 96, 64, 12, 4
	units := accessUnits(t, w, h, n, gop)
	stream := bytes.Join(units, nil)
	want, err := decodeInChunks(t, NewDecoderWithConfig(DecoderConfig{ForceSoftware: true}), [][]byte{stream})
	if err != nil || len(want) != n {
		t.Fatalf("software decode: %d pictures, %v", len(want), err)
	}
	splitSPS := append([][]byte{units[0][:8], units[0][8:]}, units[1:]...)
	cases := []struct {
		name       string
		chunks     [][]byte
		hardware   bool
		handedOver bool
	}{
		{"one access unit a call", units, true, true},
		{"the first sequence parameter set cut in two", splitSPS, true, true},
		{"chunks of 37 bytes", inFixedChunks(stream, 37), true, true},
		{"chunks of 1000 bytes", inFixedChunks(stream, 1000), true, true},
		{"the whole stream in one call", [][]byte{stream}, true, true},
		{"a backend that refuses", units, false, false},
	}
	for _, c := range cases {
		stub := &stubHardware{refuse: !c.hardware}
		d := NewDecoderWithConfig(DecoderConfig{})
		d.open = stub.open
		got, err := decodeInChunks(t, d, c.chunks)
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
		}
		if len(got) != len(want) {
			t.Errorf("%s: %d pictures out, want %d", c.name, len(got), len(want))
			continue
		}
		for i := range got {
			if !bytes.Equal(got[i], want[i]) {
				t.Errorf("%s: picture %d differs from the software decode", c.name, i)
				break
			}
		}
		if handed := d.Backend() == "stub"; handed != c.handedOver {
			t.Errorf("%s: backend %q, handed over %v, want %v", c.name, d.Backend(), handed, c.handedOver)
		}
		if len(stub.opened) > 1 {
			t.Errorf("%s: the backend was opened %d times", c.name, len(stub.opened))
		}
		if len(stub.opened) == 1 && (stub.opened[0].Width != w || stub.opened[0].Height != h) {
			t.Errorf("%s: the backend was opened at %dx%d", c.name, stub.opened[0].Width, stub.opened[0].Height)
		}
	}
}

func TestHandOverWaitsForAKeyFrameOnceThePictureHasStarted(t *testing.T) {
	const w, h, n, gop = 96, 64, 12, 4
	units := accessUnits(t, w, h, n, gop)
	splitSPS := append([][]byte{units[0][:8], units[0][8:]}, units[1:]...)
	stub := &stubHardware{}
	d := NewDecoderWithConfig(DecoderConfig{})
	d.open = stub.open
	for i, c := range splitSPS[:gop+1] {
		if _, err := d.Decode(c); err != nil {
			t.Fatal(err)
		}
		if d.Backend() != "cpu" {
			t.Fatalf("handed over after chunk %d, before the next key frame", i)
		}
	}
	if _, err := d.Decode(splitSPS[gop+1]); err != nil {
		t.Fatal(err)
	}
	if d.Backend() != "stub" {
		t.Fatalf("still %q at the second key frame", d.Backend())
	}
	if !bytes.HasPrefix(stub.decoder.fed, []byte{0, 0, 1}) && !bytes.HasPrefix(stub.decoder.fed, []byte{0, 0, 0, 1}) {
		t.Fatalf("the backend's first bytes are % x, want a start code", stub.decoder.fed[:8])
	}
}

func TestHandOverJoinedMidwayDecodesWhatSoftwareDoes(t *testing.T) {
	const w, h, n, gop = 96, 64, 12, 4
	units := accessUnits(t, w, h, n, gop)[2:]
	want, _ := decodeInChunks(t, NewDecoderWithConfig(DecoderConfig{ForceSoftware: true}), units)
	stub := &stubHardware{}
	d := NewDecoderWithConfig(DecoderConfig{})
	d.open = stub.open
	got, _ := decodeInChunks(t, d, units)
	if d.Backend() != "stub" {
		t.Fatalf("a stream joined midway stayed on %q", d.Backend())
	}
	if len(got) != len(want) {
		t.Fatalf("%d pictures out, want the %d software returns", len(got), len(want))
	}
	for i := range got {
		if !bytes.Equal(got[i], want[i]) {
			t.Fatalf("picture %d differs from the software decode", i)
		}
	}
}

func TestHandOverRespectsTheCallersLimits(t *testing.T) {
	units := accessUnits(t, 96, 64, 4, 4)
	stub := &stubHardware{}
	d := NewDecoderWithConfig(DecoderConfig{Limits: DecoderLimits{MaxFrameMBs: 23}})
	d.open = stub.open
	_, err := d.Decode(bytes.Join(units, nil))
	if !errors.Is(err, ErrOverLimit) {
		t.Fatalf("a 24 macroblock picture under a limit of 23: %v, want ErrOverLimit", err)
	}
	if len(stub.opened) != 0 {
		t.Fatalf("a backend was opened for a picture the caller's limits refuse")
	}
}
