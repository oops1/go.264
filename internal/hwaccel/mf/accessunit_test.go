package mf

import "testing"

func annexB(units ...[]byte) []byte {
	var out []byte
	for _, u := range units {
		out = append(out, 0, 0, 0, 1)
		out = append(out, u...)
	}
	return out
}

func sps() []byte { return []byte{0x67, 0x42, 0x00, 0x0a} }

func pps() []byte { return []byte{0x68, 0xce, 0x38, 0x80} }

func idrSlice(first bool) []byte {
	head := byte(0x80)
	if !first {
		head = 0x40
	}
	return []byte{0x65, head, 0x11, 0x22}
}

func slice(first bool) []byte {
	head := byte(0x80)
	if !first {
		head = 0x40
	}
	return []byte{0x41, head, 0x33, 0x44}
}

func TestSplitAccessUnitsKeepsParameterSetsWithTheirPicture(t *testing.T) {
	stream := annexB(sps(), pps(), idrSlice(true), slice(true), slice(true))
	units := SplitAccessUnits(stream)
	if len(units) != 3 {
		t.Fatalf("splitting produced %d access units, want 3", len(units))
	}
	if len(units[0]) != 4+len(sps())+4+len(pps())+4+len(idrSlice(true)) {
		t.Fatalf("the first access unit holds %d bytes", len(units[0]))
	}
}

func TestSplitAccessUnitsKeepsSlicesOfOnePictureTogether(t *testing.T) {
	stream := annexB(idrSlice(true), idrSlice(false), idrSlice(false), slice(true))
	units := SplitAccessUnits(stream)
	if len(units) != 2 {
		t.Fatalf("splitting produced %d access units, want 2", len(units))
	}
	if len(units[0]) != 3*(4+len(idrSlice(true))) {
		t.Fatalf("the first access unit holds %d bytes", len(units[0]))
	}
}

func TestSplitAccessUnitsRebuildsEveryByte(t *testing.T) {
	stream := annexB(sps(), pps(), idrSlice(true), slice(true), slice(false), slice(true))
	var joined []byte
	for _, u := range SplitAccessUnits(stream) {
		joined = append(joined, u...)
	}
	if len(joined) != len(stream) {
		t.Fatalf("the access units hold %d bytes against %d in the stream", len(joined), len(stream))
	}
	for i := range stream {
		if joined[i] != stream[i] {
			t.Fatalf("byte %d came back as %d, want %d", i, joined[i], stream[i])
		}
	}
}

func TestSplitAccessUnitsIgnoresAStreamWithoutStartCodes(t *testing.T) {
	if units := SplitAccessUnits([]byte{1, 2, 3, 4}); units != nil {
		t.Fatalf("a stream without start codes produced %d access units", len(units))
	}
	if units := SplitAccessUnits(nil); units != nil {
		t.Fatalf("an empty stream produced %d access units", len(units))
	}
}
