package encoder

import "testing"

type goldenCase struct {
	name  string
	cfg   Config
	bytes int
}

func goldenConfig(edit func(c *Config)) Config {
	c := Config{Width: 320, Height: 240, FPSNum: 30, FPSDen: 1, GOPSize: 60, QP: 22,
		RefFrames: 1, Slices: 1}
	edit(&c)
	return c
}

func TestTheCodedSizeDoesNotMoveByAccident(t *testing.T) {
	cases := []goldenCase{
		{"cavlc, full search", goldenConfig(func(c *Config) {}), 43055},
		{"cabac, full search", goldenConfig(func(c *Config) { c.CABAC = true }), 39935},
		{"cabac, half-pel", goldenConfig(func(c *Config) {
			c.CABAC = true
			c.MotionSearch = MotionSearchHalf
		}), 39867},
		{"cabac, integer only", goldenConfig(func(c *Config) {
			c.CABAC = true
			c.MotionSearch = MotionSearchInteger
		}), 40886},
		{"cabac, no search", goldenConfig(func(c *Config) {
			c.CABAC = true
			c.MotionSearch = MotionSearchZero
		}), 227578},
		{"cabac, four slices", goldenConfig(func(c *Config) {
			c.CABAC = true
			c.Slices = 4
		}), 43905},
		{"cabac, b pictures", goldenConfig(func(c *Config) {
			c.CABAC = true
			c.BFrames = 2
			c.RefFrames = 2
		}), 77533},
		{"cabac, 8x8 transform and scaling", goldenConfig(func(c *Config) {
			c.CABAC = true
			c.Transform8x8 = true
			c.ScalingMatrix = ScalingMatrixJVT
		}), 38949},
	}

	var frames [][]byte
	for i := 0; i < 8; i++ {
		frames = append(frames, panningFrame(320, 240, i))
	}
	for _, c := range cases {
		units, _ := encodeFrames(t, c.cfg, frames)
		total := 0
		for _, u := range units {
			total += len(u)
		}
		if c.bytes == 0 {
			t.Errorf("%s: %d bytes", c.name, total)
			continue
		}
		if total != c.bytes {
			t.Errorf("%s codes to %d bytes, the recorded size is %d. If you changed the encoder on purpose, record the new size here and say in the commit what moved and why; if you did not, something changed the output that was not meant to",
				c.name, total, c.bytes)
		}
	}
}
