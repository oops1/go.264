package main

import (
	"bytes"
	"errors"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func runQuiet(t *testing.T, args []string) error {
	t.Helper()
	oldErr := os.Stderr
	oldOut := os.Stdout
	devNull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = devNull
	os.Stdout = devNull
	defer func() {
		os.Stderr = oldErr
		os.Stdout = oldOut
		devNull.Close()
	}()
	return run(args)
}

func syntheticFrame(w, h, idx int) []byte {
	buf := make([]byte, w*h*3/2)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			v := (x*3 + y*5 + idx*9) % 256
			if (x/8+y/8)%2 == 0 {
				v = 255 - v
			}
			buf[y*w+x] = byte(v)
		}
	}
	cw, ch := w/2, h/2
	for i := 0; i < 2; i++ {
		base := w*h + i*cw*ch
		for y := 0; y < ch; y++ {
			for x := 0; x < cw; x++ {
				buf[base+y*cw+x] = byte((x + y*2 + idx*3 + i*60) % 256)
			}
		}
	}
	return buf
}

func syntheticI420(w, h, frames int) []byte {
	out := make([]byte, 0, w*h*3/2*frames)
	for i := 0; i < frames; i++ {
		out = append(out, syntheticFrame(w, h, i)...)
	}
	return out
}

func psnr(a, b []byte) float64 {
	var sum float64
	for i := range a {
		d := float64(a[i]) - float64(b[i])
		sum += d * d
	}
	if sum == 0 {
		return math.Inf(1)
	}
	return 10 * math.Log10(255*255/(sum/float64(len(a))))
}

func TestRunWithNoArgsReturnsUsage(t *testing.T) {
	for _, args := range [][]string{nil, {}} {
		err := runQuiet(t, args)
		if err == nil {
			t.Fatalf("run(%v) returned nil error, want usage error", args)
		}
		if !strings.Contains(err.Error(), "usage:") {
			t.Fatalf("run(%v) error = %q, want it to contain the usage text", args, err.Error())
		}
	}
}

func TestRunUnknownSubcommand(t *testing.T) {
	err := runQuiet(t, []string{"frobnicate"})
	if err == nil {
		t.Fatal("run with an unknown subcommand returned nil error")
	}
	if !strings.Contains(err.Error(), "frobnicate") {
		t.Fatalf("error = %q, want it to name the unknown subcommand", err.Error())
	}
}

func TestRunBackendsSucceeds(t *testing.T) {
	if err := runQuiet(t, []string{"backends"}); err != nil {
		t.Fatalf("run([\"backends\"]) returned %v, want nil", err)
	}
}

func TestParseSizeValid(t *testing.T) {
	w, h, err := parseSize("1280x720")
	if err != nil {
		t.Fatalf("parseSize(\"1280x720\") returned error: %v", err)
	}
	if w != 1280 || h != 720 {
		t.Fatalf("parseSize(\"1280x720\") = (%d, %d), want (1280, 720)", w, h)
	}

	w, h, err = parseSize("64x48")
	if err != nil {
		t.Fatalf("parseSize(\"64x48\") returned error: %v", err)
	}
	if w != 64 || h != 48 {
		t.Fatalf("parseSize(\"64x48\") = (%d, %d), want (64, 48)", w, h)
	}
}

func TestParseSizeMalformed(t *testing.T) {
	for _, s := range []string{"", "1280", "1280*720", "axb", "1280x"} {
		w, h, err := parseSize(s)
		if err == nil {
			t.Errorf("parseSize(%q) = (%d, %d, nil), want an error", s, w, h)
		}
	}
}

func TestEncodeRequiresSize(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "in.yuv")
	if err := os.WriteFile(in, syntheticI420(16, 16, 1), 0o600); err != nil {
		t.Fatal(err)
	}
	err := runQuiet(t, []string{"encode", "-i", in, "-o", filepath.Join(dir, "out.264")})
	if err == nil {
		t.Fatal("encode without -s returned nil error, want an error")
	}
}

