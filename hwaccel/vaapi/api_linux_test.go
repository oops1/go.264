package vaapi

import (
	"testing"
	"unsafe"
)

func TestStructSizes(t *testing.T) {
	cases := []struct {
		name string
		got  uintptr
		want uintptr
	}{
		{"ConfigAttrib", unsafe.Sizeof(ConfigAttrib{}), 8},
		{"GenericValue", unsafe.Sizeof(GenericValue{}), 16},
		{"SurfaceAttrib", unsafe.Sizeof(SurfaceAttrib{}), 24},
		{"PictureH264", unsafe.Sizeof(PictureH264{}), 36},
		{"EncSequenceParameterBufferH264", unsafe.Sizeof(EncSequenceParameterBufferH264{}), 1132},
		{"EncPictureParameterBufferH264", unsafe.Sizeof(EncPictureParameterBufferH264{}), 648},
		{"EncSliceParameterBufferH264", unsafe.Sizeof(EncSliceParameterBufferH264{}), 3140},
		{"CodedBufferSegment", unsafe.Sizeof(CodedBufferSegment{}), 48},
		{"ImageFormat", unsafe.Sizeof(ImageFormat{}), 48},
		{"Image", unsafe.Sizeof(Image{}), 120},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("sizeof(%s) = %d bytes, want %d bytes (as measured from the real va.h/va_enc_h264.h on amd64 linux via gcc against libva-dev 2.20.0)", c.name, c.got, c.want)
		}
	}
}

func TestConfigAttribOffsets(t *testing.T) {
	var a ConfigAttrib
	if off := unsafe.Offsetof(a.Type); off != 0 {
		t.Errorf("ConfigAttrib.Type offset = %d, want 0", off)
	}
	if off := unsafe.Offsetof(a.Value); off != 4 {
		t.Errorf("ConfigAttrib.Value offset = %d, want 4", off)
	}
}

func TestGenericValueOffsets(t *testing.T) {
	var v GenericValue
	if off := unsafe.Offsetof(v.Type); off != 0 {
		t.Errorf("GenericValue.Type offset = %d, want 0", off)
	}
	if off := unsafe.Offsetof(v.value); off != 8 {
		t.Errorf("GenericValue.value offset = %d, want 8", off)
	}
}

func TestSurfaceAttribOffsets(t *testing.T) {
	var a SurfaceAttrib
	if off := unsafe.Offsetof(a.Type); off != 0 {
		t.Errorf("SurfaceAttrib.Type offset = %d, want 0", off)
	}
	if off := unsafe.Offsetof(a.Flags); off != 4 {
		t.Errorf("SurfaceAttrib.Flags offset = %d, want 4", off)
	}
	if off := unsafe.Offsetof(a.Value); off != 8 {
		t.Errorf("SurfaceAttrib.Value offset = %d, want 8", off)
	}
}

func TestPictureH264Offsets(t *testing.T) {
	var p PictureH264
	if off := unsafe.Offsetof(p.PictureID); off != 0 {
		t.Errorf("PictureH264.PictureID offset = %d, want 0", off)
	}
	if off := unsafe.Offsetof(p.FrameIdx); off != 4 {
		t.Errorf("PictureH264.FrameIdx offset = %d, want 4", off)
	}
	if off := unsafe.Offsetof(p.Flags); off != 8 {
		t.Errorf("PictureH264.Flags offset = %d, want 8", off)
	}
	if off := unsafe.Offsetof(p.TopFieldOrderCnt); off != 12 {
		t.Errorf("PictureH264.TopFieldOrderCnt offset = %d, want 12", off)
	}
	if off := unsafe.Offsetof(p.BottomFieldOrderCnt); off != 16 {
		t.Errorf("PictureH264.BottomFieldOrderCnt offset = %d, want 16", off)
	}
	if off := unsafe.Offsetof(p.vaReserved); off != 20 {
		t.Errorf("PictureH264.vaReserved offset = %d, want 20", off)
	}
}

