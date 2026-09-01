package go264

import (
	"compress/gzip"
	"fmt"
	"image"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/oops1/go.264/internal/simd"
	"github.com/oops1/go.264/internal/testutil"
)

func findClip(tb testing.TB, name string) testutil.Clip {
	tb.Helper()
	for _, c := range testutil.Corpus {
		if c.Name == name {
			return c
		}
	}
	for _, c := range testutil.MainCorpus {
		if c.Name == name {
			return c
		}
	}
	tb.Fatalf("unknown corpus clip %q", name)
	return testutil.Clip{}
}

func loadClipStream(tb testing.TB, c testutil.Clip) []byte {
	tb.Helper()
	data, err := os.ReadFile(filepath.Join(testutil.CorpusDir(), c.Name+".264"))
	if err != nil {
		tb.Fatalf("reading reference stream for %s: %v", c.Name, err)
	}
	return data
}

func loadClipYUV(tb testing.TB, c testutil.Clip) []byte {
	tb.Helper()
	f, err := os.Open(filepath.Join(testutil.CorpusDir(), c.Name+".yuv.gz"))
	if err != nil {
		tb.Fatalf("opening reference frames for %s: %v", c.Name, err)
	}
	defer f.Close()
	zr, err := gzip.NewReader(f)
	if err != nil {
		tb.Fatalf("decompressing reference frames for %s: %v", c.Name, err)
	}
	defer zr.Close()
	data, err := io.ReadAll(zr)
	if err != nil {
		tb.Fatalf("reading reference frames for %s: %v", c.Name, err)
	}
	want := c.FrameSize() * c.Frames
	if len(data) != want {
		tb.Fatalf("reference frames for %s: got %d bytes, want %d", c.Name, len(data), want)
	}
	return data
}

func TestBenchmarkCorpusLoads(t *testing.T) {
	names := []string{
		"base_intra_qp26",
		"base_ip_qp26",
		"main_intra_cabac",
		"main_ip_cabac",
		"main_ipb_cabac",
	}
	for _, name := range names {
		c := findClip(t, name)
		testutil.LoadStream(t, c)
		testutil.LoadReferenceYUV(t, c)
	}
}

func TestSIMDAccelerationFlag(t *testing.T) {
	t.Logf("simd.Accelerated() = %v", simd.Accelerated())
}

func benchmarkEncodeFrames(b *testing.B, w, h int, frames [][]byte) {
	b.Helper()
	cfg := EncoderConfig{Width: w, Height: h, FPSNum: 25, FPSDen: 1, GOPSize: 1000, QP: 26, RefFrames: 1, ForceSoftware: true}
	enc, err := NewEncoder(cfg)
	if err != nil {
		b.Fatalf("NewEncoder: %v", err)
	}
	defer enc.Close()
	if enc.Backend() != "cpu" {
		b.Fatalf("ForceSoftware still selected backend %q", enc.Backend())
	}
	b.SetBytes(int64(w * h * 3 / 2))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := enc.Encode(frames[i%len(frames)]); err != nil {
			b.Fatalf("Encode: %v", err)
		}
	}
	b.StopTimer()
	b.ReportMetric(float64(b.N)/b.Elapsed().Seconds(), "frames/s")
}

func BenchmarkEncode(b *testing.B) {
	b.Run("176x144", func(b *testing.B) {
		c := findClip(b, "base_intra_qp26")
		yuv := loadClipYUV(b, c)
		frames := make([][]byte, c.Frames)
		for i := 0; i < c.Frames; i++ {
			frames[i] = c.Frame(yuv, i)
		}
		benchmarkEncodeFrames(b, c.Width, c.Height, frames)
	})
	b.Run("1280x720", func(b *testing.B) {
		const w, h = 1280, 720
		const n = 10
		frames := make([][]byte, n)
		for i := 0; i < n; i++ {
			frames[i] = pattern(w, h, i)
		}
		benchmarkEncodeFrames(b, w, h, frames)
	})
}

func BenchmarkDecode(b *testing.B) {
	names := []string{
		"base_intra_qp26",
		"base_ip_qp26",
		"main_intra_cabac",
		"main_ip_cabac",
		"main_ipb_cabac",
	}
	for _, name := range names {
		b.Run(name, func(b *testing.B) {
			c := findClip(b, name)
			stream := loadClipStream(b, c)
			b.SetBytes(int64(c.FrameSize() * c.Frames))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				dec := NewDecoderWithConfig(DecoderConfig{ForceSoftware: true})
				if dec.Backend() != "cpu" {
					b.Fatalf("ForceSoftware still selected backend %q", dec.Backend())
				}
				if _, err := dec.Decode(stream); err != nil {
					b.Fatalf("Decode: %v", err)
				}
				if _, err := dec.Flush(); err != nil {
					b.Fatalf("Flush: %v", err)
				}
				dec.Close()
			}
			b.StopTimer()
			b.ReportMetric(float64(b.N)*float64(c.Frames)/b.Elapsed().Seconds(), "frames/s")
		})
	}
}

