package nvenc

import (
	"fmt"
	"unsafe"
)

const (
	NvEncAPIMajorVersion uint32 = 11
	NvEncAPIMinorVersion uint32 = 1
)

const NvEncAPIVersion = NvEncAPIMajorVersion | (NvEncAPIMinorVersion << 24)

func NvEncAPIStructVersion(ver uint32) uint32 {
	return NvEncAPIVersion | (ver << 16) | (0x7 << 28)
}

const (
	NvEncCapsParamVersion                 = NvEncAPIVersion | (1 << 16) | (0x7 << 28)
	NvEncCreateInputBufferVersion         = NvEncAPIVersion | (1 << 16) | (0x7 << 28)
	NvEncCreateBitstreamBufferVersion     = NvEncAPIVersion | (1 << 16) | (0x7 << 28)
	NvEncRCParamsVersion                  = NvEncAPIVersion | (1 << 16) | (0x7 << 28)
	NvEncLockBitstreamVersion             = NvEncAPIVersion | (1 << 16) | (0x7 << 28)
	NvEncLockInputBufferVersion           = NvEncAPIVersion | (1 << 16) | (0x7 << 28)
	NvEncSequenceParamPayloadVersion      = NvEncAPIVersion | (1 << 16) | (0x7 << 28)
	NvEncEventParamsVersion               = NvEncAPIVersion | (1 << 16) | (0x7 << 28)
	NvEncOpenEncodeSessionExParamsVersion = NvEncAPIVersion | (1 << 16) | (0x7 << 28)
	NvEncodeAPIFunctionListVersion        = NvEncAPIVersion | (2 << 16) | (0x7 << 28)
	NvEncConfigVersion                    = NvEncAPIVersion | (7 << 16) | (0x7 << 28) | (1 << 31)
	NvEncInitializeParamsVersion          = NvEncAPIVersion | (5 << 16) | (0x7 << 28) | (1 << 31)
	NvEncPresetConfigVersion              = NvEncAPIVersion | (4 << 16) | (0x7 << 28) | (1 << 31)
	NvEncPicParamsVersion                 = NvEncAPIVersion | (4 << 16) | (0x7 << 28) | (1 << 31)
)

type NvEncStatus int32

const (
	NvEncSuccess NvEncStatus = iota
	NvEncErrNoEncodeDevice
	NvEncErrUnsupportedDevice
	NvEncErrInvalidEncoderDevice
	NvEncErrInvalidDevice
	NvEncErrDeviceNotExist
	NvEncErrInvalidPtr
	NvEncErrInvalidEvent
	NvEncErrInvalidParam
	NvEncErrInvalidCall
	NvEncErrOutOfMemory
	NvEncErrEncoderNotInitialized
	NvEncErrUnsupportedParam
	NvEncErrLockBusy
	NvEncErrNotEnoughBuffer
	NvEncErrInvalidVersion
	NvEncErrMapFailed
	NvEncErrNeedMoreInput
	NvEncErrEncoderBusy
	NvEncErrEventNotRegisterd
	NvEncErrGeneric
	NvEncErrIncompatibleClientKey
	NvEncErrUnimplemented
	NvEncErrResourceRegisterFailed
	NvEncErrResourceNotRegistered
	NvEncErrResourceNotMapped
)

var nvEncStatusNames = map[NvEncStatus]string{
	NvEncSuccess:                   "NV_ENC_SUCCESS",
	NvEncErrNoEncodeDevice:         "NV_ENC_ERR_NO_ENCODE_DEVICE",
	NvEncErrUnsupportedDevice:      "NV_ENC_ERR_UNSUPPORTED_DEVICE",
	NvEncErrInvalidEncoderDevice:   "NV_ENC_ERR_INVALID_ENCODERDEVICE",
	NvEncErrInvalidDevice:          "NV_ENC_ERR_INVALID_DEVICE",
	NvEncErrDeviceNotExist:         "NV_ENC_ERR_DEVICE_NOT_EXIST",
	NvEncErrInvalidPtr:             "NV_ENC_ERR_INVALID_PTR",
	NvEncErrInvalidEvent:           "NV_ENC_ERR_INVALID_EVENT",
	NvEncErrInvalidParam:           "NV_ENC_ERR_INVALID_PARAM",
	NvEncErrInvalidCall:            "NV_ENC_ERR_INVALID_CALL",
	NvEncErrOutOfMemory:            "NV_ENC_ERR_OUT_OF_MEMORY",
	NvEncErrEncoderNotInitialized:  "NV_ENC_ERR_ENCODER_NOT_INITIALIZED",
	NvEncErrUnsupportedParam:       "NV_ENC_ERR_UNSUPPORTED_PARAM",
	NvEncErrLockBusy:               "NV_ENC_ERR_LOCK_BUSY",
	NvEncErrNotEnoughBuffer:        "NV_ENC_ERR_NOT_ENOUGH_BUFFER",
	NvEncErrInvalidVersion:         "NV_ENC_ERR_INVALID_VERSION",
	NvEncErrMapFailed:              "NV_ENC_ERR_MAP_FAILED",
	NvEncErrNeedMoreInput:          "NV_ENC_ERR_NEED_MORE_INPUT",
	NvEncErrEncoderBusy:            "NV_ENC_ERR_ENCODER_BUSY",
	NvEncErrEventNotRegisterd:      "NV_ENC_ERR_EVENT_NOT_REGISTERD",
	NvEncErrGeneric:                "NV_ENC_ERR_GENERIC",
	NvEncErrIncompatibleClientKey:  "NV_ENC_ERR_INCOMPATIBLE_CLIENT_KEY",
	NvEncErrUnimplemented:          "NV_ENC_ERR_UNIMPLEMENTED",
	NvEncErrResourceRegisterFailed: "NV_ENC_ERR_RESOURCE_REGISTER_FAILED",
	NvEncErrResourceNotRegistered:  "NV_ENC_ERR_RESOURCE_NOT_REGISTERED",
	NvEncErrResourceNotMapped:      "NV_ENC_ERR_RESOURCE_NOT_MAPPED",
}