func TestEncSequenceParameterBufferH264Offsets(t *testing.T) {
	var s EncSequenceParameterBufferH264
	cases := []struct {
		name string
		got  uintptr
		want uintptr
	}{
		{"SeqParameterSetID", unsafe.Offsetof(s.SeqParameterSetID), 0},
		{"LevelIDC", unsafe.Offsetof(s.LevelIDC), 1},
		{"IntraPeriod", unsafe.Offsetof(s.IntraPeriod), 4},
		{"IntraIDRPeriod", unsafe.Offsetof(s.IntraIDRPeriod), 8},
		{"IPPeriod", unsafe.Offsetof(s.IPPeriod), 12},
		{"BitsPerSecond", unsafe.Offsetof(s.BitsPerSecond), 16},
		{"MaxNumRefFrames", unsafe.Offsetof(s.MaxNumRefFrames), 20},
		{"PictureWidthInMbs", unsafe.Offsetof(s.PictureWidthInMbs), 24},
		{"PictureHeightInMbs", unsafe.Offsetof(s.PictureHeightInMbs), 26},
		{"seqFields", unsafe.Offsetof(s.seqFields), 28},
		{"BitDepthLumaMinus8", unsafe.Offsetof(s.BitDepthLumaMinus8), 32},
		{"BitDepthChromaMinus8", unsafe.Offsetof(s.BitDepthChromaMinus8), 33},
		{"NumRefFramesInPicOrderCntCycle", unsafe.Offsetof(s.NumRefFramesInPicOrderCntCycle), 34},
		{"OffsetForNonRefPic", unsafe.Offsetof(s.OffsetForNonRefPic), 36},
		{"OffsetForTopToBottomField", unsafe.Offsetof(s.OffsetForTopToBottomField), 40},
		{"OffsetForRefFrame", unsafe.Offsetof(s.OffsetForRefFrame), 44},
		{"FrameCroppingFlag", unsafe.Offsetof(s.FrameCroppingFlag), 1068},
		{"FrameCropLeftOffset", unsafe.Offsetof(s.FrameCropLeftOffset), 1072},
		{"FrameCropRightOffset", unsafe.Offsetof(s.FrameCropRightOffset), 1076},
		{"FrameCropTopOffset", unsafe.Offsetof(s.FrameCropTopOffset), 1080},
		{"FrameCropBottomOffset", unsafe.Offsetof(s.FrameCropBottomOffset), 1084},
		{"VUIParametersPresentFlag", unsafe.Offsetof(s.VUIParametersPresentFlag), 1088},
		{"vuiFields", unsafe.Offsetof(s.vuiFields), 1092},
		{"AspectRatioIDC", unsafe.Offsetof(s.AspectRatioIDC), 1096},
		{"SarWidth", unsafe.Offsetof(s.SarWidth), 1100},
		{"SarHeight", unsafe.Offsetof(s.SarHeight), 1104},
		{"NumUnitsInTick", unsafe.Offsetof(s.NumUnitsInTick), 1108},
		{"TimeScale", unsafe.Offsetof(s.TimeScale), 1112},
		{"vaReserved", unsafe.Offsetof(s.vaReserved), 1116},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("EncSequenceParameterBufferH264.%s offset = %d, want %d", c.name, c.got, c.want)
		}
	}
}

func TestEncPictureParameterBufferH264Offsets(t *testing.T) {
	var p EncPictureParameterBufferH264
	cases := []struct {
		name string
		got  uintptr
		want uintptr
	}{
		{"CurrPic", unsafe.Offsetof(p.CurrPic), 0},
		{"ReferenceFrames", unsafe.Offsetof(p.ReferenceFrames), 36},
		{"CodedBuf", unsafe.Offsetof(p.CodedBuf), 612},
		{"PicParameterSetID", unsafe.Offsetof(p.PicParameterSetID), 616},
		{"SeqParameterSetID", unsafe.Offsetof(p.SeqParameterSetID), 617},
		{"LastPicture", unsafe.Offsetof(p.LastPicture), 618},
		{"FrameNum", unsafe.Offsetof(p.FrameNum), 620},
		{"PicInitQP", unsafe.Offsetof(p.PicInitQP), 622},
		{"NumRefIdxL0ActiveMinus1", unsafe.Offsetof(p.NumRefIdxL0ActiveMinus1), 623},
		{"NumRefIdxL1ActiveMinus1", unsafe.Offsetof(p.NumRefIdxL1ActiveMinus1), 624},
		{"ChromaQPIndexOffset", unsafe.Offsetof(p.ChromaQPIndexOffset), 625},
		{"SecondChromaQPIndexOffset", unsafe.Offsetof(p.SecondChromaQPIndexOffset), 626},
		{"picFields", unsafe.Offsetof(p.picFields), 628},
		{"vaReserved", unsafe.Offsetof(p.vaReserved), 632},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("EncPictureParameterBufferH264.%s offset = %d, want %d", c.name, c.got, c.want)
		}
	}
}

