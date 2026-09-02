package vaapi

import (
	"fmt"
	"unsafe"
)

type Status int32

const (
	StatusSuccess                    Status = 0x00000000
	StatusErrorOperationFailed       Status = 0x00000001
	StatusErrorAllocationFailed      Status = 0x00000002
	StatusErrorInvalidDisplay        Status = 0x00000003
	StatusErrorInvalidConfig         Status = 0x00000004
	StatusErrorInvalidContext        Status = 0x00000005
	StatusErrorInvalidSurface        Status = 0x00000006
	StatusErrorInvalidBuffer         Status = 0x00000007
	StatusErrorInvalidImage          Status = 0x00000008
	StatusErrorInvalidSubpicture     Status = 0x00000009
	StatusErrorAttrNotSupported      Status = 0x0000000a
	StatusErrorMaxNumExceeded        Status = 0x0000000b
	StatusErrorUnsupportedProfile    Status = 0x0000000c
	StatusErrorUnsupportedEntrypoint Status = 0x0000000d
	StatusErrorUnsupportedRTFormat   Status = 0x0000000e
	StatusErrorUnsupportedBufferType Status = 0x0000000f
	StatusErrorSurfaceBusy           Status = 0x00000010
	StatusErrorFlagNotSupported      Status = 0x00000011
	StatusErrorInvalidParameter      Status = 0x00000012
	StatusErrorResolutionNotSupp     Status = 0x00000013
	StatusErrorUnimplemented         Status = 0x00000014
	StatusErrorSurfaceInDisplaying   Status = 0x00000015
	StatusErrorInvalidImageFormat    Status = 0x00000016
	StatusErrorDecodingError         Status = 0x00000017
	StatusErrorEncodingError         Status = 0x00000018
	StatusErrorInvalidValue          Status = 0x00000019
	StatusErrorUnsupportedFilter     Status = 0x00000020
	StatusErrorInvalidFilterChain    Status = 0x00000021
	StatusErrorHWBusy                Status = 0x00000022
	StatusErrorUnsupportedMemType    Status = 0x00000024
	StatusErrorNotEnoughBuffer       Status = 0x00000025
	StatusErrorTimedout              Status = 0x00000026
	StatusErrorUnknown               Status = -1
)

var statusNames = map[Status]string{
	StatusSuccess:                    "VA_STATUS_SUCCESS",
	StatusErrorOperationFailed:       "VA_STATUS_ERROR_OPERATION_FAILED",
	StatusErrorAllocationFailed:      "VA_STATUS_ERROR_ALLOCATION_FAILED",
	StatusErrorInvalidDisplay:        "VA_STATUS_ERROR_INVALID_DISPLAY",
	StatusErrorInvalidConfig:         "VA_STATUS_ERROR_INVALID_CONFIG",
	StatusErrorInvalidContext:        "VA_STATUS_ERROR_INVALID_CONTEXT",
	StatusErrorInvalidSurface:        "VA_STATUS_ERROR_INVALID_SURFACE",
	StatusErrorInvalidBuffer:         "VA_STATUS_ERROR_INVALID_BUFFER",
	StatusErrorInvalidImage:          "VA_STATUS_ERROR_INVALID_IMAGE",
	StatusErrorInvalidSubpicture:     "VA_STATUS_ERROR_INVALID_SUBPICTURE",
	StatusErrorAttrNotSupported:      "VA_STATUS_ERROR_ATTR_NOT_SUPPORTED",
	StatusErrorMaxNumExceeded:        "VA_STATUS_ERROR_MAX_NUM_EXCEEDED",
	StatusErrorUnsupportedProfile:    "VA_STATUS_ERROR_UNSUPPORTED_PROFILE",
	StatusErrorUnsupportedEntrypoint: "VA_STATUS_ERROR_UNSUPPORTED_ENTRYPOINT",
	StatusErrorUnsupportedRTFormat:   "VA_STATUS_ERROR_UNSUPPORTED_RT_FORMAT",
	StatusErrorUnsupportedBufferType: "VA_STATUS_ERROR_UNSUPPORTED_BUFFERTYPE",
	StatusErrorSurfaceBusy:           "VA_STATUS_ERROR_SURFACE_BUSY",
	StatusErrorFlagNotSupported:      "VA_STATUS_ERROR_FLAG_NOT_SUPPORTED",
	StatusErrorInvalidParameter:      "VA_STATUS_ERROR_INVALID_PARAMETER",
	StatusErrorResolutionNotSupp:     "VA_STATUS_ERROR_RESOLUTION_NOT_SUPPORTED",
	StatusErrorUnimplemented:         "VA_STATUS_ERROR_UNIMPLEMENTED",
	StatusErrorSurfaceInDisplaying:   "VA_STATUS_ERROR_SURFACE_IN_DISPLAYING",
	StatusErrorInvalidImageFormat:    "VA_STATUS_ERROR_INVALID_IMAGE_FORMAT",
	StatusErrorDecodingError:         "VA_STATUS_ERROR_DECODING_ERROR",
	StatusErrorEncodingError:         "VA_STATUS_ERROR_ENCODING_ERROR",
	StatusErrorInvalidValue:          "VA_STATUS_ERROR_INVALID_VALUE",
	StatusErrorUnsupportedFilter:     "VA_STATUS_ERROR_UNSUPPORTED_FILTER",
	StatusErrorInvalidFilterChain:    "VA_STATUS_ERROR_INVALID_FILTER_CHAIN",
	StatusErrorHWBusy:                "VA_STATUS_ERROR_HW_BUSY",
	StatusErrorUnsupportedMemType:    "VA_STATUS_ERROR_UNSUPPORTED_MEMORY_TYPE",
	StatusErrorNotEnoughBuffer:       "VA_STATUS_ERROR_NOT_ENOUGH_BUFFER",
	StatusErrorTimedout:              "VA_STATUS_ERROR_TIMEDOUT",
	StatusErrorUnknown:               "VA_STATUS_ERROR_UNKNOWN",
}