func (s NvEncStatus) Error() string {
	if name, ok := nvEncStatusNames[s]; ok {
		return fmt.Sprintf("nvenc: %s (nvenc status %d)", name, int32(s))
	}
	return fmt.Sprintf("nvenc: nvenc status %d", int32(s))
}

type GUID struct {
	Data1 uint32
	Data2 uint16
	Data3 uint16
	Data4 [8]byte
}

var (
	NvEncCodecH264GUID = GUID{0x6bc82762, 0x4e63, 0x4ca4, [8]byte{0xaa, 0x85, 0x1e, 0x50, 0xf3, 0x21, 0xf6, 0xbf}}

	NvEncH264ProfileBaselineGUID = GUID{0x727bcaa, 0x78c4, 0x4c83, [8]byte{0x8c, 0x2f, 0xef, 0x3d, 0xff, 0x26, 0x7c, 0x6a}}
	NvEncH264ProfileMainGUID     = GUID{0x60b5c1d4, 0x67fe, 0x4790, [8]byte{0x94, 0xd5, 0xc4, 0x72, 0x6d, 0x7b, 0x6e, 0x6d}}
	NvEncH264ProfileHighGUID     = GUID{0xe7cbc309, 0x4f7a, 0x4b89, [8]byte{0xaf, 0x2a, 0xd5, 0x37, 0xc9, 0x2b, 0xe3, 0x10}}

	NvEncPresetDefaultGUID           = GUID{0xb2dfb705, 0x4ebd, 0x4c49, [8]byte{0x9b, 0x5f, 0x24, 0xa7, 0x77, 0xd3, 0xe5, 0x87}}
	NvEncPresetHPGUID                = GUID{0x60e4c59f, 0xe846, 0x4484, [8]byte{0xa5, 0x6d, 0xcd, 0x45, 0xbe, 0x9f, 0xdd, 0xf6}}
	NvEncPresetLowLatencyDefaultGUID = GUID{0x49df21c5, 0x6dfa, 0x4feb, [8]byte{0x97, 0x87, 0x6a, 0xcc, 0x9e, 0xff, 0xb7, 0x26}}

	NvEncPresetP1GUID = GUID{0xfc0a8d3e, 0x45f8, 0x4cf8, [8]byte{0x80, 0xc7, 0x29, 0x88, 0x71, 0x59, 0x0e, 0xbf}}
	NvEncPresetP2GUID = GUID{0xf581cfb8, 0x88d6, 0x4381, [8]byte{0x93, 0xf0, 0xdf, 0x13, 0xf9, 0xc2, 0x7d, 0xab}}
	NvEncPresetP3GUID = GUID{0x36850110, 0x3a07, 0x441f, [8]byte{0x94, 0xd5, 0x36, 0x70, 0x63, 0x1f, 0x91, 0xf6}}
	NvEncPresetP4GUID = GUID{0x90a7b826, 0xdf06, 0x4862, [8]byte{0xb9, 0xd2, 0xcd, 0x6d, 0x73, 0xa0, 0x86, 0x81}}
	NvEncPresetP5GUID = GUID{0x21c6e6b4, 0x297a, 0x4cba, [8]byte{0x99, 0x8f, 0xb6, 0xcb, 0xde, 0x72, 0xad, 0xe3}}
	NvEncPresetP6GUID = GUID{0x8e75c279, 0x6299, 0x4ab6, [8]byte{0x83, 0x02, 0x0b, 0x21, 0x5a, 0x33, 0x5c, 0xf5}}
	NvEncPresetP7GUID = GUID{0x84848c12, 0x6f71, 0x4c13, [8]byte{0x93, 0x1b, 0x53, 0xe2, 0x83, 0xf5, 0x79, 0x74}}
)

type NvEncBufferFormat uint32

const (
	NvEncBufferFormatUndefined   NvEncBufferFormat = 0x00000000
	NvEncBufferFormatNV12        NvEncBufferFormat = 0x00000001
	NvEncBufferFormatYV12        NvEncBufferFormat = 0x00000010
	NvEncBufferFormatIYUV        NvEncBufferFormat = 0x00000100
	NvEncBufferFormatYUV444      NvEncBufferFormat = 0x00001000
	NvEncBufferFormatYUV42010Bit NvEncBufferFormat = 0x00010000
	NvEncBufferFormatYUV44410Bit NvEncBufferFormat = 0x00100000
	NvEncBufferFormatARGB        NvEncBufferFormat = 0x01000000
	NvEncBufferFormatARGB10      NvEncBufferFormat = 0x02000000
	NvEncBufferFormatAYUV        NvEncBufferFormat = 0x04000000
	NvEncBufferFormatABGR        NvEncBufferFormat = 0x10000000
	NvEncBufferFormatABGR10      NvEncBufferFormat = 0x20000000
	NvEncBufferFormatU8          NvEncBufferFormat = 0x40000000
)

type NvEncDeviceType uint32

const (
	NvEncDeviceTypeDirectX NvEncDeviceType = 0x0
	NvEncDeviceTypeCUDA    NvEncDeviceType = 0x1
	NvEncDeviceTypeOpenGL  NvEncDeviceType = 0x2
)

type NvEncPicStruct uint32

const (
	NvEncPicStructFrame          NvEncPicStruct = 0x01
	NvEncPicStructFieldTopBottom NvEncPicStruct = 0x02
	NvEncPicStructFieldBottomTop NvEncPicStruct = 0x03
)

