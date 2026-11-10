package go264

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/oops1/go.264/internal/nal"
	"github.com/oops1/go.264/internal/syntax"
)

func cropStream(t *testing.T, stream []byte, left, right, top, bottom uint32) []byte {
	t.Helper()
	var out []byte
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
			sps.FrameCropping = true
			sps.FrameCropLeftOffset, sps.FrameCropRightOffset = left, right
			sps.FrameCropTopOffset, sps.FrameCropBottomOffset = top, bottom
			if u.RBSP, err = syntax.WriteSPS(sps); err != nil {
				t.Fatal(err)
			}
		}
		out = nal.AppendAnnexB(out, u, true)
	}
	return out
}

func softwareFrames(t *testing.T, stream []byte) [][]byte {
	t.Helper()
	d := NewDecoderWithConfig(DecoderConfig{ForceSoftware: true})
	defer d.Close()
	frames, err := d.Decode(stream)
	if err != nil {
		t.Fatal(err)
	}
	rest, err := d.Flush()
	if err != nil {
		t.Fatal(err)
	}
	var out [][]byte
	for _, f := range append(frames, rest...) {
		out = append(out, f.AppendI420(nil))
	}
	return out
}

func cropI420(src []byte, w, h, x, y, cw, ch int) []byte {
	out := make([]byte, 0, cw*ch*3/2)
	for r := 0; r < ch; r++ {
		out = append(out, src[(y+r)*w+x:(y+r)*w+x+cw]...)
	}
	planes := [][]byte{src[w*h : w*h+w*h/4], src[w*h+w*h/4:]}
	for _, p := range planes {
		for r := 0; r < ch/2; r++ {
			start := (y/2+r)*(w/2) + x/2
			out = append(out, p[start:start+cw/2]...)
		}
	}
	return out
}

func encodedStream(t *testing.T, w, h, n int) []byte {
	t.Helper()
	enc, err := NewEncoder(EncoderConfig{Width: w, Height: h, FPSNum: 25, FPSDen: 1, GOPSize: 4, QP: 26, ForceSoftware: true})
	if err != nil {
		t.Fatal(err)
	}
	defer enc.Close()
	var stream []byte
	for i := 0; i < n; i++ {
		pkt, err := enc.Encode(pattern(w, h, i))
		if err != nil {
			t.Fatal(err)
		}
		stream = append(stream, pkt...)
	}
	pkt, err := enc.Flush()
	if err != nil {
		t.Fatal(err)
	}
	return append(stream, pkt...)
}

func TestDecoderAppliesTheCroppingOrigin(t *testing.T) {
	const w, h, n = 96, 64, 6
	stream := encodedStream(t, w, h, n)
	full := softwareFrames(t, stream)
	if len(full) != n {
		t.Fatalf("%d pictures from the uncropped stream, want %d", len(full), n)
	}
	cases := []struct {
		left, right, top, bottom uint32
	}{
		{8, 0, 4, 0},
		{3, 5, 0, 7},
		{0, 0, 2, 2},
		{16, 8, 10, 6},
	}
	for _, c := range cases {
		x, y := 2*int(c.left), 2*int(c.top)
		cw, ch := w-2*int(c.left+c.right), h-2*int(c.top+c.bottom)
		cropped := softwareFrames(t, cropStream(t, stream, c.left, c.right, c.top, c.bottom))
		if len(cropped) != n {
			t.Fatalf("crop %+v: %d pictures, want %d", c, len(cropped), n)
		}
		for i := range cropped {
			if want := cropI420(full[i], w, h, x, y, cw, ch); !bytes.Equal(cropped[i], want) {
				t.Fatalf("crop %+v: picture %d is not the %dx%d window at (%d,%d) of the uncropped picture", c, i, cw, ch, x, y)
			}
		}
	}
}

func TestDecoderCroppingMatchesFFmpeg(t *testing.T) {
	bin, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg is not installed")
	}
	const w, h, n = 96, 64, 6
	const left, right, top, bottom = 5, 3, 4, 2
	stream := cropStream(t, encodedStream(t, w, h, n), left, right, top, bottom)
	dir := t.TempDir()
	in := filepath.Join(dir, "in.264")
	out := filepath.Join(dir, "out.yuv")
	if err := os.WriteFile(in, stream, 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(bin, "-hide_banner", "-loglevel", "error", "-y", "-i", in, "-pix_fmt", "yuv420p", "-f", "rawvideo", out)
	if combined, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("ffmpeg: %v\n%s", err, combined)
	}
	want, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	got := bytes.Join(softwareFrames(t, stream), nil)
	if len(want) != (w-2*(left+right))*(h-2*(top+bottom))*3/2*n {
		t.Fatalf("ffmpeg wrote %d bytes, which is not %d cropped pictures", len(want), n)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("the cropped pictures differ from ffmpeg's")
	}
}
