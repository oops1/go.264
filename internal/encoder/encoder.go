package encoder

import (
	"errors"
	"fmt"

	"github.com/oops1/go264/internal/bits"
	"github.com/oops1/go264/internal/frame"
	"github.com/oops1/go264/internal/loopfilter"
	"github.com/oops1/go264/internal/nal"
	"github.com/oops1/go264/internal/syntax"
)

var (
	ErrConfig    = errors.New("go264/encoder: invalid configuration")
	ErrFrameSize = errors.New("go264/encoder: frame does not match the configured size")
)

type Config struct {
	Width   int
	Height  int
	FPSNum  int
	FPSDen  int
	GOPSize int
	QP      int
}

func (c *Config) validate() error {
	if c.Width <= 0 || c.Height <= 0 {
		return fmt.Errorf("%w: size %dx%d", ErrConfig, c.Width, c.Height)
	}
	if c.Width%2 != 0 || c.Height%2 != 0 {
		return fmt.Errorf("%w: 4:2:0 requires even dimensions, got %dx%d", ErrConfig, c.Width, c.Height)
	}
	if c.Width > 16384 || c.Height > 16384 {
		return fmt.Errorf("%w: size %dx%d exceeds the supported maximum", ErrConfig, c.Width, c.Height)
	}
	if c.QP < 0 || c.QP > 51 {
		return fmt.Errorf("%w: QP %d outside 0..51", ErrConfig, c.QP)
	}
	if c.GOPSize <= 0 {
		c.GOPSize = 1
	}
	if c.FPSNum <= 0 || c.FPSDen <= 0 {
		c.FPSNum = 25
		c.FPSDen = 1
	}
	return nil
}

type Encoder struct {
	cfg Config
	sps *syntax.SPS
	pps *syntax.PPS

	widthMBs  int
	heightMBs int

	src *frame.Picture
	rec *frame.Picture
	ref *frame.Picture

	grid []mbInfo

	frameNum   uint32
	frameIndex int
	headers    []byte
}

func New(cfg Config) (*Encoder, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	e := &Encoder{cfg: cfg}
	e.widthMBs = (cfg.Width + 15) / 16
	e.heightMBs = (cfg.Height + 15) / 16
	e.buildParameterSets()
	e.src = frame.NewPicture(e.widthMBs, e.heightMBs)
	e.rec = frame.NewPicture(e.widthMBs, e.heightMBs)
	e.ref = frame.NewPicture(e.widthMBs, e.heightMBs)
	e.grid = make([]mbInfo, e.widthMBs*e.heightMBs)
	return e, nil
}

func (e *Encoder) SPS() *syntax.SPS { return e.sps }

func (e *Encoder) PPS() *syntax.PPS { return e.pps }

var levelLimits = []struct {
	level  uint8
	maxMBs int
	maxMBS int
}{
	{10, 99, 1485},
	{11, 396, 3000},
	{12, 396, 6000},
	{13, 396, 11880},
	{20, 396, 11880},
	{21, 792, 19800},
	{22, 1620, 20250},
	{30, 1620, 40500},
	{31, 3600, 108000},
	{32, 5120, 216000},
	{40, 8192, 245760},
	{42, 8704, 522240},
	{50, 22080, 589824},
	{51, 36864, 983040},
	{52, 36864, 2073600},
}

func (e *Encoder) pickLevel() uint8 {
	frameMBs := e.widthMBs * e.heightMBs
	rate := frameMBs * e.cfg.FPSNum / e.cfg.FPSDen
	for _, l := range levelLimits {
		if frameMBs <= l.maxMBs && rate <= l.maxMBS {
			return l.level
		}
	}
	return 52
}

func (e *Encoder) buildParameterSets() {
	sps := &syntax.SPS{
		ProfileIDC:                syntax.ProfileBaseline,
		ConstraintSet:             0xC0,
		LevelIDC:                  e.pickLevel(),
		ID:                        0,
		ChromaFormatIDC:           syntax.Chroma420,
		Log2MaxFrameNumMinus4:     4,
		PicOrderCntType:           2,
		MaxNumRefFrames:           1,
		PicWidthInMbsMinus1:       uint32(e.widthMBs - 1),
		PicHeightInMapUnitsMinus1: uint32(e.heightMBs - 1),
		FrameMbsOnly:              true,
		Direct8x8Inference:        true,
	}
	cropRight := (e.widthMBs*16 - e.cfg.Width) / 2
	cropBottom := (e.heightMBs*16 - e.cfg.Height) / 2
	if cropRight != 0 || cropBottom != 0 {
		sps.FrameCropping = true
		sps.FrameCropRightOffset = uint32(cropRight)
		sps.FrameCropBottomOffset = uint32(cropBottom)
	}
	sps.VUIPresent = true
	sps.VUI.TimingInfoPresent = true
	sps.VUI.NumUnitsInTick = uint32(e.cfg.FPSDen)
	sps.VUI.TimeScale = uint32(e.cfg.FPSNum) * 2
	sps.VUI.FixedFrameRate = true

	pps := &syntax.PPS{
		ID:                             0,
		SPSID:                          0,
		NumRefIdxL0DefaultActiveMinus1: 0,
		NumRefIdxL1DefaultActiveMinus1: 0,
		PicInitQPMinus26:               int32(e.cfg.QP - 26),
		DeblockingFilterControlPresent: true,
	}
	e.sps = sps
	e.pps = pps
}