func encodeThenDecode(t *testing.T, w, h, frames int, extraArgs ...string) (src, decoded []byte) {
	t.Helper()
	dir := t.TempDir()
	inPath := filepath.Join(dir, "in.yuv")
	outPath := filepath.Join(dir, "out.264")
	yuvPath := filepath.Join(dir, "out.yuv")

	src = syntheticI420(w, h, frames)
	if err := os.WriteFile(inPath, src, 0o600); err != nil {
		t.Fatal(err)
	}

	size := fmt.Sprintf("%dx%d", w, h)

	encodeArgs := append([]string{"encode", "-s", size, "-qp", "26", "-gop", "3", "-i", inPath, "-o", outPath}, extraArgs...)
	if err := runQuiet(t, encodeArgs); err != nil {
		t.Fatalf("encode %dx%d failed: %v", w, h, err)
	}

	info, err := os.Stat(outPath)
	if err != nil {
		t.Fatalf("could not stat encoded output: %v", err)
	}
	if info.Size() == 0 {
		t.Fatal("encoded output is empty")
	}

	decodeArgs := append([]string{"decode", "-i", outPath, "-o", yuvPath}, extraArgs...)
	if err := runQuiet(t, decodeArgs); err != nil {
		t.Fatalf("decode %dx%d failed: %v", w, h, err)
	}

	decoded, err = os.ReadFile(yuvPath)
	if err != nil {
		t.Fatalf("could not read decoded output: %v", err)
	}
	return src, decoded
}

func TestEncodeDecodeRoundTripAlignedSize(t *testing.T) {
	const w, h, frames = 64, 48, 5
	src, decoded := encodeThenDecode(t, w, h, frames)

	frameSize := w * h * 3 / 2
	want := frameSize * frames
	if len(decoded) != want {
		t.Fatalf("decoded %d bytes, want %d (frames*w*h*3/2)", len(decoded), want)
	}

	for i := 0; i < frames; i++ {
		a := src[i*frameSize : (i+1)*frameSize]
		b := decoded[i*frameSize : (i+1)*frameSize]
		if q := psnr(a, b); q < 30 {
			t.Errorf("frame %d: PSNR only %.2f dB, want > 30 dB", i, q)
		}
	}
}

func TestEncodeDecodeRoundTripUnalignedSize(t *testing.T) {
	const w, h, frames = 60, 40, 5
	src, decoded := encodeThenDecode(t, w, h, frames)

	frameSize := w * h * 3 / 2
	want := frameSize * frames
	if len(decoded) != want {
		t.Fatalf("decoded %d bytes, want %d (frames*w*h*3/2); cropping must survive the pipeline for non-macroblock-aligned sizes", len(decoded), want)
	}

	for i := 0; i < frames; i++ {
		a := src[i*frameSize : (i+1)*frameSize]
		b := decoded[i*frameSize : (i+1)*frameSize]
		if q := psnr(a, b); q < 30 {
			t.Errorf("frame %d: PSNR only %.2f dB, want > 30 dB", i, q)
		}
	}
}

func TestEncodeDecodeRoundTripSoftwareFlag(t *testing.T) {
	const w, h, frames = 32, 32, 3
	src, decoded := encodeThenDecode(t, w, h, frames, "-software")

	frameSize := w * h * 3 / 2
	want := frameSize * frames
	if len(decoded) != want {
		t.Fatalf("decoded %d bytes, want %d", len(decoded), want)
	}
	for i := 0; i < frames; i++ {
		a := src[i*frameSize : (i+1)*frameSize]
		b := decoded[i*frameSize : (i+1)*frameSize]
		if q := psnr(a, b); q < 30 {
			t.Errorf("frame %d: PSNR only %.2f dB, want > 30 dB", i, q)
		}
	}
}

