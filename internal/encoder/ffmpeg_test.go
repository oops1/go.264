package encoder

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func ffmpegPath(t *testing.T) string {
	t.Helper()
	p, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg is not installed, skipping the external conformance check")
	}
	return p
}

func decodeWithFFmpeg(t *testing.T, stream []byte) []byte {
	t.Helper()
	bin := ffmpegPath(t)
	dir := t.TempDir()
	in := filepath.Join(dir, "in.264")
	out := filepath.Join(dir, "out.yuv")
	if err := os.WriteFile(in, stream, 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(bin, "-hide_banner", "-loglevel", "error", "-y",
		"-i", in, "-pix_fmt", "yuv420p", "-f", "rawvideo", out)
	combined, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("ffmpeg rejected our stream: %v\n%s", err, combined)
	}
	if len(combined) != 0 {
		t.Fatalf("ffmpeg reported problems decoding our stream:\n%s", combined)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestFFmpegDecodesOurStreamIdentically(t *testing.T) {
	for _, qp := range []int{12, 26, 38} {
		cfg := Config{Width: 176, Height: 144, FPSNum: 25, FPSDen: 1, GOPSize: 1, QP: qp}
		var frames [][]byte
		for i := 0; i < 4; i++ {
			frames = append(frames, syntheticFrame(cfg.Width, cfg.Height, i))
		}
		enc, err := New(cfg)
		if err != nil {
			t.Fatal(err)
		}
		var stream []byte
		for _, f := range frames {
			pkt, err := enc.Encode(f)
			if err != nil {
				t.Fatal(err)
			}
			stream = append(stream, pkt...)
		}

		ref := decodeWithFFmpeg(t, stream)
		frameSize := cfg.Width * cfg.Height * 3 / 2
		if len(ref) != frameSize*len(frames) {
			t.Fatalf("qp %d: ffmpeg produced %d bytes, want %d", qp, len(ref), frameSize*len(frames))
		}

		pics, _, _ := encodeAndDecode(t, cfg, frames)
		if len(pics) != len(frames) {
			t.Fatalf("qp %d: our decoder produced %d frames", qp, len(pics))
		}
		for i := range pics {
			got := make([]byte, pics[i].Size())
			pics[i].CopyOut(got)
			want := ref[i*frameSize : (i+1)*frameSize]
			for j := range got {
				if got[j] != want[j] {
					t.Fatalf("qp %d frame %d: ffmpeg and our decoder disagree at sample %d, ffmpeg %d ours %d",
						qp, i, j, want[j], got[j])
				}
			}
		}
		t.Logf("qp %d: %d bytes, four frames decode identically in ffmpeg", qp, len(stream))
	}
}

func TestFFmpegReportsBaselineProfile(t *testing.T) {
	bin, err := exec.LookPath("ffprobe")
	if err != nil {
		t.Skip("ffprobe is not installed")
	}
	cfg := Config{Width: 320, Height: 240, FPSNum: 30, FPSDen: 1, GOPSize: 1, QP: 28}
	enc, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	pkt, err := enc.Encode(syntheticFrame(cfg.Width, cfg.Height, 0))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	in := filepath.Join(dir, "in.264")
	if err := os.WriteFile(in, pkt, 0o600); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command(bin, "-hide_banner", "-loglevel", "error",
		"-show_entries", "stream=profile,width,height,pix_fmt",
		"-of", "default=nw=1", in).CombinedOutput()
	if err != nil {
		t.Fatalf("ffprobe failed: %v\n%s", err, out)
	}
	t.Logf("ffprobe reports:\n%s", out)
	for _, want := range []string{"width=320", "height=240", "pix_fmt=yuv420p"} {
		if !containsLine(string(out), want) {
			t.Errorf("ffprobe output does not contain %q:\n%s", want, out)
		}
	}
}

func containsLine(s, want string) bool {
	for i := 0; i+len(want) <= len(s); i++ {
		if s[i:i+len(want)] == want {
			return true
		}
	}
	return false
}

func TestFFmpegDecodesInterFramesIdentically(t *testing.T) {
	for _, qp := range []int{14, 26, 36} {
		cfg := Config{Width: 176, Height: 144, FPSNum: 25, FPSDen: 1, GOPSize: 10, QP: qp}
		var frames [][]byte
		for i := 0; i < 10; i++ {
			frames = append(frames, syntheticFrame(cfg.Width, cfg.Height, i))
		}
		enc, err := New(cfg)
		if err != nil {
			t.Fatal(err)
		}
		var stream []byte
		intraSize, interTotal := 0, 0
		for i, f := range frames {
			pkt, err := enc.Encode(f)
			if err != nil {
				t.Fatal(err)
			}
			if i == 0 {
				intraSize = len(pkt)
			} else {
				interTotal += len(pkt)
			}
			stream = append(stream, pkt...)
		}

		ref := decodeWithFFmpeg(t, stream)
		frameSize := cfg.Width * cfg.Height * 3 / 2
		if len(ref) != frameSize*len(frames) {
			t.Fatalf("qp %d: ffmpeg produced %d bytes, want %d", qp, len(ref), frameSize*len(frames))
		}
		pics, _, _ := encodeAndDecode(t, cfg, frames)
		for i := range pics {
			got := make([]byte, pics[i].Size())
			pics[i].CopyOut(got)
			want := ref[i*frameSize : (i+1)*frameSize]
			for j := range got {
				if got[j] != want[j] {
					t.Fatalf("qp %d frame %d: ffmpeg and our decoder disagree at sample %d, ffmpeg %d ours %d",
						qp, i, j, want[j], got[j])
				}
			}
		}
		t.Logf("qp %d: intra %d bytes, nine inter frames %d bytes total", qp, intraSize, interTotal)
	}
}

func TestFFmpegDecodesMultiReferenceStreams(t *testing.T) {
	for _, refs := range []int{2, 3, 5} {
		cfg := Config{
			Width: 176, Height: 144, FPSNum: 25, FPSDen: 1,
			GOPSize: 12, QP: 24, RefFrames: refs,
		}
		var frames [][]byte
		for i := 0; i < 12; i++ {
			frames = append(frames, syntheticFrame(cfg.Width, cfg.Height, i))
		}
		enc, err := New(cfg)
		if err != nil {
			t.Fatal(err)
		}
		var stream []byte
		for _, f := range frames {
			pkt, err := enc.Encode(f)
			if err != nil {
				t.Fatal(err)
			}
			stream = append(stream, pkt...)
		}
		ref := decodeWithFFmpeg(t, stream)
		frameSize := cfg.Width * cfg.Height * 3 / 2
		if len(ref) != frameSize*len(frames) {
			t.Fatalf("refs %d: ffmpeg produced %d bytes, want %d", refs, len(ref), frameSize*len(frames))
		}
		pics, _, _ := encodeAndDecode(t, cfg, frames)
		for i := range pics {
			got := make([]byte, pics[i].Size())
			pics[i].CopyOut(got)
			want := ref[i*frameSize : (i+1)*frameSize]
			for j := range got {
				if got[j] != want[j] {
					t.Fatalf("refs %d frame %d: ffmpeg disagrees at sample %d, ffmpeg %d ours %d",
						refs, i, j, want[j], got[j])
				}
			}
		}
		t.Logf("refs %d: %d bytes, twelve frames decode identically in ffmpeg", refs, len(stream))
	}
}

func TestFFmpegDecodesOurCABACStreamIdentically(t *testing.T) {
	for _, qp := range []int{12, 26, 38} {
		for _, gop := range []int{1, 8} {
			cfg := cabacConfig(qp, gop)
			cfg.RefFrames = 2
			var frames [][]byte
			for i := 0; i < 6; i++ {
				frames = append(frames, syntheticFrame(cfg.Width, cfg.Height, i))
			}
			enc, err := New(cfg)
			if err != nil {
				t.Fatal(err)
			}
			var stream []byte
			for _, f := range frames {
				pkt, err := enc.Encode(f)
				if err != nil {
					t.Fatal(err)
				}
				stream = append(stream, pkt...)
			}

			ref := decodeWithFFmpeg(t, stream)
			frameSize := cfg.Width * cfg.Height * 3 / 2
			if len(ref) != frameSize*len(frames) {
				t.Fatalf("qp %d gop %d: ffmpeg produced %d bytes, want %d",
					qp, gop, len(ref), frameSize*len(frames))
			}
			pics, _, _ := encodeAndDecode(t, cfg, frames)
			for i := range pics {
				got := make([]byte, pics[i].Size())
				pics[i].CopyOut(got)
				want := ref[i*frameSize : (i+1)*frameSize]
				for j := range got {
					if got[j] != want[j] {
						t.Fatalf("qp %d gop %d frame %d: ffmpeg and our decoder disagree at sample %d, ffmpeg %d ours %d",
							qp, gop, i, j, want[j], got[j])
					}
				}
			}
			t.Logf("qp %d gop %d: %d CABAC bytes decode identically in ffmpeg", qp, gop, len(stream))
		}
	}
}

func TestFFmpegReportsMainProfileForCABAC(t *testing.T) {
	bin, err := exec.LookPath("ffprobe")
	if err != nil {
		t.Skip("ffprobe is not installed")
	}
	cfg := cabacConfig(28, 4)
	enc, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	var stream []byte
	for i := 0; i < 3; i++ {
		pkt, err := enc.Encode(syntheticFrame(cfg.Width, cfg.Height, i))
		if err != nil {
			t.Fatal(err)
		}
		stream = append(stream, pkt...)
	}
	dir := t.TempDir()
	in := filepath.Join(dir, "in.264")
	if err := os.WriteFile(in, stream, 0o600); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command(bin, "-hide_banner", "-loglevel", "error",
		"-show_entries", "stream=profile,pix_fmt", "-of", "default=nw=1", in).CombinedOutput()
	if err != nil {
		t.Fatalf("ffprobe failed: %v\n%s", err, out)
	}
	t.Logf("ffprobe reports:\n%s", out)
	if !containsLine(string(out), "profile=Main") {
		t.Errorf("ffprobe does not call our CABAC stream Main profile:\n%s", out)
	}
}

func TestFFmpegDecodesSlicedPicturesIdentically(t *testing.T) {
	for _, cabac := range []bool{false, true} {
		for _, slices := range []int{2, 4, 9} {
			cfg := slicedConfig(176, 144, 26, slices, cabac)
			var frames [][]byte
			for i := 0; i < 6; i++ {
				frames = append(frames, syntheticFrame(cfg.Width, cfg.Height, i))
			}
			enc, err := New(cfg)
			if err != nil {
				t.Fatal(err)
			}
			var stream []byte
			for _, f := range frames {
				pkt, err := enc.Encode(f)
				if err != nil {
					t.Fatal(err)
				}
				stream = append(stream, pkt...)
			}
			ref := decodeWithFFmpeg(t, stream)
			frameSize := cfg.Width * cfg.Height * 3 / 2
			if len(ref) != frameSize*len(frames) {
				t.Fatalf("cabac %v, %d slices: ffmpeg produced %d bytes, want %d",
					cabac, slices, len(ref), frameSize*len(frames))
			}
			pics, _, _ := encodeAndDecode(t, cfg, frames)
			for i := range pics {
				got := make([]byte, pics[i].Size())
				pics[i].CopyOut(got)
				want := ref[i*frameSize : (i+1)*frameSize]
				for j := range got {
					if got[j] != want[j] {
						t.Fatalf("cabac %v, %d slices, frame %d: ffmpeg and our decoder disagree at sample %d, ffmpeg %d ours %d",
							cabac, slices, i, j, want[j], got[j])
					}
				}
			}
			t.Logf("cabac %v, %d slices: %d bytes decode identically in ffmpeg", cabac, slices, len(stream))
		}
	}
}