func TestEncSliceParameterBufferH264Offsets(t *testing.T) {
	var s EncSliceParameterBufferH264
	cases := []struct {
		name string
		got  uintptr
		want uintptr
	}{
		{"MacroblockAddress", unsafe.Offsetof(s.MacroblockAddress), 0},
		{"NumMacroblocks", unsafe.Offsetof(s.NumMacroblocks), 4},
		{"MacroblockInfo", unsafe.Offsetof(s.MacroblockInfo), 8},
		{"SliceType", unsafe.Offsetof(s.SliceType), 12},
		{"PicParameterSetID", unsafe.Offsetof(s.PicParameterSetID), 13},
		{"IdrPicID", unsafe.Offsetof(s.IdrPicID), 14},
		{"PicOrderCntLsb", unsafe.Offsetof(s.PicOrderCntLsb), 16},
		{"DeltaPicOrderCntBottom", unsafe.Offsetof(s.DeltaPicOrderCntBottom), 20},
		{"DeltaPicOrderCnt", unsafe.Offsetof(s.DeltaPicOrderCnt), 24},
		{"DirectSpatialMvPredFlag", unsafe.Offsetof(s.DirectSpatialMvPredFlag), 32},
		{"NumRefIdxActiveOverrideFlag", unsafe.Offsetof(s.NumRefIdxActiveOverrideFlag), 33},
		{"NumRefIdxL0ActiveMinus1", unsafe.Offsetof(s.NumRefIdxL0ActiveMinus1), 34},
		{"NumRefIdxL1ActiveMinus1", unsafe.Offsetof(s.NumRefIdxL1ActiveMinus1), 35},
		{"RefPicList0", unsafe.Offsetof(s.RefPicList0), 36},
		{"RefPicList1", unsafe.Offsetof(s.RefPicList1), 1188},
		{"LumaLog2WeightDenom", unsafe.Offsetof(s.LumaLog2WeightDenom), 2340},
		{"ChromaLog2WeightDenom", unsafe.Offsetof(s.ChromaLog2WeightDenom), 2341},
		{"LumaWeightL0Flag", unsafe.Offsetof(s.LumaWeightL0Flag), 2342},
		{"LumaWeightL0", unsafe.Offsetof(s.LumaWeightL0), 2344},
		{"LumaOffsetL0", unsafe.Offsetof(s.LumaOffsetL0), 2408},
		{"ChromaWeightL0Flag", unsafe.Offsetof(s.ChromaWeightL0Flag), 2472},
		{"ChromaWeightL0", unsafe.Offsetof(s.ChromaWeightL0), 2474},
		{"ChromaOffsetL0", unsafe.Offsetof(s.ChromaOffsetL0), 2602},
		{"LumaWeightL1Flag", unsafe.Offsetof(s.LumaWeightL1Flag), 2730},
		{"LumaWeightL1", unsafe.Offsetof(s.LumaWeightL1), 2732},
		{"LumaOffsetL1", unsafe.Offsetof(s.LumaOffsetL1), 2796},
		{"ChromaWeightL1Flag", unsafe.Offsetof(s.ChromaWeightL1Flag), 2860},
		{"ChromaWeightL1", unsafe.Offsetof(s.ChromaWeightL1), 2862},
		{"ChromaOffsetL1", unsafe.Offsetof(s.ChromaOffsetL1), 2990},
		{"CabacInitIDC", unsafe.Offsetof(s.CabacInitIDC), 3118},
		{"SliceQPDelta", unsafe.Offsetof(s.SliceQPDelta), 3119},
		{"DisableDeblockingFilterIDC", unsafe.Offsetof(s.DisableDeblockingFilterIDC), 3120},
		{"SliceAlphaC0OffsetDiv2", unsafe.Offsetof(s.SliceAlphaC0OffsetDiv2), 3121},
		{"SliceBetaOffsetDiv2", unsafe.Offsetof(s.SliceBetaOffsetDiv2), 3122},
		{"vaReserved", unsafe.Offsetof(s.vaReserved), 3124},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("EncSliceParameterBufferH264.%s offset = %d, want %d", c.name, c.got, c.want)
		}
	}
}