func TestTrailingPartialFrameIsIgnored(t *testing.T) {
	const w, h, frames = 32, 32, 4
	frameSize := w * h * 3 / 2
	clean := syntheticI420(w, h, frames)
	dirty := append(append([]byte(nil), clean...), []byte{1, 2, 3, 4, 5, 6, 7}...)

	dir := t.TempDir()
	inClean := filepath.Join(dir, "clean.yuv")
	inDirty := filepath.Join(dir, "dirty.yuv")
	if err := os.WriteFile(inClean, clean, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(inDirty, dirty, 0o600); err != nil {
		t.Fatal(err)
	}

	outClean := filepath.Join(dir, "clean.264")
	outDirty := filepath.Join(dir, "dirty.264")
	if err := runQuiet(t, []string{"encode", "-s", "32x32", "-i", inClean, "-o", outClean}); err != nil {
		t.Fatalf("encode of the clean input failed: %v", err)
	}
	if err := runQuiet(t, []string{"encode", "-s", "32x32", "-i", inDirty, "-o", outDirty}); err != nil {
		t.Fatalf("encode with a trailing partial frame returned an error, want it to be ignored: %v", err)
	}

	yuvClean := filepath.Join(dir, "clean.yuv.out")
	yuvDirty := filepath.Join(dir, "dirty.yuv.out")
	if err := runQuiet(t, []string{"decode", "-i", outClean, "-o", yuvClean}); err != nil {
		t.Fatal(err)
	}
	if err := runQuiet(t, []string{"decode", "-i", outDirty, "-o", yuvDirty}); err != nil {
		t.Fatal(err)
	}

	decodedClean, err := os.ReadFile(yuvClean)
	if err != nil {
		t.Fatal(err)
	}
	decodedDirty, err := os.ReadFile(yuvDirty)
	if err != nil {
		t.Fatal(err)
	}

	want := frameSize * frames
	if len(decodedClean) != want {
		t.Fatalf("clean decode produced %d bytes, want %d", len(decodedClean), want)
	}
	if len(decodedDirty) != want {
		t.Fatalf("dirty decode (with trailing partial frame) produced %d bytes, want %d (same frame count as clean)", len(decodedDirty), want)
	}
}

func TestEncodeNonexistentInputFails(t *testing.T) {
	dir := t.TempDir()
	err := runQuiet(t, []string{"encode", "-s", "32x32", "-i", filepath.Join(dir, "does-not-exist.yuv"), "-o", filepath.Join(dir, "out.264")})
	if err == nil {
		t.Fatal("encode with a nonexistent input path returned nil error")
	}
}

func TestEncodeUnwritableOutputFails(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "in.yuv")
	if err := os.WriteFile(in, syntheticI420(32, 32, 1), 0o600); err != nil {
		t.Fatal(err)
	}
	blocker := filepath.Join(dir, "blocker")
	if err := os.WriteFile(blocker, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	badOut := filepath.Join(blocker, "x")
	err := runQuiet(t, []string{"encode", "-s", "32x32", "-i", in, "-o", badOut})
	if err == nil {
		t.Fatal("encode with an unwritable output path returned nil error")
	}
}

func TestDecodeGarbageDoesNotPanic(t *testing.T) {
	dir := t.TempDir()
	garbage := filepath.Join(dir, "garbage.264")
	if err := os.WriteFile(garbage, []byte{0x00, 0x00, 0x01, 0xFF, 0x12, 0x34, 0x56, 0x78, 0x9A, 0xDE, 0xAD, 0xBE, 0xEF}, 0o600); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "out.yuv")

	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("decoding garbage input panicked: %v", r)
			}
		}()
		_ = runQuiet(t, []string{"decode", "-i", garbage, "-o", out})
	}()
}

