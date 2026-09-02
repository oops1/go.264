package encoder

import (
	"errors"
	"testing"

	"github.com/oops1/go.264/internal/syntax"
)

func TestLevelTableIsMonotonic(t *testing.T) {
	columns := []struct {
		name string
		of   func(int) int
	}{
		{"MaxMBPS", func(i int) int { return levelLimits[i].maxMBPS }},
		{"MaxFS", func(i int) int { return levelLimits[i].maxFS }},
		{"MaxDpbMbs", func(i int) int { return levelLimits[i].maxDpbMbs }},
		{"MaxBR", func(i int) int { return levelLimits[i].maxBR }},
		{"MaxCPB", func(i int) int { return levelLimits[i].maxCPB }},
	}
	for _, c := range columns {
		for i := 1; i < len(levelLimits); i++ {
			if c.of(i) < c.of(i-1) {
				t.Errorf("%s falls from %d at level %d to %d at level %d",
					c.name, c.of(i-1), levelLimits[i-1].level, c.of(i), levelLimits[i].level)
			}
		}
	}
	for i := 1; i < len(levelLimits); i++ {
		if levelLimits[i].level <= levelLimits[i-1].level {
			t.Errorf("level %d does not follow level %d", levelLimits[i].level, levelLimits[i-1].level)
		}
	}
	if levelLimits[0].level != 10 {
		t.Errorf("the table starts at level %d, want 10", levelLimits[0].level)
	}
	for _, l := range levelLimits {
		if l.maxCPB < l.maxBR {
			t.Errorf("level %d holds %d of buffer against a peak of %d, so it cannot hold one second",
				l.level, l.maxCPB, l.maxBR)
		}
		if l.maxDpbMbs < l.maxFS {
			t.Errorf("level %d buffers %d macroblocks but allows pictures of %d",
				l.level, l.maxDpbMbs, l.maxFS)
		}
		if l.maxMBPS < l.maxFS {
			t.Errorf("level %d allows %d macroblocks per second but pictures of %d",
				l.level, l.maxMBPS, l.maxFS)
		}
	}
}

func TestLevelTableMatchesTheDecoderBufferSizes(t *testing.T) {
	for _, l := range levelLimits {
		sps := &syntax.SPS{
			LevelIDC:                  l.level,
			PicWidthInMbsMinus1:       0,
			PicHeightInMapUnitsMinus1: 0,
			FrameMbsOnly:              true,
		}
		want := l.maxDpbMbs
		if want > 16 {
			want = 16
		}
		if got := sps.MaxDpbFrames(); got != want {
			t.Errorf("level %d: the decoder allows %d frames of buffer for one macroblock, the encoder table says %d",
				l.level, got, want)
		}
	}
}

func levelFor(t *testing.T, cfg Config) uint8 {
	t.Helper()
	enc, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return enc.SPS().LevelIDC
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
	limit := int64(0)
	for _, l := range levelLimits {
		if l.level == big {
			limit = int64(l.maxCPB) * cpbBrNalFactor(syntax.ProfileBaseline)
		}
	}
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
	level := levelFor(t, cfg)
	frameMBs := 120 * 68
	for _, l := range levelLimits {
		if l.level != level {
			continue
		}
		if l.maxDpbMbs/frameMBs < 5 {
			t.Fatalf("level %d holds %d frames of buffer at 1920x1088, below the five asked for",
				level, l.maxDpbMbs/frameMBs)
		}
	}
}

func TestLevelCoversTheLongTermSlots(t *testing.T) {
	cfg := Config{Width: 1920, Height: 1088, FPSNum: 30, FPSDen: 1, GOPSize: 30, QP: 26, RefFrames: 4}
	plain := levelFor(t, cfg)
	cfg.LongTermReferences = 4
	held := levelFor(t, cfg)
	frameMBs := 120 * 68
	for _, l := range levelLimits {
		if l.level != held {
			continue
		}
		if l.maxDpbMbs/frameMBs < 8 {
			t.Fatalf("level %d holds %d frames of buffer at 1920x1088, below the eight the slots need",
				held, l.maxDpbMbs/frameMBs)
		}
	}
	t.Logf("four references announce level %d, four more long-term slots announce level %d", plain, held)
}

func TestLevelCoversTheAspectRatio(t *testing.T) {
	cfg := Config{Width: 4096, Height: 64, FPSNum: 25, FPSDen: 1, GOPSize: 25, QP: 26}
	level := levelFor(t, cfg)
	widthMBs := 256
	for _, l := range levelLimits {
		if l.level != level {
			continue
		}
		if widthMBs*widthMBs > 8*l.maxFS {
			t.Fatalf("level %d allows %d macroblocks of picture, too few for a picture %d macroblocks wide",
				level, l.maxFS, widthMBs)
		}
	}
	t.Logf("4096x64 announces level %d", level)
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
	level := enc.SPS().LevelIDC
	factor := cpbBrNalFactor(syntax.ProfileMain)
	for _, l := range levelLimits {
		if l.level != level {
			continue
		}
		if int64(cfg.VBVMaxrateKbps)*1000 > int64(l.maxBR)*factor {
			t.Fatalf("level %d allows %d bit/s, below the %d asked for",
				level, int64(l.maxBR)*factor, cfg.VBVMaxrateKbps*1000)
		}
		if int64(cfg.VBVBufferKbits)*1000 > int64(l.maxCPB)*factor {
			t.Fatalf("level %d allows %d bits of buffer, below the %d asked for",
				level, int64(l.maxCPB)*factor, cfg.VBVBufferKbits*1000)
		}
	}
	t.Logf("176x144 at 900 kbit/s with a 900 kbit buffer announces level %d", level)
}