func BenchmarkPipeline(b *testing.B) {
	c := findClip(b, "base_intra_qp26")
	yuv := loadClipYUV(b, c)
	frames := make([][]byte, c.Frames)
	for i := 0; i < c.Frames; i++ {
		frames[i] = c.Frame(yuv, i)
	}
	b.SetBytes(int64(c.FrameSize() * c.Frames))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		enc, err := NewEncoder(EncoderConfig{Width: c.Width, Height: c.Height, FPSNum: 25, FPSDen: 1, GOPSize: 1000, QP: 26, RefFrames: 1, ForceSoftware: true})
		if err != nil {
			b.Fatalf("NewEncoder: %v", err)
		}
		var stream []byte
		for _, f := range frames {
			pkt, err := enc.Encode(f)
			if err != nil {
				b.Fatalf("Encode: %v", err)
			}
			stream = append(stream, pkt...)
		}
		enc.Close()

		dec := NewDecoderWithConfig(DecoderConfig{ForceSoftware: true})
		if _, err := dec.Decode(stream); err != nil {
			b.Fatalf("Decode: %v", err)
		}
		if _, err := dec.Flush(); err != nil {
			b.Fatalf("Flush: %v", err)
		}
		dec.Close()
	}
	b.StopTimer()
	b.ReportMetric(float64(b.N)*float64(c.Frames)/b.Elapsed().Seconds(), "frames/s")
}

func desktopFrame(w, h, t int, video image.Rectangle) []byte {
	buf := make([]byte, w*h*3/2)
	cw, ch := w/2, h/2
	for y := 0; y < h; y++ {
		row := buf[y*w:]
		for x := 0; x < w; x++ {
			switch {
			case y < 32:
				row[x] = 64
			case x < 220:
				row[x] = 40 + byte((y/24)%3)*6
			case y%19 < 2 && x%7 < 4 && x > 260 && x < w-80:
				row[x] = 220
			default:
				row[x] = 200
			}
		}
	}
	for y := 0; y < ch; y++ {
		for x := 0; x < cw; x++ {
			buf[w*h+y*cw+x] = 128
			buf[w*h+cw*ch+y*cw+x] = 128
		}
	}
	for y := video.Min.Y; y < video.Max.Y && y < h; y++ {
		row := buf[y*w:]
		for x := video.Min.X; x < video.Max.X && x < w; x++ {
			v := (x*3 + y*5 + t*11) % 256
			if ((x+t*2)/16+y/16)%2 == 0 {
				v = 255 - v
			}
			row[x] = byte(v)
		}
	}
	for y := video.Min.Y / 2; y < video.Max.Y/2 && y < ch; y++ {
		for x := video.Min.X / 2; x < video.Max.X/2 && x < cw; x++ {
			buf[w*h+y*cw+x] = byte((x + y*2 + t*3) % 256)
			buf[w*h+cw*ch+y*cw+x] = byte((x*2 + y + t*5) % 256)
		}
	}
	return buf
}

func BenchmarkEncodeDesktop(b *testing.B) {
	const w, h = 1920, 1080
	const n = 12
	video := image.Rect(640, 300, 1280, 660)
	frames := make([][]byte, n)
	for i := 0; i < n; i++ {
		frames[i] = desktopFrame(w, h, i, video)
	}
	cfg := EncoderConfig{Width: w, Height: h, FPSNum: 30, FPSDen: 1, GOPSize: 1000, QP: 26,
		RefFrames: 1, ForceSoftware: true}

	b.Run("whole screen", func(b *testing.B) {
		enc, err := NewEncoder(cfg)
		if err != nil {
			b.Fatal(err)
		}
		defer enc.Close()
		b.SetBytes(int64(w * h * 3 / 2))
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if _, err := enc.Encode(frames[i%n]); err != nil {
				b.Fatal(err)
			}
		}
		b.StopTimer()
		b.ReportMetric(float64(b.N)/b.Elapsed().Seconds(), "frames/s")
	})

	b.Run("whole screen, exhaustive", func(b *testing.B) {
		slow := cfg
		slow.ModeDecision = ModeDecisionExhaustive
		enc, err := NewEncoder(slow)
		if err != nil {
			b.Fatal(err)
		}
		defer enc.Close()
		b.SetBytes(int64(w * h * 3 / 2))
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if _, err := enc.Encode(frames[i%n]); err != nil {
				b.Fatal(err)
			}
		}
		b.StopTimer()
		b.ReportMetric(float64(b.N)/b.Elapsed().Seconds(), "frames/s")
	})

	for _, slices := range []int{2, 4, 8, 17, 34, 68} {
		b.Run(fmt.Sprintf("whole screen, %d slices", slices), func(b *testing.B) {
			parallel := cfg
			parallel.Slices = slices
			enc, err := NewEncoder(parallel)
			if err != nil {
				b.Fatal(err)
			}
			defer enc.Close()
			b.SetBytes(int64(w * h * 3 / 2))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := enc.Encode(frames[i%n]); err != nil {
					b.Fatal(err)
				}
			}
			b.StopTimer()
			b.ReportMetric(float64(b.N)/b.Elapsed().Seconds(), "frames/s")
		})
	}

	b.Run("video region named", func(b *testing.B) {
		enc, err := NewEncoder(cfg)
		if err != nil {
			b.Fatal(err)
		}
		defer enc.Close()
		changed := []image.Rectangle{video}
		b.SetBytes(int64(w * h * 3 / 2))
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if _, err := enc.EncodeWithHints(frames[i%n], changed, nil); err != nil {
				b.Fatal(err)
			}
		}
		b.StopTimer()
		b.ReportMetric(float64(b.N)/b.Elapsed().Seconds(), "frames/s")
	})
}
