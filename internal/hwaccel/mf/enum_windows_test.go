package mf

import "testing"

func TestListH264EncodersNamesEveryTransform(t *testing.T) {
	if !Loaded() {
		t.Skip("Media Foundation is not present on this machine")
	}
	list, err := ListH264Encoders()
	if err != nil {
		t.Fatalf("ListH264Encoders: %v", err)
	}
	if len(list) == 0 {
		t.Skip("this Windows install carries no H.264 encoder transform")
	}
	for i, d := range list {
		if d.Name == "" {
			t.Fatalf("encoder %d has no friendly name", i)
		}
		t.Logf("encoder hardware=%v async=%v %s", d.Hardware, d.Async, d.Name)
	}
}

func TestListH264DecodersNamesEveryTransform(t *testing.T) {
	if !Loaded() {
		t.Skip("Media Foundation is not present on this machine")
	}
	list, err := ListH264Decoders()
	if err != nil {
		t.Fatalf("ListH264Decoders: %v", err)
	}
	if len(list) == 0 {
		t.Skip("this Windows install carries no H.264 decoder transform")
	}
	for i, d := range list {
		if d.Name == "" {
			t.Fatalf("decoder %d has no friendly name", i)
		}
		t.Logf("decoder hardware=%v async=%v %s", d.Hardware, d.Async, d.Name)
	}
}

func TestListTransformsOfAnUnknownCategoryIsEmptyRatherThanFailing(t *testing.T) {
	if !Loaded() {
		t.Skip("Media Foundation is not present on this machine")
	}
	list, err := ListTransforms(GUID{Data1: 0xDEADBEEF}, nil, nil)
	if err != nil {
		t.Fatalf("ListTransforms: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("an unknown category produced %d transforms", len(list))
	}
}

func TestOpenTransformReportsWhenNothingIsAcceptable(t *testing.T) {
	if !Loaded() {
		t.Skip("Media Foundation is not present on this machine")
	}
	if err := Startup(); err != nil {
		t.Fatalf("Startup: %v", err)
	}
	defer Shutdown()
	_, out := H264EncoderTypes()
	_, _, err := openTransform(MFTCategoryVideoEncoder, nil, &out,
		func(TransformDescription) bool { return false })
	if err == nil {
		t.Fatal("openTransform accepted a transform after every candidate was refused")
	}
}

func TestOpenTransformActivatesAndReportsWhatItChose(t *testing.T) {
	if !Loaded() {
		t.Skip("Media Foundation is not present on this machine")
	}
	if err := Startup(); err != nil {
		t.Fatalf("Startup: %v", err)
	}
	defer Shutdown()
	_, out := H264EncoderTypes()
	tr, chosen, err := openTransform(MFTCategoryVideoEncoder, nil, &out, nil)
	if err != nil {
		t.Skipf("no H.264 encoder transform could be activated: %v", err)
	}
	defer tr.Release()
	if chosen.Name == "" {
		t.Fatal("openTransform chose a transform with no name")
	}
	in, outStream, err := tr.StreamIDs()
	if err != nil {
		t.Fatalf("StreamIDs: %v", err)
	}
	t.Logf("opened %q with input stream %d and output stream %d", chosen.Name, in, outStream)
}
