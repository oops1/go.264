package nvenc

import (
	"testing"
	"unsafe"
)

func TestNvEncAPIVersionConstant(t *testing.T) {
	if NvEncAPIVersion != 0x0100000B {
		t.Fatalf("NvEncAPIVersion = 0x%08X, want 0x0100000B", NvEncAPIVersion)
	}
}

func TestNvEncAPIStructVersionFunction(t *testing.T) {
	cases := []struct {
		ver  uint32
		want uint32
	}{
		{1, 0x7101000B},
		{2, 0x7102000B},
		{4, 0x7104000B},
		{5, 0x7105000B},
		{7, 0x7107000B},
	}
	for _, c := range cases {
		if got := NvEncAPIStructVersion(c.ver); got != c.want {
			t.Errorf("NvEncAPIStructVersion(%d) = 0x%08X, want 0x%08X", c.ver, got, c.want)
		}
	}
}

func TestStructVersionConstants(t *testing.T) {
	cases := []struct {
		name string
		got  uint32
		want uint32
	}{
		{"NvEncCapsParamVersion", NvEncCapsParamVersion, 0x7101000B},
		{"NvEncCreateInputBufferVersion", NvEncCreateInputBufferVersion, 0x7101000B},
		{"NvEncCreateBitstreamBufferVersion", NvEncCreateBitstreamBufferVersion, 0x7101000B},
		{"NvEncRCParamsVersion", NvEncRCParamsVersion, 0x7101000B},
		{"NvEncLockBitstreamVersion", NvEncLockBitstreamVersion, 0x7101000B},
		{"NvEncLockInputBufferVersion", NvEncLockInputBufferVersion, 0x7101000B},
		{"NvEncSequenceParamPayloadVersion", NvEncSequenceParamPayloadVersion, 0x7101000B},
		{"NvEncEventParamsVersion", NvEncEventParamsVersion, 0x7101000B},
		{"NvEncOpenEncodeSessionExParamsVersion", NvEncOpenEncodeSessionExParamsVersion, 0x7101000B},
		{"NvEncodeAPIFunctionListVersion", NvEncodeAPIFunctionListVersion, 0x7102000B},
		{"NvEncConfigVersion", NvEncConfigVersion, 0xF107000B},
		{"NvEncInitializeParamsVersion", NvEncInitializeParamsVersion, 0xF105000B},
		{"NvEncPresetConfigVersion", NvEncPresetConfigVersion, 0xF104000B},
		{"NvEncPicParamsVersion", NvEncPicParamsVersion, 0xF104000B},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("%s = 0x%08X, want 0x%08X", c.name, c.got, c.want)
		}
	}
}

