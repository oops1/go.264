package mf

import "testing"

func TestPlatformStartupAndShutdownBalance(t *testing.T) {
	if !Loaded() {
		t.Skip("Media Foundation is not present on this machine")
	}
	if err := Startup(); err != nil {
		t.Fatalf("Startup: %v", err)
	}
	if err := Startup(); err != nil {
		t.Fatalf("nested Startup: %v", err)
	}
	if platform.count != 2 {
		t.Fatalf("two starts left a count of %d, want 2", platform.count)
	}
	if err := Shutdown(); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	if platform.count != 1 {
		t.Fatalf("one shutdown left a count of %d, want 1", platform.count)
	}
	if err := Shutdown(); err != nil {
		t.Fatalf("final Shutdown: %v", err)
	}
	if platform.count != 0 {
		t.Fatalf("the final shutdown left a count of %d, want 0", platform.count)
	}
	if err := Shutdown(); err != nil {
		t.Fatalf("shutting down what was never started: %v", err)
	}
}

func TestPlatformStartsAgainAfterAFullShutdown(t *testing.T) {
	if !Loaded() {
		t.Skip("Media Foundation is not present on this machine")
	}
	for i := 0; i < 3; i++ {
		if err := Startup(); err != nil {
			t.Fatalf("Startup round %d: %v", i, err)
		}
		if err := Shutdown(); err != nil {
			t.Fatalf("Shutdown round %d: %v", i, err)
		}
	}
}

func TestOpeningLeavesTheCountWhereItWasWhicheverWayItEnds(t *testing.T) {
	if !Loaded() {
		t.Skip("Media Foundation is not present on this machine")
	}
	before := platform.count
	for _, opt := range []DecoderOptions{
		{},
		{HardwareTransform: true},
		{Direct3D: true},
		{HardwareTransform: true, Direct3D: true},
	} {
		d, err := OpenDecoderWithOptions(opt)
		if err == nil {
			if err := d.Close(); err != nil {
				t.Fatalf("%+v: Close: %v", opt, err)
			}
		}
		if platform.count != before {
			t.Fatalf("opening a decoder with %+v ended in %v and left the count at %d, want %d",
				opt, err, platform.count, before)
		}
	}
	format := EncoderFormat{Width: 320, Height: 240, FPSNum: 30, FPSDen: 1, BitrateBitsPerSecond: 400000}
	for _, hardware := range []bool{false, true} {
		e, err := OpenEncoder(format, hardware)
		if err == nil {
			if err := e.Close(); err != nil {
				t.Fatalf("hardware %v: Close: %v", hardware, err)
			}
		}
		if platform.count != before {
			t.Fatalf("opening an encoder with hardware %v ended in %v and left the count at %d, want %d",
				hardware, err, platform.count, before)
		}
	}
}

func TestClosingAnUnconfiguredDecoderStillReleasesThePlatform(t *testing.T) {
	if !Loaded() {
		t.Skip("Media Foundation is not present on this machine")
	}
	before := platform.count
	thread := newCOMThread()
	defer thread.stop()
	var opened bool
	thread.run(func() {
		if err := Startup(); err != nil {
			return
		}
		inInfo, _ := H264DecoderTypes()
		transform, chosen, err := openTransform(MFTCategoryVideoDecoder, &inInfo, nil,
			func(TransformDescription) bool { return true })
		if err != nil {
			Shutdown()
			return
		}
		opened = true
		d := &Decoder{name: chosen.Name, transform: transform}
		d.closeHere()
	})
	if !opened {
		t.Skip("no decoder transform to open on this machine")
	}
	if platform.count != before {
		t.Fatalf("closing a decoder whose configuration never finished left the count at %d, want %d",
			platform.count, before)
	}
}

func TestClosingAnUnconfiguredEncoderStillReleasesThePlatform(t *testing.T) {
	if !Loaded() {
		t.Skip("Media Foundation is not present on this machine")
	}
	before := platform.count
	thread := newCOMThread()
	defer thread.stop()
	var opened bool
	thread.run(func() {
		if err := Startup(); err != nil {
			return
		}
		_, outInfo := H264EncoderTypes()
		transform, chosen, err := openTransform(MFTCategoryVideoEncoder, nil, &outInfo,
			func(TransformDescription) bool { return true })
		if err != nil {
			Shutdown()
			return
		}
		opened = true
		e := &Encoder{name: chosen.Name, transform: transform}
		e.closeHere()
	})
	if !opened {
		t.Skip("no encoder transform to open on this machine")
	}
	if platform.count != before {
		t.Fatalf("closing an encoder whose configuration never finished left the count at %d, want %d",
			platform.count, before)
	}
}