type NvEncPicFlags uint32

const (
	NvEncPicFlagForceIntra   NvEncPicFlags = 0x1
	NvEncPicFlagForceIDR     NvEncPicFlags = 0x2
	NvEncPicFlagOutputSPSPPS NvEncPicFlags = 0x4
	NvEncPicFlagEOS          NvEncPicFlags = 0x8
)

type NvEncPicType uint32

const (
	NvEncPicTypeP            NvEncPicType = 0x0
	NvEncPicTypeB            NvEncPicType = 0x01
	NvEncPicTypeI            NvEncPicType = 0x02
	NvEncPicTypeIDR          NvEncPicType = 0x03
	NvEncPicTypeBI           NvEncPicType = 0x04
	NvEncPicTypeSkipped      NvEncPicType = 0x05
	NvEncPicTypeIntraRefresh NvEncPicType = 0x06
	NvEncPicTypeNonRefP      NvEncPicType = 0x07
	NvEncPicTypeUnknown      NvEncPicType = 0xFF
)

type NvEncTuningInfo uint32

const (
	NvEncTuningInfoUndefined       NvEncTuningInfo = 0
	NvEncTuningInfoHighQuality     NvEncTuningInfo = 1
	NvEncTuningInfoLowLatency      NvEncTuningInfo = 2
	NvEncTuningInfoUltraLowLatency NvEncTuningInfo = 3
	NvEncTuningInfoLossless        NvEncTuningInfo = 4
	NvEncTuningInfoCount           NvEncTuningInfo = 5
)

type NvEncParamsRCMode uint32

const (
	NvEncParamsRCConstQP       NvEncParamsRCMode = 0x0
	NvEncParamsRCVBR           NvEncParamsRCMode = 0x1
	NvEncParamsRCCBR           NvEncParamsRCMode = 0x2
	NvEncParamsRCCBRLowDelayHQ NvEncParamsRCMode = 0x8
	NvEncParamsRCCBRHQ         NvEncParamsRCMode = 0x10
	NvEncParamsRCVBRHQ         NvEncParamsRCMode = 0x20
)

type NvEncMemoryHeap uint32

const (
	NvEncMemoryHeapAutoSelect     NvEncMemoryHeap = 0
	NvEncMemoryHeapVid            NvEncMemoryHeap = 1
	NvEncMemoryHeapSysMemCached   NvEncMemoryHeap = 2
	NvEncMemoryHeapSysMemUncached NvEncMemoryHeap = 3
)

type NvEncInputResourceType uint32

const (
	NvEncInputResourceTypeDirectX       NvEncInputResourceType = 0x0
	NvEncInputResourceTypeCUDADevicePtr NvEncInputResourceType = 0x1
	NvEncInputResourceTypeCUDAArray     NvEncInputResourceType = 0x2
	NvEncInputResourceTypeOpenGLTex     NvEncInputResourceType = 0x3
)

type NvEncParamsFrameFieldMode uint32

const (
	NvEncParamsFrameFieldModeFrame NvEncParamsFrameFieldMode = 0x01
	NvEncParamsFrameFieldModeField NvEncParamsFrameFieldMode = 0x02
	NvEncParamsFrameFieldModeMBAFF NvEncParamsFrameFieldMode = 0x03
)

type NvEncMVPrecision uint32

const (
	NvEncMVPrecisionDefault    NvEncMVPrecision = 0x0
	NvEncMVPrecisionFullPel    NvEncMVPrecision = 0x01
	NvEncMVPrecisionHalfPel    NvEncMVPrecision = 0x02
	NvEncMVPrecisionQuarterPel NvEncMVPrecision = 0x03
)

type NvEncQPMapMode uint32

const (
	NvEncQPMapDisabled NvEncQPMapMode = 0x0
	NvEncQPMapEmphasis NvEncQPMapMode = 0x1
	NvEncQPMapDelta    NvEncQPMapMode = 0x2
	NvEncQPMapValue    NvEncQPMapMode = 0x3
)

type NvEncMultiPass uint32

const (
	NvEncMultiPassDisabled        NvEncMultiPass = 0x0
	NvEncTwoPassQuarterResolution NvEncMultiPass = 0x1
	NvEncTwoPassFullResolution    NvEncMultiPass = 0x2
)

type NvEncH264AdaptiveTransformMode uint32

const (
	NvEncH264AdaptiveTransformAutoSelect NvEncH264AdaptiveTransformMode = 0x0
	NvEncH264AdaptiveTransformDisable    NvEncH264AdaptiveTransformMode = 0x1
	NvEncH264AdaptiveTransformEnable     NvEncH264AdaptiveTransformMode = 0x2
)

type NvEncH264FMOMode uint32

const (
	NvEncH264FMOAutoSelect NvEncH264FMOMode = 0x0
	NvEncH264FMOEnable     NvEncH264FMOMode = 0x1
	NvEncH264FMODisable    NvEncH264FMOMode = 0x2
)

type NvEncH264BDirectMode uint32

const (
	NvEncH264BDirectModeAutoSelect NvEncH264BDirectMode = 0x0
	NvEncH264BDirectModeDisable    NvEncH264BDirectMode = 0x1
	NvEncH264BDirectModeTemporal   NvEncH264BDirectMode = 0x2
	NvEncH264BDirectModeSpatial    NvEncH264BDirectMode = 0x3
)

type NvEncH264EntropyCodingMode uint32

const (
	NvEncH264EntropyCodingModeAutoSelect NvEncH264EntropyCodingMode = 0x0
	NvEncH264EntropyCodingModeCABAC      NvEncH264EntropyCodingMode = 0x1
	NvEncH264EntropyCodingModeCAVLC      NvEncH264EntropyCodingMode = 0x2
)