func TestStructSizes(t *testing.T) {
	cases := []struct {
		name string
		got  uintptr
		want uintptr
	}{
		{"GUID", unsafe.Sizeof(GUID{}), 16},
		{"NvEncQP", unsafe.Sizeof(NvEncQP{}), 12},
		{"NvEncCapsParam", unsafe.Sizeof(NvEncCapsParam{}), 256},
		{"NvEncCreateInputBuffer", unsafe.Sizeof(NvEncCreateInputBuffer{}), 776},
		{"NvEncCreateBitstreamBuffer", unsafe.Sizeof(NvEncCreateBitstreamBuffer{}), 776},
		{"NvEncLockInputBuffer", unsafe.Sizeof(NvEncLockInputBuffer{}), 1544},
		{"NvEncLockBitstream", unsafe.Sizeof(NvEncLockBitstream{}), 1544},
		{"NvEncExternalMEHintCountsPerBlockType", unsafe.Sizeof(NvEncExternalMEHintCountsPerBlockType{}), 16},
		{"NvEncPicParamsMVC", unsafe.Sizeof(NvEncPicParamsMVC{}), 128},
		{"NvEncPicParamsH264Ext", unsafe.Sizeof(NvEncPicParamsH264Ext{}), 128},
		{"NvEncPicParamsH264", unsafe.Sizeof(NvEncPicParamsH264{}), 1536},
		{"NvEncCodecPicParams", unsafe.Sizeof(NvEncCodecPicParams{}), 1536},
		{"NvEncPicParams", unsafe.Sizeof(NvEncPicParams{}), 3344},
		{"NvEncConfigH264VUIParameters", unsafe.Sizeof(NvEncConfigH264VUIParameters{}), 112},
		{"NvEncConfigH264", unsafe.Sizeof(NvEncConfigH264{}), 1792},
		{"NvEncCodecConfig", unsafe.Sizeof(NvEncCodecConfig{}), 1792},
		{"NvEncRCParams", unsafe.Sizeof(NvEncRCParams{}), 128},
		{"NvEncConfig", unsafe.Sizeof(NvEncConfig{}), 3584},
		{"NvEncPresetConfig", unsafe.Sizeof(NvEncPresetConfig{}), 5128},
		{"NvEncInitializeParams", unsafe.Sizeof(NvEncInitializeParams{}), 1808},
		{"NvEncSequenceParamPayload", unsafe.Sizeof(NvEncSequenceParamPayload{}), 1544},
		{"NvEncEventParams", unsafe.Sizeof(NvEncEventParams{}), 1544},
		{"NvEncOpenEncodeSessionExParams", unsafe.Sizeof(NvEncOpenEncodeSessionExParams{}), 1552},
		{"NvEncodeAPIFunctionList", unsafe.Sizeof(NvEncodeAPIFunctionList{}), 2552},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("sizeof(%s) = %d bytes, want %d bytes (as measured from the real nvEncodeAPI.h on amd64 linux via gcc)", c.name, c.got, c.want)
		}
	}
}

func TestGUIDFieldOffsets(t *testing.T) {
	var g GUID
	if off := unsafe.Offsetof(g.Data1); off != 0 {
		t.Errorf("GUID.Data1 offset = %d, want 0", off)
	}
	if off := unsafe.Offsetof(g.Data2); off != 4 {
		t.Errorf("GUID.Data2 offset = %d, want 4", off)
	}
	if off := unsafe.Offsetof(g.Data3); off != 6 {
		t.Errorf("GUID.Data3 offset = %d, want 6", off)
	}
	if off := unsafe.Offsetof(g.Data4); off != 8 {
		t.Errorf("GUID.Data4 offset = %d, want 8", off)
	}
}

func TestCreateInputBufferOffsets(t *testing.T) {
	var b NvEncCreateInputBuffer
	if off := unsafe.Offsetof(b.Version); off != 0 {
		t.Errorf("NvEncCreateInputBuffer.Version offset = %d, want 0", off)
	}
	if off := unsafe.Offsetof(b.InputBuffer); off != 24 {
		t.Errorf("NvEncCreateInputBuffer.InputBuffer offset = %d, want 24", off)
	}
	if off := unsafe.Offsetof(b.Reserved1); off != 40 {
		t.Errorf("NvEncCreateInputBuffer.Reserved1 offset = %d, want 40", off)
	}
	if off := unsafe.Offsetof(b.Reserved2); off != 272 {
		t.Errorf("NvEncCreateInputBuffer.Reserved2 offset = %d, want 272", off)
	}
}

func TestLockInputBufferOffsets(t *testing.T) {
	var b NvEncLockInputBuffer
	if off := unsafe.Offsetof(b.Version); off != 0 {
		t.Errorf("NvEncLockInputBuffer.Version offset = %d, want 0", off)
	}
	if off := unsafe.Offsetof(b.flags); off != 4 {
		t.Errorf("NvEncLockInputBuffer.flags offset = %d, want 4", off)
	}
	if off := unsafe.Offsetof(b.InputBuffer); off != 8 {
		t.Errorf("NvEncLockInputBuffer.InputBuffer offset = %d, want 8", off)
	}
	if off := unsafe.Offsetof(b.Reserved1); off != 28 {
		t.Errorf("NvEncLockInputBuffer.Reserved1 offset = %d, want 28", off)
	}
	if off := unsafe.Offsetof(b.Reserved2); off != 1032 {
		t.Errorf("NvEncLockInputBuffer.Reserved2 offset = %d, want 1032", off)
	}
}

