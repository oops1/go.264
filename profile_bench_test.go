package go264

import "testing"

func profileEncoderConfig(w, h int, extra bool) EncoderConfig {
	cfg := EncoderConfig{
		Width: w, Height: h, FPSNum: 25, FPSDen: 1, GOPSize: 30, QP: 26,
		RefFrames: 1, ForceSoftware: true,
	}
	if extra {
		cfg.CABAC = true
		cfg.BFrames = 2
		cfg.Transform8x8 = true
		cfg.Trellis = true
		cfg.RefFrames = 2
	}
	return cfg
}

func benchmarkProfileEncode(b *testing.B, w, h int, extra bool) {
	b.Helper()
	const n = 8
	frames := make([][]byte, n)
	for i := 0; i < n; i++ {
		frames[i] = pattern(w, h, i)
	}
	cfg := profileEncoderConfig(w, h, extra)
	enc, err := NewEncoder(cfg)
	if err != nil {
		b.Fatalf("NewEncoder: %v", err)
	}
	defer enc.Close()
	if enc.Backend() != "cpu" {
		b.Fatalf("ForceSoftware still selected backend %q", enc.Backend())
	}
	b.SetBytes(int64(w * h * 3 / 2))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := enc.Encode(frames[i%n]); err != nil {
			b.Fatalf("Encode: %v", err)
		}
	}
	b.StopTimer()
	b.ReportMetric(float64(b.N)/b.Elapsed().Seconds(), "frames/s")
}

func BenchmarkProfileEncode1080pDefault(b *testing.B) {
	benchmarkProfileEncode(b, 1920, 1080, false)
}

func BenchmarkProfileEncode1080pHeavy(b *testing.B) {
	benchmarkProfileEncode(b, 1920, 1080, true)
}

func BenchmarkProfileEncode480pDefault(b *testing.B) {
	benchmarkProfileEncode(b, 640, 480, false)
}

func BenchmarkProfileEncode480pHeavy(b *testing.B) {
	benchmarkProfileEncode(b, 640, 480, true)
}
