package nal

import "bytes"

var startCodePrefix = []byte{0x00, 0x00, 0x01}

func indexStartCode(data []byte, from int) int {
	if from < 0 {
		from = 0
	}
	if from >= len(data) {
		return -1
	}
	i := bytes.Index(data[from:], startCodePrefix)
	if i < 0 {
		return -1
	}
	return from + i
}

func trimTrailingZeros(b []byte) []byte {
	n := len(b)
	for n > 0 && b[n-1] == 0 {
		n--
	}
	return b[:n]
}

func SplitAnnexB(data []byte) [][]byte {
	k := indexStartCode(data, 0)
	if k < 0 {
		return nil
	}
	var out [][]byte
	i := k + 3
	for {
		j := indexStartCode(data, i)
		var payload []byte
		if j < 0 {
			payload = trimTrailingZeros(data[i:])
		} else {
			payload = trimTrailingZeros(data[i:j])
		}
		if len(payload) > 0 {
			out = append(out, payload)
		}
		if j < 0 {
			return out
		}
		i = j + 3
	}
}

type Scanner struct {
	buf   []byte
	start int
	scan  int
}

func NewScanner() *Scanner { return &Scanner{start: -1} }

func (s *Scanner) Reset() {
	s.buf = s.buf[:0]
	s.start = -1
	s.scan = 0
}

func (s *Scanner) Buffered() int { return len(s.buf) }

func (s *Scanner) Append(p []byte) { s.buf = append(s.buf, p...) }

func (s *Scanner) rewindScan(floor int) {
	s.scan = len(s.buf) - 2
	if s.scan < floor {
		s.scan = floor
	}
}

func (s *Scanner) Next() ([]byte, bool) {
	for {
		if s.start < 0 {
			k := indexStartCode(s.buf, s.scan)
			if k < 0 {
				s.rewindScan(0)
				return nil, false
			}
			s.start = k + 3
			s.scan = s.start
		}
		j := indexStartCode(s.buf, s.scan)
		if j < 0 {
			s.rewindScan(s.start)
			return nil, false
		}
		payload := trimTrailingZeros(s.buf[s.start:j])
		out := append([]byte(nil), payload...)
		s.buf = s.buf[:copy(s.buf, s.buf[j+3:])]
		s.start = 0
		s.scan = 0
		if len(out) > 0 {
			return out, true
		}
	}
}

func (s *Scanner) Flush() ([]byte, bool) {
	if s.start < 0 || s.start >= len(s.buf) {
		s.Reset()
		return nil, false
	}
	payload := append([]byte(nil), trimTrailingZeros(s.buf[s.start:])...)
	s.Reset()
	if len(payload) == 0 {
		return nil, false
	}
	return payload, true
}
