package encoder

import (
	"bytes"
	"image"
	"runtime"
	"testing"
)

type rowCase struct {
	name   string
	cfg    Config
	hints  func(i int) Hints
	forced map[int]bool
}

func rowCases(w, h int) []rowCase {
	base := func(edit func(c *Config)) Config {
		c := Config{Width: w, Height: h, FPSNum: 30, FPSDen: 1, GOPSize: 12, QP: 27, RefFrames: 1}
		edit(&c)
		return c
	}
	changed := func(i int) Hints {
		return Hints{Changed: []image.Rectangle{image.Rect(w/8, h/8+i*8%(h/2), w/2, h/8+i*8%(h/2)+h/4)}}
	}
	text := func(i int) Hints {
		return Hints{Regions: []Region{{Rect: image.Rect(0, 0, w/2, h/2), Kind: RegionText}}}
	}
	return []rowCase{
		{name: "one slice", cfg: base(func(c *Config) {})},
		{name: "four slices", cfg: base(func(c *Config) { c.Slices = 4 })},
		{name: "a slice per row", cfg: base(func(c *Config) { c.Slices = h / 16 })},
		{name: "three references", cfg: base(func(c *Config) { c.RefFrames = 3; c.Slices = 3 })},
		{name: "rate factor", cfg: base(func(c *Config) { c.RateFactor = 23; c.Slices = 2 })},
		{name: "b pictures", cfg: base(func(c *Config) { c.BFrames = 2; c.RefFrames = 2; c.Slices = 2 })},
		{name: "intra refresh", cfg: base(func(c *Config) { c.IntraRefresh = 6; c.GOPSize = 600 })},
		{name: "long-term references", cfg: base(func(c *Config) { c.LongTermReferences = 1; c.RefFrames = 2 })},
		{name: "trellis and exhaustive decisions", cfg: base(func(c *Config) {
			c.Trellis = true
			c.ModeDecision = ModeDecisionExhaustive
		})},
		{name: "weighted prediction", cfg: base(func(c *Config) { c.WeightedPrediction = WeightedPredictionExplicit })},
		{name: "no motion search", cfg: base(func(c *Config) { c.MotionSearch = MotionSearchZero })},
		{name: "changed rectangles", cfg: base(func(c *Config) {}), hints: changed},
		{name: "text regions", cfg: base(func(c *Config) {}), hints: text},
		{name: "forced key frames", cfg: base(func(c *Config) { c.GOPSize = 600 }), forced: map[int]bool{5: true, 9: true}},
		{name: "deblocking kept inside slices", cfg: base(func(c *Config) {
			c.Deblocking = DeblockingNotAcrossSlices
			c.Slices = 3
		})},
		{name: "a buffer that drops pictures", cfg: base(func(c *Config) {
			c.BitrateKbps = 150
			c.VBVBufferKbits = 100
			c.VBVMaxrateKbps = 150
		})},
	}
}

func encodeRowCase(t *testing.T, c rowCase, frames [][]byte) []byte {
	t.Helper()
	enc, err := New(c.cfg)
	if err != nil {
		t.Fatalf("%s: %v", c.name, err)
	}
	var stream []byte
	for i, f := range frames {
		if c.forced[i] {
			enc.ForceKeyFrame()
		}
		var h Hints
		if c.hints != nil {
			h = c.hints(i)
		}
		pkt, err := enc.EncodeWithHints(f, h)
		if err != nil {
			t.Fatalf("%s, frame %d: %v", c.name, i, err)
		}
		stream = append(stream, pkt...)
	}
	tail, err := enc.Flush()
	if err != nil {
		t.Fatalf("%s: %v", c.name, err)
	}
	return append(stream, tail...)
}

func TestRowsInParallelWriteTheSameStream(t *testing.T) {
	if runtime.GOMAXPROCS(0) < 2 {
		t.Skip("one processor codes in raster order anyway")
	}
	const w, h = 640, 480
	var frames [][]byte
	for _, clip := range screencastClips() {
		frames = append(frames, clip.frames(w, h, 3)...)
	}
	for i := 0; i < 4; i++ {
		frames = append(frames, movingFrame(w, h, i*5))
	}
	for _, c := range rowCases(w, h) {
		rows := encodeRowCase(t, c, frames)
		prev := runtime.GOMAXPROCS(1)
		raster := encodeRowCase(t, c, frames)
		runtime.GOMAXPROCS(prev)
		if !bytes.Equal(rows, raster) {
			at := 0
			for at < len(rows) && at < len(raster) && rows[at] == raster[at] {
				at++
			}
			t.Errorf("%s: coding rows in parallel wrote %d bytes, raster order %d, first difference at byte %d",
				c.name, len(rows), len(raster), at)
		}
	}
}

func TestRowsInParallelOnlyWhereTheyCanMatchRasterOrder(t *testing.T) {
	small, err := New(Config{Width: 320, Height: 240, QP: 26})
	if err != nil {
		t.Fatal(err)
	}
	if small.rowsInParallel(nil) {
		t.Error("a picture of 300 macroblocks is coded in rows")
	}
	cabac, err := New(Config{Width: 1280, Height: 720, QP: 26, CABAC: true})
	if err != nil {
		t.Fatal(err)
	}
	if cabac.rowsInParallel(nil) {
		t.Error("CABAC, whose contexts run through the whole slice, is coded in rows")
	}
	plain, err := New(Config{Width: 1280, Height: 720, QP: 26})
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOMAXPROCS(0) > 1 && !plain.rowsInParallel(nil) {
		t.Error("a 1280x720 CAVLC picture is not coded in rows")
	}
	varied := &frameHints{qpOffset: make([]int8, plain.widthMBs*plain.heightMBs), widthMBs: plain.widthMBs}
	if runtime.GOMAXPROCS(0) > 1 && !plain.rowsInParallel(varied) {
		t.Error("a quantiser map of zeros stops the rows")
	}
	varied.qpOffset[17] = -3
	if plain.rowsInParallel(varied) {
		t.Error("a quantiser that varies by macroblock, which carries mb_qp_delta from row to row, is coded in rows")
	}
}
