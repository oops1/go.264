package decoder

import (
	"bytes"
	"testing"

	"github.com/oops1/go.264/internal/frame"
	"github.com/oops1/go.264/internal/nal"
	"github.com/oops1/go.264/internal/syntax"
	"github.com/oops1/go.264/internal/testutil"
)

func withoutVUI(t *testing.T, stream []byte) ([]byte, *syntax.SPS) {
	t.Helper()
	var out []byte
	var last *syntax.SPS
	for _, ebsp := range nal.SplitAnnexB(stream) {
		u, err := nal.Parse(ebsp)
		if err != nil {
			t.Fatal(err)
		}
		if u.Header.Type == nal.TypeSPS {
			sps, err := syntax.ParseSPS(u.RBSP)
			if err != nil {
				t.Fatal(err)
			}
			sps.VUIPresent = false
			sps.VUI = syntax.VUI{}
			if u.RBSP, err = syntax.WriteSPS(sps); err != nil {
				t.Fatal(err)
			}
			last = sps
		}
		out = nal.AppendAnnexB(out, u, true)
	}
	if last == nil {
		t.Fatal("the stream has no sequence parameter set")
	}
	return out, last
}

func decodeSplit(t *testing.T, stream []byte) (beforeFlush []*frame.Picture, all []*frame.Picture) {
	t.Helper()
	d := New()
	pics, err := d.Decode(stream)
	if err != nil {
		t.Fatal(err)
	}
	beforeFlush = pics
	rest, err := d.Flush()
	if err != nil {
		t.Fatal(err)
	}
	return beforeFlush, append(append([]*frame.Picture{}, pics...), rest...)
}

func samePictures(t *testing.T, label string, got, want []*frame.Picture) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s: %d pictures, want %d", label, len(got), len(want))
	}
	for i := range got {
		if !bytes.Equal(got[i].Y, want[i].Y) || !bytes.Equal(got[i].Cb, want[i].Cb) || !bytes.Equal(got[i].Cr, want[i].Cr) {
			t.Fatalf("%s: picture %d differs from the stream that states its reordering", label, i)
		}
	}
}

func TestOrderCountType2WithoutARestrictionIsNotHeldBack(t *testing.T) {
	checked := 0
	for _, c := range testutil.Corpus {
		stream := testutil.LoadStream(t, c)
		stripped, sps := withoutVUI(t, stream)
		if sps.PicOrderCntType != 2 {
			continue
		}
		checked++
		early, all := decodeSplit(t, stripped)
		if len(early) < c.Frames-2 {
			t.Errorf("%s: %d of %d pictures out before Flush, want all but at most the last two: the last unit ends only at Flush, and with one slice a picture ends only when the next one starts",
				c.Name, len(early), c.Frames)
		}
		_, want := decodeSplit(t, stream)
		samePictures(t, c.Name, all, want)
	}
	if checked == 0 {
		t.Fatal("no clip in the corpus uses picture order count type 2, so nothing was measured")
	}
}

func TestOrderCountType0WithoutARestrictionStillReorders(t *testing.T) {
	checked := 0
	for _, c := range testutil.MainCorpus {
		stream := testutil.LoadStream(t, c)
		stripped, sps := withoutVUI(t, stream)
		if sps.PicOrderCntType != 0 {
			continue
		}
		checked++
		_, all := decodeSplit(t, stripped)
		_, want := decodeSplit(t, stream)
		samePictures(t, c.Name, all, want)
	}
	if checked == 0 {
		t.Fatal("no clip in the corpus uses picture order count type 0, so nothing was measured")
	}
}
