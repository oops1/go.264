package encoder

import (
	"image"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/oops1/go.264/internal/decoder"
	"github.com/oops1/go.264/internal/frame"
	"github.com/oops1/go.264/internal/nal"
	"github.com/oops1/go.264/internal/syntax"
	"github.com/oops1/go.264/internal/testutil"
)

func high8x8Config(qp int) Config {
	return Config{Width: 176, Height: 144, FPSNum: 25, FPSDen: 1, GOPSize: 8, QP: qp,
		RefFrames: 1, Transform8x8: true}
}

func TestTransform8x8IsOffByDefault(t *testing.T) {
	cfg := Config{Width: 64, Height: 48, FPSNum: 25, FPSDen: 1, GOPSize: 1, QP: 26}
	enc, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if enc.PPS().Transform8x8Mode {
		t.Fatal("transform_8x8_mode_flag is set although Transform8x8 was not asked for")
	}
	if enc.SPS().ProfileIDC != syntax.ProfileBaseline {
		t.Fatalf("profile_idc is %d, want baseline", enc.SPS().ProfileIDC)
	}
}

func TestTransform8x8LeavesTheOldStreamsByteForByte(t *testing.T) {
	for _, cabac := range []bool{false, true} {
		cfg := Config{Width: 96, Height: 80, FPSNum: 25, FPSDen: 1, GOPSize: 4, QP: 27, CABAC: cabac}
		var frames [][]byte
		for i := 0; i < 5; i++ {
			frames = append(frames, syntheticFrame(cfg.Width, cfg.Height, i))
		}
		before := encodeStream(t, cfg, frames)
		after := encodeStream(t, cfg, frames)
		if len(before) != len(after) {
			t.Fatalf("cabac %v: the same configuration produced %d then %d bytes", cabac, len(before), len(after))
		}
		for i := range before {
			if before[i] != after[i] {
				t.Fatalf("cabac %v: byte %d differs between two identical encodes", cabac, i)
			}
		}
	}
}

func TestHighProfileParameterSets(t *testing.T) {
	enc, err := New(high8x8Config(26))
	if err != nil {
		t.Fatal(err)
	}
	sps, pps := enc.SPS(), enc.PPS()
	if sps.ProfileIDC != syntax.ProfileHigh {
		t.Fatalf("profile_idc is %d, want %d", sps.ProfileIDC, syntax.ProfileHigh)
	}
	if sps.ConstraintSet != 0 {
		t.Fatalf("constraint flags are %#x, want 0 for High", sps.ConstraintSet)
	}
	if sps.ChromaFormatIDC != syntax.Chroma420 {
		t.Fatalf("chroma_format_idc is %d, want 4:2:0", sps.ChromaFormatIDC)
	}
	if sps.BitDepthLumaMinus8 != 0 || sps.BitDepthChromaMinus8 != 0 {
		t.Fatalf("bit depths are %d and %d, want eight bits", sps.BitDepthLumaMinus8+8, sps.BitDepthChromaMinus8+8)
	}
	if sps.QpprimeYZeroTransformBypass {
		t.Fatal("qpprime_y_zero_transform_bypass_flag is set")
	}
	if sps.SeqScalingMatrixPresent {
		t.Fatal("the sequence parameter set carries a scaling matrix, which x264 never writes")
	}
	if !pps.HasExtension || !pps.Transform8x8Mode {
		t.Fatalf("the picture parameter set has extension %v and transform_8x8_mode_flag %v",
			pps.HasExtension, pps.Transform8x8Mode)
	}

	headers, err := enc.Headers()
	if err != nil {
		t.Fatal(err)
	}
	rt := parseHeaders(t, headers)
	if rt.sps.ProfileIDC != syntax.ProfileHigh || !rt.pps.Transform8x8Mode {
		t.Fatalf("re-reading our own headers gave profile %d and transform_8x8_mode_flag %v",
			rt.sps.ProfileIDC, rt.pps.Transform8x8Mode)
	}
}

