package syntax

import (
	"reflect"
	"testing"

	"github.com/oops1/go264/internal/bits"
	"github.com/oops1/go264/internal/nal"
)

func FuzzParseSPS(f *testing.F) {
	for _, sc := range buildSliceScenarios() {
		if b, err := WriteSPS(sc.sps); err == nil {
			f.Add(b)
		}
	}
	for _, c := range []*SPS{baseSPS()} {
		if b, err := WriteSPS(c); err == nil {
			f.Add(b)
		}
	}
	f.Add([]byte{})
	f.Add([]byte{0x64, 0x00, 0x1f})

	f.Fuzz(func(t *testing.T, data []byte) {
		s, err := ParseSPS(data)
		if err != nil {
			return
		}
		b, err := WriteSPS(s)
		if err != nil {
			t.Fatalf("WriteSPS failed after successful ParseSPS: %v", err)
		}
		s2, err := ParseSPS(b)
		if err != nil {
			t.Fatalf("second ParseSPS failed: %v", err)
		}
		if !reflect.DeepEqual(s, s2) {
			t.Fatalf("round trip mismatch\n first: %+v\nsecond: %+v", s, s2)
		}
	})
}

func FuzzParsePPS(f *testing.F) {
	sps := baseSPS()
	sps.ChromaFormatIDC = Chroma444
	lookup := lookupSPSFunc(sps)

	for _, name := range []func() *PPS{
		func() *PPS { return basePPS(sps.ID) },
		func() *PPS {
			p := basePPS(sps.ID)
			p.HasExtension = true
			p.Transform8x8Mode = true
			p.PicScalingMatrixPresent = true
			for i := 0; i < 6; i++ {
				p.ScalingList4x4Present[i] = true
				p.ScalingList4x4[i] = flatList4x4(uint8(i))
			}
			for i := 0; i < 6; i++ {
				p.ScalingList8x8Present[i] = true
				p.ScalingList8x8[i] = flatList8x8(uint8(i))
			}
			return p
		},
	} {
		if b, err := WritePPS(name(), lookup); err == nil {
			f.Add(b)
		}
	}
	f.Add([]byte{})

	f.Fuzz(func(t *testing.T, data []byte) {
		p, err := ParsePPS(data, lookup)
		if err != nil {
			return
		}
		b, err := WritePPS(p, lookup)
		if err != nil {
			t.Fatalf("WritePPS failed after successful ParsePPS: %v", err)
		}
		p2, err := ParsePPS(b, lookup)
		if err != nil {
			t.Fatalf("second ParsePPS failed: %v", err)
		}
		if !reflect.DeepEqual(p, p2) {
			t.Fatalf("round trip mismatch\n first: %+v\nsecond: %+v", p, p2)
		}
	})
}

func FuzzParseSliceHeader(f *testing.F) {
	for _, sc := range buildSliceScenarios() {
		b := func() []byte {
			w := bits.NewWriter()
			if err := WriteSliceHeader(w, sc.h, sc.sps, sc.pps); err != nil {
				return nil
			}
			w.WriteRBSPTrailingBits()
			if w.Err() != nil {
				return nil
			}
			return w.Bytes()
		}()
		if b != nil {
			f.Add(b)
		}
	}

	sps := baseSPS()
	pps := basePPS(sps.ID)
	sets := newFakeParams().addSPS(sps).addPPS(pps)
	unit := nal.Header{RefIDC: 1, Type: nal.TypeSliceNonIDR}

	f.Fuzz(func(t *testing.T, data []byte) {
		r := bits.NewReader(data)
		_, _, _, _ = ParseSliceHeader(r, unit, sets)
	})
}
