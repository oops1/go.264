package encoder

import (
	"testing"

	"github.com/oops1/go.264/internal/frame"
)

func assertFFmpegAgrees(t *testing.T, label string, cfg Config, units [][]byte, pics []*frame.Picture) {
	t.Helper()
	var stream []byte
	for _, u := range units {
		stream = append(stream, u...)
	}
	ref := decodeWithFFmpeg(t, stream)
	frameSize := cfg.Width * cfg.Height * 3 / 2
	if len(ref) != frameSize*len(pics) {
		t.Fatalf("%s: ffmpeg produced %d bytes, want %d", label, len(ref), frameSize*len(pics))
	}
	for i := range pics {
		got := make([]byte, pics[i].Size())
		pics[i].CopyOut(got)
		want := ref[i*frameSize : (i+1)*frameSize]
		for j := range got {
			if got[j] != want[j] {
				t.Fatalf("%s frame %d: ffmpeg and our decoder disagree at sample %d, ffmpeg %d ours %d",
					label, i, j, want[j], got[j])
			}
		}
	}
	t.Logf("%s: %d bytes, %d frames decode identically in ffmpeg", label, len(stream), len(pics))
}

func TestFFmpegDecodesIntraRefreshStreams(t *testing.T) {
	skipUnderRace(t)
	for _, cabac := range []bool{false, true} {
		for _, period := range trim([]int{4, 9}, 1) {
			for _, qp := range trim([]int{18, 30, 42}, 1) {
				cfg := refreshConfig(qp, period)
				cfg.CABAC = cabac
				frames := screenSequence(cfg.Width, cfg.Height, 20)
				units, _ := encodeUnits(t, cfg, frames)
				assertFFmpegAgrees(t, "intra refresh", cfg, units, decodeUnits(t, units))
			}
		}
	}
}

func TestFFmpegDecodesIntraRefreshWithSlicesAndTransform8x8(t *testing.T) {
	skipUnderRace(t)
	cases := []struct {
		name string
		edit func(c *Config)
	}{
		{"slices", func(c *Config) { c.Slices = 4 }},
		{"slices cabac", func(c *Config) { c.Slices = 4; c.CABAC = true }},
		{"transform 8x8", func(c *Config) { c.Transform8x8 = true; c.CABAC = true }},
		{"three references", func(c *Config) { c.RefFrames = 3 }},
	}
	for _, tc := range trim(cases, 2) {
		cfg := refreshConfig(27, 6)
		tc.edit(&cfg)
		frames := panningSequence(cfg.Width, cfg.Height, 18)
		units, _ := encodeUnits(t, cfg, frames)
		assertFFmpegAgrees(t, "intra refresh "+tc.name, cfg, units, decodeUnits(t, units))
	}
}

func TestFFmpegDecodesDeblockingSettings(t *testing.T) {
	skipUnderRace(t)
	for _, mode := range []DeblockMode{DeblockingOn, DeblockingOff, DeblockingNotAcrossSlices} {
		for _, offsets := range trim([][2]int{{0, 0}, {-6, 6}, {6, -6}, {3, 3}}, 1) {
			for _, cabac := range trim([]bool{false, true}, 1) {
				cfg := deblockConfig(29)
				cfg.Slices = 3
				cfg.CABAC = cabac
				cfg.Deblocking = mode
				cfg.DeblockAlphaOffset = offsets[0]
				cfg.DeblockBetaOffset = offsets[1]
				frames := screenSequence(cfg.Width, cfg.Height, 8)
				units, _ := encodeUnits(t, cfg, frames)
				assertFFmpegAgrees(t, "deblocking", cfg, units, decodeUnits(t, units))
			}
		}
	}
}

func TestFFmpegDecodesDeblockingWithBPictures(t *testing.T) {
	skipUnderRace(t)
	for _, mode := range []DeblockMode{DeblockingOff, DeblockingNotAcrossSlices} {
		cfg := deblockConfig(28)
		cfg.CABAC = true
		cfg.BFrames = 2
		cfg.Slices = 2
		cfg.Deblocking = mode
		cfg.DeblockAlphaOffset = -4
		cfg.DeblockBetaOffset = 5
		frames := screenSequence(cfg.Width, cfg.Height, 12)
		stream, _ := encodeBStream(t, cfg, frames, nil)
		assertFFmpegAgrees(t, "deblocking with B pictures", cfg,
			[][]byte{stream}, decodeUnits(t, [][]byte{stream}))
	}
}

func TestFFmpegSeesIntraRefreshConverge(t *testing.T) {
	skipUnderRace(t)
	for _, src := range trim(convergenceSources, 1) {
		for _, period := range trim([]int{5, 9}, 1) {
			cfg := refreshConfig(26, period)
			const count = 40
			frames := src.make(cfg.Width, cfg.Height, count)
			units, _ := encodeUnits(t, cfg, frames)

			frameSize := cfg.Width * cfg.Height * 3 / 2
			full := decodeWithFFmpeg(t, flattenUnits(units))
			if len(full) != frameSize*count {
				t.Fatalf("ffmpeg produced %d bytes for %d frames", len(full), count)
			}

			for _, start := range []int{1 + period, 1 + 2*period} {
				joined := decodeWithFFmpeg(t, joinStream(t, cfg, units, start))
				want := count - start + 1
				if len(joined) != frameSize*want {
					t.Fatalf("ffmpeg produced %d bytes for %d joined frames", len(joined), want)
				}
				dirty := false
				for i := 1; i < want; i++ {
					at := start + i - 1
					a := joined[i*frameSize : (i+1)*frameSize]
					b := full[at*frameSize : (at+1)*frameSize]
					same := true
					for j := range a {
						if a[j] != b[j] {
							same = false
							break
						}
					}
					switch {
					case at >= start+period-1 && !same:
						t.Fatalf("%s, period %d, recovery point %d: ffmpeg still disagrees at frame %d after a full sweep",
							src.name, period, start, at)
					case at < start+period-1 && !same:
						dirty = true
					}
				}
				if !dirty {
					t.Fatalf("%s, period %d, recovery point %d: ffmpeg matched from the first frame, so the test proves nothing",
						src.name, period, start)
				}
			}
			t.Logf("%s, period %d: ffmpeg reaches the reference decode %d frames after a recovery point",
				src.name, period, period-1)
		}
	}
}