func TestTransform8x8RoundTripsAcrossQuantisers(t *testing.T) {
	for _, cabac := range []bool{false, true} {
		for _, qp := range []int{0, 12, 26, 40, 51} {
			cfg := high8x8Config(qp)
			cfg.CABAC = cabac
			cfg.GOPSize = 1
			var frames [][]byte
			for i := 0; i < 3; i++ {
				frames = append(frames, syntheticFrame(cfg.Width, cfg.Height, i))
			}
			pics, _, recons := encodeAndDecode(t, cfg, frames)
			assertMatchesReconstruction(t, label("intra", cabac, qp), pics, recons)
		}
	}
}

func TestTransform8x8RoundTripsWithPrediction(t *testing.T) {
	for _, cabac := range []bool{false, true} {
		for _, qp := range []int{0, 22, 37, 51} {
			cfg := high8x8Config(qp)
			cfg.CABAC = cabac
			cfg.RefFrames = 2
			var frames [][]byte
			for i := 0; i < 7; i++ {
				frames = append(frames, syntheticFrame(cfg.Width, cfg.Height, i))
			}
			pics, _, recons := encodeAndDecode(t, cfg, frames)
			assertMatchesReconstruction(t, label("predicted", cabac, qp), pics, recons)
		}
	}
}

func TestTransform8x8RoundTripsWithBFrames(t *testing.T) {
	for _, cabac := range []bool{false, true} {
		for _, qp := range []int{18, 30, 44} {
			cfg := high8x8Config(qp)
			cfg.CABAC = cabac
			cfg.BFrames = 2
			cfg.RefFrames = 2
			var frames [][]byte
			for i := 0; i < 9; i++ {
				frames = append(frames, syntheticFrame(cfg.Width, cfg.Height, i))
			}
			pics, recons := encodeAndDecodeReordered(t, cfg, frames)
			assertMatchesReconstruction(t, label("bipredictive", cabac, qp), pics, recons)
		}
	}
}

func TestTransform8x8RoundTripsAcrossSlices(t *testing.T) {
	for _, cabac := range []bool{false, true} {
		cfg := high8x8Config(28)
		cfg.CABAC = cabac
		cfg.Slices = 3
		var frames [][]byte
		for i := 0; i < 5; i++ {
			frames = append(frames, syntheticFrame(cfg.Width, cfg.Height, i))
		}
		pics, _, recons := encodeAndDecode(t, cfg, frames)
		assertMatchesReconstruction(t, label("sliced", cabac, 28), pics, recons)
	}
}

func TestTransform8x8RoundTripsWithHints(t *testing.T) {
	for _, cabac := range []bool{false, true} {
		cfg := high8x8Config(26)
		cfg.CABAC = cabac
		frames := [][]byte{syntheticFrame(cfg.Width, cfg.Height, 0)}
		hints := []Hints{{}}
		for i := 1; i < 6; i++ {
			frames = append(frames, syntheticFrame(cfg.Width, cfg.Height, i))
			hints = append(hints, Hints{Changed: changedBand(cfg, i)})
		}
		units, recons := encodeWithHints(t, cfg, frames, hints)
		assertMatchesReconstruction(t, label("hinted", cabac, 26), decodeUnits(t, units), recons)
	}
}

func TestTransform8x8ActuallyGetsChosen(t *testing.T) {
	for _, cabac := range []bool{false, true} {
		cfg := high8x8Config(30)
		cfg.CABAC = cabac
		var frames [][]byte
		for i := 0; i < 4; i++ {
			frames = append(frames, syntheticFrame(cfg.Width, cfg.Height, i))
		}
		enc, err := New(cfg)
		if err != nil {
			t.Fatal(err)
		}
		intra8x8, inter8x8 := 0, 0
		for _, f := range frames {
			if _, err := enc.Encode(f); err != nil {
				t.Fatal(err)
			}
			for i := range enc.grid {
				m := &enc.grid[i]
				if !m.Transform8x8 {
					continue
				}
				if m.Intra {
					intra8x8++
				} else {
					inter8x8++
				}
			}
		}
		if intra8x8 == 0 {
			t.Fatalf("cabac %v: no macroblock ever chose Intra_8x8", cabac)
		}
		if inter8x8 == 0 {
			t.Fatalf("cabac %v: no predicted macroblock ever chose the 8x8 transform", cabac)
		}
		t.Logf("cabac %v: %d intra and %d predicted macroblocks used the 8x8 transform", cabac, intra8x8, inter8x8)
	}
}