func (s Status) Error() string {
	if name, ok := statusNames[s]; ok {
		return fmt.Sprintf("vaapi: %s (status 0x%08x)", name, uint32(s))
	}
	return fmt.Sprintf("vaapi: status 0x%08x", uint32(s))
}

const InvalidID uint32 = 0xffffffff

const Progressive int32 = 0x1

type Profile int32

const (
	ProfileH264Main                Profile = 6
	ProfileH264High                Profile = 7
	ProfileH264ConstrainedBaseline Profile = 13
)

type Entrypoint int32

const EntrypointEncSlice Entrypoint = 6

type ConfigAttribType int32

const (
	ConfigAttribRTFormat    ConfigAttribType = 0
	ConfigAttribRateControl ConfigAttribType = 5
)

const RTFormatYUV420 uint32 = 0x00000001

const (
	RCNone           uint32 = 0x00000001
	RCCBR            uint32 = 0x00000002
	RCVBR            uint32 = 0x00000004
	RCCQP            uint32 = 0x00000010
	RCVBRConstrained uint32 = 0x00000020
)

const FourCCNV12 uint32 = 0x3231564e

type BufferType int32

const (
	BufferTypeEncCoded             BufferType = 21
	BufferTypeEncSequenceParameter BufferType = 22
	BufferTypeEncPictureParameter  BufferType = 23
	BufferTypeEncSliceParameter    BufferType = 24
)

type GenericValueType int32

const GenericValueTypeInteger GenericValueType = 1

type SurfaceAttribType int32

const SurfaceAttribPixelFormat SurfaceAttribType = 1

const SurfaceAttribSettable uint32 = 0x00000002

const (
	PictureH264Invalid            uint32 = 0x00000001
	PictureH264ShortTermReference uint32 = 0x00000008
)

type ConfigAttrib struct {
	Type  ConfigAttribType
	Value uint32
}

type GenericValue struct {
	Type  GenericValueType
	value uint64
}

func (v *GenericValue) SetInt(i int32) {
	v.Type = GenericValueTypeInteger
	v.value = uint64(uint32(i))
}

type SurfaceAttrib struct {
	Type  SurfaceAttribType
	Flags uint32
	Value GenericValue
}

