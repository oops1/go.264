package go264

import (
	"errors"
	"image"

	"github.com/oops1/go.264/internal/decoder"
	"github.com/oops1/go.264/internal/encoder"
	"github.com/oops1/go.264/internal/frame"
	"github.com/oops1/go.264/internal/hwaccel"
)

var ErrClosed = errors.New("go264: use of a closed codec")

type Frame struct {
	Y       []byte
	Cb      []byte
	Cr      []byte
	StrideY int
	StrideC int
	Width   int
	Height  int
}

func (f *Frame) I420Size() int { return f.Width * f.Height * 3 / 2 }

func (f *Frame) AppendI420(dst []byte) []byte {
	for y := 0; y < f.Height; y++ {
		dst = append(dst, f.Y[y*f.StrideY:y*f.StrideY+f.Width]...)
	}
	cw, ch := f.Width/2, f.Height/2
	for _, plane := range [][]byte{f.Cb, f.Cr} {
		for y := 0; y < ch; y++ {
			dst = append(dst, plane[y*f.StrideC:y*f.StrideC+cw]...)
		}
	}
	return dst
}

func frameFromPicture(p *frame.Picture, width, height int) *Frame {
	if width <= 0 || width > p.Width {
		width = p.Width
	}
	if height <= 0 || height > p.Height {
		height = p.Height
	}
	return &Frame{
		Y:       p.Y[p.LumaOffset(0, 0):],
		Cb:      p.Cb[p.ChromaOffset(0, 0):],
		Cr:      p.Cr[p.ChromaOffset(0, 0):],
		StrideY: p.StrideY,
		StrideC: p.StrideC,
		Width:   width,
		Height:  height,
	}
}

type EncoderConfig struct {
	Width   int
	Height  int
	FPSNum  int
	FPSDen  int
	GOPSize int
	QP      int

	BitrateKbps int
	RefFrames   int
	BFrames     int

	CABAC         bool
	MotionSearch  MotionSearch
	ModeDecision  ModeDecision
	Slices        int
	ForceSoftware bool
}

type MotionSearch = encoder.MotionSearch

const (
	MotionSearchFull = encoder.MotionSearchFull
	MotionSearchZero = encoder.MotionSearchZero
)

type ModeDecision = encoder.ModeDecision

const (
	ModeDecisionFast       = encoder.ModeDecisionFast
	ModeDecisionExhaustive = encoder.ModeDecisionExhaustive
)

type RegionKind = encoder.RegionKind

const (
	RegionUnknown = encoder.RegionUnknown
	RegionFill    = encoder.RegionFill
	RegionText    = encoder.RegionText
	RegionImage   = encoder.RegionImage
)

type Region = encoder.Region

type Encoder struct {
	cpu     *encoder.Encoder
	hw      hwaccel.Encoder
	backend string
	closed  bool
}

func NewEncoder(cfg EncoderConfig) (*Encoder, error) {
	e := &Encoder{backend: "cpu"}
	if !cfg.ForceSoftware {
		if hw, name, ok := hwaccel.OpenEncoder(hwaccel.EncoderParams{
			Width:   cfg.Width,
			Height:  cfg.Height,
			FPSNum:  cfg.FPSNum,
			FPSDen:  cfg.FPSDen,
			GOPSize: cfg.GOPSize,
			QP:      cfg.QP,

			BitrateKbps: cfg.BitrateKbps,
		}); ok {
			e.hw = hw
			e.backend = name
			return e, nil
		}
	}
	cpu, err := encoder.New(encoder.Config{
		Width:        cfg.Width,
		Height:       cfg.Height,
		FPSNum:       cfg.FPSNum,
		FPSDen:       cfg.FPSDen,
		GOPSize:      cfg.GOPSize,
		QP:           cfg.QP,
		BitrateKbps:  cfg.BitrateKbps,
		RefFrames:    cfg.RefFrames,
		BFrames:      cfg.BFrames,
		CABAC:        cfg.CABAC,
		MotionSearch: cfg.MotionSearch,
		ModeDecision: cfg.ModeDecision,
		Slices:       cfg.Slices,
	})
	if err != nil {
		return nil, err
	}
	e.cpu = cpu
	return e, nil
}