type NvEncStereoPackingMode uint32

const (
	NvEncStereoPackingModeNone          NvEncStereoPackingMode = 0x0
	NvEncStereoPackingModeCheckerboard  NvEncStereoPackingMode = 0x1
	NvEncStereoPackingModeColInterleave NvEncStereoPackingMode = 0x2
	NvEncStereoPackingModeRowInterleave NvEncStereoPackingMode = 0x3
	NvEncStereoPackingModeSideBySide    NvEncStereoPackingMode = 0x4
	NvEncStereoPackingModeTopBottom     NvEncStereoPackingMode = 0x5
	NvEncStereoPackingModeFrameSeq      NvEncStereoPackingMode = 0x6
)

type NvEncBFrameRefMode uint32

const (
	NvEncBFrameRefModeDisabled NvEncBFrameRefMode = 0x0
	NvEncBFrameRefModeEach     NvEncBFrameRefMode = 0x1
	NvEncBFrameRefModeMiddle   NvEncBFrameRefMode = 0x2
)

type NvEncNumRefFrames uint32

const (
	NvEncNumRefFramesAutoSelect NvEncNumRefFrames = 0x0
	NvEncNumRefFrames1          NvEncNumRefFrames = 0x1
	NvEncNumRefFrames2          NvEncNumRefFrames = 0x2
	NvEncNumRefFrames3          NvEncNumRefFrames = 0x3
	NvEncNumRefFrames4          NvEncNumRefFrames = 0x4
	NvEncNumRefFrames5          NvEncNumRefFrames = 0x5
	NvEncNumRefFrames6          NvEncNumRefFrames = 0x6
	NvEncNumRefFrames7          NvEncNumRefFrames = 0x7
)

type NvEncCaps uint32

func bitGet(word uint32, shift uint) bool {
	return (word>>shift)&1 != 0
}

func bitSet(word *uint32, shift uint, v bool) {
	if v {
		*word |= 1 << shift
	} else {
		*word &^= 1 << shift
	}
}

func fieldGet(word uint32, shift, width uint) uint32 {
	mask := uint32(1)<<width - 1
	return (word >> shift) & mask
}

func fieldSet(word *uint32, shift, width uint, v uint32) {
	mask := uint32(1)<<width - 1
	*word = (*word &^ (mask << shift)) | ((v & mask) << shift)
}

type NvEncQP struct {
	QPInterP uint32
	QPInterB uint32
	QPIntra  uint32
}

type NvEncCapsParam struct {
	Version     uint32
	CapsToQuery NvEncCaps
	Reserved    [62]uint32
}

type NvEncCreateInputBuffer struct {
	Version       uint32
	Width         uint32
	Height        uint32
	MemoryHeap    NvEncMemoryHeap
	BufferFmt     NvEncBufferFormat
	Reserved      uint32
	InputBuffer   uintptr
	PSysMemBuffer uintptr
	Reserved1     [57]uint32
	Reserved2     [63]uintptr
}

type NvEncCreateBitstreamBuffer struct {
	Version            uint32
	Size               uint32
	MemoryHeap         NvEncMemoryHeap
	Reserved           uint32
	BitstreamBuffer    uintptr
	BitstreamBufferPtr uintptr
	Reserved1          [58]uint32
	Reserved2          [64]uintptr
}

type NvEncLockInputBuffer struct {
	Version       uint32
	flags         uint32
	InputBuffer   uintptr
	BufferDataPtr uintptr
	Pitch         uint32
	Reserved1     [251]uint32
	Reserved2     [64]uintptr
}

func (p *NvEncLockInputBuffer) DoNotWait() bool     { return bitGet(p.flags, 0) }
func (p *NvEncLockInputBuffer) SetDoNotWait(v bool) { bitSet(&p.flags, 0, v) }
func (p *NvEncLockInputBuffer) RawFlags() uint32    { return p.flags }

type NvEncLockBitstream struct {
	Version               uint32
	flags                 uint32
	OutputBitstream       uintptr
	SliceOffsets          uintptr
	FrameIdx              uint32
	HwEncodeStatus        uint32
	NumSlices             uint32
	BitstreamSizeInBytes  uint32
	OutputTimeStamp       uint64
	OutputDuration        uint64
	BitstreamBufferPtr    uintptr
	PictureType           NvEncPicType
	PictureStruct         NvEncPicStruct
	FrameAvgQP            uint32
	FrameSatd             uint32
	LTRFrameIdx           uint32
	LTRFrameBitmap        uint32
	TemporalID            uint32
	Reserved              [12]uint32
	IntraMBCount          uint32
	InterMBCount          uint32
	AverageMVX            int32
	AverageMVY            int32
	AlphaLayerSizeInBytes uint32
	Reserved1             [218]uint32
	Reserved2             [64]uintptr
}

func (p *NvEncLockBitstream) DoNotWait() bool      { return bitGet(p.flags, 0) }
func (p *NvEncLockBitstream) SetDoNotWait(v bool)  { bitSet(&p.flags, 0, v) }
func (p *NvEncLockBitstream) LTRFrame() bool       { return bitGet(p.flags, 1) }
func (p *NvEncLockBitstream) SetLTRFrame(v bool)   { bitSet(&p.flags, 1, v) }
func (p *NvEncLockBitstream) GetRCStats() bool     { return bitGet(p.flags, 2) }
func (p *NvEncLockBitstream) SetGetRCStats(v bool) { bitSet(&p.flags, 2, v) }
func (p *NvEncLockBitstream) RawFlags() uint32     { return p.flags }