func TestDecodeTruncatedStreamDoesNotPanic(t *testing.T) {
	const w, h, frames = 32, 32, 3
	dir := t.TempDir()
	in := filepath.Join(dir, "in.yuv")
	if err := os.WriteFile(in, syntheticI420(w, h, frames), 0o600); err != nil {
		t.Fatal(err)
	}
	full := filepath.Join(dir, "full.264")
	if err := runQuiet(t, []string{"encode", "-s", "32x32", "-i", in, "-o", full}); err != nil {
		t.Fatal(err)
	}
	stream, err := os.ReadFile(full)
	if err != nil {
		t.Fatal(err)
	}
	if len(stream) < 4 {
		t.Fatal("encoded stream is too small to truncate meaningfully")
	}
	truncated := filepath.Join(dir, "truncated.264")
	if err := os.WriteFile(truncated, stream[:len(stream)/2], 0o600); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "out.yuv")

	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("decoding a truncated stream panicked: %v", r)
			}
		}()
		_ = runQuiet(t, []string{"decode", "-i", truncated, "-o", out})
	}()
}

func TestEncodeInvalidFlagFails(t *testing.T) {
	dir := t.TempDir()
	err := runQuiet(t, []string{"encode", "-s", "32x32", "-not-a-real-flag", "-i", filepath.Join(dir, "in.yuv"), "-o", filepath.Join(dir, "out.264")})
	if err == nil {
		t.Fatal("encode with an unrecognized flag returned nil error")
	}
}

func TestEncodeMalformedSizeFails(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "in.yuv")
	if err := os.WriteFile(in, syntheticI420(32, 32, 1), 0o600); err != nil {
		t.Fatal(err)
	}
	err := runQuiet(t, []string{"encode", "-s", "not-a-size", "-i", in, "-o", filepath.Join(dir, "out.264")})
	if err == nil {
		t.Fatal("encode with a malformed -s value returned nil error")
	}
	if !strings.Contains(err.Error(), "not-a-size") {
		t.Fatalf("error = %q, want it to mention the malformed size", err.Error())
	}
}

func TestEncodeInvalidQPFails(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "in.yuv")
	if err := os.WriteFile(in, syntheticI420(32, 32, 1), 0o600); err != nil {
		t.Fatal(err)
	}
	err := runQuiet(t, []string{"encode", "-s", "32x32", "-qp", "99", "-i", in, "-o", filepath.Join(dir, "out.264")})
	if err == nil {
		t.Fatal("encode with an out-of-range QP returned nil error")
	}
}

func TestDecodeInvalidFlagFails(t *testing.T) {
	dir := t.TempDir()
	err := runQuiet(t, []string{"decode", "-not-a-real-flag", "-i", filepath.Join(dir, "in.264"), "-o", filepath.Join(dir, "out.yuv")})
	if err == nil {
		t.Fatal("decode with an unrecognized flag returned nil error")
	}
}

func TestDecodeNonexistentInputFails(t *testing.T) {
	dir := t.TempDir()
	err := runQuiet(t, []string{"decode", "-i", filepath.Join(dir, "does-not-exist.264"), "-o", filepath.Join(dir, "out.yuv")})
	if err == nil {
		t.Fatal("decode with a nonexistent input path returned nil error")
	}
}

func TestDecodeUnwritableOutputFails(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "in.264")
	if err := os.WriteFile(in, []byte{0x00, 0x00, 0x01, 0x09}, 0o600); err != nil {
		t.Fatal(err)
	}
	blocker := filepath.Join(dir, "blocker")
	if err := os.WriteFile(blocker, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	err := runQuiet(t, []string{"decode", "-i", in, "-o", filepath.Join(blocker, "x")})
	if err == nil {
		t.Fatal("decode with an unwritable output path returned nil error")
	}
}

func TestEncodeDecodeDefaultToStdinStdout(t *testing.T) {
	const w, h, frames = 32, 32, 2
	dir := t.TempDir()
	src := syntheticI420(w, h, frames)
	srcPath := filepath.Join(dir, "in.yuv")
	if err := os.WriteFile(srcPath, src, 0o600); err != nil {
		t.Fatal(err)
	}

	encoded := withRedirectedStdio(t, srcPath, func() error {
		return run([]string{"encode", "-s", "32x32"})
	})
	if len(encoded) == 0 {
		t.Fatal("encoding to stdout produced no bytes")
	}

	encPath := filepath.Join(dir, "enc.264")
	if err := os.WriteFile(encPath, encoded, 0o600); err != nil {
		t.Fatal(err)
	}

	decoded := withRedirectedStdio(t, encPath, func() error {
		return run([]string{"decode"})
	})
	frameSize := w * h * 3 / 2
	if len(decoded) != frameSize*frames {
		t.Fatalf("decoding from stdin/stdout produced %d bytes, want %d", len(decoded), frameSize*frames)
	}
}

