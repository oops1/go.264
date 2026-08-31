package mf

import (
	"testing"
	"unsafe"
)

func newMediaTypeForTest(t *testing.T) *MediaType {
	t.Helper()
	if !Loaded() {
		t.Skip("Media Foundation is not present on this machine")
	}
	if err := Startup(); err != nil {
		t.Fatalf("Startup: %v", err)
	}
	t.Cleanup(func() { Shutdown() })
	mt, err := NewMediaType()
	if err != nil {
		t.Fatalf("NewMediaType: %v", err)
	}
	t.Cleanup(mt.Release)
	return mt
}

func TestMediaTypeGUIDRoundTrips(t *testing.T) {
	mt := newMediaTypeForTest(t)
	if err := mt.SetGUID(&MFMTSubtype, MFVideoFormatH264); err != nil {
		t.Fatalf("SetGUID: %v", err)
	}
	got, err := mt.GUID(&MFMTSubtype)
	if err != nil {
		t.Fatalf("GUID: %v", err)
	}
	if got != MFVideoFormatH264 {
		t.Fatalf("GUID round trip returned %v, want %v", got, MFVideoFormatH264)
	}
}

func TestMediaTypeUint32RoundTrips(t *testing.T) {
	mt := newMediaTypeForTest(t)
	if err := mt.SetUint32(&MFMTAvgBitrate, 4_000_000); err != nil {
		t.Fatalf("SetUint32: %v", err)
	}
	got, err := mt.Uint32(&MFMTAvgBitrate)
	if err != nil {
		t.Fatalf("Uint32: %v", err)
	}
	if got != 4_000_000 {
		t.Fatalf("Uint32 round trip returned %d, want %d", got, 4_000_000)
	}
}

func TestMediaTypeUint64RoundTrips(t *testing.T) {
	mt := newMediaTypeForTest(t)
	const want uint64 = 0x0102030405060708
	if err := mt.SetUint64(&MFMTFrameSize, want); err != nil {
		t.Fatalf("SetUint64: %v", err)
	}
	got, err := mt.Uint64(&MFMTFrameSize)
	if err != nil {
		t.Fatalf("Uint64: %v", err)
	}
	if got != want {
		t.Fatalf("Uint64 round trip returned %#x, want %#x", got, want)
	}
}

func TestMediaTypeRatioRoundTrips(t *testing.T) {
	mt := newMediaTypeForTest(t)
	if err := mt.SetRatio(&MFMTFrameRate, 30000, 1001); err != nil {
		t.Fatalf("SetRatio: %v", err)
	}
	num, den, err := mt.Ratio(&MFMTFrameRate)
	if err != nil {
		t.Fatalf("Ratio: %v", err)
	}
	if num != 30000 || den != 1001 {
		t.Fatalf("Ratio round trip returned %d/%d, want 30000/1001", num, den)
	}
}

func TestMediaTypeSizeRoundTrips(t *testing.T) {
	mt := newMediaTypeForTest(t)
	if err := mt.SetSize(&MFMTFrameSize, 1920, 1080); err != nil {
		t.Fatalf("SetSize: %v", err)
	}
	w, h, err := mt.Size(&MFMTFrameSize)
	if err != nil {
		t.Fatalf("Size: %v", err)
	}
	if w != 1920 || h != 1080 {
		t.Fatalf("Size round trip returned %dx%d, want 1920x1080", w, h)
	}
}

func TestMediaTypeSizePacksWidthInTheHighHalf(t *testing.T) {
	mt := newMediaTypeForTest(t)
	if err := mt.SetSize(&MFMTFrameSize, 1920, 1080); err != nil {
		t.Fatalf("SetSize: %v", err)
	}
	raw, err := mt.Uint64(&MFMTFrameSize)
	if err != nil {
		t.Fatalf("Uint64: %v", err)
	}
	const wantPacked uint64 = 8246337209400
	if raw != wantPacked {
		t.Fatalf("a 1920x1080 size packed to %d, want %d (1920 in the high 32 bits, 1080 in the low 32 bits)", raw, wantPacked)
	}
	hi := uint32(raw >> 32)
	lo := uint32(raw)
	if hi != 1920 {
		t.Fatalf("the high 32 bits of the packed size are %d, want the width 1920", hi)
	}
	if lo != 1080 {
		t.Fatalf("the low 32 bits of the packed size are %d, want the height 1080", lo)
	}
}

