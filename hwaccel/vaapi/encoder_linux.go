package vaapi

import (
	"errors"
	"fmt"
	"unsafe"

	"github.com/oops1/go.264/internal/level"
	"github.com/oops1/go.264/internal/syntax"
)

const (
	maxPicOrderCntLsb        = 1 << 12
	log2MaxPicOrderCntLsbM4  = 8
	maxFrameNum              = 1 << 12
	log2MaxFrameNumM4        = 8
	codedBufferHeadroomBytes = 1 << 16
	maxNumRefFrames          = 1
)

type Config struct {
	Width     int
	Height    int
	FPSNum    int
	FPSDen    int
	GOPLength int
	QP        int
}

func (c Config) valid() error {
	switch {
	case c.Width <= 0 || c.Height <= 0 || c.Width%2 != 0 || c.Height%2 != 0:
		return errors.New("vaapi: the picture must have even positive dimensions")
	case c.FPSNum <= 0 || c.FPSDen <= 0:
		return errors.New("vaapi: the frame rate must be positive")
	}
	return nil
}

func mbAlign(v int) int { return (v + 15) &^ 15 }

type Encoder struct {
	disp     *display
	profile  Profile
	baseline bool
	levelIDC uint8

	config  uint32
	context uint32

	width, height     int
	alignedWidth      int
	alignedHeight     int
	mbWidth, mbHeight int
	fpsNum, fpsDen    int

	srcSurface  uint32
	refSurfaces [2]uint32

	gopLength int
	qp        uint8

	gopPos      int
	frameNum    uint32
	idrPicID    uint16
	havePrevRef bool
	prevRef     PictureH264
	nextSlot    int

	closed bool
}

func (e *Encoder) Name() string {
	if e.baseline {
		return "vaapi-constrained-baseline"
	}
	return "vaapi-main"
}

func Open(cfg Config) (*Encoder, error) {
	if err := cfg.valid(); err != nil {
		return nil, err
	}
	if err := loadLibrary(); err != nil {
		return nil, err
	}
	disp, err := openDisplay()
	if err != nil {
		return nil, err
	}
	e, err := openHere(disp, cfg)
	if err != nil {
		disp.close()
		return nil, err
	}
	return e, nil
}

func openHere(disp *display, cfg Config) (*Encoder, error) {
	choice, err := disp.findEncodeProfile()
	if err != nil {
		return nil, err
	}
	levelIDC, err := pickLevelIDC(cfg, choice.profile)
	if err != nil {
		return nil, err
	}
	vaConfig, err := disp.createConfig(choice.profile)
	if err != nil {
		return nil, err
	}

	alignedW, alignedH := mbAlign(cfg.Width), mbAlign(cfg.Height)
	e := &Encoder{
		disp:          disp,
		profile:       choice.profile,
		baseline:      choice.baseline,
		levelIDC:      levelIDC,
		config:        vaConfig,
		width:         cfg.Width,
		height:        cfg.Height,
		alignedWidth:  alignedW,
		alignedHeight: alignedH,
		mbWidth:       alignedW / 16,
		mbHeight:      alignedH / 16,
		fpsNum:        cfg.FPSNum,
		fpsDen:        cfg.FPSDen,
		gopLength:     cfg.GOPLength,
		qp:            clampQP(cfg.QP),
	}
	if e.gopLength <= 0 {
		e.gopLength = 1
	}

	if err := e.createSurfacesAndContext(); err != nil {
		e.releaseHere()
		return nil, err
	}
	return e, nil
}

func roughBitsPerSecond(width, height, fpsNum, fpsDen int) uint32 {
	num, den := fpsNum, fpsDen
	if num <= 0 || den <= 0 {
		num, den = 30, 1
	}
	rate := width * height * num / den / 4
	if rate < 100000 {
		rate = 100000
	}
	return uint32(rate)
}