func TestLockBitstreamOffsets(t *testing.T) {
	var b NvEncLockBitstream
	if off := unsafe.Offsetof(b.Version); off != 0 {
		t.Errorf("NvEncLockBitstream.Version offset = %d, want 0", off)
	}
	if off := unsafe.Offsetof(b.OutputBitstream); off != 8 {
		t.Errorf("NvEncLockBitstream.OutputBitstream offset = %d, want 8", off)
	}
	if off := unsafe.Offsetof(b.Reserved1); off != 160 {
		t.Errorf("NvEncLockBitstream.Reserved1 offset = %d, want 160", off)
	}
	if off := unsafe.Offsetof(b.Reserved2); off != 1032 {
		t.Errorf("NvEncLockBitstream.Reserved2 offset = %d, want 1032", off)
	}
}

func TestPicParamsH264Offsets(t *testing.T) {
	var p NvEncPicParamsH264
	if off := unsafe.Offsetof(p.DisplayPOCSyntax); off != 0 {
		t.Errorf("NvEncPicParamsH264.DisplayPOCSyntax offset = %d, want 0", off)
	}
	if off := unsafe.Offsetof(p.SliceTypeData); off != 24 {
		t.Errorf("NvEncPicParamsH264.SliceTypeData offset = %d, want 24", off)
	}
	if off := unsafe.Offsetof(p.H264ExtPicParams); off != 80 {
		t.Errorf("NvEncPicParamsH264.H264ExtPicParams offset = %d, want 80", off)
	}
	if off := unsafe.Offsetof(p.Reserved); off != 208 {
		t.Errorf("NvEncPicParamsH264.Reserved offset = %d, want 208", off)
	}
	if off := unsafe.Offsetof(p.Reserved2); off != 1048 {
		t.Errorf("NvEncPicParamsH264.Reserved2 offset = %d, want 1048", off)
	}
}

func TestPicParamsOffsets(t *testing.T) {
	var p NvEncPicParams
	if off := unsafe.Offsetof(p.Version); off != 0 {
		t.Errorf("NvEncPicParams.Version offset = %d, want 0", off)
	}
	if off := unsafe.Offsetof(p.PictureType); off != 72 {
		t.Errorf("NvEncPicParams.PictureType offset = %d, want 72", off)
	}
	if off := unsafe.Offsetof(p.CodecPicParams); off != 80 {
		t.Errorf("NvEncPicParams.CodecPicParams offset = %d, want 80 (must be 8-byte aligned right after the union)", off)
	}
	if off := unsafe.Offsetof(p.MeHintCountsPerBlock); off != 1616 {
		t.Errorf("NvEncPicParams.MeHintCountsPerBlock offset = %d, want 1616", off)
	}
	if off := unsafe.Offsetof(p.Reserved4); off != 2872 {
		t.Errorf("NvEncPicParams.Reserved4 offset = %d, want 2872", off)
	}
}

func TestConfigH264Offsets(t *testing.T) {
	var c NvEncConfigH264
	if off := unsafe.Offsetof(c.Level); off != 4 {
		t.Errorf("NvEncConfigH264.Level offset = %d, want 4 (must follow the 32-bit flag word)", off)
	}
	if off := unsafe.Offsetof(c.H264VUIParameters); off != 72 {
		t.Errorf("NvEncConfigH264.H264VUIParameters offset = %d, want 72", off)
	}
	if off := unsafe.Offsetof(c.LTRNumFrames); off != 184 {
		t.Errorf("NvEncConfigH264.LTRNumFrames offset = %d, want 184", off)
	}
	if off := unsafe.Offsetof(c.Reserved1); off != 212 {
		t.Errorf("NvEncConfigH264.Reserved1 offset = %d, want 212", off)
	}
	if off := unsafe.Offsetof(c.Reserved2); off != 1280 {
		t.Errorf("NvEncConfigH264.Reserved2 offset = %d, want 1280", off)
	}
}