type PictureH264 struct {
	PictureID           uint32
	FrameIdx            uint32
	Flags               uint32
	TopFieldOrderCnt    int32
	BottomFieldOrderCnt int32
	vaReserved          [4]uint32
}

func invalidPictureH264() PictureH264 {
	return PictureH264{PictureID: InvalidID, Flags: PictureH264Invalid}
}

type EncSequenceParameterBufferH264 struct {
	SeqParameterSetID              uint8
	LevelIDC                       uint8
	IntraPeriod                    uint32
	IntraIDRPeriod                 uint32
	IPPeriod                       uint32
	BitsPerSecond                  uint32
	MaxNumRefFrames                uint32
	PictureWidthInMbs              uint16
	PictureHeightInMbs             uint16
	seqFields                      uint32
	BitDepthLumaMinus8             uint8
	BitDepthChromaMinus8           uint8
	NumRefFramesInPicOrderCntCycle uint8
	OffsetForNonRefPic             int32
	OffsetForTopToBottomField      int32
	OffsetForRefFrame              [256]int32
	FrameCroppingFlag              uint8
	FrameCropLeftOffset            uint32
	FrameCropRightOffset           uint32
	FrameCropTopOffset             uint32
	FrameCropBottomOffset          uint32
	VUIParametersPresentFlag       uint8
	vuiFields                      uint32
	AspectRatioIDC                 uint8
	SarWidth                       uint32
	SarHeight                      uint32
	NumUnitsInTick                 uint32
	TimeScale                      uint32
	vaReserved                     [4]uint32
}

func (s *EncSequenceParameterBufferH264) SetChromaFormatIDC(v uint32) {
	fieldSet(&s.seqFields, 0, 2, v)
}
func (s *EncSequenceParameterBufferH264) SetFrameMbsOnlyFlag(v bool) { bitSet(&s.seqFields, 2, v) }
func (s *EncSequenceParameterBufferH264) SetMbAdaptiveFrameFieldFlag(v bool) {
	bitSet(&s.seqFields, 3, v)
}
func (s *EncSequenceParameterBufferH264) SetSeqScalingMatrixPresentFlag(v bool) {
	bitSet(&s.seqFields, 4, v)
}
func (s *EncSequenceParameterBufferH264) SetDirect8x8InferenceFlag(v bool) {
	bitSet(&s.seqFields, 5, v)
}
func (s *EncSequenceParameterBufferH264) SetLog2MaxFrameNumMinus4(v uint32) {
	fieldSet(&s.seqFields, 6, 4, v)
}
func (s *EncSequenceParameterBufferH264) SetPicOrderCntType(v uint32) {
	fieldSet(&s.seqFields, 10, 2, v)
}
func (s *EncSequenceParameterBufferH264) SetLog2MaxPicOrderCntLsbMinus4(v uint32) {
	fieldSet(&s.seqFields, 12, 4, v)
}
func (s *EncSequenceParameterBufferH264) SetDeltaPicOrderAlwaysZeroFlag(v bool) {
	bitSet(&s.seqFields, 16, v)
}
func (s *EncSequenceParameterBufferH264) RawSeqFields() uint32 { return s.seqFields }

type EncPictureParameterBufferH264 struct {
	CurrPic                   PictureH264
	ReferenceFrames           [16]PictureH264
	CodedBuf                  uint32
	PicParameterSetID         uint8
	SeqParameterSetID         uint8
	LastPicture               uint8
	FrameNum                  uint16
	PicInitQP                 uint8
	NumRefIdxL0ActiveMinus1   uint8
	NumRefIdxL1ActiveMinus1   uint8
	ChromaQPIndexOffset       int8
	SecondChromaQPIndexOffset int8
	picFields                 uint32
	vaReserved                [4]uint32
}

