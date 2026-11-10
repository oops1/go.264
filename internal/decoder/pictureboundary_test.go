package decoder

import (
	"bytes"
	"testing"

	"github.com/oops1/go.264/internal/bits"
	"github.com/oops1/go.264/internal/frame"
	"github.com/oops1/go.264/internal/nal"
	"github.com/oops1/go.264/internal/syntax"
	"github.com/oops1/go.264/internal/testutil"
)

type parsedUnit struct {
	unit nal.Unit
	hdr  *syntax.SliceHeader
	data []byte
	bits int
}

type setMaps struct {
	sps map[uint32]*syntax.SPS
	pps map[uint32]*syntax.PPS
}

func (s setMaps) SPS(id uint32) *syntax.SPS { return s.sps[id] }
func (s setMaps) PPS(id uint32) *syntax.PPS { return s.pps[id] }

func sliceDataBits(rbsp []byte) int {
	for i := len(rbsp) - 1; i >= 0; i-- {
		if b := rbsp[i]; b != 0 {
			trailing := 1
			for b&1 == 0 {
				b >>= 1
				trailing++
			}
			return i*8 + 8 - trailing
		}
	}
	return 0
}

func parseUnits(t *testing.T, stream []byte) ([]parsedUnit, setMaps) {
	t.Helper()
	sets := setMaps{sps: map[uint32]*syntax.SPS{}, pps: map[uint32]*syntax.PPS{}}
	var units []parsedUnit
	for _, ebsp := range nal.SplitAnnexB(stream) {
		u, err := nal.Parse(ebsp)
		if err != nil {
			t.Fatal(err)
		}
		p := parsedUnit{unit: u}
		switch u.Type {
		case nal.TypeSPS:
			sps, err := syntax.ParseSPS(u.RBSP)
			if err != nil {
				t.Fatal(err)
			}
			sets.sps[sps.ID] = sps
		case nal.TypePPS:
			pps, err := syntax.ParsePPS(u.RBSP, sets.SPS)
			if err != nil {
				t.Fatal(err)
			}
			sets.pps[pps.ID] = pps
		case nal.TypeSliceIDR, nal.TypeSliceNonIDR:
			r := bits.NewReader(u.RBSP)
			hdr, _, _, err := syntax.ParseSliceHeader(r, u.Header, sets)
			if err != nil {
				t.Fatal(err)
			}
			start := len(u.RBSP)*8 - r.BitsLeft()
			p.hdr, p.bits = hdr, sliceDataBits(u.RBSP)-start
			w := bits.NewWriter()
			for left, at := p.bits, start; left > 0; {
				n := min(left, 16)
				w.WriteBits(readBitsAt(u.RBSP, at, n), n)
				at += n
				left -= n
			}
			p.data = w.Bytes()
		}
		units = append(units, p)
	}
	return units, sets
}

func readBitsAt(b []byte, at, n int) uint32 {
	var v uint32
	for i := 0; i < n; i++ {
		bit := b[(at+i)/8] >> (7 - uint((at+i)%8)) & 1
		v = v<<1 | uint32(bit)
	}
	return v
}

func rewriteSlice(t *testing.T, p parsedUnit, sets setMaps, data []byte, dataBits int) nal.Unit {
	t.Helper()
	pps := sets.pps[p.hdr.PPSID]
	w := bits.NewWriter()
	if err := syntax.WriteSliceHeader(w, p.hdr, sets.sps[pps.SPSID], pps); err != nil {
		t.Fatal(err)
	}
	for at := 0; at < dataBits; {
		n := min(dataBits-at, 16)
		w.WriteBits(readBitsAt(data, at, n), n)
		at += n
	}
	w.WriteRBSPTrailingBits()
	u := p.unit
	u.RBSP = w.Bytes()
	return u
}

func sameAs(t *testing.T, label string, got, want *frame.Picture) {
	t.Helper()
	if !bytes.Equal(got.Y, want.Y) || !bytes.Equal(got.Cb, want.Cb) || !bytes.Equal(got.Cr, want.Cr) {
		t.Fatalf("%s differs from the undamaged stream", label)
	}
}

func clip(t *testing.T, name string) testutil.Clip {
	t.Helper()
	for _, c := range testutil.Corpus {
		if c.Name == name {
			return c
		}
	}
	t.Fatalf("no clip %s in the corpus", name)
	return testutil.Clip{}
}

