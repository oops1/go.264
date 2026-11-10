package sliceqp

import (
	"fmt"

	"github.com/oops1/go.264/internal/bits"
	"github.com/oops1/go.264/internal/nal"
	"github.com/oops1/go.264/internal/syntax"
)

type sets struct {
	sps map[uint32]*syntax.SPS
	pps map[uint32]*syntax.PPS
}

func (s *sets) SPS(id uint32) *syntax.SPS { return s.sps[id] }

func (s *sets) PPS(id uint32) *syntax.PPS { return s.pps[id] }

type Slice struct {
	IDR bool
	QP  int
}

func Read(annexB []byte) ([]Slice, error) {
	s := &sets{sps: map[uint32]*syntax.SPS{}, pps: map[uint32]*syntax.PPS{}}
	var out []Slice
	for _, ebsp := range nal.SplitAnnexB(annexB) {
		u, err := nal.Parse(ebsp)
		if err != nil {
			return nil, err
		}
		switch u.Header.Type {
		case nal.TypeSPS:
			sps, err := syntax.ParseSPS(u.RBSP)
			if err != nil {
				return nil, err
			}
			s.sps[sps.ID] = sps
		case nal.TypePPS:
			pps, err := syntax.ParsePPS(u.RBSP, s.SPS)
			if err != nil {
				return nil, err
			}
			s.pps[pps.ID] = pps
		case nal.TypeSliceIDR, nal.TypeSliceNonIDR:
			hdr, _, pps, err := syntax.ParseSliceHeader(bits.NewReader(u.RBSP), u.Header, s)
			if err != nil {
				return nil, fmt.Errorf("slice %d: %w", len(out), err)
			}
			out = append(out, Slice{IDR: hdr.IDR, QP: hdr.QPY(pps)})
		}
	}
	return out, nil
}