func (e *Encoder) Backend() string { return e.backend }

func (e *Encoder) Encode(i420 []byte) ([]byte, error) {
	if e.closed {
		return nil, ErrClosed
	}
	if e.hw != nil {
		return e.hw.Encode(i420)
	}
	return e.cpu.Encode(i420)
}

func (e *Encoder) EncodeWithHints(i420 []byte, changed []image.Rectangle, regions []Region) ([]byte, error) {
	if e.closed {
		return nil, ErrClosed
	}
	if e.hw != nil {
		return e.hw.Encode(i420)
	}
	return e.cpu.EncodeWithHints(i420, encoder.Hints{Changed: changed, Regions: regions})
}

func (e *Encoder) EncodeFrame(f *Frame, changed []image.Rectangle, regions []Region) ([]byte, error) {
	if e.closed {
		return nil, ErrClosed
	}
	if f == nil {
		return nil, errors.New("go264: EncodeFrame was given no frame")
	}
	yuv := f.AppendI420(make([]byte, 0, f.I420Size()))
	if e.hw != nil {
		return e.hw.Encode(yuv)
	}
	return e.cpu.EncodeWithHints(yuv, encoder.Hints{Changed: changed, Regions: regions})
}

func (e *Encoder) ForceKeyFrame() {
	if e.cpu != nil {
		e.cpu.ForceKeyFrame()
	}
}

func (e *Encoder) Flush() ([]byte, error) {
	if e.closed {
		return nil, ErrClosed
	}
	if e.hw != nil {
		return e.hw.Drain()
	}
	return e.cpu.Flush()
}

func (e *Encoder) Close() error {
	if e.closed {
		return nil
	}
	e.closed = true
	if e.hw != nil {
		return e.hw.Close()
	}
	return nil
}

type Decoder struct {
	cpu     *decoder.Decoder
	hw      hwaccel.Decoder
	backend string
	closed  bool
}

type DecoderConfig struct {
	ForceSoftware bool
}

func NewDecoder() *Decoder { return NewDecoderWithConfig(DecoderConfig{}) }

func NewDecoderWithConfig(cfg DecoderConfig) *Decoder {
	d := &Decoder{backend: "cpu"}
	if !cfg.ForceSoftware {
		if hw, name, ok := hwaccel.OpenDecoder(); ok {
			d.hw = hw
			d.backend = name
			return d
		}
	}
	d.cpu = decoder.New()
	return d
}

func (d *Decoder) Backend() string { return d.backend }

func (d *Decoder) Decode(annexB []byte) ([]*Frame, error) {
	if d.closed {
		return nil, ErrClosed
	}
	if d.hw != nil {
		pics, err := d.hw.Decode(annexB)
		return fromHardware(pics), err
	}
	pics, err := d.cpu.Decode(annexB)
	return d.convert(pics), err
}

func (d *Decoder) Flush() ([]*Frame, error) {
	if d.closed {
		return nil, ErrClosed
	}
	if d.hw != nil {
		pics, err := d.hw.Flush()
		return fromHardware(pics), err
	}
	pics, err := d.cpu.Flush()
	return d.convert(pics), err
}

func (d *Decoder) convert(pics []*frame.Picture) []*Frame {
	if len(pics) == 0 {
		return nil
	}
	out := make([]*Frame, 0, len(pics))
	for _, p := range pics {
		out = append(out, frameFromPicture(p, p.CropWidth, p.CropHeight))
	}
	return out
}

func (d *Decoder) Close() error {
	if d.closed {
		return nil
	}
	d.closed = true
	if d.hw != nil {
		return d.hw.Close()
	}
	return nil
}

func Backends() []string {
	out := []string{"cpu"}
	out = append(out, hwaccel.Available()...)
	return out
}

func fromHardware(pics []*hwaccel.Picture) []*Frame {
	if len(pics) == 0 {
		return nil
	}
	out := make([]*Frame, 0, len(pics))
	for _, p := range pics {
		out = append(out, &Frame{
			Y: p.Y, Cb: p.Cb, Cr: p.Cr,
			StrideY: p.StrideY, StrideC: p.StrideC,
			Width: p.Width, Height: p.Height,
		})
	}
	return out
}