func TestALostFirstSliceStartsItsOwnPicture(t *testing.T) {
	c := clip(t, "base_ip_slices")
	stream := testutil.LoadStream(t, c)
	units, _ := parseUnits(t, stream)

	const damaged = 5
	picture, cut, second := -1, -1, -1
	for i, u := range units {
		if u.hdr == nil {
			continue
		}
		if u.hdr.FirstMBInSlice == 0 {
			picture++
			if picture == damaged {
				cut = i
			}
		} else if picture == damaged && second < 0 {
			second = i
		}
	}
	if cut < 0 || second < 0 {
		t.Fatalf("%s has no picture %d with more than one slice", c.Name, damaged)
	}
	var lossy []byte
	for i, u := range units {
		if i != cut {
			lossy = nal.AppendAnnexB(lossy, u.unit, true)
		}
	}

	_, want := decodeSplit(t, stream)
	_, got := decodeSplit(t, lossy)
	if len(got) != len(want) {
		t.Fatalf("%d pictures from the stream with a lost slice, want %d: the rest of the damaged picture was taken for part of another", len(got), len(want))
	}
	for i := 0; i < damaged; i++ {
		sameAs(t, "a picture before the loss", got[i], want[i])
	}

	widthMBs := c.Width / 16
	startRow := int(units[second].hdr.FirstMBInSlice)/widthMBs + 2
	g, w := got[damaged], want[damaged]
	for y := startRow * 16; y < c.Height; y++ {
		at := g.LumaOffset(0, y)
		if !bytes.Equal(g.Y[at:at+c.Width], w.Y[at:at+c.Width]) {
			t.Fatalf("luma row %d of the damaged picture, inside the slice that arrived, differs from the undamaged stream", y)
		}
	}
	for y := 0; y < 16; y++ {
		at := g.LumaOffset(0, y)
		if !bytes.Equal(g.Y[at:at+c.Width], got[damaged-1].Y[at:at+c.Width]) {
			t.Fatalf("luma row %d of the lost slice is not concealed from the picture before it", y)
		}
	}
}

func TestFlushGivesUpItsPicturesWhenTheLastUnitIsBroken(t *testing.T) {
	c := clip(t, "base_ip_qp26")
	stream := testutil.LoadStream(t, c)
	_, want := decodeSplit(t, stream)

	d := New()
	got, err := d.Decode(append(append([]byte{}, stream...), 0, 0, 0, 1, 0xe5, 0xff, 0xff))
	if err != nil {
		t.Fatal(err)
	}
	rest, err := d.Flush()
	if err == nil {
		t.Fatal("Flush accepted a unit with forbidden_zero_bit set")
	}
	samePictures(t, c.Name, append(got, rest...), want)
}

func TestRedundantSlicesAreIgnored(t *testing.T) {
	c := clip(t, "base_ip_slices")
	stream := testutil.LoadStream(t, c)
	units, sets := parseUnits(t, stream)

	var out []byte
	for _, u := range units {
		switch {
		case u.unit.Type == nal.TypePPS:
			parsed, err := syntax.ParsePPS(u.unit.RBSP, sets.SPS)
			if err != nil {
				t.Fatal(err)
			}
			pps := *parsed
			pps.RedundantPicCntPresent = true
			rbsp, err := syntax.WritePPS(&pps, sets.SPS)
			if err != nil {
				t.Fatal(err)
			}
			sets.pps[pps.ID] = &pps
			u.unit.RBSP = rbsp
			out = nal.AppendAnnexB(out, u.unit, true)
		case u.hdr != nil:
			out = nal.AppendAnnexB(out, rewriteSlice(t, u, sets, u.data, u.bits), true)
			redundant := u
			hdr := *u.hdr
			hdr.RedundantPicCnt = 1
			redundant.hdr = &hdr
			garbage := bytes.Repeat([]byte{0xa5}, len(u.data))
			out = nal.AppendAnnexB(out, rewriteSlice(t, redundant, sets, garbage, u.bits), true)
		default:
			out = nal.AppendAnnexB(out, u.unit, true)
		}
	}

	_, want := decodeSplit(t, stream)
	d := New()
	got, err := d.Decode(out)
	if err == nil {
		var rest []*frame.Picture
		rest, err = d.Flush()
		got = append(got, rest...)
	}
	if err != nil {
		t.Fatalf("a redundant slice was decoded: %v", err)
	}
	samePictures(t, c.Name, got, want)
}
