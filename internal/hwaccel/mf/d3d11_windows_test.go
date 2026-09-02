package mf

import (
	"testing"

	"github.com/oops1/go.264/internal/decoder"
	"github.com/oops1/go.264/internal/frame"
	"github.com/oops1/go.264/internal/testutil"
)

func openDirect3DDecoderForTest(t *testing.T) *Decoder {
	t.Helper()
	if !Loaded() {
		t.Skip("Media Foundation is not present on this machine")
	}
	if !d3d11Available() {
		t.Skip("d3d11.dll is not present on this machine")
	}
	dec, err := OpenDecoderWithOptions(DecoderOptions{Direct3D: true})
	if err != nil {
		t.Skipf("no Direct3D decoder could be opened: %v", err)
	}
	return dec
}

func TestDirect3DDeviceCanBeCreatedAndReleased(t *testing.T) {
	if !d3d11Available() {
		t.Skip("d3d11.dll is not present on this machine")
	}
	dev, err := newD3DDevice()
	if err != nil {
		t.Skipf("no Direct3D 11 adapter answered: %v", err)
	}
	if dev.device.p == nil || dev.context.p == nil {
		t.Fatal("D3D11CreateDevice reported success without a device or a context")
	}
	if dev.featureLevel == 0 {
		t.Fatal("D3D11CreateDevice reported no feature level")
	}
	dev.release()
}

func TestStagingTextureRoundTripsNV12(t *testing.T) {
	if !d3d11Available() {
		t.Skip("d3d11.dll is not present on this machine")
	}
	dev, err := newD3DDevice()
	if err != nil {
		t.Skipf("no Direct3D 11 adapter answered: %v", err)
	}
	defer dev.release()
	tex, err := dev.createStagingTexture(64, 32, dxgiFormatNV12)
	if err != nil {
		t.Skipf("the adapter refused an NV12 staging texture: %v", err)
	}
	defer tex.release()
	desc := textureDesc(tex)
	if desc.Width != 64 || desc.Height != 32 || desc.Format != dxgiFormatNV12 {
		t.Fatalf("the staging texture came back as %dx%d format %d", desc.Width, desc.Height, desc.Format)
	}
	if desc.Usage != d3d11UsageStaging || desc.CPUAccessFlags&d3d11CPUAccessRead == 0 {
		t.Fatalf("the staging texture came back with usage %d and access %#x", desc.Usage, desc.CPUAccessFlags)
	}
	mapped, err := dev.mapRead(tex)
	if err != nil {
		t.Fatalf("Map on a staging texture: %v", err)
	}
	if mapped.Data == nil || mapped.RowPitch < 64 {
		t.Fatalf("Map reported data %v with a pitch of %d", mapped.Data, mapped.RowPitch)
	}
	dev.unmap(tex)
}

func TestDeviceManagerResetsTheDevice(t *testing.T) {
	if !Loaded() || !d3d11Available() {
		t.Skip("Media Foundation or Direct3D 11 is not present on this machine")
	}
	if err := Startup(); err != nil {
		t.Skipf("Startup: %v", err)
	}
	defer Shutdown()
	m, err := newDeviceManager()
	if err != nil {
		t.Skipf("no device manager could be made: %v", err)
	}
	if m.obj.p == nil || m.device == nil {
		t.Fatal("the device manager came back empty")
	}
	m.release()
}

func TestDirect3DDecoderBindsTheAdapter(t *testing.T) {
	dec := openDirect3DDecoderForTest(t)
	defer dec.Close()
	if !dec.Direct3D() {
		t.Fatal("a decoder opened for Direct3D reports that it is not")
	}
	clip := testutil.Corpus[0]
	stream := testutil.LoadStream(t, clip)
	pics, err := dec.Decode(stream)
	if err != nil {
		t.Fatalf("%s: Decode: %v", dec.Name(), err)
	}
	rest, err := dec.Flush()
	if err != nil {
		t.Fatalf("%s: Flush: %v", dec.Name(), err)
	}
	if len(pics)+len(rest) == 0 {
		t.Fatalf("%s produced no pictures from %s", dec.Name(), clip.Name)
	}
	if !dec.Accelerated() {
		t.Fatalf("%s took the Direct3D manager but never returned a texture", dec.Name())
	}
	t.Logf("%s decoded %s on the adapter at feature level %#x",
		dec.Name(), clip.Name, dec.FeatureLevel())
}

