package encoder

import (
	"fmt"
	"sort"
	"testing"
	"time"
)

type sessionPart struct {
	frames func(w, h, n int) [][]byte
	count  int
}

func officeSession() []sessionPart {
	return []sessionPart{
		{typingClip, 45},
		{scrollingTextClip, 10},
		{typingClip, 30},
		{windowDragClip, 10},
		{typingClip, 25},
	}
}

func busySession() []sessionPart {
	var out []sessionPart
	for _, c := range screencastClips() {
		out = append(out, sessionPart{c.frames, 20})
	}
	return out
}

func percentile(d []time.Duration, p float64) time.Duration {
	s := append([]time.Duration(nil), d...)
	sort.Slice(s, func(i, j int) bool { return s[i] < s[j] })
	return s[min(int(p*float64(len(s))), len(s)-1)]
}

func milliseconds(d time.Duration) float64 { return float64(d.Microseconds()) / 1000 }

func BenchmarkLargeScreenSession(b *testing.B) {
	const w, h = 3440, 1440
	sessions := []struct {
		name  string
		parts func() []sessionPart
	}{
		{"office", officeSession},
		{"busy", busySession},
	}
	for _, s := range sessions {
		for _, slices := range []int{1, 4, 16} {
			b.Run(fmt.Sprintf("%s/slices=%d", s.name, slices), func(b *testing.B) {
				var idr, pics []time.Duration
				for i := 0; i < b.N; i++ {
					enc, err := New(Config{Width: w, Height: h, FPSNum: 30, FPSDen: 1, GOPSize: 600, QP: 23,
						RateFactor: 23, Slices: slices})
					if err != nil {
						b.Fatal(err)
					}
					first := true
					for _, part := range s.parts() {
						b.StopTimer()
						frames := part.frames(w, h, part.count)
						b.StartTimer()
						for _, f := range frames {
							start := time.Now()
							if _, err := enc.Encode(f); err != nil {
								b.Fatal(err)
							}
							if first {
								idr = append(idr, time.Since(start))
								first = false
							} else {
								pics = append(pics, time.Since(start))
							}
						}
					}
				}
				b.ReportMetric(milliseconds(percentile(idr, 0.5)), "idr-ms")
				b.ReportMetric(milliseconds(percentile(pics, 0.5)), "p50-ms")
				b.ReportMetric(milliseconds(percentile(pics, 0.95)), "p95-ms")
				b.ReportMetric(milliseconds(percentile(pics, 0.99)), "p99-ms")
			})
		}
	}
}
