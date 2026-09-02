package mf

import (
	"strconv"
	"testing"

	"github.com/oops1/go.264/internal/decoder"
	"github.com/oops1/go.264/internal/testutil"
)

type benchStream struct {
	name   string
	width  int
	height int
	data   []byte
}

func syntheticFrame(w, h, n int) []byte {
	buf := make([]byte, I420Size(w, h))
	for y := 0; y < h; y++ {
		row := buf[y*w : y*w+w]
		for x := 0; x < w; x++ {
			row[x] = byte((x*3 + y*5 + n*11) ^ (x >> 3) ^ (y >> 2))
		}
	}
	cw, ch := w/2, h/2
	cb := buf[w*h : w*h+cw*ch]
	cr := buf[w*h+cw*ch:]
	for y := 0; y < ch; y++ {
		for x := 0; x < cw; x++ {
			cb[y*cw+x] = byte(96 + (x+n)%64)
			cr[y*cw+x] = byte(160 - (y+n)%64)
		}
	}
	return buf
}

func syntheticStream(tb testing.TB, w, h, frames int) benchStream {
	tb.Helper()
	enc, err := OpenEncoder(EncoderFormat{
		Width:                w,
		Height:               h,
		FPSNum:               25,
		FPSDen:               1,
		BitrateBitsPerSecond: w * h * 25 / 8,
		Profile:              AVEncH264VProfileMain,
	}, false)
	if err != nil {
		tb.Skipf("no encoder transform could be opened for %dx%d: %v", w, h, err)
	}
	defer enc.Close()
	var data []byte
	for i := 0; i < frames; i++ {
		packets, err := enc.Encode(syntheticFrame(w, h, i))
		if err != nil {
			tb.Skipf("the encoder refused a %dx%d frame: %v", w, h, err)
		}
		for _, p := range packets {
			data = append(data, p...)
		}
	}
	packets, err := enc.Drain()
	if err != nil {
		tb.Skipf("the encoder refused to drain at %dx%d: %v", w, h, err)
	}
	for _, p := range packets {
		data = append(data, p...)
	}
	if len(data) == 0 {
		tb.Skipf("the encoder produced nothing at %dx%d", w, h)
	}
	return benchStream{name: "synthetic", width: w, height: h, data: data}
}

func corpusStream(tb testing.TB, name string) benchStream {
	tb.Helper()
	all := append(append([]testutil.Clip{}, testutil.Corpus...), testutil.MainCorpus...)
	for _, c := range all {
		if c.Name != name {
			continue
		}
		t := &testing.T{}
		stream := testutil.LoadStream(t, c)
		if t.Failed() {
			tb.Skipf("the clip %s could not be loaded", name)
		}
		return benchStream{name: c.Name, width: c.Width, height: c.Height, data: stream}
	}
	tb.Skipf("no clip named %s", name)
	return benchStream{}
}

func benchStreams(tb testing.TB) []benchStream {
	return []benchStream{
		corpusStream(tb, "main_ip_cabac"),
		corpusStream(tb, "main_cif_pyramid"),
		syntheticStream(tb, 640, 480, 30),
		syntheticStream(tb, 1280, 720, 30),
		syntheticStream(tb, 1920, 1080, 30),
	}
}

func benchmarkTransformStream(b *testing.B, s benchStream, opt DecoderOptions, drain bool) {
	b.Helper()
	if !Loaded() {
		b.Skip("Media Foundation is not present on this machine")
	}
	dec, err := OpenDecoderWithOptions(opt)
	if err != nil {
		b.Skipf("no decoder transform could be opened: %v", err)
	}
	defer dec.Close()
	if opt.Direct3D && !dec.Direct3D() {
		b.Skip("the transform refused the Direct3D device manager")
	}
	if _, err := dec.Decode(s.data); err != nil {
		b.Skipf("Decode: %v", err)
	}
	if _, err := dec.Flush(); err != nil {
		b.Skipf("Flush: %v", err)
	}
	pictures := 0
	b.SetBytes(int64(s.width * s.height * 3 / 2))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pics, err := dec.Decode(s.data)
		if err != nil {
			b.Fatalf("Decode: %v", err)
		}
		pictures += len(pics)
		if !drain {
			continue
		}
		rest, err := dec.Flush()
		if err != nil {
			b.Fatalf("Flush: %v", err)
		}
		pictures += len(rest)
	}
	b.StopTimer()
	if pictures == 0 {
		b.Skipf("%s produced no pictures", dec.Name())
	}
	if opt.Direct3D && !dec.Accelerated() {
		b.Skip("the transform never returned a texture")
	}
	b.ReportMetric(float64(pictures)/b.Elapsed().Seconds(), "frames/s")
}

func benchmarkOurStream(b *testing.B, s benchStream, drain bool) {
	b.Helper()
	d := decoder.New()
	if _, err := d.Decode(s.data); err != nil {
		b.Skipf("our decoder refused the stream: %v", err)
	}
	if _, err := d.Flush(); err != nil {
		b.Skipf("our decoder refused to flush: %v", err)
	}
	pictures := 0
	b.SetBytes(int64(s.width * s.height * 3 / 2))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pics, err := d.Decode(s.data)
		if err != nil {
			b.Fatalf("Decode: %v", err)
		}
		pictures += len(pics)
		if !drain {
			continue
		}
		rest, err := d.Flush()
		if err != nil {
			b.Fatalf("Flush: %v", err)
		}
		pictures += len(rest)
	}
	b.StopTimer()
	b.ReportMetric(float64(pictures)/b.Elapsed().Seconds(), "frames/s")
}

func BenchmarkDecodeOnTheAdapter(b *testing.B) {
	for _, s := range benchStreams(b) {
		b.Run(sizeLabel(s), func(b *testing.B) {
			benchmarkTransformStream(b, s, DecoderOptions{Direct3D: true}, true)
		})
	}
}

func BenchmarkDecodeOnTheSoftwareTransform(b *testing.B) {
	for _, s := range benchStreams(b) {
		b.Run(sizeLabel(s), func(b *testing.B) {
			benchmarkTransformStream(b, s, DecoderOptions{}, true)
		})
	}
}

func BenchmarkDecodeOnTheProcessor(b *testing.B) {
	for _, s := range benchStreams(b) {
		b.Run(sizeLabel(s), func(b *testing.B) {
			benchmarkOurStream(b, s, true)
		})
	}
}

func BenchmarkStreamOnTheAdapter(b *testing.B) {
	for _, s := range benchStreams(b) {
		b.Run(sizeLabel(s), func(b *testing.B) {
			benchmarkTransformStream(b, s, DecoderOptions{Direct3D: true}, false)
		})
	}
}

func BenchmarkStreamOnTheSoftwareTransform(b *testing.B) {
	for _, s := range benchStreams(b) {
		b.Run(sizeLabel(s), func(b *testing.B) {
			benchmarkTransformStream(b, s, DecoderOptions{}, false)
		})
	}
}

func BenchmarkStreamOnTheProcessor(b *testing.B) {
	for _, s := range benchStreams(b) {
		b.Run(sizeLabel(s), func(b *testing.B) {
			benchmarkOurStream(b, s, false)
		})
	}
}

func sizeLabel(s benchStream) string {
	return s.name + "_" + strconv.Itoa(s.width) + "x" + strconv.Itoa(s.height)
}