func smoothFrame(w, h, t int) []byte {
	buf := make([]byte, w*h*3/2)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			v := 40 + (x+t*2)*120/w + y*80/h
			if (x-w/2)*(x-w/2)+(y-h/2)*(y-h/2) < (w/4)*(w/4) {
				v = 200 - y*60/h
			}
			buf[y*w+x] = byte(v)
		}
	}
	cw, ch := w/2, h/2
	for i := 0; i < 2; i++ {
		base := w*h + i*cw*ch
		for y := 0; y < ch; y++ {
			for x := 0; x < cw; x++ {
				buf[base+y*cw+x] = byte(110 + x*30/cw + i*10)
			}
		}
	}
	return buf
}

func TestIntra8x8CarriesSmoothContent(t *testing.T) {
	for _, cabac := range []bool{false, true} {
		cfg := high8x8Config(26)
		cfg.CABAC = cabac
		cfg.GOPSize = 1
		var frames [][]byte
		for i := 0; i < 3; i++ {
			frames = append(frames, smoothFrame(cfg.Width, cfg.Height, i))
		}
		enc, err := New(cfg)
		if err != nil {
			t.Fatal(err)
		}
		intra8x8, intraNxN := 0, 0
		for _, f := range frames {
			if _, err := enc.Encode(f); err != nil {
				t.Fatal(err)
			}
			for i := range enc.grid {
				m := &enc.grid[i]
				if m.kind != mbTypeINxN {
					continue
				}
				intraNxN++
				if m.Transform8x8 {
					intra8x8++
				}
			}
		}
		if intra8x8*2 < intraNxN {
			t.Fatalf("cabac %v: only %d of %d Intra_NxN macroblocks chose the 8x8 transform on smooth content",
				cabac, intra8x8, intraNxN)
		}
		t.Logf("cabac %v: %d of %d Intra_NxN macroblocks chose Intra_8x8", cabac, intra8x8, intraNxN)

		pics, _, recons := encodeAndDecode(t, cfg, frames)
		assertMatchesReconstruction(t, label("smooth intra", cabac, 26), pics, recons)

		stream := encodeStream(t, cfg, frames)
		ref := decodeWithFFmpeg(t, stream)
		frameSize := cfg.Width * cfg.Height * 3 / 2
		for i := range pics {
			got := make([]byte, pics[i].Size())
			pics[i].CopyOut(got)
			want := ref[i*frameSize : (i+1)*frameSize]
			for j := range got {
				if got[j] != want[j] {
					t.Fatalf("cabac %v frame %d: ffmpeg and our decoder disagree at sample %d, ffmpeg %d ours %d",
						cabac, i, j, want[j], got[j])
				}
			}
		}
	}
}

func TestFFmpegDecodesOurHighProfileStream(t *testing.T) {
	for _, cabac := range []bool{false, true} {
		for _, qp := range []int{0, 20, 34, 51} {
			cfg := high8x8Config(qp)
			cfg.CABAC = cabac
			var frames [][]byte
			for i := 0; i < 5; i++ {
				frames = append(frames, syntheticFrame(cfg.Width, cfg.Height, i))
			}
			stream := encodeStream(t, cfg, frames)
			ref := decodeWithFFmpeg(t, stream)
			frameSize := cfg.Width * cfg.Height * 3 / 2
			if len(ref) != frameSize*len(frames) {
				t.Fatalf("cabac %v qp %d: ffmpeg produced %d bytes, want %d",
					cabac, qp, len(ref), frameSize*len(frames))
			}
			pics, _, _ := encodeAndDecode(t, cfg, frames)
			for i := range pics {
				got := make([]byte, pics[i].Size())
				pics[i].CopyOut(got)
				want := ref[i*frameSize : (i+1)*frameSize]
				for j := range got {
					if got[j] != want[j] {
						t.Fatalf("cabac %v qp %d frame %d: ffmpeg and our decoder disagree at sample %d, ffmpeg %d ours %d",
							cabac, qp, i, j, want[j], got[j])
					}
				}
			}
		}
	}
}