func withRedirectedStdio(t *testing.T, inPath string, fn func() error) []byte {
	t.Helper()
	in, err := os.Open(inPath)
	if err != nil {
		t.Fatal(err)
	}
	defer in.Close()

	outFile, err := os.CreateTemp(t.TempDir(), "stdout")
	if err != nil {
		t.Fatal(err)
	}
	devNull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer devNull.Close()

	oldIn, oldOut, oldErr := os.Stdin, os.Stdout, os.Stderr
	os.Stdin = in
	os.Stdout = outFile
	os.Stderr = devNull
	runErr := fn()
	os.Stdin, os.Stdout, os.Stderr = oldIn, oldOut, oldErr

	if runErr != nil {
		t.Fatalf("run failed: %v", runErr)
	}
	outFile.Close()
	data, err := os.ReadFile(outFile.Name())
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestMainExitsNonZeroOnUnknownCommand(t *testing.T) {
	if os.Getenv("GO264_MAIN_SUBPROCESS") == "fail" {
		os.Args = []string{"go264", "not-a-real-subcommand"}
		main()
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestMainExitsNonZeroOnUnknownCommand$")
	cmd.Env = append(os.Environ(), "GO264_MAIN_SUBPROCESS=fail")
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("subprocess succeeded, want a nonzero exit; output: %s", output)
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected *exec.ExitError, got %v (%T)", err, err)
	}
	if exitErr.ExitCode() != 1 {
		t.Fatalf("exit code = %d, want 1", exitErr.ExitCode())
	}
	if !bytes.Contains(output, []byte("go264:")) {
		t.Fatalf("subprocess output = %q, want it to contain the go264: prefix", output)
	}
}

func TestMainSucceedsForBackends(t *testing.T) {
	if os.Getenv("GO264_MAIN_SUBPROCESS") == "ok" {
		os.Args = []string{"go264", "backends"}
		main()
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestMainSucceedsForBackends$")
	cmd.Env = append(os.Environ(), "GO264_MAIN_SUBPROCESS=ok")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("subprocess failed: %v\n%s", err, output)
	}
	if !bytes.Contains(output, []byte("cpu")) {
		t.Fatalf("subprocess output = %q, want it to contain \"cpu\"", output)
	}
}

func encodeWithFlagsThenDecode(t *testing.T, w, h, frames int, encodeArgs ...string) (src, decoded []byte) {
	t.Helper()
	dir := t.TempDir()
	inPath := filepath.Join(dir, "in.yuv")
	outPath := filepath.Join(dir, "out.264")
	yuvPath := filepath.Join(dir, "out.yuv")

	src = syntheticI420(w, h, frames)
	if err := os.WriteFile(inPath, src, 0o600); err != nil {
		t.Fatal(err)
	}
	args := append([]string{"encode", "-s", fmt.Sprintf("%dx%d", w, h), "-qp", "26",
		"-gop", "8", "-i", inPath, "-o", outPath}, encodeArgs...)
	if err := runQuiet(t, args); err != nil {
		t.Fatalf("encode with %v failed: %v", encodeArgs, err)
	}
	if err := runQuiet(t, []string{"decode", "-i", outPath, "-o", yuvPath}); err != nil {
		t.Fatalf("decode after %v failed: %v", encodeArgs, err)
	}
	decoded, err := os.ReadFile(yuvPath)
	if err != nil {
		t.Fatal(err)
	}
	return src, decoded
}

func TestEveryEncodeFlagProducesAStreamThatDecodesBack(t *testing.T) {
	const w, h, frames = 96, 64, 12
	cases := [][]string{
		{},
		{"-cabac"},
		{"-slices", "3"},
		{"-slices", "-1"},
		{"-bframes", "2"},
		{"-bframes", "3", "-cabac", "-refs", "2"},
		{"-exhaustive"},
		{"-nosearch"},
		{"-cabac", "-slices", "2", "-bframes", "1", "-refs", "2"},
	}
	frameSize := w * h * 3 / 2
	for _, args := range cases {
		t.Run(fmt.Sprint(args), func(t *testing.T) {
			src, decoded := encodeWithFlagsThenDecode(t, w, h, frames, args...)
			if len(decoded) != frameSize*frames {
				t.Fatalf("%v: decoded %d frames, want %d; a frame the encoder buffered never came out",
					args, len(decoded)/frameSize, frames)
			}
			for i := 0; i < frames; i++ {
				a := src[i*frameSize : (i+1)*frameSize]
				b := decoded[i*frameSize : (i+1)*frameSize]
				if q := psnr(a, b); q < 30 {
					t.Errorf("%v frame %d: PSNR only %.2f dB", args, i, q)
				}
			}
		})
	}
}

func TestBFramesWithoutFlushingWouldLoseFrames(t *testing.T) {
	const w, h, frames = 96, 64, 10
	_, withB := encodeWithFlagsThenDecode(t, w, h, frames, "-bframes", "3", "-refs", "2", "-gop", "100")
	_, withoutB := encodeWithFlagsThenDecode(t, w, h, frames, "-gop", "100")
	if len(withB) != len(withoutB) {
		t.Fatalf("bi-predictive coding returned %d bytes of pictures against %d without it",
			len(withB), len(withoutB))
	}
}

func encodeWithFlags(t *testing.T, w, h, frames int, extra ...string) []byte {
	t.Helper()
	dir := t.TempDir()
	inPath := filepath.Join(dir, "in.yuv")
	outPath := filepath.Join(dir, "out.264")
	yuvPath := filepath.Join(dir, "out.yuv")
	if err := os.WriteFile(inPath, syntheticI420(w, h, frames), 0o600); err != nil {
		t.Fatal(err)
	}
	args := append([]string{"encode", "-s", fmt.Sprintf("%dx%d", w, h), "-qp", "28",
		"-software", "-i", inPath, "-o", outPath}, extra...)
	if err := runQuiet(t, args); err != nil {
		t.Fatalf("encode %v failed: %v", extra, err)
	}
	if err := runQuiet(t, []string{"decode", "-i", outPath, "-o", yuvPath}); err != nil {
		t.Fatalf("decode after %v failed: %v", extra, err)
	}
	decoded, err := os.ReadFile(yuvPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(decoded) != w*h*3/2*frames {
		t.Fatalf("encode %v produced %d bytes of video, want %d", extra, len(decoded), w*h*3/2*frames)
	}
	stream, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	return stream
}

func TestEncodeAcceptsTheNewPredictionFlags(t *testing.T) {
	cases := [][]string{
		{"-weightp", "explicit"},
		{"-weightp", "implicit", "-bframes", "2"},
		{"-direct", "temporal", "-bframes", "2"},
		{"-intra-refresh", "4", "-repeat-headers"},
	}
	for _, extra := range cases {
		if n := len(encodeWithFlags(t, 64, 48, 8, extra...)); n == 0 {
			t.Fatalf("encode %v produced an empty stream", extra)
		}
	}
}

func TestEncodeRejectsUnknownPredictionFlagValues(t *testing.T) {
	for _, extra := range [][]string{{"-weightp", "sometimes"}, {"-direct", "sideways"}} {
		args := append([]string{"encode", "-s", "64x48", "-qp", "28", "-i", os.DevNull, "-o", os.DevNull}, extra...)
		if err := runQuiet(t, args); err == nil {
			t.Fatalf("encode accepted %v", extra)
		}
	}
}
