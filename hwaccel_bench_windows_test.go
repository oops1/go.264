package go264

import (
	"testing"

	"github.com/oops1/go.264/internal/hwaccel/mf"
)

func benchmarkHardwareEncodeFrames(b *testing.B, w, h int, frames [][]byte) {
	b.Helper()
	if !mf.Loaded() {
		b.Skip("Media Foundation is not present on this machine")
	}
	cfg := EncoderConfig{Width: w, Height: h, FPSNum: 25, FPSDen: 1, GOPSize: 1000, QP: 26, RefFrames: 1}
	enc, err := NewEncoder(cfg)
	if err != nil {
		b.Fatalf("NewEncoder: %v", err)
	}
	defer enc.Close()
	if enc.Backend() == "cpu" {
		b.Skip("no hardware encoder could be opened on this machine")
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

func BenchmarkHardwareEncode(b *testing.B) {
	b.Run("176x144", func(b *testing.B) {
		c := findClip(b, "base_intra_qp26")
		yuv := loadClipYUV(b, c)
		frames := make([][]byte, c.Frames)
		for i := 0; i < c.Frames; i++ {
			frames[i] = c.Frame(yuv, i)
		}
		benchmarkHardwareEncodeFrames(b, c.Width, c.Height, frames)
	})
	b.Run("1280x720", func(b *testing.B) {
		const w, h = 1280, 720
		const n = 10
		frames := make([][]byte, n)
		for i := 0; i < n; i++ {
			frames[i] = pattern(w, h, i)
		}
		benchmarkHardwareEncodeFrames(b, w, h, frames)
	})
	b.Run("1920x1080", func(b *testing.B) {
		const w, h = 1920, 1080
		const n = 10
		frames := make([][]byte, n)
		for i := 0; i < n; i++ {
			frames[i] = pattern(w, h, i)
		}
		benchmarkHardwareEncodeFrames(b, w, h, frames)
	})
}