func profileIDC(p Profile) uint8 {
	switch p {
	case ProfileH264High:
		return syntax.ProfileHigh
	case ProfileH264Main:
		return syntax.ProfileMain
	}
	return syntax.ProfileBaseline
}

func pickLevelIDC(cfg Config, profile Profile) (uint8, error) {
	peak := roughBitsPerSecond(cfg.Width, cfg.Height, cfg.FPSNum, cfg.FPSDen)
	idc, err := level.Select(level.Stream{
		Width:      cfg.Width,
		Height:     cfg.Height,
		FPSNum:     cfg.FPSNum,
		FPSDen:     cfg.FPSDen,
		RefFrames:  maxNumRefFrames,
		PeakKbps:   int((peak + 999) / 1000),
		ProfileIDC: profileIDC(profile),
	})
	if err != nil {
		return 0, fmt.Errorf("vaapi: %w", err)
	}
	return idc, nil
}

func clampQP(qp int) uint8 {
	if qp <= 0 {
		return 26
	}
	if qp > 51 {
		return 51
	}
	return uint8(qp)
}

func (e *Encoder) createSurfacesAndContext() error {
	src, err := e.disp.createNV12Surfaces(e.alignedWidth, e.alignedHeight, 1)
	if err != nil {
		return fmt.Errorf("vaapi: source surface: %w", err)
	}
	e.srcSurface = src[0]

	ref, err := e.disp.createNV12Surfaces(e.alignedWidth, e.alignedHeight, 2)
	if err != nil {
		return fmt.Errorf("vaapi: reference surfaces: %w", err)
	}
	e.refSurfaces[0], e.refSurfaces[1] = ref[0], ref[1]

	all := []uint32{e.srcSurface, e.refSurfaces[0], e.refSurfaces[1]}
	var ctx uint32
	err = check("vaCreateContext", vaCreateContext(e.disp.handle, e.config, int32(e.alignedWidth), int32(e.alignedHeight),
		Progressive, unsafe.Pointer(&all[0]), int32(len(all)), &ctx))
	if err != nil {
		return err
	}
	e.context = ctx
	return nil
}

func (d *display) createNV12Surfaces(width, height, count int) ([]uint32, error) {
	var attrib SurfaceAttrib
	attrib.Type = SurfaceAttribPixelFormat
	attrib.Flags = SurfaceAttribSettable
	attrib.Value.SetInt(int32(FourCCNV12))

	ids := make([]uint32, count)
	err := check("vaCreateSurfaces", vaCreateSurfaces(d.handle, RTFormatYUV420, uint32(width), uint32(height),
		unsafe.Pointer(&ids[0]), uint32(count), unsafe.Pointer(&attrib), 1))
	if err != nil {
		return nil, err
	}
	return ids, nil
}

func (e *Encoder) Encode(i420 []byte) (out []byte, err error) {
	if e.closed {
		return nil, errors.New("vaapi: encode on a closed encoder")
	}
	if len(i420) < I420Size(e.width, e.height) {
		return nil, errors.New("vaapi: the frame is shorter than the configured picture")
	}
	return e.encodeHere(i420)
}