type NvEncExternalMEHintCountsPerBlockType struct {
	flags     uint32
	Reserved1 [3]uint32
}

func (p *NvEncExternalMEHintCountsPerBlockType) NumCandsPerBlk16x16() uint32 {
	return fieldGet(p.flags, 0, 4)
}
func (p *NvEncExternalMEHintCountsPerBlockType) SetNumCandsPerBlk16x16(v uint32) {
	fieldSet(&p.flags, 0, 4, v)
}
func (p *NvEncExternalMEHintCountsPerBlockType) NumCandsPerBlk16x8() uint32 {
	return fieldGet(p.flags, 4, 4)
}
func (p *NvEncExternalMEHintCountsPerBlockType) SetNumCandsPerBlk16x8(v uint32) {
	fieldSet(&p.flags, 4, 4, v)
}
func (p *NvEncExternalMEHintCountsPerBlockType) NumCandsPerBlk8x16() uint32 {
	return fieldGet(p.flags, 8, 4)
}
func (p *NvEncExternalMEHintCountsPerBlockType) SetNumCandsPerBlk8x16(v uint32) {
	fieldSet(&p.flags, 8, 4, v)
}
func (p *NvEncExternalMEHintCountsPerBlockType) NumCandsPerBlk8x8() uint32 {
	return fieldGet(p.flags, 12, 4)
}
func (p *NvEncExternalMEHintCountsPerBlockType) SetNumCandsPerBlk8x8(v uint32) {
	fieldSet(&p.flags, 12, 4, v)
}
func (p *NvEncExternalMEHintCountsPerBlockType) RawFlags() uint32 { return p.flags }

type NvEncPicParamsMVC struct {
	Version    uint32
	ViewID     uint32
	TemporalID uint32
	PriorityID uint32
	Reserved1  [12]uint32
	Reserved2  [8]uintptr
}

type NvEncPicParamsH264Ext struct {
	raw NvEncPicParamsMVC
}

func (u *NvEncPicParamsH264Ext) MVCPicParams() *NvEncPicParamsMVC {
	return &u.raw
}

func (u *NvEncPicParamsH264Ext) Reserved1() *[32]uint32 {
	return (*[32]uint32)(unsafe.Pointer(&u.raw))
}

type NvEncPicParamsH264 struct {
	DisplayPOCSyntax              uint32
	Reserved3                     uint32
	RefPicFlag                    uint32
	ColourPlaneID                 uint32
	ForceIntraRefreshWithFrameCnt uint32
	flags                         uint32
	SliceTypeData                 uintptr
	SliceTypeArrayCnt             uint32
	SEIPayloadArrayCnt            uint32
	SEIPayloadArray               uintptr
	SliceMode                     uint32
	SliceModeData                 uint32
	LTRMarkFrameIdx               uint32
	LTRUseFrameBitmap             uint32
	LTRUsageMode                  uint32
	ForceIntraSliceCount          uint32
	ForceIntraSliceIdx            uintptr
	H264ExtPicParams              NvEncPicParamsH264Ext
	Reserved                      [210]uint32
	Reserved2                     [61]uintptr
}

func (p *NvEncPicParamsH264) ConstrainedFrame() bool        { return bitGet(p.flags, 0) }
func (p *NvEncPicParamsH264) SetConstrainedFrame(v bool)    { bitSet(&p.flags, 0, v) }
func (p *NvEncPicParamsH264) SliceModeDataUpdate() bool     { return bitGet(p.flags, 1) }
func (p *NvEncPicParamsH264) SetSliceModeDataUpdate(v bool) { bitSet(&p.flags, 1, v) }
func (p *NvEncPicParamsH264) LTRMarkFrame() bool            { return bitGet(p.flags, 2) }
func (p *NvEncPicParamsH264) SetLTRMarkFrame(v bool)        { bitSet(&p.flags, 2, v) }
func (p *NvEncPicParamsH264) LTRUseFrames() bool            { return bitGet(p.flags, 3) }
func (p *NvEncPicParamsH264) SetLTRUseFrames(v bool)        { bitSet(&p.flags, 3, v) }
func (p *NvEncPicParamsH264) RawFlags() uint32              { return p.flags }

type NvEncCodecPicParams struct {
	raw NvEncPicParamsH264
}

func (u *NvEncCodecPicParams) H264PicParams() *NvEncPicParamsH264 {
	return &u.raw
}

func (u *NvEncCodecPicParams) Reserved() *[256]uint32 {
	return (*[256]uint32)(unsafe.Pointer(&u.raw))
}

type NvEncPicParams struct {
	Version              uint32
	InputWidth           uint32
	InputHeight          uint32
	InputPitch           uint32
	EncodePicFlags       uint32
	FrameIdx             uint32
	InputTimeStamp       uint64
	InputDuration        uint64
	InputBuffer          uintptr
	OutputBitstream      uintptr
	CompletionEvent      uintptr
	BufferFmt            NvEncBufferFormat
	PictureStruct        NvEncPicStruct
	PictureType          NvEncPicType
	CodecPicParams       NvEncCodecPicParams
	MeHintCountsPerBlock [2]NvEncExternalMEHintCountsPerBlockType
	MeExternalHints      uintptr
	Reserved1            [6]uint32
	Reserved2            [2]uintptr
	QPDeltaMap           uintptr
	QPDeltaMapSize       uint32
	ReservedBitFields    uint32
	MeHintRefPicDist     [2]uint16
	AlphaBuffer          uintptr
	Reserved3            [286]uint32
	Reserved4            [59]uintptr
}