func (p *EncPictureParameterBufferH264) SetIdrPicFlag(v bool) { bitSet(&p.picFields, 0, v) }
func (p *EncPictureParameterBufferH264) SetReferencePicFlag(v uint32) {
	fieldSet(&p.picFields, 1, 2, v)
}
func (p *EncPictureParameterBufferH264) SetEntropyCodingModeFlag(v bool) {
	bitSet(&p.picFields, 3, v)
}
func (p *EncPictureParameterBufferH264) SetWeightedPredFlag(v bool) { bitSet(&p.picFields, 4, v) }
func (p *EncPictureParameterBufferH264) SetWeightedBipredIDC(v uint32) {
	fieldSet(&p.picFields, 5, 2, v)
}
func (p *EncPictureParameterBufferH264) SetConstrainedIntraPredFlag(v bool) {
	bitSet(&p.picFields, 7, v)
}
func (p *EncPictureParameterBufferH264) SetTransform8x8ModeFlag(v bool) {
	bitSet(&p.picFields, 8, v)
}
func (p *EncPictureParameterBufferH264) SetDeblockingFilterControlPresentFlag(v bool) {
	bitSet(&p.picFields, 9, v)
}
func (p *EncPictureParameterBufferH264) SetRedundantPicCntPresentFlag(v bool) {
	bitSet(&p.picFields, 10, v)
}
func (p *EncPictureParameterBufferH264) SetPicOrderPresentFlag(v bool) {
	bitSet(&p.picFields, 11, v)
}
func (p *EncPictureParameterBufferH264) SetPicScalingMatrixPresentFlag(v bool) {
	bitSet(&p.picFields, 12, v)
}
func (p *EncPictureParameterBufferH264) RawPicFields() uint32 { return p.picFields }

type EncSliceParameterBufferH264 struct {
	MacroblockAddress           uint32
	NumMacroblocks              uint32
	MacroblockInfo              uint32
	SliceType                   uint8
	PicParameterSetID           uint8
	IdrPicID                    uint16
	PicOrderCntLsb              uint16
	DeltaPicOrderCntBottom      int32
	DeltaPicOrderCnt            [2]int32
	DirectSpatialMvPredFlag     uint8
	NumRefIdxActiveOverrideFlag uint8
	NumRefIdxL0ActiveMinus1     uint8
	NumRefIdxL1ActiveMinus1     uint8
	RefPicList0                 [32]PictureH264
	RefPicList1                 [32]PictureH264
	LumaLog2WeightDenom         uint8
	ChromaLog2WeightDenom       uint8
	LumaWeightL0Flag            uint8
	LumaWeightL0                [32]int16
	LumaOffsetL0                [32]int16
	ChromaWeightL0Flag          uint8
	ChromaWeightL0              [32][2]int16
	ChromaOffsetL0              [32][2]int16
	LumaWeightL1Flag            uint8
	LumaWeightL1                [32]int16
	LumaOffsetL1                [32]int16
	ChromaWeightL1Flag          uint8
	ChromaWeightL1              [32][2]int16
	ChromaOffsetL1              [32][2]int16
	CabacInitIDC                uint8
	SliceQPDelta                int8
	DisableDeblockingFilterIDC  uint8
	SliceAlphaC0OffsetDiv2      int8
	SliceBetaOffsetDiv2         int8
	vaReserved                  [4]uint32
}

type CodedBufferSegment struct {
	Size       uint32
	BitOffset  uint32
	Status     uint32
	Reserved   uint32
	Buf        unsafe.Pointer
	Next       unsafe.Pointer
	vaReserved [4]uint32
}

type ImageFormat struct {
	FourCC       uint32
	ByteOrder    uint32
	BitsPerPixel uint32
	Depth        uint32
	RedMask      uint32
	GreenMask    uint32
	BlueMask     uint32
	AlphaMask    uint32
	vaReserved   [4]uint32
}

type Image struct {
	ImageID           uint32
	Format            ImageFormat
	Buf               uint32
	Width             uint16
	Height            uint16
	DataSize          uint32
	NumPlanes         uint32
	Pitches           [3]uint32
	Offsets           [3]uint32
	NumPaletteEntries int32
	EntryBytes        int32
	ComponentOrder    [4]int8
	vaReserved        [4]uint32
}

func bitSet(word *uint32, shift uint, v bool) {
	if v {
		*word |= 1 << shift
	} else {
		*word &^= 1 << shift
	}
}

func fieldSet(word *uint32, shift, width uint, v uint32) {
	mask := uint32(1)<<width - 1
	*word = (*word &^ (mask << shift)) | ((v & mask) << shift)
}