func (e *Encoder) encodeHere(i420 []byte) ([]byte, error) {
	isIDR := e.gopPos%e.gopLength == 0

	if err := e.uploadFrame(i420); err != nil {
		return nil, err
	}

	currSlot := e.nextSlot
	e.nextSlot = (e.nextSlot + 1) % 2

	codedBuf, err := e.createCodedBuffer()
	if err != nil {
		return nil, err
	}

	if err := check("vaBeginPicture", vaBeginPicture(e.disp.handle, e.context, e.srcSurface)); err != nil {
		vaDestroyBuffer(e.disp.handle, codedBuf)
		return nil, err
	}

	if isIDR {
		if err := e.renderSequence(); err != nil {
			vaDestroyBuffer(e.disp.handle, codedBuf)
			return nil, err
		}
	}

	currPic := PictureH264{
		PictureID:           e.refSurfaces[currSlot],
		FrameIdx:            e.frameNum,
		TopFieldOrderCnt:    int32(e.gopPos % maxPicOrderCntLsb),
		BottomFieldOrderCnt: int32(e.gopPos % maxPicOrderCntLsb),
	}

	if err := e.renderPicture(codedBuf, currPic, isIDR); err != nil {
		vaDestroyBuffer(e.disp.handle, codedBuf)
		return nil, err
	}
	if err := e.renderSlice(isIDR); err != nil {
		vaDestroyBuffer(e.disp.handle, codedBuf)
		return nil, err
	}

	if err := check("vaEndPicture", vaEndPicture(e.disp.handle, e.context)); err != nil {
		vaDestroyBuffer(e.disp.handle, codedBuf)
		return nil, err
	}
	if err := check("vaSyncSurface", vaSyncSurface(e.disp.handle, e.srcSurface)); err != nil {
		vaDestroyBuffer(e.disp.handle, codedBuf)
		return nil, err
	}

	out, err := e.readCodedBuffer(codedBuf)
	vaDestroyBuffer(e.disp.handle, codedBuf)
	if err != nil {
		return nil, err
	}

	currPic.Flags = PictureH264ShortTermReference
	e.prevRef = currPic
	e.havePrevRef = true
	if isIDR {
		e.frameNum = 0
		e.idrPicID++
	} else {
		e.frameNum = (e.frameNum + 1) % maxFrameNum
	}
	e.gopPos++

	return out, nil
}

func (e *Encoder) uploadFrame(i420 []byte) error {
	var image Image
	if err := check("vaDeriveImage", vaDeriveImage(e.disp.handle, e.srcSurface, unsafe.Pointer(&image))); err != nil {
		return e.uploadFrameViaPutImage(i420)
	}
	defer vaDestroyImage(e.disp.handle, image.ImageID)

	if image.Format.FourCC != FourCCNV12 {
		return fmt.Errorf("vaapi: the surface derived as fourcc 0x%08x, want NV12", image.Format.FourCC)
	}

	var mapped unsafe.Pointer
	if err := check("vaMapBuffer", vaMapBuffer(e.disp.handle, image.Buf, &mapped)); err != nil {
		return err
	}
	strideY := int(image.Pitches[0])
	strideUV := int(image.Pitches[1])
	ySize := strideY * int(image.Height)
	uvSize := strideUV * (int(image.Height) / 2)
	dstY := unsafe.Slice((*byte)(unsafe.Add(mapped, image.Offsets[0])), ySize)
	dstUV := unsafe.Slice((*byte)(unsafe.Add(mapped, image.Offsets[1])), uvSize)
	I420ToNV12(dstY, strideY, dstUV, strideUV, i420, e.width, e.height)
	return check("vaUnmapBuffer", vaUnmapBuffer(e.disp.handle, image.Buf))
}

func (e *Encoder) uploadFrameViaPutImage(i420 []byte) error {
	format, err := e.disp.findNV12ImageFormat()
	if err != nil {
		return err
	}
	var image Image
	if err := check("vaCreateImage", vaCreateImage(e.disp.handle, unsafe.Pointer(format), int32(e.width), int32(e.height), unsafe.Pointer(&image))); err != nil {
		return err
	}
	defer vaDestroyImage(e.disp.handle, image.ImageID)

	var mapped unsafe.Pointer
	if err := check("vaMapBuffer", vaMapBuffer(e.disp.handle, image.Buf, &mapped)); err != nil {
		return err
	}
	strideY := int(image.Pitches[0])
	strideUV := int(image.Pitches[1])
	ySize := strideY * int(image.Height)
	uvSize := strideUV * (int(image.Height) / 2)
	dstY := unsafe.Slice((*byte)(unsafe.Add(mapped, image.Offsets[0])), ySize)
	dstUV := unsafe.Slice((*byte)(unsafe.Add(mapped, image.Offsets[1])), uvSize)
	I420ToNV12(dstY, strideY, dstUV, strideUV, i420, e.width, e.height)
	if err := check("vaUnmapBuffer", vaUnmapBuffer(e.disp.handle, image.Buf)); err != nil {
		return err
	}
	return check("vaPutImage", vaPutImage(e.disp.handle, e.srcSurface, image.ImageID,
		0, 0, uint32(e.width), uint32(e.height), 0, 0, uint32(e.width), uint32(e.height)))
}

