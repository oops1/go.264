package go264

import (
	"errors"
	"testing"

	"github.com/oops1/go.264/internal/nal"
	"github.com/oops1/go.264/internal/syntax"
)

func spsUnit(t *testing.T, s *syntax.SPS) []byte {
	t.Helper()
	rbsp, err := syntax.WriteSPS(s)
	if err != nil {
		t.Fatalf("WriteSPS: %v", err)
	}
	return append([]byte{0, 0, 1, 0x67}, nal.Escape(nil, rbsp)...)
}

func hostileSPS(widthMBs, heightMBs uint32) *syntax.SPS {
	return &syntax.SPS{
		ProfileIDC:                  66,
		LevelIDC:                    10,
		FrameMbsOnly:                true,
		ChromaFormatIDC:             syntax.Chroma420,
		PicWidthInMbsMinus1:         widthMBs - 1,
		PicHeightInMapUnitsMinus1:   heightMBs - 1,
		MaxNumRefFrames:             1,
		Log2MaxFrameNumMinus4:       0,
		Log2MaxPicOrderCntLsbMinus4: 0,
	}
}

func TestAPictureNoLevelCanCarryNeverSizesTheHardware(t *testing.T) {
	unit := spsUnit(t, hostileSPS(1024, 1024))
	if w, h := streamPictureSize(unit); w != 0 || h != 0 {
		t.Fatalf("a %d byte stream offered the hardware a %d by %d decoder", len(unit), w, h)
	}
	d := NewDecoder()
	defer d.Close()
	if _, err := d.Decode(unit); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if d.Backend() != "cpu" {
		t.Fatalf("the decoder settled on %q", d.Backend())
	}
}

func TestAPictureAnyLevelCanCarryStillSizesTheDecoder(t *testing.T) {
	unit := spsUnit(t, hostileSPS(120, 68))
	w, h := streamPictureSize(unit)
	if w != 1920 || h != 1088 {
		t.Fatalf("streamPictureSize = %d by %d, want 1920 by 1088", w, h)
	}
}

func TestTheCallerCanBoundWhatTheDecoderAccepts(t *testing.T) {
	cfg := EncoderConfig{Width: 176, Height: 144, FPSNum: 25, FPSDen: 1, GOPSize: 8,
		QP: 26, ForceSoftware: true}
	stream, _ := encodeThroughPublicAPI(t, cfg, 2)

	open := NewDecoderWithConfig(DecoderConfig{ForceSoftware: true})
	defer open.Close()
	if _, err := open.Decode(stream); err != nil {
		t.Fatalf("99 macroblocks were refused without a ceiling: %v", err)
	}

	rows := 99
	bounded := NewDecoderWithConfig(DecoderConfig{
		ForceSoftware: true,
		Limits:        DecoderLimits{MaxFrameMBs: rows - 1},
	})
	defer bounded.Close()
	if _, err := bounded.Decode(stream); !errors.Is(err, ErrOverLimit) {
		t.Fatalf("Decode() = %v, want the caller ceiling of %d macroblocks to refuse it", err, rows-1)
	}
}
