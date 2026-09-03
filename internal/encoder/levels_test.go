package encoder

import (
	"errors"
	"testing"

	"github.com/oops1/go.264/internal/level"
	"github.com/oops1/go.264/internal/syntax"
)

func levelFor(t *testing.T, cfg Config) uint8 {
	t.Helper()
	enc, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return enc.SPS().LevelIDC
}

func limitsFor(t *testing.T, idc uint8) level.Limits {
	t.Helper()
	l, ok := level.Lookup(idc)
	if !ok {
		t.Fatalf("level %d is not one the table defines", idc)
	}
	return l
}

func TestLevelStreamCarriesTheConfiguration(t *testing.T) {
	cfg := Config{Width: 1280, Height: 720, FPSNum: 30, FPSDen: 1, GOPSize: 30, QP: 26,
		RefFrames: 3, LongTermReferences: 2, BitrateKbps: 4000, VBVMaxrateKbps: 6000,
		VBVBufferKbits: 12000}
	enc, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	want := level.Stream{
		Width:       1280,
		Height:      720,
		FPSNum:      30,
		FPSDen:      1,
		RefFrames:   5,
		PeakKbps:    6000,
		BufferKbits: 12000,
		ProfileIDC:  syntax.ProfileHigh,
	}
	if got := enc.levelStream(syntax.ProfileHigh); got != want {
		t.Errorf("levelStream = %+v, want %+v", got, want)
	}
}

func TestLevelIgnoresTheBitrateWhenNoneIsAsked(t *testing.T) {
	cases := []struct {
		w, h, fps int
		want      uint8
	}{
		{176, 144, 15, 10},
		{176, 144, 25, 11},
		{352, 288, 30, 13},
		{640, 480, 30, 30},
		{1280, 720, 30, 31},
		{1920, 1088, 30, 40},
		{3840, 2160, 30, 51},
	}
	for _, c := range cases {
		cfg := Config{Width: c.w, Height: c.h, FPSNum: c.fps, FPSDen: 1, GOPSize: 30, QP: 26}
		if got := levelFor(t, cfg); got != c.want {
			t.Errorf("%dx%d at %d frames per second announces level %d, want %d",
				c.w, c.h, c.fps, got, c.want)
		}
	}
}

func TestLevelRisesWithTheBitrate(t *testing.T) {
	base := Config{Width: 1280, Height: 720, FPSNum: 30, FPSDen: 1, GOPSize: 30, QP: 26}
	cases := []struct {
		kbps int
		want uint8
	}{
		{0, 31},
		{16000, 31},
		{16800, 31},
		{16801, 32},
		{24000, 32},
		{24001, 41},
		{60000, 41},
		{60001, 50},
	}
	for _, c := range cases {
		cfg := base
		cfg.BitrateKbps = c.kbps
		if got := levelFor(t, cfg); got != c.want {
			t.Errorf("%d kbit/s announces level %d, want %d", c.kbps, got, c.want)
		}
	}
}

func TestLevelRisesWithTheBufferSize(t *testing.T) {
	base := Config{Width: 352, Height: 288, FPSNum: 25, FPSDen: 1, GOPSize: 25, QP: 26}
	plain := levelFor(t, base)

	cfg := base
	cfg.BitrateKbps = 1000
	cfg.VBVMaxrateKbps = 1000
	cfg.VBVBufferKbits = 60000
	big := levelFor(t, cfg)
	if big <= plain {
		t.Fatalf("a 60000 kbit buffer announces level %d, no higher than level %d without one", big, plain)
	}
	limit := limitsFor(t, big).MaxBufferBits(syntax.ProfileBaseline)
	if limit < 60000*1000 {
		t.Fatalf("level %d allows only %d bits of buffer, below the 60000 kbit asked for", big, limit)
	}
	t.Logf("a 60000 kbit buffer lifts level %d to level %d", plain, big)
}

func TestLevelUsesTheHighProfileFactor(t *testing.T) {
	base := Config{Width: 1280, Height: 720, FPSNum: 30, FPSDen: 1, GOPSize: 30, QP: 26,
		BitrateKbps: 19000}
	plain := levelFor(t, base)

	high := base
	high.Transform8x8 = true
	got := levelFor(t, high)
	if plain != 32 {
		t.Fatalf("19000 kbit/s in Baseline profile announces level %d, want 32", plain)
	}
	if got != 31 {
		t.Fatalf("19000 kbit/s in High profile announces level %d, want 31 because the High factor is larger", got)
	}
}