func TestCodedBufferSegmentOffsets(t *testing.T) {
	var s CodedBufferSegment
	cases := []struct {
		name string
		got  uintptr
		want uintptr
	}{
		{"Size", unsafe.Offsetof(s.Size), 0},
		{"BitOffset", unsafe.Offsetof(s.BitOffset), 4},
		{"Status", unsafe.Offsetof(s.Status), 8},
		{"Reserved", unsafe.Offsetof(s.Reserved), 12},
		{"Buf", unsafe.Offsetof(s.Buf), 16},
		{"Next", unsafe.Offsetof(s.Next), 24},
		{"vaReserved", unsafe.Offsetof(s.vaReserved), 32},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("CodedBufferSegment.%s offset = %d, want %d", c.name, c.got, c.want)
		}
	}
}

func TestImageFormatOffsets(t *testing.T) {
	var f ImageFormat
	cases := []struct {
		name string
		got  uintptr
		want uintptr
	}{
		{"FourCC", unsafe.Offsetof(f.FourCC), 0},
		{"ByteOrder", unsafe.Offsetof(f.ByteOrder), 4},
		{"BitsPerPixel", unsafe.Offsetof(f.BitsPerPixel), 8},
		{"Depth", unsafe.Offsetof(f.Depth), 12},
		{"RedMask", unsafe.Offsetof(f.RedMask), 16},
		{"GreenMask", unsafe.Offsetof(f.GreenMask), 20},
		{"BlueMask", unsafe.Offsetof(f.BlueMask), 24},
		{"AlphaMask", unsafe.Offsetof(f.AlphaMask), 28},
		{"vaReserved", unsafe.Offsetof(f.vaReserved), 32},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("ImageFormat.%s offset = %d, want %d", c.name, c.got, c.want)
		}
	}
}

func TestImageOffsets(t *testing.T) {
	var im Image
	cases := []struct {
		name string
		got  uintptr
		want uintptr
	}{
		{"ImageID", unsafe.Offsetof(im.ImageID), 0},
		{"Format", unsafe.Offsetof(im.Format), 4},
		{"Buf", unsafe.Offsetof(im.Buf), 52},
		{"Width", unsafe.Offsetof(im.Width), 56},
		{"Height", unsafe.Offsetof(im.Height), 58},
		{"DataSize", unsafe.Offsetof(im.DataSize), 60},
		{"NumPlanes", unsafe.Offsetof(im.NumPlanes), 64},
		{"Pitches", unsafe.Offsetof(im.Pitches), 68},
		{"Offsets", unsafe.Offsetof(im.Offsets), 80},
		{"NumPaletteEntries", unsafe.Offsetof(im.NumPaletteEntries), 92},
		{"EntryBytes", unsafe.Offsetof(im.EntryBytes), 96},
		{"ComponentOrder", unsafe.Offsetof(im.ComponentOrder), 100},
		{"vaReserved", unsafe.Offsetof(im.vaReserved), 104},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("Image.%s offset = %d, want %d", c.name, c.got, c.want)
		}
	}
}