func TestConfigOffsets(t *testing.T) {
	var c NvEncConfig
	if off := unsafe.Offsetof(c.Version); off != 0 {
		t.Errorf("NvEncConfig.Version offset = %d, want 0", off)
	}
	if off := unsafe.Offsetof(c.RCParams); off != 40 {
		t.Errorf("NvEncConfig.RCParams offset = %d, want 40", off)
	}
	if off := unsafe.Offsetof(c.EncodeCodecConfig); off != 168 {
		t.Errorf("NvEncConfig.EncodeCodecConfig offset = %d, want 168 (right after NvEncRCParams)", off)
	}
	if off := unsafe.Offsetof(c.Reserved); off != 1960 {
		t.Errorf("NvEncConfig.Reserved offset = %d, want 1960 (right after the codec config union)", off)
	}
	if off := unsafe.Offsetof(c.Reserved2); off != 3072 {
		t.Errorf("NvEncConfig.Reserved2 offset = %d, want 3072", off)
	}
}

func TestInitializeParamsOffsets(t *testing.T) {
	var p NvEncInitializeParams
	if off := unsafe.Offsetof(p.Version); off != 0 {
		t.Errorf("NvEncInitializeParams.Version offset = %d, want 0", off)
	}
	if off := unsafe.Offsetof(p.PresetGUID); off != 20 {
		t.Errorf("NvEncInitializeParams.PresetGUID offset = %d, want 20", off)
	}
	if off := unsafe.Offsetof(p.PrivData); off != 80 {
		t.Errorf("NvEncInitializeParams.PrivData offset = %d, want 80 (must be 8-byte aligned after the flag word)", off)
	}
	if off := unsafe.Offsetof(p.MaxMEHintCountsPerBlock); off != 104 {
		t.Errorf("NvEncInitializeParams.MaxMEHintCountsPerBlock offset = %d, want 104", off)
	}
	if off := unsafe.Offsetof(p.TuningInfo); off != 136 {
		t.Errorf("NvEncInitializeParams.TuningInfo offset = %d, want 136", off)
	}
	if off := unsafe.Offsetof(p.Reserved2); off != 1296 {
		t.Errorf("NvEncInitializeParams.Reserved2 offset = %d, want 1296", off)
	}
}

func TestRCParamsOffsets(t *testing.T) {
	var r NvEncRCParams
	if off := unsafe.Offsetof(r.Version); off != 0 {
		t.Errorf("NvEncRCParams.Version offset = %d, want 0", off)
	}
	if off := unsafe.Offsetof(r.MinQP); off != 40 {
		t.Errorf("NvEncRCParams.MinQP offset = %d, want 40 (must follow the 32-bit flag word)", off)
	}
	if off := unsafe.Offsetof(r.Reserved); off != 112 {
		t.Errorf("NvEncRCParams.Reserved offset = %d, want 112", off)
	}
}

func TestFunctionListOffsets(t *testing.T) {
	var f NvEncodeAPIFunctionList
	if off := unsafe.Offsetof(f.Version); off != 0 {
		t.Errorf("NvEncodeAPIFunctionList.Version offset = %d, want 0", off)
	}
	if off := unsafe.Offsetof(f.NvEncOpenEncodeSession); off != 8 {
		t.Errorf("NvEncodeAPIFunctionList.NvEncOpenEncodeSession offset = %d, want 8", off)
	}
	if off := unsafe.Offsetof(f.NvEncReconfigureEncoder); off != 264 {
		t.Errorf("NvEncodeAPIFunctionList.NvEncReconfigureEncoder offset = %d, want 264", off)
	}
	if off := unsafe.Offsetof(f.Reserved1); off != 272 {
		t.Errorf("NvEncodeAPIFunctionList.Reserved1 offset = %d, want 272", off)
	}
	if off := unsafe.Offsetof(f.NvEncCreateMVBuffer); off != 280 {
		t.Errorf("NvEncodeAPIFunctionList.NvEncCreateMVBuffer offset = %d, want 280", off)
	}
	if off := unsafe.Offsetof(f.NvEncGetSequenceParamEx); off != 328 {
		t.Errorf("NvEncodeAPIFunctionList.NvEncGetSequenceParamEx offset = %d, want 328", off)
	}
	if off := unsafe.Offsetof(f.Reserved2); off != 336 {
		t.Errorf("NvEncodeAPIFunctionList.Reserved2 offset = %d, want 336", off)
	}
}

