package decoder

import (
	"testing"

	"github.com/oops1/go264/internal/testutil"
)

func decodeAll(t *testing.T, stream []byte) []*Picture {
	t.Helper()
	d := New()
	pics, err := d.Decode(stream)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	rest, err := d.Flush()
	if err != nil {
		t.Fatalf("Flush: %v", err)
	}
	return append(pics, rest...)
}

func runClip(t *testing.T, clip testutil.Clip, frames int) {
	stream := testutil.LoadStream(t, clip)
	want := testutil.LoadReferenceYUV(t, clip)
	pics := decodeAll(t, stream)
	if len(pics) < frames {
		t.Fatalf("decoded %d pictures, want at least %d", len(pics), frames)
	}
	for i := 0; i < frames; i++ {
		p := pics[i]
		if p.Width != clip.Width || p.Height != clip.Height {
			t.Fatalf("frame %d: got %dx%d, want %dx%d", i, p.Width, p.Height, clip.Width, clip.Height)
		}
		got := make([]byte, clip.FrameSize())
		p.CopyOut(got)
		ref := clip.Frame(want, i)
		n := clip.Width * clip.Height
		testutil.ComparePlanes(t, labelf(clip.Name, i, "Y"), got[:n], ref[:n], clip.Width, clip.Height)
		cw, ch := clip.Width/2, clip.Height/2
		testutil.ComparePlanes(t, labelf(clip.Name, i, "Cb"), got[n:n+cw*ch], ref[n:n+cw*ch], cw, ch)
		testutil.ComparePlanes(t, labelf(clip.Name, i, "Cr"), got[n+cw*ch:], ref[n+cw*ch:], cw, ch)
		if t.Failed() {
			t.Fatalf("%s: stopping after the first mismatching frame", clip.Name)
		}
	}
}

func labelf(name string, frame int, plane string) string {
	return name + " frame " + itoa(frame) + " " + plane
}

func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	var buf [8]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	return string(buf[i:])
}

func TestIntraNoDeblockFirstFrame(t *testing.T) {
	runClip(t, testutil.Corpus[1], 1)
}

func TestIntraNoDeblockAllFrames(t *testing.T) {
	clip := testutil.Corpus[1]
	runClip(t, clip, clip.Frames)
}

func TestIntraDeblockedAllFrames(t *testing.T) {
	clip := testutil.Corpus[0]
	runClip(t, clip, clip.Frames)
}