type NvEncConfigH264VUIParameters struct {
	OverscanInfoPresentFlag      uint32
	OverscanInfo                 uint32
	VideoSignalTypePresentFlag   uint32
	VideoFormat                  uint32
	VideoFullRangeFlag           uint32
	ColourDescriptionPresentFlag uint32
	ColourPrimaries              uint32
	TransferCharacteristics      uint32
	ColourMatrix                 uint32
	ChromaSampleLocationFlag     uint32
	ChromaSampleLocationTop      uint32
	ChromaSampleLocationBot      uint32
	BitstreamRestrictionFlag     uint32
	Reserved                     [15]uint32
}

type NvEncConfigH264 struct {
	flags                      uint32
	Level                      uint32
	IDRPeriod                  uint32
	SeparateColourPlaneFlag    uint32
	DisableDeblockingFilterIDC uint32
	NumTemporalLayers          uint32
	SpsID                      uint32
	PpsID                      uint32
	AdaptiveTransformMode      NvEncH264AdaptiveTransformMode
	FmoMode                    NvEncH264FMOMode
	BdirectMode                NvEncH264BDirectMode
	EntropyCodingMode          NvEncH264EntropyCodingMode
	StereoMode                 NvEncStereoPackingMode
	IntraRefreshPeriod         uint32
	IntraRefreshCnt            uint32
	MaxNumRefFrames            uint32
	SliceMode                  uint32
	SliceModeData              uint32
	H264VUIParameters          NvEncConfigH264VUIParameters
	LTRNumFrames               uint32
	LTRTrustMode               uint32
	ChromaFormatIDC            uint32
	MaxTemporalLayers          uint32
	UseBFramesAsRef            NvEncBFrameRefMode
	NumRefL0                   NvEncNumRefFrames
	NumRefL1                   NvEncNumRefFrames
	Reserved1                  [267]uint32
	Reserved2                  [64]uintptr
}

func (c *NvEncConfigH264) EnableTemporalSVC() bool               { return bitGet(c.flags, 0) }
func (c *NvEncConfigH264) SetEnableTemporalSVC(v bool)           { bitSet(&c.flags, 0, v) }
func (c *NvEncConfigH264) EnableStereoMVC() bool                 { return bitGet(c.flags, 1) }
func (c *NvEncConfigH264) SetEnableStereoMVC(v bool)             { bitSet(&c.flags, 1, v) }
func (c *NvEncConfigH264) HierarchicalPFrames() bool             { return bitGet(c.flags, 2) }
func (c *NvEncConfigH264) SetHierarchicalPFrames(v bool)         { bitSet(&c.flags, 2, v) }
func (c *NvEncConfigH264) HierarchicalBFrames() bool             { return bitGet(c.flags, 3) }
func (c *NvEncConfigH264) SetHierarchicalBFrames(v bool)         { bitSet(&c.flags, 3, v) }
func (c *NvEncConfigH264) OutputBufferingPeriodSEI() bool        { return bitGet(c.flags, 4) }
func (c *NvEncConfigH264) SetOutputBufferingPeriodSEI(v bool)    { bitSet(&c.flags, 4, v) }
func (c *NvEncConfigH264) OutputPictureTimingSEI() bool          { return bitGet(c.flags, 5) }
func (c *NvEncConfigH264) SetOutputPictureTimingSEI(v bool)      { bitSet(&c.flags, 5, v) }
func (c *NvEncConfigH264) OutputAUD() bool                       { return bitGet(c.flags, 6) }
func (c *NvEncConfigH264) SetOutputAUD(v bool)                   { bitSet(&c.flags, 6, v) }
func (c *NvEncConfigH264) DisableSPSPPS() bool                   { return bitGet(c.flags, 7) }
func (c *NvEncConfigH264) SetDisableSPSPPS(v bool)               { bitSet(&c.flags, 7, v) }
func (c *NvEncConfigH264) OutputFramePackingSEI() bool           { return bitGet(c.flags, 8) }
func (c *NvEncConfigH264) SetOutputFramePackingSEI(v bool)       { bitSet(&c.flags, 8, v) }
func (c *NvEncConfigH264) OutputRecoveryPointSEI() bool          { return bitGet(c.flags, 9) }
func (c *NvEncConfigH264) SetOutputRecoveryPointSEI(v bool)      { bitSet(&c.flags, 9, v) }
func (c *NvEncConfigH264) EnableIntraRefresh() bool              { return bitGet(c.flags, 10) }
func (c *NvEncConfigH264) SetEnableIntraRefresh(v bool)          { bitSet(&c.flags, 10, v) }
func (c *NvEncConfigH264) EnableConstrainedEncoding() bool       { return bitGet(c.flags, 11) }
func (c *NvEncConfigH264) SetEnableConstrainedEncoding(v bool)   { bitSet(&c.flags, 11, v) }
func (c *NvEncConfigH264) RepeatSPSPPS() bool                    { return bitGet(c.flags, 12) }
func (c *NvEncConfigH264) SetRepeatSPSPPS(v bool)                { bitSet(&c.flags, 12, v) }
func (c *NvEncConfigH264) EnableVFR() bool                       { return bitGet(c.flags, 13) }
func (c *NvEncConfigH264) SetEnableVFR(v bool)                   { bitSet(&c.flags, 13, v) }
func (c *NvEncConfigH264) EnableLTR() bool                       { return bitGet(c.flags, 14) }
func (c *NvEncConfigH264) SetEnableLTR(v bool)                   { bitSet(&c.flags, 14, v) }
func (c *NvEncConfigH264) QpPrimeYZeroTransformBypassFlag() bool { return bitGet(c.flags, 15) }
func (c *NvEncConfigH264) SetQpPrimeYZeroTransformBypassFlag(v bool) {
	bitSet(&c.flags, 15, v)
}
func (c *NvEncConfigH264) UseConstrainedIntraPred() bool       { return bitGet(c.flags, 16) }
func (c *NvEncConfigH264) SetUseConstrainedIntraPred(v bool)   { bitSet(&c.flags, 16, v) }
func (c *NvEncConfigH264) EnableFillerDataInsertion() bool     { return bitGet(c.flags, 17) }
func (c *NvEncConfigH264) SetEnableFillerDataInsertion(v bool) { bitSet(&c.flags, 17, v) }
func (c *NvEncConfigH264) DisableSVCPrefixNalu() bool          { return bitGet(c.flags, 18) }
func (c *NvEncConfigH264) SetDisableSVCPrefixNalu(v bool)      { bitSet(&c.flags, 18, v) }
func (c *NvEncConfigH264) EnableScalabilityInfoSEI() bool      { return bitGet(c.flags, 19) }
func (c *NvEncConfigH264) SetEnableScalabilityInfoSEI(v bool)  { bitSet(&c.flags, 19, v) }
func (c *NvEncConfigH264) SingleSliceIntraRefresh() bool       { return bitGet(c.flags, 20) }
func (c *NvEncConfigH264) SetSingleSliceIntraRefresh(v bool)   { bitSet(&c.flags, 20, v) }
func (c *NvEncConfigH264) RawFlags() uint32                    { return c.flags }