func TestLevelRejectsABitrateNoLevelCarries(t *testing.T) {
	cfg := Config{Width: 1280, Height: 720, FPSNum: 30, FPSDen: 1, GOPSize: 30, QP: 26,
		BitrateKbps: 1000000}
	_, err := New(cfg)
	if err == nil {
		t.Fatal("a bitrate above every level was accepted")
	}
	if !errors.Is(err, ErrConfig) {
		t.Fatalf("the error is %v, want one wrapping ErrConfig", err)
	}
	if !errors.Is(err, level.ErrNoLevel) {
		t.Fatalf("the error is %v, want one wrapping level.ErrNoLevel", err)
	}
	t.Logf("%v", err)
}

func TestLevelRejectsABufferNoLevelCarries(t *testing.T) {
	cfg := Config{Width: 1280, Height: 720, FPSNum: 30, FPSDen: 1, GOPSize: 30, QP: 26,
		BitrateKbps: 1000, VBVMaxrateKbps: 1000, VBVBufferKbits: 2000000}
	if _, err := New(cfg); err == nil {
		t.Fatal("a buffer above every level was accepted")
	}
}

func TestLevelCoversTheReferenceCount(t *testing.T) {
	cfg := Config{Width: 1920, Height: 1088, FPSNum: 30, FPSDen: 1, GOPSize: 30, QP: 26, RefFrames: 5}
	idc := levelFor(t, cfg)
	frameMBs := 120 * 68
	if held := limitsFor(t, idc).MaxDpbMbs / frameMBs; held < 5 {
		t.Fatalf("level %d holds %d frames of buffer at 1920x1088, below the five asked for", idc, held)
	}
}

func TestLevelCoversTheLongTermSlots(t *testing.T) {
	cfg := Config{Width: 1920, Height: 1088, FPSNum: 30, FPSDen: 1, GOPSize: 30, QP: 26, RefFrames: 4}
	plain := levelFor(t, cfg)
	cfg.LongTermReferences = 4
	held := levelFor(t, cfg)
	frameMBs := 120 * 68
	if frames := limitsFor(t, held).MaxDpbMbs / frameMBs; frames < 8 {
		t.Fatalf("level %d holds %d frames of buffer at 1920x1088, below the eight the slots need",
			held, frames)
	}
	t.Logf("four references announce level %d, four more long-term slots announce level %d", plain, held)
}

func TestLevelCoversTheAspectRatio(t *testing.T) {
	cfg := Config{Width: 4096, Height: 64, FPSNum: 25, FPSDen: 1, GOPSize: 25, QP: 26}
	idc := levelFor(t, cfg)
	widthMBs := 256
	if maxFS := limitsFor(t, idc).MaxFS; widthMBs*widthMBs > 8*maxFS {
		t.Fatalf("level %d allows %d macroblocks of picture, too few for a picture %d macroblocks wide",
			idc, maxFS, widthMBs)
	}
	t.Logf("4096x64 announces level %d", idc)
}

func TestLevelStreamsStayReadable(t *testing.T) {
	cfg := Config{Width: 176, Height: 144, FPSNum: 25, FPSDen: 1, GOPSize: 8, QP: 28,
		CABAC: true, BitrateKbps: 700, VBVMaxrateKbps: 900, VBVBufferKbits: 900}
	frames := screenContent(cfg.Width, cfg.Height, 12)
	units, recons, _ := encodeLongTermStream(t, cfg, frames, nil)
	assertMatchesReconstruction(t, "level", decodeUnits(t, units), recons)

	enc, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	idc := enc.SPS().LevelIDC
	l := limitsFor(t, idc)
	if peak := l.MaxBitsPerSecond(syntax.ProfileMain); int64(cfg.VBVMaxrateKbps)*1000 > peak {
		t.Fatalf("level %d allows %d bit/s, below the %d asked for", idc, peak, cfg.VBVMaxrateKbps*1000)
	}
	if buffer := l.MaxBufferBits(syntax.ProfileMain); int64(cfg.VBVBufferKbits)*1000 > buffer {
		t.Fatalf("level %d allows %d bits of buffer, below the %d asked for",
			idc, buffer, cfg.VBVBufferKbits*1000)
	}
	t.Logf("176x144 at 900 kbit/s with a 900 kbit buffer announces level %d", idc)
}
