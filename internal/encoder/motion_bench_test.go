package encoder

import (
	"math/rand"
	"testing"
)

func motionBenchEncoder(b *testing.B) *Encoder {
	b.Helper()
	cfg := Config{Width: 176, Height: 144, FPSNum: 25, FPSDen: 1, GOPSize: 1000, QP: 26, RefFrames: 1}
	e, err := New(cfg)
	if err != nil {
		b.Fatalf("New: %v", err)
	}
	if _, err := e.Encode(syntheticFrame(cfg.Width, cfg.Height, 0)); err != nil {
		b.Fatalf("Encode reference frame: %v", err)
	}
	e.loadSourceInto(e.src, syntheticFrame(cfg.Width, cfg.Height, 1))
	e.refL0 = append(e.refL0[:0], e.refs...)
	return e
}

func BenchmarkMotionEstimation(b *testing.B) {
	e := motionBenchEncoder(b)
	lambda := lambdaTable[e.cfg.QP]
	part := partition{0, 0, 16, 16}
	b.SetBytes(int64(e.cfg.Width * e.cfg.Height))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for j := range e.grid {
			e.grid[j] = mbInfo{}
		}
		s := &mbEncoder{e: e, qpY: e.cfg.QP, isP: true, numRefs: 1}
		for mby := 0; mby < e.heightMBs; mby++ {
			for mbx := 0; mbx < e.widthMBs; mbx++ {
				s.mbx = mbx
				s.mby = mby
				s.cur = e.at(mbx, mby)
				for k := range s.cur.refIdx {
					s.cur.refIdx[k] = -1
				}
				s.nb = e.around(mbx, mby, sliceRange{0, e.widthMBs * e.heightMBs})
				r := s.searchPartition(part, 0, mbTypeP16x16, lambda)
				s.storePartitionMotion(part, r.mv, r.ref)
				s.cur.Decoded = true
			}
		}
	}
	b.StopTimer()
	b.ReportMetric(float64(b.N)/b.Elapsed().Seconds(), "frames/s")
}

func BenchmarkSAD(b *testing.B) {
	rng := rand.New(rand.NewSource(20260831))
	src := make([]byte, 64*64)
	ref := make([]byte, 64*64)
	for i := range src {
		src[i] = byte(rng.Intn(256))
	}
	for i := range ref {
		ref[i] = byte(rng.Intn(256))
	}
	b.SetBytes(16 * 16)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sad(src, 64, 0, ref, 64, 0, 16, 16)
	}
	b.StopTimer()
	b.ReportMetric(float64(b.N)/b.Elapsed().Seconds(), "frames/s")
}