type NvEncCodecConfig struct {
	raw NvEncConfigH264
}

func (u *NvEncCodecConfig) H264Config() *NvEncConfigH264 {
	return &u.raw
}

func (u *NvEncCodecConfig) Reserved() *[320]uint32 {
	return (*[320]uint32)(unsafe.Pointer(&u.raw))
}

type NvEncRCParams struct {
	Version                uint32
	RateControlMode        NvEncParamsRCMode
	ConstQP                NvEncQP
	AverageBitRate         uint32
	MaxBitRate             uint32
	VBVBufferSize          uint32
	VBVInitialDelay        uint32
	flags                  uint32
	MinQP                  NvEncQP
	MaxQP                  NvEncQP
	InitialRCQP            NvEncQP
	TemporalLayerIdxMask   uint32
	TemporalLayerQP        [8]uint8
	TargetQuality          uint8
	TargetQualityLSB       uint8
	LookaheadDepth         uint16
	LowDelayKeyFrameScale  uint8
	Reserved1              [3]uint8
	QPMapMode              NvEncQPMapMode
	MultiPass              NvEncMultiPass
	AlphaLayerBitrateRatio uint32
	CbQPIndexOffset        int8
	CrQPIndexOffset        int8
	Reserved2              uint16
	Reserved               [4]uint32
}

func (r *NvEncRCParams) EnableMinQP() bool           { return bitGet(r.flags, 0) }
func (r *NvEncRCParams) SetEnableMinQP(v bool)       { bitSet(&r.flags, 0, v) }
func (r *NvEncRCParams) EnableMaxQP() bool           { return bitGet(r.flags, 1) }
func (r *NvEncRCParams) SetEnableMaxQP(v bool)       { bitSet(&r.flags, 1, v) }
func (r *NvEncRCParams) EnableInitialRCQP() bool     { return bitGet(r.flags, 2) }
func (r *NvEncRCParams) SetEnableInitialRCQP(v bool) { bitSet(&r.flags, 2, v) }
func (r *NvEncRCParams) EnableAQ() bool              { return bitGet(r.flags, 3) }
func (r *NvEncRCParams) SetEnableAQ(v bool)          { bitSet(&r.flags, 3, v) }
func (r *NvEncRCParams) EnableLookahead() bool       { return bitGet(r.flags, 5) }
func (r *NvEncRCParams) SetEnableLookahead(v bool)   { bitSet(&r.flags, 5, v) }
func (r *NvEncRCParams) DisableIadapt() bool         { return bitGet(r.flags, 6) }
func (r *NvEncRCParams) SetDisableIadapt(v bool)     { bitSet(&r.flags, 6, v) }
func (r *NvEncRCParams) DisableBadapt() bool         { return bitGet(r.flags, 7) }
func (r *NvEncRCParams) SetDisableBadapt(v bool)     { bitSet(&r.flags, 7, v) }
func (r *NvEncRCParams) EnableTemporalAQ() bool      { return bitGet(r.flags, 8) }
func (r *NvEncRCParams) SetEnableTemporalAQ(v bool)  { bitSet(&r.flags, 8, v) }
func (r *NvEncRCParams) ZeroReorderDelay() bool      { return bitGet(r.flags, 9) }
func (r *NvEncRCParams) SetZeroReorderDelay(v bool)  { bitSet(&r.flags, 9, v) }
func (r *NvEncRCParams) EnableNonRefP() bool         { return bitGet(r.flags, 10) }
func (r *NvEncRCParams) SetEnableNonRefP(v bool)     { bitSet(&r.flags, 10, v) }
func (r *NvEncRCParams) StrictGOPTarget() bool       { return bitGet(r.flags, 11) }
func (r *NvEncRCParams) SetStrictGOPTarget(v bool)   { bitSet(&r.flags, 11, v) }
func (r *NvEncRCParams) AQStrength() uint32          { return fieldGet(r.flags, 12, 4) }
func (r *NvEncRCParams) SetAQStrength(v uint32)      { fieldSet(&r.flags, 12, 4, v) }
func (r *NvEncRCParams) RawFlags() uint32            { return r.flags }

type NvEncConfig struct {
	Version            uint32
	ProfileGUID        GUID
	GopLength          uint32
	FrameIntervalP     int32
	MonoChromeEncoding uint32
	FrameFieldMode     NvEncParamsFrameFieldMode
	MVPrecision        NvEncMVPrecision
	RCParams           NvEncRCParams
	EncodeCodecConfig  NvEncCodecConfig
	Reserved           [278]uint32
	Reserved2          [64]uintptr
}

