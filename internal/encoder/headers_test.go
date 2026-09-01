package encoder

import (
	"bytes"
	"testing"

	"github.com/oops1/go.264/internal/decoder"
	"github.com/oops1/go.264/internal/frame"
	"github.com/oops1/go.264/internal/nal"
)

func repeatConfig(qp, period int, repeat bool) Config {
	cfg := refreshConfig(qp, period)
	cfg.RepeatParameterSets = repeat
	return cfg
}

func unitTypesIn(t *testing.T, pkt []byte) []uint8 {
	t.Helper()
	var out []uint8
	for _, ebsp := range nal.SplitAnnexB(pkt) {
		u, err := nal.Parse(ebsp)
		if err != nil {
			t.Fatalf("parsing our own unit: %v", err)
		}
		out = append(out, uint8(u.Type))
	}
	return out
}

func countParameterSets(t *testing.T, units [][]byte) (sps, pps int) {
	t.Helper()
	for _, u := range units {
		for _, ty := range unitTypesIn(t, u) {
			switch ty {
			case uint8(nal.TypeSPS):
				sps++
			case uint8(nal.TypePPS):
				pps++
			}
		}
	}
	return sps, pps
}

func TestParameterSetsAreSentOnceByDefault(t *testing.T) {
	cfg := repeatConfig(26, 6, false)
	frames := screenSequence(cfg.Width, cfg.Height, 24)
	units, _ := encodeUnits(t, cfg, frames)
	sps, pps := countParameterSets(t, units)
	if sps != 1 || pps != 1 {
		t.Fatalf("the default stream carries %d sequence and %d picture parameter sets, want one of each", sps, pps)
	}
}

func TestParameterSetsPrecedeEveryRecoveryPoint(t *testing.T) {
	const period = 6
	cfg := repeatConfig(26, period, true)
	frames := screenSequence(cfg.Width, cfg.Height, 24)
	units, _ := encodeUnits(t, cfg, frames)
	enc, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}

	sweeps := 0
	for i, u := range units {
		points := recoveryPointsIn(t, u, enc.SPS())
		types := unitTypesIn(t, u)
		if i == 0 {
			continue
		}
		if len(points) == 0 {
			for _, ty := range types {
				if ty == uint8(nal.TypeSPS) || ty == uint8(nal.TypePPS) {
					t.Fatalf("frame %d carries a parameter set without a recovery point", i)
				}
			}
			continue
		}
		sweeps++
		want := []uint8{uint8(nal.TypeSPS), uint8(nal.TypePPS), uint8(nal.TypeSEI)}
		if len(types) < len(want) {
			t.Fatalf("frame %d carries units %v, want the parameter sets ahead of the recovery point", i, types)
		}
		for j, ty := range want {
			if types[j] != ty {
				t.Fatalf("frame %d carries units %v, want %v first", i, types, want)
			}
		}
	}
	if sweeps < 3 {
		t.Fatalf("the stream carries %d recovery points, too few to prove anything", sweeps)
	}
	sps, pps := countParameterSets(t, units)
	if sps != sweeps+1 || pps != sweeps+1 {
		t.Fatalf("%d recovery points and one key frame carry %d sequence and %d picture parameter sets",
			sweeps, sps, pps)
	}
}

func TestRepeatingParameterSetsAddsNothingElse(t *testing.T) {
	const period = 6
	frames := screenSequence(176, 144, 24)
	plain, _ := encodeUnits(t, repeatConfig(26, period, false), frames)
	repeated, _ := encodeUnits(t, repeatConfig(26, period, true), frames)
	if len(plain) != len(repeated) {
		t.Fatalf("the two streams carry %d and %d pictures", len(plain), len(repeated))
	}
	headers := headerBytesOf(t, repeatConfig(26, period, true))
	for i := range plain {
		if bytes.Equal(plain[i], repeated[i]) {
			continue
		}
		if bytes.Equal(repeated[i], append(append([]byte(nil), headers...), plain[i]...)) {
			continue
		}
		t.Fatalf("frame %d differs by more than a repeated parameter set: %d bytes against %d",
			i, len(repeated[i]), len(plain[i]))
	}
}

