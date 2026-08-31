package mf

import (
	"testing"

	"github.com/oops1/go.264/internal/decoder"
	"github.com/oops1/go.264/internal/testutil"
)

func benchClip(b *testing.B, name string) ([]byte, int) {
	b.Helper()
	all := append(append([]testutil.Clip{}, testutil.Corpus...), testutil.MainCorpus...)
	for _, c := range all {
		if c.Name != name {
			continue
		}
		t := &testing.T{}
		stream := testutil.LoadStream(t, c)
		if t.Failed() {
			b.Skipf("the clip %s could not be loaded", name)
		}
		return stream, c.Width * c.Height * c.Frames
	}
	b.Fatalf("no clip named %s", name)
	return nil, 0
}

func BenchmarkTransformDecode(b *testing.B) {
	if !Loaded() {
		b.Skip("Media Foundation is not present on this machine")
	}
	for _, name := range []string{"base_ip_qp26", "main_ip_cabac", "main_ipb_cabac"} {
		stream, pixels := benchClip(b, name)
		b.Run(name, func(b *testing.B) {
			b.SetBytes(int64(pixels))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				dec, err := OpenDecoder(false)
				if err != nil {
					b.Skipf("OpenDecoder: %v", err)
				}
				if _, err := dec.Decode(stream); err != nil {
					b.Fatalf("Decode: %v", err)
				}
				if _, err := dec.Flush(); err != nil {
					b.Fatalf("Flush: %v", err)
				}
				dec.Close()
			}
		})
	}
}

func BenchmarkOurDecodeForComparison(b *testing.B) {
	for _, name := range []string{"base_ip_qp26", "main_ip_cabac", "main_ipb_cabac"} {
		stream, pixels := benchClip(b, name)
		b.Run(name, func(b *testing.B) {
			b.SetBytes(int64(pixels))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				d := decoder.New()
				if _, err := d.Decode(stream); err != nil {
					b.Fatalf("Decode: %v", err)
				}
				if _, err := d.Flush(); err != nil {
					b.Fatalf("Flush: %v", err)
				}
			}
		})
	}
}

func BenchmarkOpenAndCloseDecoder(b *testing.B) {
	if !Loaded() {
		b.Skip("Media Foundation is not present on this machine")
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		dec, err := OpenDecoder(false)
		if err != nil {
			b.Skipf("OpenDecoder: %v", err)
		}
		dec.Close()
	}
}

func BenchmarkTransformDecodeWithoutSetup(b *testing.B) {
	if !Loaded() {
		b.Skip("Media Foundation is not present on this machine")
	}
	stream, pixels := benchClip(b, "main_ip_cabac")
	dec, err := OpenDecoder(false)
	if err != nil {
		b.Skipf("OpenDecoder: %v", err)
	}
	defer dec.Close()
	b.SetBytes(int64(pixels))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := dec.Decode(stream); err != nil {
			b.Fatalf("Decode: %v", err)
		}
		if _, err := dec.Flush(); err != nil {
			b.Fatalf("Flush: %v", err)
		}
	}
}