func TestFFmpegDecodesOurHighProfileBFrames(t *testing.T) {
	cfg := high8x8Config(27)
	cfg.CABAC = true
	cfg.BFrames = 2
	cfg.RefFrames = 2
	var frames [][]byte
	for i := 0; i < 9; i++ {
		frames = append(frames, syntheticFrame(cfg.Width, cfg.Height, i))
	}
	stream := encodeStream(t, cfg, frames)
	ref := decodeWithFFmpeg(t, stream)
	frameSize := cfg.Width * cfg.Height * 3 / 2
	if len(ref) != frameSize*len(frames) {
		t.Fatalf("ffmpeg produced %d bytes, want %d", len(ref), frameSize*len(frames))
	}
	pics, recons := encodeAndDecodeReordered(t, cfg, frames)
	assertMatchesReconstruction(t, "bipredictive high", pics, recons)
	for i := range pics {
		got := make([]byte, pics[i].Size())
		pics[i].CopyOut(got)
		want := ref[i*frameSize : (i+1)*frameSize]
		for j := range got {
			if got[j] != want[j] {
				t.Fatalf("frame %d: ffmpeg and our decoder disagree at sample %d, ffmpeg %d ours %d",
					i, j, want[j], got[j])
			}
		}
	}
}

func TestFFprobeReportsHighProfile(t *testing.T) {
	bin, err := exec.LookPath("ffprobe")
	if err != nil {
		t.Skip("ffprobe is not installed")
	}
	cfg := high8x8Config(28)
	stream := encodeStream(t, cfg, [][]byte{syntheticFrame(cfg.Width, cfg.Height, 0)})
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
	if !strings.Contains(string(out), "profile=High") {
		t.Fatalf("ffprobe does not report High profile:\n%s", out)
	}
	t.Logf("ffprobe reports:\n%s", out)
}

func TestTransform8x8CostsFewerBits(t *testing.T) {
	for _, cabac := range []bool{false, true} {
		for _, qp := range []int{22, 32} {
			base := high8x8Config(qp)
			base.CABAC = cabac
			base.Transform8x8 = false
			with := base
			with.Transform8x8 = true

			var frames [][]byte
			for i := 0; i < 8; i++ {
				frames = append(frames, syntheticFrame(base.Width, base.Height, i))
			}
			flatBytes, flatTime := measureStream(t, base, frames)
			eightBytes, eightTime := measureStream(t, with, frames)
			t.Logf("cabac %v qp %d: 4x4 only %d bytes in %v, with 8x8 %d bytes in %v (%+.1f%% bits, %.2fx time)",
				cabac, qp, flatBytes, flatTime.Round(time.Millisecond),
				eightBytes, eightTime.Round(time.Millisecond),
				100*float64(eightBytes-flatBytes)/float64(flatBytes),
				float64(eightTime)/float64(flatTime))
		}
	}
}

func encodeStream(t *testing.T, cfg Config, frames [][]byte) []byte {
	t.Helper()
	enc, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	var stream []byte
	for i, f := range frames {
		pkt, err := enc.Encode(f)
		if err != nil {
			t.Fatalf("frame %d: %v", i, err)
		}
		stream = append(stream, pkt...)
	}
	tail, err := enc.Flush()
	if err != nil {
		t.Fatalf("Flush: %v", err)
	}
	return append(stream, tail...)
}

func measureStream(t *testing.T, cfg Config, frames [][]byte) (int, time.Duration) {
	t.Helper()
	start := time.Now()
	stream := encodeStream(t, cfg, frames)
	return len(stream), time.Since(start)
}

type measurement struct {
	bytes int
	took  time.Duration
	psnr  float64
}

func measure(t *testing.T, cfg Config, frames [][]byte) measurement {
	t.Helper()
	start := time.Now()
	stream := encodeStream(t, cfg, frames)
	took := time.Since(start)

	d := decoder.New()
	pics, err := d.Decode(stream)
	if err != nil {
		t.Fatalf("decoding our own stream: %v", err)
	}
	rest, err := d.Flush()
	if err != nil {
		t.Fatalf("flushing our own stream: %v", err)
	}
	pics = append(pics, rest...)
	if len(pics) != len(frames) {
		t.Fatalf("decoded %d pictures for %d frames", len(pics), len(frames))
	}
	var sum float64
	for i, p := range pics {
		got := make([]byte, p.Size())
		p.CopyOut(got)
		sum += psnr(got, frames[i])
	}
	return measurement{bytes: len(stream), took: took, psnr: sum / float64(len(pics))}
}