func headerBytesOf(t *testing.T, cfg Config) []byte {
	t.Helper()
	enc, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	h, err := enc.Headers()
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func firstSliceOffset(pkt []byte) int {
	for i := 0; i+3 < len(pkt); i++ {
		if pkt[i] != 0 || pkt[i+1] != 0 || pkt[i+2] != 1 {
			continue
		}
		switch pkt[i+3] & 0x1F {
		case uint8(nal.TypeSPS), uint8(nal.TypePPS):
			continue
		}
		for i > 0 && pkt[i-1] == 0 {
			i--
		}
		return i
	}
	return -1
}

func parameterSetPrefix(pkt []byte) []byte {
	at := firstSliceOffset(pkt)
	if at <= 0 {
		return nil
	}
	return pkt[:at]
}

func anchorWithoutParameterSets(t *testing.T, cfg Config) []byte {
	t.Helper()
	stray, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	pkt, err := stray.Encode(flatFrame(cfg.Width, cfg.Height, 17, 200, 40))
	if err != nil {
		t.Fatal(err)
	}
	at := firstSliceOffset(pkt)
	if at <= 0 {
		t.Fatalf("the key frame carries no slice after its parameter sets")
	}
	return pkt[at:]
}

func decodeOrError(units [][]byte) ([]*frame.Picture, error) {
	var stream []byte
	for _, u := range units {
		stream = append(stream, u...)
	}
	d := decoder.New()
	pics, err := d.Decode(stream)
	if err != nil {
		return nil, err
	}
	rest, err := d.Flush()
	if err != nil {
		return pics, err
	}
	return append(pics, rest...), nil
}

func TestALateJoinerNeedsTheRepeatedParameterSets(t *testing.T) {
	const period = 6
	frames := screenSequence(176, 144, 30)
	for _, repeat := range []bool{false, true} {
		cfg := repeatConfig(26, period, repeat)
		units, _ := encodeUnits(t, cfg, frames)
		full := decodeUnits(t, units)
		anchor := anchorWithoutParameterSets(t, cfg)

		start := 1 + 2*period
		inBand := parameterSetPrefix(units[start])
		if !repeat {
			if len(inBand) != 0 {
				t.Fatal("a stream that should not repeat its parameter sets carries them anyway")
			}
			pics, err := decodeOrError([][]byte{anchor, units[start]})
			if err == nil {
				t.Fatalf("a late joiner decoded %d pictures from a stream with no parameter sets in band", len(pics))
			}
			t.Logf("without repeated parameter sets a late joiner is refused: %v", err)
			continue
		}
		if len(inBand) == 0 {
			t.Fatal("the recovery point carries no parameter sets")
		}
		joined := [][]byte{inBand, anchor}
		joined = append(joined, units[start:]...)
		pics, err := decodeOrError(joined)
		if err != nil {
			t.Fatalf("a late joiner was refused although the parameter sets are in band: %v", err)
		}
		if len(pics) < 2 {
			t.Fatalf("the late joiner decoded %d pictures", len(pics))
		}
		pics = pics[1:]
		converged := false
		for i, p := range pics {
			at := start + i
			if at < start+period-1 {
				continue
			}
			if d := planeDiff(p, full[at]); d != 0 {
				t.Fatalf("late joiner: frame %d differs in %d samples after a full sweep", at, d)
			}
			converged = true
		}
		if !converged {
			t.Fatal("the late joiner never reached a frame past a full sweep")
		}
		t.Logf("a late joiner starting at frame %d is exact %d frames later", start, period-1)
	}
}

func TestRepeatedParameterSetsDoNotDisturbTheDecoder(t *testing.T) {
	const period = 5
	frames := screenSequence(176, 144, 24)
	cfg := repeatConfig(26, period, true)
	units, recons := encodeUnits(t, cfg, frames)
	assertMatchesReconstruction(t, "repeated parameter sets", decodeUnits(t, units), recons)
}

func TestRepeatedParameterSetsSurviveAnExternalDecoder(t *testing.T) {
	const period = 5
	cfg := repeatConfig(28, period, true)
	frames := screenSequence(cfg.Width, cfg.Height, 20)
	units, recons := encodeUnits(t, cfg, frames)
	var stream []byte
	for _, u := range units {
		stream = append(stream, u...)
	}
	ref := decodeWithFFmpeg(t, stream)
	frameSize := cfg.Width * cfg.Height * 3 / 2
	if len(ref) != frameSize*len(recons) {
		t.Fatalf("ffmpeg produced %d bytes for %d pictures", len(ref), len(recons))
	}
	for i := range recons {
		got := make([]byte, recons[i].Size())
		recons[i].CopyOut(got)
		want := ref[i*frameSize : (i+1)*frameSize]
		if !bytes.Equal(got, want) {
			t.Fatalf("frame %d: ffmpeg and our reconstruction disagree", i)
		}
	}
}