func (d *display) findNV12ImageFormat() (*ImageFormat, error) {
	max := vaMaxNumImageFormats(d.handle)
	if max <= 0 {
		max = 64
	}
	formats := make([]ImageFormat, max)
	n := int32(len(formats))
	if err := check("vaQueryImageFormats", vaQueryImageFormats(d.handle, unsafe.Pointer(&formats[0]), &n)); err != nil {
		return nil, err
	}
	for i := range formats[:n] {
		if formats[i].FourCC == FourCCNV12 {
			f := formats[i]
			return &f, nil
		}
	}
	return nil, errors.New("vaapi: the driver does not advertise an NV12 image format")
}

func (e *Encoder) createCodedBuffer() (uint32, error) {
	size := uint32(e.mbWidth*e.mbHeight*400 + codedBufferHeadroomBytes)
	var buf uint32
	err := check("vaCreateBuffer", vaCreateBuffer(e.disp.handle, e.context, int32(BufferTypeEncCoded),
		size, 1, nil, &buf))
	if err != nil {
		return 0, err
	}
	return buf, nil
}

func (e *Encoder) renderSequence() error {
	var seq EncSequenceParameterBufferH264
	seq.LevelIDC = e.levelIDC
	seq.PictureWidthInMbs = uint16(e.mbWidth)
	seq.PictureHeightInMbs = uint16(e.mbHeight)
	seq.MaxNumRefFrames = maxNumRefFrames
	seq.IntraPeriod = uint32(e.gopLength)
	seq.IntraIDRPeriod = uint32(e.gopLength)
	seq.IPPeriod = 1
	seq.BitsPerSecond = roughBitsPerSecond(e.width, e.height, e.fpsNum, e.fpsDen)
	seq.SetChromaFormatIDC(1)
	seq.SetFrameMbsOnlyFlag(true)
	seq.SetDirect8x8InferenceFlag(true)
	seq.SetLog2MaxFrameNumMinus4(log2MaxFrameNumM4)
	seq.SetPicOrderCntType(0)
	seq.SetLog2MaxPicOrderCntLsbMinus4(log2MaxPicOrderCntLsbM4)
	if e.alignedWidth != e.width || e.alignedHeight != e.height {
		seq.FrameCroppingFlag = 1
		seq.FrameCropRightOffset = uint32(e.alignedWidth-e.width) / 2
		seq.FrameCropBottomOffset = uint32(e.alignedHeight-e.height) / 2
	}

	var buf uint32
	if err := check("vaCreateBuffer(seq)", vaCreateBuffer(e.disp.handle, e.context, int32(BufferTypeEncSequenceParameter),
		uint32(unsafe.Sizeof(seq)), 1, unsafe.Pointer(&seq), &buf)); err != nil {
		return err
	}
	return check("vaRenderPicture(seq)", vaRenderPicture(e.disp.handle, e.context, unsafe.Pointer(&buf), 1))
}