func (e *Encoder) parameterSetBytes() ([]byte, error) {
	if e.headers != nil {
		return e.headers, nil
	}
	spsRBSP, err := syntax.WriteSPS(e.sps)
	if err != nil {
		return nil, err
	}
	ppsRBSP, err := syntax.WritePPS(e.pps, func(uint32) *syntax.SPS { return e.sps })
	if err != nil {
		return nil, err
	}
	out := nal.AppendAnnexB(nil, nal.Unit{
		Header: nal.Header{RefIDC: 3, Type: nal.TypeSPS},
		RBSP:   spsRBSP,
	}, true)
	out = nal.AppendAnnexB(out, nal.Unit{
		Header: nal.Header{RefIDC: 3, Type: nal.TypePPS},
		RBSP:   ppsRBSP,
	}, true)
	e.headers = out
	return out, nil
}

func (e *Encoder) Headers() ([]byte, error) { return e.parameterSetBytes() }

func (e *Encoder) Encode(yuv []byte) ([]byte, error) {
	want := e.cfg.Width * e.cfg.Height * 3 / 2
	if len(yuv) != want {
		return nil, fmt.Errorf("%w: got %d bytes, want %d", ErrFrameSize, len(yuv), want)
	}
	e.loadSource(yuv)

	idr := e.frameIndex%e.cfg.GOPSize == 0
	var out []byte
	if idr {
		hdrs, err := e.parameterSetBytes()
		if err != nil {
			return nil, err
		}
		out = append(out, hdrs...)
		e.frameNum = 0
	}

	sliceType := syntax.SliceI
	nalType := nal.TypeSliceIDR
	if !idr {
		sliceType = syntax.SliceP
		nalType = nal.TypeSliceNonIDR
	}

	hdr := &syntax.SliceHeader{
		FirstMBInSlice:             0,
		SliceType:                  sliceType + 5,
		PPSID:                      0,
		FrameNum:                   e.frameNum,
		IDR:                        idr,
		NalRefIDC:                  1,
		DisableDeblockingFilterIDC: 0,
	}

	w := bits.NewWriterSize(e.cfg.Width * e.cfg.Height / 2)
	if err := syntax.WriteSliceHeader(w, hdr, e.sps, e.pps); err != nil {
		return nil, err
	}
	if err := e.encodeSlice(w, hdr); err != nil {
		return nil, err
	}
	w.WriteRBSPTrailingBits()
	if err := w.Err(); err != nil {
		return nil, err
	}

	out = nal.AppendAnnexB(out, nal.Unit{
		Header: nal.Header{RefIDC: 1, Type: nalType},
		RBSP:   w.Bytes(),
	}, true)

	loopfilter.Apply(e.rec, e.widthMBs, e.heightMBs, func(mbx, mby int) *loopfilter.MB {
		m := e.at(mbx, mby)
		if m == nil {
			return nil
		}
		return &m.MB
	})
	e.rec.ExtendBorders()
	e.rec, e.ref = e.ref, e.rec
	e.frameIndex++
	e.frameNum = (e.frameNum + 1) % e.sps.MaxFrameNum()
	return out, nil
}

func (e *Encoder) loadSource(yuv []byte) {
	w, h := e.cfg.Width, e.cfg.Height
	pw, ph := e.widthMBs*16, e.heightMBs*16
	n := 0
	for y := 0; y < h; y++ {
		row := e.src.Y[e.src.LumaOffset(0, y):]
		copy(row[:w], yuv[n:n+w])
		for x := w; x < pw; x++ {
			row[x] = row[w-1]
		}
		n += w
	}
	for y := h; y < ph; y++ {
		copy(e.src.Y[e.src.LumaOffset(0, y):e.src.LumaOffset(0, y)+pw],
			e.src.Y[e.src.LumaOffset(0, h-1):e.src.LumaOffset(0, h-1)+pw])
	}
	cw, ch := w/2, h/2
	pcw, pch := pw/2, ph/2
	for _, plane := range [][]byte{e.src.Cb, e.src.Cr} {
		for y := 0; y < ch; y++ {
			row := plane[e.src.ChromaOffset(0, y):]
			copy(row[:cw], yuv[n:n+cw])
			for x := cw; x < pcw; x++ {
				row[x] = row[cw-1]
			}
			n += cw
		}
		for y := ch; y < pch; y++ {
			copy(plane[e.src.ChromaOffset(0, y):e.src.ChromaOffset(0, y)+pcw],
				plane[e.src.ChromaOffset(0, ch-1):e.src.ChromaOffset(0, ch-1)+pcw])
		}
	}
}