func encodeAndDecodeReordered(t *testing.T, cfg Config, frames [][]byte) ([]*frame.Picture, []*frame.Picture) {
	t.Helper()
	stream, recons := encodeBStream(t, cfg, frames, nil)
	d := decoder.New()
	pics, err := d.Decode(stream)
	if err != nil {
		t.Fatalf("decoding our own stream: %v", err)
	}
	rest, err := d.Flush()
	if err != nil {
		t.Fatalf("flushing our own stream: %v", err)
	}
	return append(pics, rest...), recons
}

func changedBand(cfg Config, i int) []image.Rectangle {
	top := (i * 16) % cfg.Height
	return []image.Rectangle{image.Rect(0, top, cfg.Width, top+32)}
}

type headerPair struct {
	sps *syntax.SPS
	pps *syntax.PPS
}

func parseHeaders(t *testing.T, headers []byte) headerPair {
	t.Helper()
	var out headerPair
	for _, ebsp := range nal.SplitAnnexB(headers) {
		u, err := nal.Parse(ebsp)
		if err != nil {
			t.Fatalf("parsing our own unit: %v", err)
		}
		switch u.Header.Type {
		case nal.TypeSPS:
			sps, err := syntax.ParseSPS(u.RBSP)
			if err != nil {
				t.Fatalf("re-reading our own sequence parameter set: %v", err)
			}
			out.sps = sps
		case nal.TypePPS:
			pps, err := syntax.ParsePPS(u.RBSP, func(uint32) *syntax.SPS { return out.sps })
			if err != nil {
				t.Fatalf("re-reading our own picture parameter set: %v", err)
			}
			out.pps = pps
		}
	}
	if out.sps == nil || out.pps == nil {
		t.Fatal("our own headers do not carry both parameter sets")
	}
	return out
}

func TestTransform8x8OnRealContent(t *testing.T) {
	if testing.Short() {
		t.Skip("the measurement decodes a corpus clip")
	}
	clips := []testutil.Clip{
		{Name: "main_ip_cabac", Width: 176, Height: 144, Frames: 10},
		{Name: "main_cif_pyramid", Width: 352, Height: 288, Frames: 8},
	}
	for _, clip := range clips {
		all := testutil.LoadReferenceYUV(t, clip)
		var frames [][]byte
		for i := 0; i < clip.Frames; i++ {
			frames = append(frames, clip.Frame(all, i))
		}
		for _, cabac := range []bool{false, true} {
			for _, qp := range []int{18, 26, 34, 42} {
				base := Config{Width: clip.Width, Height: clip.Height, FPSNum: 25, FPSDen: 1,
					GOPSize: clip.Frames, QP: qp, RefFrames: 2, CABAC: cabac}
				with := base
				with.Transform8x8 = true
				flat := measure(t, base, frames)
				eight := measure(t, with, frames)

				pics, _, recons := encodeAndDecode(t, with, frames)
				assertMatchesReconstruction(t, label("real", cabac, qp), pics, recons)

				t.Logf("%s %s: 4x4 %d bytes %.2f dB in %v, 8x8 %d bytes %.2f dB in %v (%+.1f%% bits, %+.2f dB, %.2fx time)",
					clip.Name, label("", cabac, qp),
					flat.bytes, flat.psnr, flat.took.Round(time.Millisecond),
					eight.bytes, eight.psnr, eight.took.Round(time.Millisecond),
					100*float64(eight.bytes-flat.bytes)/float64(flat.bytes),
					eight.psnr-flat.psnr,
					float64(eight.took)/float64(flat.took))
			}
		}
	}
}

func label(what string, cabac bool, qp int) string {
	entropy := "cavlc"
	if cabac {
		entropy = "cabac"
	}
	return what + " " + entropy + " qp " + itoa(qp)
}

func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	var buf [4]byte
	n := len(buf)
	for v > 0 {
		n--
		buf[n] = byte('0' + v%10)
		v /= 10
	}
	return string(buf[n:])
}