func (e *Encoder) renderPicture(codedBuf uint32, currPic PictureH264, isIDR bool) error {
	var pic EncPictureParameterBufferH264
	pic.CurrPic = currPic
	for i := range pic.ReferenceFrames {
		pic.ReferenceFrames[i] = invalidPictureH264()
	}
	if !isIDR && e.havePrevRef {
		pic.ReferenceFrames[0] = e.prevRef
	}
	pic.CodedBuf = codedBuf
	pic.FrameNum = uint16(e.frameNum)
	pic.PicInitQP = e.qp
	pic.SetIdrPicFlag(isIDR)
	pic.SetReferencePicFlag(1)
	pic.SetEntropyCodingModeFlag(!e.baseline)
	pic.SetDeblockingFilterControlPresentFlag(true)

	var buf uint32
	if err := check("vaCreateBuffer(pic)", vaCreateBuffer(e.disp.handle, e.context, int32(BufferTypeEncPictureParameter),
		uint32(unsafe.Sizeof(pic)), 1, unsafe.Pointer(&pic), &buf)); err != nil {
		return err
	}
	return check("vaRenderPicture(pic)", vaRenderPicture(e.disp.handle, e.context, unsafe.Pointer(&buf), 1))
}

func (e *Encoder) renderSlice(isIDR bool) error {
	var slice EncSliceParameterBufferH264
	slice.NumMacroblocks = uint32(e.mbWidth * e.mbHeight)
	for i := range slice.RefPicList0 {
		slice.RefPicList0[i] = invalidPictureH264()
	}
	for i := range slice.RefPicList1 {
		slice.RefPicList1[i] = invalidPictureH264()
	}
	if isIDR {
		slice.SliceType = 2
		slice.IdrPicID = e.idrPicID
	} else {
		slice.SliceType = 0
		if e.havePrevRef {
			slice.RefPicList0[0] = e.prevRef
		}
	}
	slice.PicOrderCntLsb = uint16(e.gopPos % maxPicOrderCntLsb)

	var buf uint32
	if err := check("vaCreateBuffer(slice)", vaCreateBuffer(e.disp.handle, e.context, int32(BufferTypeEncSliceParameter),
		uint32(unsafe.Sizeof(slice)), 1, unsafe.Pointer(&slice), &buf)); err != nil {
		return err
	}
	return check("vaRenderPicture(slice)", vaRenderPicture(e.disp.handle, e.context, unsafe.Pointer(&buf), 1))
}

func (e *Encoder) readCodedBuffer(buf uint32) ([]byte, error) {
	var mapped unsafe.Pointer
	if err := check("vaMapBuffer(coded)", vaMapBuffer(e.disp.handle, buf, &mapped)); err != nil {
		return nil, err
	}
	var out []byte
	for seg := (*CodedBufferSegment)(mapped); seg != nil; {
		if seg.Size > 0 && seg.Buf != nil {
			out = append(out, unsafe.Slice((*byte)(seg.Buf), int(seg.Size))...)
		}
		if seg.Next == nil {
			seg = nil
		} else {
			seg = (*CodedBufferSegment)(seg.Next)
		}
	}
	if err := check("vaUnmapBuffer(coded)", vaUnmapBuffer(e.disp.handle, buf)); err != nil {
		return out, err
	}
	return out, nil
}

func (e *Encoder) Drain() ([]byte, error) {
	return nil, nil
}

func (e *Encoder) releaseHere() {
	if e.disp == nil {
		return
	}
	h := e.disp.handle
	if e.context != 0 {
		vaDestroyContext(h, e.context)
		e.context = 0
	}
	surfaces := []uint32{}
	if e.srcSurface != 0 {
		surfaces = append(surfaces, e.srcSurface)
	}
	for _, s := range e.refSurfaces {
		if s != 0 {
			surfaces = append(surfaces, s)
		}
	}
	if len(surfaces) > 0 {
		vaDestroySurfaces(h, unsafe.Pointer(&surfaces[0]), int32(len(surfaces)))
	}
	e.srcSurface = 0
	e.refSurfaces = [2]uint32{}
	if e.config != 0 {
		vaDestroyConfig(h, e.config)
		e.config = 0
	}
}

func (e *Encoder) Close() error {
	if e.closed {
		return nil
	}
	e.closed = true
	e.releaseHere()
	e.disp.close()
	e.disp = nil
	return nil
}