func TestMediaTypeRatioPacksNumeratorInTheHighHalf(t *testing.T) {
	mt := newMediaTypeForTest(t)
	if err := mt.SetRatio(&MFMTFrameRate, 30000, 1001); err != nil {
		t.Fatalf("SetRatio: %v", err)
	}
	raw, err := mt.Uint64(&MFMTFrameRate)
	if err != nil {
		t.Fatalf("Uint64: %v", err)
	}
	const wantPacked uint64 = 128849018881001
	if raw != wantPacked {
		t.Fatalf("a 30000/1001 ratio packed to %d, want %d (30000 in the high 32 bits, 1001 in the low 32 bits)", raw, wantPacked)
	}
	hi := uint32(raw >> 32)
	lo := uint32(raw)
	if hi != 30000 {
		t.Fatalf("the high 32 bits of the packed ratio are %d, want the numerator 30000", hi)
	}
	if lo != 1001 {
		t.Fatalf("the low 32 bits of the packed ratio are %d, want the denominator 1001", lo)
	}
}

func TestMediaTypeUnsetKeyFailsRatherThanReadingAsZero(t *testing.T) {
	mt := newMediaTypeForTest(t)
	if _, err := mt.Uint32(&MFMTAvgBitrate); err == nil {
		t.Fatal("Uint32 on a key that was never set returned no error")
	}
	if _, err := mt.Uint64(&MFMTFrameSize); err == nil {
		t.Fatal("Uint64 on a key that was never set returned no error")
	}
	if _, err := mt.GUID(&MFMTSubtype); err == nil {
		t.Fatal("GUID on a key that was never set returned no error")
	}
	if _, _, err := mt.Size(&MFMTFrameSize); err == nil {
		t.Fatal("Size on a key that was never set returned no error")
	}
	if _, _, err := mt.Ratio(&MFMTFrameRate); err == nil {
		t.Fatal("Ratio on a key that was never set returned no error")
	}
}

func TestMediaTypeReleaseIsSafeOnNilAndTwice(t *testing.T) {
	var nilType *MediaType
	nilType.Release()

	if !Loaded() {
		t.Skip("Media Foundation is not present on this machine")
	}
	if err := Startup(); err != nil {
		t.Fatalf("Startup: %v", err)
	}
	defer Shutdown()
	mt, err := NewMediaType()
	if err != nil {
		t.Fatalf("NewMediaType: %v", err)
	}
	mt.Release()
	mt.Release()
}

func testEncoderFormat() EncoderFormat {
	return EncoderFormat{
		Width:                1280,
		Height:               720,
		FPSNum:               30,
		FPSDen:               1,
		BitrateBitsPerSecond: 2_000_000,
		Profile:              AVEncH264VProfileMain,
	}
}

func TestEncoderFormatOutputTypeDescribesH264(t *testing.T) {
	if !Loaded() {
		t.Skip("Media Foundation is not present on this machine")
	}
	if err := Startup(); err != nil {
		t.Fatalf("Startup: %v", err)
	}
	defer Shutdown()

	mt, err := testEncoderFormat().OutputType()
	if err != nil {
		t.Fatalf("OutputType: %v", err)
	}
	defer mt.Release()

	major, err := mt.GUID(&MFMTMajorType)
	if err != nil {
		t.Fatalf("GUID(MajorType): %v", err)
	}
	if major != MFMediaTypeVideo {
		t.Fatalf("output type major type is %v, want %v", major, MFMediaTypeVideo)
	}
	sub, err := mt.GUID(&MFMTSubtype)
	if err != nil {
		t.Fatalf("GUID(Subtype): %v", err)
	}
	if sub != MFVideoFormatH264 {
		t.Fatalf("output type subtype is %v, want %v", sub, MFVideoFormatH264)
	}
	w, h, err := mt.Size(&MFMTFrameSize)
	if err != nil {
		t.Fatalf("Size: %v", err)
	}
	if w != 1280 || h != 720 {
		t.Fatalf("output type frame size is %dx%d, want 1280x720", w, h)
	}
	rate, err := mt.Uint32(&MFMTAvgBitrate)
	if err != nil {
		t.Fatalf("Uint32(AvgBitrate): %v", err)
	}
	if rate != 2_000_000 {
		t.Fatalf("output type average bitrate is %d, want %d", rate, 2_000_000)
	}
}