type NvEncPresetConfig struct {
	Version   uint32
	PresetCfg NvEncConfig
	Reserved1 [255]uint32
	Reserved2 [64]uintptr
}

type NvEncInitializeParams struct {
	Version                 uint32
	EncodeGUID              GUID
	PresetGUID              GUID
	EncodeWidth             uint32
	EncodeHeight            uint32
	DarWidth                uint32
	DarHeight               uint32
	FrameRateNum            uint32
	FrameRateDen            uint32
	EnableEncodeAsync       uint32
	EnablePTD               uint32
	flags                   uint32
	PrivDataSize            uint32
	PrivData                uintptr
	EncodeConfig            uintptr
	MaxEncodeWidth          uint32
	MaxEncodeHeight         uint32
	MaxMEHintCountsPerBlock [2]NvEncExternalMEHintCountsPerBlockType
	TuningInfo              NvEncTuningInfo
	BufferFormat            NvEncBufferFormat
	Reserved                [287]uint32
	Reserved2               [64]uintptr
}

func (p *NvEncInitializeParams) ReportSliceOffsets() bool           { return bitGet(p.flags, 0) }
func (p *NvEncInitializeParams) SetReportSliceOffsets(v bool)       { bitSet(&p.flags, 0, v) }
func (p *NvEncInitializeParams) EnableSubFrameWrite() bool          { return bitGet(p.flags, 1) }
func (p *NvEncInitializeParams) SetEnableSubFrameWrite(v bool)      { bitSet(&p.flags, 1, v) }
func (p *NvEncInitializeParams) EnableExternalMEHints() bool        { return bitGet(p.flags, 2) }
func (p *NvEncInitializeParams) SetEnableExternalMEHints(v bool)    { bitSet(&p.flags, 2, v) }
func (p *NvEncInitializeParams) EnableMEOnlyMode() bool             { return bitGet(p.flags, 3) }
func (p *NvEncInitializeParams) SetEnableMEOnlyMode(v bool)         { bitSet(&p.flags, 3, v) }
func (p *NvEncInitializeParams) EnableWeightedPrediction() bool     { return bitGet(p.flags, 4) }
func (p *NvEncInitializeParams) SetEnableWeightedPrediction(v bool) { bitSet(&p.flags, 4, v) }
func (p *NvEncInitializeParams) EnableOutputInVidmem() bool         { return bitGet(p.flags, 5) }
func (p *NvEncInitializeParams) SetEnableOutputInVidmem(v bool)     { bitSet(&p.flags, 5, v) }
func (p *NvEncInitializeParams) RawFlags() uint32                   { return p.flags }

type NvEncSequenceParamPayload struct {
	Version              uint32
	InBufferSize         uint32
	SpsID                uint32
	PpsID                uint32
	SpsPpsBuffer         uintptr
	OutSPSPPSPayloadSize uintptr
	Reserved             [250]uint32
	Reserved2            [64]uintptr
}

type NvEncEventParams struct {
	Version         uint32
	Reserved        uint32
	CompletionEvent uintptr
	Reserved1       [253]uint32
	Reserved2       [64]uintptr
}

type NvEncOpenEncodeSessionExParams struct {
	Version    uint32
	DeviceType NvEncDeviceType
	Device     uintptr
	Reserved   uintptr
	APIVersion uint32
	Reserved1  [253]uint32
	Reserved2  [64]uintptr
}

type NvEncodeAPIFunctionList struct {
	Version                        uint32
	Reserved                       uint32
	NvEncOpenEncodeSession         uintptr
	NvEncGetEncodeGUIDCount        uintptr
	NvEncGetEncodeProfileGUIDCount uintptr
	NvEncGetEncodeProfileGUIDs     uintptr
	NvEncGetEncodeGUIDs            uintptr
	NvEncGetInputFormatCount       uintptr
	NvEncGetInputFormats           uintptr
	NvEncGetEncodeCaps             uintptr
	NvEncGetEncodePresetCount      uintptr
	NvEncGetEncodePresetGUIDs      uintptr
	NvEncGetEncodePresetConfig     uintptr
	NvEncInitializeEncoder         uintptr
	NvEncCreateInputBuffer         uintptr
	NvEncDestroyInputBuffer        uintptr
	NvEncCreateBitstreamBuffer     uintptr
	NvEncDestroyBitstreamBuffer    uintptr
	NvEncEncodePicture             uintptr
	NvEncLockBitstream             uintptr
	NvEncUnlockBitstream           uintptr
	NvEncLockInputBuffer           uintptr
	NvEncUnlockInputBuffer         uintptr
	NvEncGetEncodeStats            uintptr
	NvEncGetSequenceParams         uintptr
	NvEncRegisterAsyncEvent        uintptr
	NvEncUnregisterAsyncEvent      uintptr
	NvEncMapInputResource          uintptr
	NvEncUnmapInputResource        uintptr
	NvEncDestroyEncoder            uintptr
	NvEncInvalidateRefFrames       uintptr
	NvEncOpenEncodeSessionEx       uintptr
	NvEncRegisterResource          uintptr
	NvEncUnregisterResource        uintptr
	NvEncReconfigureEncoder        uintptr
	Reserved1                      uintptr
	NvEncCreateMVBuffer            uintptr
	NvEncDestroyMVBuffer           uintptr
	NvEncRunMotionEstimationOnly   uintptr
	NvEncGetLastErrorString        uintptr
	NvEncSetIOCudaStreams          uintptr
	NvEncGetEncodePresetConfigEx   uintptr
	NvEncGetSequenceParamEx        uintptr
	Reserved2                      [277]uintptr
}