func TestSeqFieldsBitLayout(t *testing.T) {
	var s EncSequenceParameterBufferH264
	s.SetChromaFormatIDC(3)
	if got := s.RawSeqFields(); got != 0x00000003 {
		t.Errorf("SetChromaFormatIDC(3): raw = 0x%08X, want 0x00000003", got)
	}
	s = EncSequenceParameterBufferH264{}
	s.SetFrameMbsOnlyFlag(true)
	if got := s.RawSeqFields(); got != 0x00000004 {
		t.Errorf("SetFrameMbsOnlyFlag(true): raw = 0x%08X, want 0x00000004", got)
	}
	s = EncSequenceParameterBufferH264{}
	s.SetMbAdaptiveFrameFieldFlag(true)
	if got := s.RawSeqFields(); got != 0x00000008 {
		t.Errorf("SetMbAdaptiveFrameFieldFlag(true): raw = 0x%08X, want 0x00000008", got)
	}
	s = EncSequenceParameterBufferH264{}
	s.SetSeqScalingMatrixPresentFlag(true)
	if got := s.RawSeqFields(); got != 0x00000010 {
		t.Errorf("SetSeqScalingMatrixPresentFlag(true): raw = 0x%08X, want 0x00000010", got)
	}
	s = EncSequenceParameterBufferH264{}
	s.SetDirect8x8InferenceFlag(true)
	if got := s.RawSeqFields(); got != 0x00000020 {
		t.Errorf("SetDirect8x8InferenceFlag(true): raw = 0x%08X, want 0x00000020", got)
	}
	s = EncSequenceParameterBufferH264{}
	s.SetLog2MaxFrameNumMinus4(0xF)
	if got := s.RawSeqFields(); got != 0x000003C0 {
		t.Errorf("SetLog2MaxFrameNumMinus4(0xF): raw = 0x%08X, want 0x000003C0", got)
	}
	s = EncSequenceParameterBufferH264{}
	s.SetPicOrderCntType(3)
	if got := s.RawSeqFields(); got != 0x00000C00 {
		t.Errorf("SetPicOrderCntType(3): raw = 0x%08X, want 0x00000C00", got)
	}
	s = EncSequenceParameterBufferH264{}
	s.SetLog2MaxPicOrderCntLsbMinus4(0xF)
	if got := s.RawSeqFields(); got != 0x0000F000 {
		t.Errorf("SetLog2MaxPicOrderCntLsbMinus4(0xF): raw = 0x%08X, want 0x0000F000", got)
	}
	s = EncSequenceParameterBufferH264{}
	s.SetDeltaPicOrderAlwaysZeroFlag(true)
	if got := s.RawSeqFields(); got != 0x00010000 {
		t.Errorf("SetDeltaPicOrderAlwaysZeroFlag(true): raw = 0x%08X, want 0x00010000", got)
	}

	s = EncSequenceParameterBufferH264{}
	s.SetChromaFormatIDC(3)
	s.SetFrameMbsOnlyFlag(true)
	s.SetMbAdaptiveFrameFieldFlag(true)
	s.SetSeqScalingMatrixPresentFlag(true)
	s.SetDirect8x8InferenceFlag(true)
	s.SetLog2MaxFrameNumMinus4(0xF)
	s.SetPicOrderCntType(3)
	s.SetLog2MaxPicOrderCntLsbMinus4(0xF)
	s.SetDeltaPicOrderAlwaysZeroFlag(true)
	if got := s.RawSeqFields(); got != 0x0001FFFF {
		t.Errorf("all seq_fields bits set: raw = 0x%08X, want 0x0001FFFF", got)
	}
}

func TestPicFieldsBitLayout(t *testing.T) {
	var p EncPictureParameterBufferH264
	p.SetIdrPicFlag(true)
	if got := p.RawPicFields(); got != 0x00000001 {
		t.Errorf("SetIdrPicFlag(true): raw = 0x%08X, want 0x00000001", got)
	}
	p = EncPictureParameterBufferH264{}
	p.SetReferencePicFlag(3)
	if got := p.RawPicFields(); got != 0x00000006 {
		t.Errorf("SetReferencePicFlag(3): raw = 0x%08X, want 0x00000006", got)
	}
	p = EncPictureParameterBufferH264{}
	p.SetEntropyCodingModeFlag(true)
	if got := p.RawPicFields(); got != 0x00000008 {
		t.Errorf("SetEntropyCodingModeFlag(true): raw = 0x%08X, want 0x00000008", got)
	}
	p = EncPictureParameterBufferH264{}
	p.SetWeightedPredFlag(true)
	if got := p.RawPicFields(); got != 0x00000010 {
		t.Errorf("SetWeightedPredFlag(true): raw = 0x%08X, want 0x00000010", got)
	}
	p = EncPictureParameterBufferH264{}
	p.SetWeightedBipredIDC(3)
	if got := p.RawPicFields(); got != 0x00000060 {
		t.Errorf("SetWeightedBipredIDC(3): raw = 0x%08X, want 0x00000060", got)
	}
	p = EncPictureParameterBufferH264{}
	p.SetConstrainedIntraPredFlag(true)
	if got := p.RawPicFields(); got != 0x00000080 {
		t.Errorf("SetConstrainedIntraPredFlag(true): raw = 0x%08X, want 0x00000080", got)
	}
	p = EncPictureParameterBufferH264{}
	p.SetTransform8x8ModeFlag(true)
	if got := p.RawPicFields(); got != 0x00000100 {
		t.Errorf("SetTransform8x8ModeFlag(true): raw = 0x%08X, want 0x00000100", got)
	}
	p = EncPictureParameterBufferH264{}
	p.SetDeblockingFilterControlPresentFlag(true)
	if got := p.RawPicFields(); got != 0x00000200 {
		t.Errorf("SetDeblockingFilterControlPresentFlag(true): raw = 0x%08X, want 0x00000200", got)
	}
	p = EncPictureParameterBufferH264{}
	p.SetRedundantPicCntPresentFlag(true)
	if got := p.RawPicFields(); got != 0x00000400 {
		t.Errorf("SetRedundantPicCntPresentFlag(true): raw = 0x%08X, want 0x00000400", got)
	}
	p = EncPictureParameterBufferH264{}
	p.SetPicOrderPresentFlag(true)
	if got := p.RawPicFields(); got != 0x00000800 {
		t.Errorf("SetPicOrderPresentFlag(true): raw = 0x%08X, want 0x00000800", got)
	}
	p = EncPictureParameterBufferH264{}
	p.SetPicScalingMatrixPresentFlag(true)
	if got := p.RawPicFields(); got != 0x00001000 {
		t.Errorf("SetPicScalingMatrixPresentFlag(true): raw = 0x%08X, want 0x00001000", got)
	}

	p = EncPictureParameterBufferH264{}
	p.SetIdrPicFlag(true)
	p.SetReferencePicFlag(3)
	p.SetEntropyCodingModeFlag(true)
	p.SetWeightedPredFlag(true)
	p.SetWeightedBipredIDC(3)
	p.SetConstrainedIntraPredFlag(true)
	p.SetTransform8x8ModeFlag(true)
	p.SetDeblockingFilterControlPresentFlag(true)
	p.SetRedundantPicCntPresentFlag(true)
	p.SetPicOrderPresentFlag(true)
	p.SetPicScalingMatrixPresentFlag(true)
	if got := p.RawPicFields(); got != 0x00001FFF {
		t.Errorf("all pic_fields bits set: raw = 0x%08X, want 0x00001FFF", got)
	}
}