func TestDirect3DDecoderAgreesWithOursOnEveryClip(t *testing.T) {
	probe := openDirect3DDecoderForTest(t)
	accelerated := probe.Direct3D()
	probe.Close()
	if !accelerated {
		t.Skip("the transform refused the Direct3D device manager")
	}

	clips := append(append([]testutil.Clip{}, testutil.Corpus...), testutil.MainCorpus...)
	for _, clip := range clips {
		clip := clip
		t.Run(clip.Name, func(t *testing.T) {
			stream := testutil.LoadStream(t, clip)

			dec, err := OpenDecoderWithOptions(DecoderOptions{Direct3D: true})
			if err != nil {
				t.Skipf("no Direct3D decoder could be opened: %v", err)
			}
			theirs, err := dec.Decode(stream)
			if err != nil {
				dec.Close()
				t.Fatalf("%s: Decode: %v", dec.Name(), err)
			}
			rest, err := dec.Flush()
			if err != nil {
				dec.Close()
				t.Fatalf("%s: Flush: %v", dec.Name(), err)
			}
			theirs = append(theirs, rest...)
			name, onAdapter := dec.Name(), dec.Accelerated()
			dec.Close()

			if len(theirs) == 0 {
				t.Fatalf("%s produced no pictures from %s", name, clip.Name)
			}
			if !onAdapter {
				t.Fatalf("%s fell back off the adapter on %s", name, clip.Name)
			}

			d := decoder.New()
			ours, err := d.Decode(stream)
			if err != nil {
				t.Fatalf("our decoder refused %s: %v", clip.Name, err)
			}
			more, err := d.Flush()
			if err != nil {
				t.Fatalf("our decoder refused to flush %s: %v", clip.Name, err)
			}
			ours = append(ours, more...)

			n := len(theirs)
			if len(ours) < n {
				n = len(ours)
			}
			if n == 0 {
				t.Fatalf("nothing to compare: %s produced %d pictures, ours %d", name, len(theirs), len(ours))
			}
			size := clip.FrameSize()
			got := make([]byte, size)
			for i := 0; i < n; i++ {
				if len(theirs[i].I420) != size {
					t.Fatalf("%s picture %d holds %d bytes, want %d", name, i, len(theirs[i].I420), size)
				}
				ours[i].CopyOut(got)
				for j := range got {
					if got[j] != theirs[i].I420[j] {
						t.Fatalf("%s picture %d: the adapter says %d at sample %d, ours says %d",
							clip.Name, i, theirs[i].I420[j], j, got[j])
					}
				}
			}
			if len(theirs) != len(ours) {
				t.Logf("%s: the adapter produced %d pictures, ours %d, the first %d agree exactly",
					clip.Name, len(theirs), len(ours), n)
			}
		})
	}
}

func TestDirect3DDecoderSurvivesRepeatedOpenAndClose(t *testing.T) {
	if testing.Short() {
		t.Skip("the repeated open and close cycle is not short")
	}
	probe := openDirect3DDecoderForTest(t)
	probe.Close()

	clip := testutil.Corpus[0]
	stream := testutil.LoadStream(t, clip)
	for i := 0; i < 32; i++ {
		dec, err := OpenDecoderWithOptions(DecoderOptions{Direct3D: true})
		if err != nil {
			t.Fatalf("cycle %d: OpenDecoderWithOptions: %v", i, err)
		}
		if _, err := dec.Decode(stream); err != nil {
			dec.Close()
			t.Fatalf("cycle %d: Decode: %v", i, err)
		}
		if _, err := dec.Flush(); err != nil {
			dec.Close()
			t.Fatalf("cycle %d: Flush: %v", i, err)
		}
		if err := dec.Close(); err != nil {
			t.Fatalf("cycle %d: Close: %v", i, err)
		}
	}
}

func TestDirect3DDecoderCloseIsSafeTwice(t *testing.T) {
	dec := openDirect3DDecoderForTest(t)
	if err := dec.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := dec.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
}

func TestDirect3DDecoderCropsAHeightThatIsNotAMultipleOfSixteen(t *testing.T) {
	if testing.Short() {
		t.Skip("building a 1920x1080 stream is not short")
	}
	probe := openDirect3DDecoderForTest(t)
	probe.Close()

	const w, h = 1920, 1080
	stream := syntheticStream(t, w, h, 6)

	dec, err := OpenDecoderWithOptions(DecoderOptions{Direct3D: true})
	if err != nil {
		t.Skipf("no Direct3D decoder could be opened: %v", err)
	}
	theirs, err := dec.Decode(stream.data)
	if err != nil {
		dec.Close()
		t.Fatalf("Decode: %v", err)
	}
	rest, err := dec.Flush()
	if err != nil {
		dec.Close()
		t.Fatalf("Flush: %v", err)
	}
	theirs = append(theirs, rest...)
	surfaceHeight, onAdapter := dec.surfaceHeight, dec.Accelerated()
	dec.Close()

	if len(theirs) == 0 {
		t.Fatal("the adapter produced no pictures")
	}
	if !onAdapter {
		t.Fatal("the transform never returned a texture")
	}
	if surfaceHeight <= h {
		t.Skipf("the adapter chose a %d row surface, so nothing was cropped", surfaceHeight)
	}
	for i, p := range theirs {
		if p.Width != w || p.Height != h {
			t.Fatalf("picture %d came back as %dx%d", i, p.Width, p.Height)
		}
		if len(p.I420) != I420Size(w, h) {
			t.Fatalf("picture %d holds %d bytes, want %d", i, len(p.I420), I420Size(w, h))
		}
	}

	d := decoder.New()
	ours, err := d.Decode(stream.data)
	if err != nil {
		t.Skipf("our decoder refused the stream: %v", err)
	}
	more, err := d.Flush()
	if err != nil {
		t.Fatalf("our decoder refused to flush: %v", err)
	}
	ours = append(ours, more...)

	n := len(theirs)
	if len(ours) < n {
		n = len(ours)
	}
	if n == 0 {
		t.Fatalf("the adapter produced %d pictures, ours %d", len(theirs), len(ours))
	}
	for i := 0; i < n; i++ {
		got := croppedI420(ours[i], w, h)
		for j := range got {
			if got[j] != theirs[i].I420[j] {
				t.Fatalf("picture %d: the adapter says %d at sample %d, ours says %d",
					i, theirs[i].I420[j], j, got[j])
			}
		}
	}
	t.Logf("a %d row surface cropped to %dx%d agrees with our decoder on %d pictures", surfaceHeight, w, h, n)
}

func croppedI420(p *frame.Picture, w, h int) []byte {
	out := make([]byte, 0, I420Size(w, h))
	for y := 0; y < h; y++ {
		off := p.LumaOffset(0, y)
		out = append(out, p.Y[off:off+w]...)
	}
	for _, plane := range [][]byte{p.Cb, p.Cr} {
		for y := 0; y < h/2; y++ {
			off := p.ChromaOffset(0, y)
			out = append(out, plane[off:off+w/2]...)
		}
	}
	return out
}
