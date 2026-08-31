package mf

import (
	"testing"

	"github.com/oops1/go.264/internal/decoder"
	"github.com/oops1/go.264/internal/testutil"
)

func decodeThroughTransform(t *testing.T, stream []byte) ([]*DecodedPicture, string) {
	t.Helper()
	dec, err := OpenDecoder(false)
	if err != nil {
		t.Skipf("no decoder transform could be opened: %v", err)
	}
	defer dec.Close()
	pics, err := dec.Decode(stream)
	if err != nil {
		t.Fatalf("%s: Decode: %v", dec.Name(), err)
	}
	rest, err := dec.Flush()
	if err != nil {
		t.Fatalf("%s: Flush: %v", dec.Name(), err)
	}
	return append(pics, rest...), dec.Name()
}

func TestTransformDecoderAgreesWithOursOnEveryClip(t *testing.T) {
	if !Loaded() {
		t.Skip("Media Foundation is not present on this machine")
	}
	clips := append(append([]testutil.Clip{}, testutil.Corpus...), testutil.MainCorpus...)
	for _, clip := range clips {
		clip := clip
		t.Run(clip.Name, func(t *testing.T) {
			stream := testutil.LoadStream(t, clip)
			theirs, name := decodeThroughTransform(t, stream)
			if len(theirs) == 0 {
				t.Fatalf("%s produced no pictures from %s", name, clip.Name)
			}

			d := decoder.New()
			ours, err := d.Decode(stream)
			if err != nil {
				t.Fatalf("our decoder refused %s: %v", clip.Name, err)
			}
			rest, err := d.Flush()
			if err != nil {
				t.Fatalf("our decoder refused to flush %s: %v", clip.Name, err)
			}
			ours = append(ours, rest...)

			n := len(theirs)
			if len(ours) < n {
				n = len(ours)
			}
			if n == 0 {
				t.Fatalf("nothing to compare: %s produced %d pictures, ours %d", name, len(theirs), len(ours))
			}
			size := clip.FrameSize()
			got := make([]byte, size)
			for i := 0; i < n; i++ {
				if len(theirs[i].I420) != size {
					t.Fatalf("%s picture %d holds %d bytes, want %d", name, i, len(theirs[i].I420), size)
				}
				ours[i].CopyOut(got)
				for j := range got {
					if got[j] != theirs[i].I420[j] {
						t.Fatalf("%s picture %d: %s says %d at sample %d, ours says %d",
							clip.Name, i, name, theirs[i].I420[j], j, got[j])
					}
				}
			}
			if len(theirs) != len(ours) {
				t.Logf("%s: %s produced %d pictures, ours %d, the first %d agree exactly",
					clip.Name, name, len(theirs), len(ours), n)
			}
		})
	}
}

func TestDecoderCloseIsSafeTwice(t *testing.T) {
	if !Loaded() {
		t.Skip("Media Foundation is not present on this machine")
	}
	dec, err := OpenDecoder(false)
	if err != nil {
		t.Skipf("no decoder transform could be opened: %v", err)
	}
	if err := dec.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := dec.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
	if _, err := dec.Decode([]byte{0, 0, 0, 1}); err == nil {
		t.Fatal("a closed decoder accepted a stream")
	}
}

func TestDecoderIgnoresAnEmptyHandOver(t *testing.T) {
	if !Loaded() {
		t.Skip("Media Foundation is not present on this machine")
	}
	dec, err := OpenDecoder(false)
	if err != nil {
		t.Skipf("no decoder transform could be opened: %v", err)
	}
	defer dec.Close()
	pics, err := dec.Decode(nil)
	if err != nil {
		t.Fatalf("Decode of nothing: %v", err)
	}
	if len(pics) != 0 {
		t.Fatalf("decoding nothing produced %d pictures", len(pics))
	}
}