func TestInvalidPictureH264(t *testing.T) {
	p := invalidPictureH264()
	if p.PictureID != InvalidID {
		t.Errorf("invalidPictureH264().PictureID = 0x%08X, want 0x%08X", p.PictureID, InvalidID)
	}
	if p.Flags != PictureH264Invalid {
		t.Errorf("invalidPictureH264().Flags = 0x%08X, want 0x%08X", p.Flags, PictureH264Invalid)
	}
}

func TestGenericValueSetInt(t *testing.T) {
	var v GenericValue
	v.SetInt(int32(FourCCNV12))
	if v.Type != GenericValueTypeInteger {
		t.Errorf("GenericValue.Type = %d after SetInt, want %d", v.Type, GenericValueTypeInteger)
	}
	if v.value != uint64(FourCCNV12) {
		t.Errorf("GenericValue.value = 0x%016X after SetInt(0x%08X), want 0x%016X", v.value, FourCCNV12, uint64(FourCCNV12))
	}
}

func TestStatusErrorStrings(t *testing.T) {
	if StatusSuccess != 0 {
		t.Errorf("StatusSuccess = %d, want 0", StatusSuccess)
	}
	if got := StatusErrorInvalidValue.Error(); got == "" {
		t.Error("StatusErrorInvalidValue.Error() returned an empty string")
	}
	unknown := Status(0x7fffffff)
	if got := unknown.Error(); got == "" {
		t.Error("unrecognized Status.Error() returned an empty string")
	}
}

func TestWellKnownConstants(t *testing.T) {
	cases := []struct {
		name string
		got  int64
		want int64
	}{
		{"ProfileH264Main", int64(ProfileH264Main), 6},
		{"ProfileH264High", int64(ProfileH264High), 7},
		{"ProfileH264ConstrainedBaseline", int64(ProfileH264ConstrainedBaseline), 13},
		{"EntrypointEncSlice", int64(EntrypointEncSlice), 6},
		{"ConfigAttribRTFormat", int64(ConfigAttribRTFormat), 0},
		{"ConfigAttribRateControl", int64(ConfigAttribRateControl), 5},
		{"RTFormatYUV420", int64(RTFormatYUV420), 0x1},
		{"RCCQP", int64(RCCQP), 0x10},
		{"FourCCNV12", int64(FourCCNV12), 0x3231564e},
		{"SurfaceAttribPixelFormat", int64(SurfaceAttribPixelFormat), 1},
		{"GenericValueTypeInteger", int64(GenericValueTypeInteger), 1},
		{"BufferTypeEncCoded", int64(BufferTypeEncCoded), 21},
		{"BufferTypeEncSequenceParameter", int64(BufferTypeEncSequenceParameter), 22},
		{"BufferTypeEncPictureParameter", int64(BufferTypeEncPictureParameter), 23},
		{"BufferTypeEncSliceParameter", int64(BufferTypeEncSliceParameter), 24},
		{"InvalidID", int64(InvalidID), 0xffffffff},
		{"Progressive", int64(Progressive), 0x1},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("%s = 0x%X, want 0x%X", c.name, c.got, c.want)
		}
	}
}
