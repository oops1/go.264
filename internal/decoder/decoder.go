package decoder

import (
	"fmt"

	"github.com/oops1/go264/internal/bits"
	"github.com/oops1/go264/internal/nal"
	"github.com/oops1/go264/internal/syntax"
)

type Decoder struct {
	spsMap map[uint32]*syntax.SPS
	ppsMap map[uint32]*syntax.PPS

	scanner *nal.Scanner
	grid    *mbGrid
	cur     *Picture
	sps     *syntax.SPS

	sliceCount int
	pending    []*Picture
}

func New() *Decoder {
	return &Decoder{
		spsMap:  make(map[uint32]*syntax.SPS),
		ppsMap:  make(map[uint32]*syntax.PPS),
		scanner: nal.NewScanner(),
	}
}

func (d *Decoder) SPS(id uint32) *syntax.SPS { return d.spsMap[id] }

func (d *Decoder) PPS(id uint32) *syntax.PPS { return d.ppsMap[id] }

func (d *Decoder) Decode(data []byte) ([]*Picture, error) {
	d.scanner.Append(data)
	var out []*Picture
	for {
		unit, ok := d.scanner.Next()
		if !ok {
			break
		}
		pics, err := d.handleUnit(unit)
		if err != nil {
			return out, err
		}
		out = append(out, pics...)
	}
	return out, nil
}

func (d *Decoder) Flush() ([]*Picture, error) {
	var out []*Picture
	if unit, ok := d.scanner.Flush(); ok {
		pics, err := d.handleUnit(unit)
		out = append(out, pics...)
		if err != nil {
			return out, err
		}
	}
	if d.cur != nil {
		out = append(out, d.cur)
		d.cur = nil
	}
	return out, nil
}

func (d *Decoder) handleUnit(ebsp []byte) ([]*Picture, error) {
	u, err := nal.Parse(ebsp)
	if err != nil {
		return nil, err
	}
	switch u.Type {
	case nal.TypeSPS:
		sps, err := syntax.ParseSPS(u.RBSP)
		if err != nil {
			return nil, err
		}
		if err := d.checkSupported(sps); err != nil {
			return nil, err
		}
		d.spsMap[sps.ID] = sps
		return nil, nil

	case nal.TypePPS:
		pps, err := syntax.ParsePPS(u.RBSP, d.SPS)
		if err != nil {
			return nil, err
		}
		if pps.CABAC {
			return nil, fmt.Errorf("%w: CABAC entropy coding", ErrUnsupported)
		}
		d.ppsMap[pps.ID] = pps
		return nil, nil

	case nal.TypeSliceIDR, nal.TypeSliceNonIDR:
		return d.decodeSlice(u)
	}
	return nil, nil
}

func (d *Decoder) checkSupported(sps *syntax.SPS) error {
	if sps.ChromaArrayType() != syntax.Chroma420 {
		return fmt.Errorf("%w: chroma format %d", ErrUnsupported, sps.ChromaFormatIDC)
	}
	if sps.BitDepthLumaMinus8 != 0 || sps.BitDepthChromaMinus8 != 0 {
		return fmt.Errorf("%w: bit depth above 8", ErrUnsupported)
	}
	if !sps.FrameMbsOnly {
		return fmt.Errorf("%w: field or MBAFF coding", ErrUnsupported)
	}
	if sps.SeqScalingMatrixPresent {
		return fmt.Errorf("%w: sequence scaling matrices", ErrUnsupported)
	}
	return nil
}

func (d *Decoder) decodeSlice(u nal.Unit) ([]*Picture, error) {
	r := bits.NewReader(u.RBSP)
	hdr, sps, pps, err := syntax.ParseSliceHeader(r, u.Header, d)
	if err != nil {
		return nil, err
	}
	if pps.PicScalingMatrixPresent {
		return nil, fmt.Errorf("%w: picture scaling matrices", ErrUnsupported)
	}

	var out []*Picture
	if hdr.FirstMBInSlice == 0 {
		if d.cur != nil {
			out = append(out, d.cur)
		}
		d.sps = sps
		d.cur = NewPicture(sps.PicWidthInMbs(), sps.FrameHeightInMbs())
		d.cur.FrameNum = hdr.FrameNum
		d.cur.IDR = hdr.IDR
		d.grid = newMBGrid(sps.PicWidthInMbs(), sps.FrameHeightInMbs())
		d.sliceCount = 0
	}
	if d.cur == nil {
		return out, ErrNoParameters
	}
	d.sliceCount++

	sd := &sliceDecoder{
		r:       r,
		sps:     sps,
		pps:     pps,
		hdr:     hdr,
		pic:     d.cur,
		grid:    d.grid,
		sliceID: d.sliceCount,
	}
	if err := sd.run(); err != nil {
		return out, err
	}
	return out, nil
}