func testSingleBitAccessor(t *testing.T, name string, bit uint, set func(bool), get func() bool, raw func() uint32) {
	t.Helper()
	set(false)
	if get() {
		t.Errorf("%s: get() = true after set(false)", name)
	}
	if r := raw(); r&(1<<bit) != 0 {
		t.Errorf("%s: raw word = 0x%08X after set(false), bit %d must be clear", name, r, bit)
	}
	set(true)
	if !get() {
		t.Errorf("%s: get() = false after set(true)", name)
	}
	if r := raw(); r != 1<<bit {
		t.Errorf("%s: raw word = 0x%08X after set(true), want 0x%08X (only bit %d set)", name, r, uint32(1<<bit), bit)
	}
	set(false)
	if r := raw(); r != 0 {
		t.Errorf("%s: raw word = 0x%08X after clearing, want 0", name, r)
	}
}

func TestLockInputBufferBitfield(t *testing.T) {
	var b NvEncLockInputBuffer
	testSingleBitAccessor(t, "DoNotWait", 0, b.SetDoNotWait, b.DoNotWait, b.RawFlags)
}

func TestLockBitstreamBitfield(t *testing.T) {
	var b NvEncLockBitstream
	testSingleBitAccessor(t, "DoNotWait", 0, b.SetDoNotWait, b.DoNotWait, b.RawFlags)
	testSingleBitAccessor(t, "LTRFrame", 1, b.SetLTRFrame, b.LTRFrame, b.RawFlags)
	testSingleBitAccessor(t, "GetRCStats", 2, b.SetGetRCStats, b.GetRCStats, b.RawFlags)

	b.SetDoNotWait(true)
	b.SetLTRFrame(true)
	b.SetGetRCStats(true)
	if b.RawFlags() != 0x7 {
		t.Errorf("NvEncLockBitstream raw flags = 0x%08X with all three bits set, want 0x00000007", b.RawFlags())
	}
}

func TestPicParamsH264Bitfield(t *testing.T) {
	var p NvEncPicParamsH264
	testSingleBitAccessor(t, "ConstrainedFrame", 0, p.SetConstrainedFrame, p.ConstrainedFrame, p.RawFlags)
	testSingleBitAccessor(t, "SliceModeDataUpdate", 1, p.SetSliceModeDataUpdate, p.SliceModeDataUpdate, p.RawFlags)
	testSingleBitAccessor(t, "LTRMarkFrame", 2, p.SetLTRMarkFrame, p.LTRMarkFrame, p.RawFlags)
	testSingleBitAccessor(t, "LTRUseFrames", 3, p.SetLTRUseFrames, p.LTRUseFrames, p.RawFlags)
}

func TestInitializeParamsBitfield(t *testing.T) {
	var p NvEncInitializeParams
	testSingleBitAccessor(t, "ReportSliceOffsets", 0, p.SetReportSliceOffsets, p.ReportSliceOffsets, p.RawFlags)
	testSingleBitAccessor(t, "EnableSubFrameWrite", 1, p.SetEnableSubFrameWrite, p.EnableSubFrameWrite, p.RawFlags)
	testSingleBitAccessor(t, "EnableExternalMEHints", 2, p.SetEnableExternalMEHints, p.EnableExternalMEHints, p.RawFlags)
	testSingleBitAccessor(t, "EnableMEOnlyMode", 3, p.SetEnableMEOnlyMode, p.EnableMEOnlyMode, p.RawFlags)
	testSingleBitAccessor(t, "EnableWeightedPrediction", 4, p.SetEnableWeightedPrediction, p.EnableWeightedPrediction, p.RawFlags)
	testSingleBitAccessor(t, "EnableOutputInVidmem", 5, p.SetEnableOutputInVidmem, p.EnableOutputInVidmem, p.RawFlags)
}

