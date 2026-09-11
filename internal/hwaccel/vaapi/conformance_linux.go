//go:build linux && (amd64 || arm64)

package vaapi

import (
	"fmt"
	"strings"

	"github.com/oops1/go.264/internal/bits"
	"github.com/oops1/go.264/internal/nal"
	"github.com/oops1/go.264/internal/syntax"
)

type streamSets struct {
	sps map[uint32]*syntax.SPS
	pps map[uint32]*syntax.PPS
}

func (s *streamSets) SPS(id uint32) *syntax.SPS { return s.sps[id] }

func (s *streamSets) PPS(id uint32) *syntax.PPS { return s.pps[id] }

func checkSliceNumbering(stream []byte) error {
	sets := &streamSets{sps: map[uint32]*syntax.SPS{}, pps: map[uint32]*syntax.PPS{}}
	pic := 0
	var prevFrameNum uint32
	havePrev := false
	for i, ebsp := range nal.SplitAnnexB(stream) {
		u, err := nal.Parse(ebsp)
		if err != nil {
			return fmt.Errorf("unit %d does not parse: %w", i, err)
		}
		switch u.Header.Type {
		case nal.TypeSPS:
			s, err := syntax.ParseSPS(u.RBSP)
			if err != nil {
				return fmt.Errorf("the sequence parameter set does not parse: %w", err)
			}
			sets.sps[s.ID] = s
		case nal.TypePPS:
			p, err := syntax.ParsePPS(u.RBSP, sets.SPS)
			if err != nil {
				return fmt.Errorf("the picture parameter set does not parse: %w", err)
			}
			sets.pps[p.ID] = p
		case nal.TypeSliceIDR, nal.TypeSliceNonIDR:
			h, sps, _, err := syntax.ParseSliceHeader(bits.NewReader(u.RBSP), u.Header, sets)
			if err != nil {
				return fmt.Errorf("picture %d: the slice header does not parse: %w", pic, err)
			}
			if h.FirstMBInSlice != 0 {
				continue
			}
			maxFrameNum := uint32(1) << (sps.Log2MaxFrameNumMinus4 + 4)
			switch {
			case h.IDR && h.FrameNum != 0:
				return fmt.Errorf("picture %d is an IDR with frame_num %d, and an IDR's must be 0", pic, h.FrameNum)
			case !h.IDR && havePrev && !sps.GapsInFrameNumValueAllowed && h.FrameNum != (prevFrameNum+1)%maxFrameNum:
				return fmt.Errorf("picture %d has frame_num %d after a reference picture numbered %d; with no gaps allowed it must be %d",
					pic, h.FrameNum, prevFrameNum, (prevFrameNum+1)%maxFrameNum)
			}
			if u.Header.RefIDC != 0 {
				prevFrameNum, havePrev = h.FrameNum, true
			}
			pic++
		}
	}
	return nil
}

func crashReason(stderr []byte) string {
	lines := strings.Split(strings.ReplaceAll(string(stderr), "\r", ""), "\n")
	for _, l := range lines {
		if strings.Contains(l, "Assertion") {
			return strings.TrimSpace(l)
		}
	}
	for i, l := range lines {
		t := strings.TrimSpace(l)
		if strings.HasPrefix(t, "SIGABRT") || strings.HasPrefix(t, "SIGSEGV") || strings.HasPrefix(t, "SIGBUS") ||
			strings.HasPrefix(t, "fatal error") || strings.Contains(t, "signal arrived during") {
			for j := i - 1; j >= 0; j-- {
				if s := strings.TrimSpace(lines[j]); s != "" && !strings.HasPrefix(s, "libva ") {
					return s
				}
			}
			return t
		}
	}
	return lastLine(stderr)
}