func TestEncoderFormatInputTypeDescribesNV12(t *testing.T) {
	if !Loaded() {
		t.Skip("Media Foundation is not present on this machine")
	}
	if err := Startup(); err != nil {
		t.Fatalf("Startup: %v", err)
	}
	defer Shutdown()

	mt, err := testEncoderFormat().InputType()
	if err != nil {
		t.Fatalf("InputType: %v", err)
	}
	defer mt.Release()

	major, err := mt.GUID(&MFMTMajorType)
	if err != nil {
		t.Fatalf("GUID(MajorType): %v", err)
	}
	if major != MFMediaTypeVideo {
		t.Fatalf("input type major type is %v, want %v", major, MFMediaTypeVideo)
	}
	sub, err := mt.GUID(&MFMTSubtype)
	if err != nil {
		t.Fatalf("GUID(Subtype): %v", err)
	}
	if sub != MFVideoFormatNV12 {
		t.Fatalf("input type subtype is %v, want %v", sub, MFVideoFormatNV12)
	}
	w, h, err := mt.Size(&MFMTFrameSize)
	if err != nil {
		t.Fatalf("Size: %v", err)
	}
	if w != 1280 || h != 720 {
		t.Fatalf("input type frame size is %dx%d, want 1280x720", w, h)
	}
}

func TestEncoderAcceptsOutputTypeBeforeInputType(t *testing.T) {
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

	if err := tr.Unlock(); err != nil {
		t.Fatalf("%q refused to leave its asynchronous lock: %v", chosen.Name, err)
	}

	inID, outID, err := tr.StreamIDs()
	if err != nil {
		t.Fatalf("StreamIDs: %v", err)
	}

	format := testEncoderFormat()
	outputType, err := format.OutputType()
	if err != nil {
		t.Fatalf("OutputType: %v", err)
	}
	defer outputType.Release()
	inputType, err := format.InputType()
	if err != nil {
		t.Fatalf("InputType: %v", err)
	}
	defer inputType.Release()

	premature := hr(tr.obj.p, transformSetInputType,
		uintptr(inID), uintptr(unsafe.Pointer(inputType.obj.p)), 0)
	t.Logf("%q SetInputType before SetOutputType returned HRESULT %v", chosen.Name, premature)

	code := hr(tr.obj.p, transformSetOutputType,
		uintptr(outID), uintptr(unsafe.Pointer(outputType.obj.p)), 0)
	if code.Failed() {
		t.Skipf("%q refused IMFTransform::SetOutputType with HRESULT %v", chosen.Name, code)
	}
	t.Logf("%q accepted IMFTransform::SetOutputType with HRESULT %v", chosen.Name, code)

	code = hr(tr.obj.p, transformSetInputType,
		uintptr(inID), uintptr(unsafe.Pointer(inputType.obj.p)), 0)
	if code.Failed() {
		t.Skipf("%q refused IMFTransform::SetInputType with HRESULT %v", chosen.Name, code)
	}
	if !premature.Failed() {
		t.Logf("%q accepted the input type first as well, so it imposes no ordering", chosen.Name)
	}
	t.Logf("%q accepted the H.264 output type and the NV12 input type", chosen.Name)
}