func TestRCParamsBitfield(t *testing.T) {
	var r NvEncRCParams
	testSingleBitAccessor(t, "EnableMinQP", 0, r.SetEnableMinQP, r.EnableMinQP, r.RawFlags)
	testSingleBitAccessor(t, "EnableMaxQP", 1, r.SetEnableMaxQP, r.EnableMaxQP, r.RawFlags)
	testSingleBitAccessor(t, "EnableInitialRCQP", 2, r.SetEnableInitialRCQP, r.EnableInitialRCQP, r.RawFlags)
	testSingleBitAccessor(t, "EnableAQ", 3, r.SetEnableAQ, r.EnableAQ, r.RawFlags)
	testSingleBitAccessor(t, "EnableLookahead", 5, r.SetEnableLookahead, r.EnableLookahead, r.RawFlags)
	testSingleBitAccessor(t, "DisableIadapt", 6, r.SetDisableIadapt, r.DisableIadapt, r.RawFlags)
	testSingleBitAccessor(t, "DisableBadapt", 7, r.SetDisableBadapt, r.DisableBadapt, r.RawFlags)
	testSingleBitAccessor(t, "EnableTemporalAQ", 8, r.SetEnableTemporalAQ, r.EnableTemporalAQ, r.RawFlags)
	testSingleBitAccessor(t, "ZeroReorderDelay", 9, r.SetZeroReorderDelay, r.ZeroReorderDelay, r.RawFlags)
	testSingleBitAccessor(t, "EnableNonRefP", 10, r.SetEnableNonRefP, r.EnableNonRefP, r.RawFlags)
	testSingleBitAccessor(t, "StrictGOPTarget", 11, r.SetStrictGOPTarget, r.StrictGOPTarget, r.RawFlags)

	r.SetAQStrength(0xF)
	if r.RawFlags() != 0xF000 {
		t.Errorf("NvEncRCParams raw flags = 0x%08X after SetAQStrength(0xF), want 0x0000F000 (bits 12-15)", r.RawFlags())
	}
	if r.AQStrength() != 0xF {
		t.Errorf("NvEncRCParams.AQStrength() = %d after SetAQStrength(0xF), want 15", r.AQStrength())
	}
	r.SetAQStrength(0)
	if r.RawFlags() != 0 {
		t.Errorf("NvEncRCParams raw flags = 0x%08X after clearing AQStrength, want 0", r.RawFlags())
	}
}

