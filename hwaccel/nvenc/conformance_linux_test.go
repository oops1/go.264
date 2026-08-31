package nvenc

import (
	"testing"

	go264 "github.com/oops1/go.264"
	"github.com/oops1/go.264/internal/decoder"
)

func TestOurDecoderReadsWhatTheAdapterWrote(t *testing.T) {
	requireAdapter(t)
	stream, name := encodeProbeStream(t)

	d := decoder.New()
	pics, err := d.Decode(stream)
	if err != nil {
		t.Fatalf("our decoder refused the stream from %s: %v", name, err)
	}
	rest, err := d.Flush()
	if err != nil {
		t.Fatalf("our decoder refused to flush the stream from %s: %v", name, err)
	}
	pics = append(pics, rest...)
	if len(pics) == 0 {
		t.Fatalf("%s produced a stream our decoder read as no pictures", name)
	}
	if len(pics) > probeFrames {
		t.Fatalf("%s produced %d pictures from %d frames", name, len(pics), probeFrames)
	}
	for i, p := range pics {
		if p.CropWidth != probeWidth || p.CropHeight != probeHeight {
			t.Fatalf("%s picture %d is %dx%d, want %dx%d",
				name, i, p.CropWidth, p.CropHeight, probeWidth, probeHeight)
		}
	}
	t.Logf("%s: our decoder read %d pictures from %d bytes", name, len(pics), len(stream))
}

func TestOurDecoderAgreesWithFFmpegOnAnAdapterStream(t *testing.T) {
	requireAdapter(t)
	stream, name := encodeProbeStream(t)
	ref := decodeWithFFmpeg(t, stream)

	d := decoder.New()
	pics, err := d.Decode(stream)
	if err != nil {
		t.Fatalf("our decoder refused the stream from %s: %v", name, err)
	}
	rest, err := d.Flush()
	if err != nil {
		t.Fatalf("flush: %v", err)
	}
	pics = append(pics, rest...)

	size := I420Size(probeWidth, probeHeight)
	if len(ref) != size*len(pics) {
		t.Fatalf("ffmpeg produced %d bytes for %d pictures, want %d", len(ref), len(pics), size*len(pics))
	}
	got := make([]byte, size)
	for i, p := range pics {
		p.CopyOut(got)
		want := ref[i*size : (i+1)*size]
		for j := range got {
			if got[j] != want[j] {
				t.Fatalf("%s picture %d: ffmpeg says %d at sample %d, ours says %d",
					name, i, want[j], j, got[j])
			}
		}
	}
	t.Logf("%s: %d pictures decode identically in ffmpeg and in our decoder", name, len(pics))
}

func TestPublicEncoderPicksTheAdapterUp(t *testing.T) {
	requireAdapter(t)
	enc, err := go264.NewEncoder(go264.EncoderConfig{
		Width: probeWidth, Height: probeHeight,
		FPSNum: 25, FPSDen: 1, GOPSize: 30, QP: 26,
	})
	if err != nil {
		t.Fatalf("NewEncoder: %v", err)
	}
	defer enc.Close()
	if enc.Backend() == "cpu" {
		t.Skip("the adapter refused this configuration")
	}
	if enc.Backend() != backendName {
		t.Fatalf("the encoder chose backend %q", enc.Backend())
	}
	total := 0
	for i := 0; i < probeFrames; i++ {
		out, err := enc.Encode(movingPattern(i))
		if err != nil {
			t.Fatalf("frame %d: %v", i, err)
		}
		total += len(out)
	}
	tail, err := enc.Flush()
	if err != nil {
		t.Fatalf("Flush: %v", err)
	}
	total += len(tail)
	if total == 0 {
		t.Fatal("the adapter produced nothing through the public encoder")
	}
	t.Logf("%s produced %d bytes through the public encoder", enc.Backend(), total)
}