func TestConfigH264Bitfield(t *testing.T) {
	var c NvEncConfigH264
	bits := []struct {
		name string
		bit  uint
		set  func(bool)
		get  func() bool
	}{
		{"EnableTemporalSVC", 0, c.SetEnableTemporalSVC, c.EnableTemporalSVC},
		{"EnableStereoMVC", 1, c.SetEnableStereoMVC, c.EnableStereoMVC},
		{"HierarchicalPFrames", 2, c.SetHierarchicalPFrames, c.HierarchicalPFrames},
		{"HierarchicalBFrames", 3, c.SetHierarchicalBFrames, c.HierarchicalBFrames},
		{"OutputBufferingPeriodSEI", 4, c.SetOutputBufferingPeriodSEI, c.OutputBufferingPeriodSEI},
		{"OutputPictureTimingSEI", 5, c.SetOutputPictureTimingSEI, c.OutputPictureTimingSEI},
		{"OutputAUD", 6, c.SetOutputAUD, c.OutputAUD},
		{"DisableSPSPPS", 7, c.SetDisableSPSPPS, c.DisableSPSPPS},
		{"OutputFramePackingSEI", 8, c.SetOutputFramePackingSEI, c.OutputFramePackingSEI},
		{"OutputRecoveryPointSEI", 9, c.SetOutputRecoveryPointSEI, c.OutputRecoveryPointSEI},
		{"EnableIntraRefresh", 10, c.SetEnableIntraRefresh, c.EnableIntraRefresh},
		{"EnableConstrainedEncoding", 11, c.SetEnableConstrainedEncoding, c.EnableConstrainedEncoding},
		{"RepeatSPSPPS", 12, c.SetRepeatSPSPPS, c.RepeatSPSPPS},
		{"EnableVFR", 13, c.SetEnableVFR, c.EnableVFR},
		{"EnableLTR", 14, c.SetEnableLTR, c.EnableLTR},
		{"QpPrimeYZeroTransformBypassFlag", 15, c.SetQpPrimeYZeroTransformBypassFlag, c.QpPrimeYZeroTransformBypassFlag},
		{"UseConstrainedIntraPred", 16, c.SetUseConstrainedIntraPred, c.UseConstrainedIntraPred},
		{"EnableFillerDataInsertion", 17, c.SetEnableFillerDataInsertion, c.EnableFillerDataInsertion},
		{"DisableSVCPrefixNalu", 18, c.SetDisableSVCPrefixNalu, c.DisableSVCPrefixNalu},
		{"EnableScalabilityInfoSEI", 19, c.SetEnableScalabilityInfoSEI, c.EnableScalabilityInfoSEI},
		{"SingleSliceIntraRefresh", 20, c.SetSingleSliceIntraRefresh, c.SingleSliceIntraRefresh},
	}
	for _, b := range bits {
		testSingleBitAccessor(t, b.name, b.bit, b.set, b.get, c.RawFlags)
	}

	for _, b := range bits {
		b.set(true)
	}
	want := uint32(0)
	for _, b := range bits {
		want |= 1 << b.bit
	}
	if want != 0x1FFFFF {
		t.Fatalf("test bug: expected mask of 21 set bits is 0x%08X, want 0x001FFFFF", want)
	}
	if c.RawFlags() != want {
		t.Errorf("NvEncConfigH264 raw flags = 0x%08X with all 21 flags set, want 0x%08X", c.RawFlags(), want)
	}
}

func TestExternalMEHintCountsPerBlockTypeBitfield(t *testing.T) {
	var m NvEncExternalMEHintCountsPerBlockType
	m.SetNumCandsPerBlk16x16(0xF)
	if m.RawFlags() != 0xF {
		t.Errorf("raw flags = 0x%08X after SetNumCandsPerBlk16x16(0xF), want 0x0000000F", m.RawFlags())
	}
	m.SetNumCandsPerBlk16x8(0xA)
	if m.RawFlags() != 0xAF {
		t.Errorf("raw flags = 0x%08X after SetNumCandsPerBlk16x8(0xA), want 0x000000AF", m.RawFlags())
	}
	m.SetNumCandsPerBlk8x16(0x5)
	if m.RawFlags() != 0x5AF {
		t.Errorf("raw flags = 0x%08X after SetNumCandsPerBlk8x16(0x5), want 0x000005AF", m.RawFlags())
	}
	m.SetNumCandsPerBlk8x8(0x3)
	if m.RawFlags() != 0x35AF {
		t.Errorf("raw flags = 0x%08X after SetNumCandsPerBlk8x8(0x3), want 0x000035AF", m.RawFlags())
	}
	if m.NumCandsPerBlk16x16() != 0xF || m.NumCandsPerBlk16x8() != 0xA || m.NumCandsPerBlk8x16() != 0x5 || m.NumCandsPerBlk8x8() != 0x3 {
		t.Errorf("field readback mismatch: 16x16=%X 16x8=%X 8x16=%X 8x8=%X, want F A 5 3",
			m.NumCandsPerBlk16x16(), m.NumCandsPerBlk16x8(), m.NumCandsPerBlk8x16(), m.NumCandsPerBlk8x8())
	}
}

func TestUnionAccessorsShareStorage(t *testing.T) {
	var pp NvEncCodecPicParams
	pp.H264PicParams().DisplayPOCSyntax = 0xDEADBEEF
	if got := pp.Reserved()[0]; got != 0xDEADBEEF {
		t.Errorf("NvEncCodecPicParams.Reserved()[0] = 0x%08X after writing H264PicParams().DisplayPOCSyntax, want 0xDEADBEEF (union members must alias)", got)
	}

	var cc NvEncCodecConfig
	cc.H264Config().Level = 0xCAFEF00D
	if got := cc.Reserved()[1]; got != 0xCAFEF00D {
		t.Errorf("NvEncCodecConfig.Reserved()[1] = 0x%08X after writing H264Config().Level, want 0xCAFEF00D (union members must alias)", got)
	}

	var ext NvEncPicParamsH264Ext
	ext.MVCPicParams().ViewID = 0x12345678
	if got := ext.Reserved1()[1]; got != 0x12345678 {
		t.Errorf("NvEncPicParamsH264Ext.Reserved1()[1] = 0x%08X after writing MVCPicParams().ViewID, want 0x12345678 (union members must alias)", got)
	}
}

func TestStatusErrorStrings(t *testing.T) {
	if NvEncSuccess != 0 {
		t.Errorf("NvEncSuccess = %d, want 0", NvEncSuccess)
	}
	if NvEncErrResourceNotMapped != 25 {
		t.Errorf("NvEncErrResourceNotMapped = %d, want 25", NvEncErrResourceNotMapped)
	}
	if got := NvEncErrInvalidVersion.Error(); got == "" {
		t.Errorf("NvEncErrInvalidVersion.Error() returned an empty string")
	}
	unknown := NvEncStatus(999)
	if got := unknown.Error(); got == "" {
		t.Errorf("unrecognized NvEncStatus(999).Error() returned an empty string")
	}
}

func TestGUIDValuesAreDistinct(t *testing.T) {
	guids := map[GUID]string{
		NvEncCodecH264GUID:               "NvEncCodecH264GUID",
		NvEncH264ProfileBaselineGUID:     "NvEncH264ProfileBaselineGUID",
		NvEncH264ProfileMainGUID:         "NvEncH264ProfileMainGUID",
		NvEncH264ProfileHighGUID:         "NvEncH264ProfileHighGUID",
		NvEncPresetDefaultGUID:           "NvEncPresetDefaultGUID",
		NvEncPresetHPGUID:                "NvEncPresetHPGUID",
		NvEncPresetLowLatencyDefaultGUID: "NvEncPresetLowLatencyDefaultGUID",
		NvEncPresetP1GUID:                "NvEncPresetP1GUID",
		NvEncPresetP2GUID:                "NvEncPresetP2GUID",
		NvEncPresetP3GUID:                "NvEncPresetP3GUID",
		NvEncPresetP4GUID:                "NvEncPresetP4GUID",
		NvEncPresetP5GUID:                "NvEncPresetP5GUID",
		NvEncPresetP6GUID:                "NvEncPresetP6GUID",
		NvEncPresetP7GUID:                "NvEncPresetP7GUID",
	}
	if len(guids) != 14 {
		t.Fatalf("collected %d distinct GUID values from the 14 required constants, want 14 (a transcription collision aliased two constants)", len(guids))
	}
	if NvEncCodecH264GUID.Data1 != 0x6bc82762 {
		t.Errorf("NvEncCodecH264GUID.Data1 = 0x%08X, want 0x6bc82762", NvEncCodecH264GUID.Data1)
	}
	if NvEncPresetP1GUID.Data4[6] != 0x0e {
		t.Errorf("NvEncPresetP1GUID.Data4[6] = 0x%02X, want 0x0e", NvEncPresetP1GUID.Data4[6])
	}
}
