package encoder

import (
	"fmt"
	"image"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/oops1/go.264/internal/bits"
	"github.com/oops1/go.264/internal/frame"
	"github.com/oops1/go.264/internal/nal"
	"github.com/oops1/go.264/internal/syntax"
)

func movingFrame(w, h, t int) []byte {
	buf := make([]byte, w*h*3/2)
	bx := 20 + (t*3)%(w-60)
	by := 15 + (t*2)%(h-50)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			v := 60 + (x*x/97+y*y/89)%80
			if x >= bx && x < bx+40 && y >= by && y < by+35 {
				v = 200 - (x-bx+y-by)%40
			}
			buf[y*w+x] = byte(v)
		}
	}
	cw, ch := w/2, h/2
	for i := 0; i < 2; i++ {
		base := w*h + i*cw*ch
		for y := 0; y < ch; y++ {
			for x := 0; x < cw; x++ {
				v := 128
				if 2*x >= bx && 2*x < bx+40 && 2*y >= by && 2*y < by+35 {
					v = 100 + i*50
				}
				buf[base+y*cw+x] = byte(v)
			}
		}
	}
	return buf
}

func rectAt(i, width int) image.Rectangle {
	y := (i * 16) % 96
	return image.Rect(0, y, width, y+32)
}

func bConfig(w, h, qp, bframes, gop int, cabac bool) Config {
	return Config{Width: w, Height: h, FPSNum: 25, FPSDen: 1, GOPSize: gop, QP: qp,
		RefFrames: 2, BFrames: bframes, CABAC: cabac}
}

type reconRecord struct {
	display int
	pic     *frame.Picture
}

func encodeBStream(t *testing.T, cfg Config, frames [][]byte, hints []Hints) ([]byte, []*frame.Picture) {
	t.Helper()
	enc, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	var records []reconRecord
	enc.onPicture = func(display int, rec *frame.Picture) {
		records = append(records, reconRecord{display: display, pic: snapshotOf(cfg, rec)})
	}
	var stream []byte
	for i, f := range frames {
		h := Hints{}
		if hints != nil {
			h = hints[i]
		}
		pkt, err := enc.EncodeWithHints(f, h)
		if err != nil {
			t.Fatalf("frame %d: %v", i, err)
		}
		stream = append(stream, pkt...)
	}
	tail, err := enc.Flush()
	if err != nil {
		t.Fatalf("Flush: %v", err)
	}
	stream = append(stream, tail...)

	if len(records) != len(frames) {
		t.Fatalf("the encoder produced %d pictures for %d input frames", len(records), len(frames))
	}
	sort.SliceStable(records, func(i, j int) bool { return records[i].display < records[j].display })
	recons := make([]*frame.Picture, 0, len(records))
	for i, r := range records {
		if r.display != i {
			t.Fatalf("picture %d carries display index %d", i, r.display)
		}
		recons = append(recons, r.pic)
	}
	return stream, recons
}

func sliceTypesOf(t *testing.T, stream []byte) []syntax.SliceType {
	t.Helper()
	var out []syntax.SliceType
	for _, ebsp := range nal.SplitAnnexB(stream) {
		u, err := nal.Parse(ebsp)
		if err != nil {
			t.Fatalf("parsing our own unit: %v", err)
		}
		if u.Header.Type != nal.TypeSliceIDR && u.Header.Type != nal.TypeSliceNonIDR {
			continue
		}
		r := bits.NewReader(u.RBSP)
		first, err := r.ReadUE()
		if err != nil {
			t.Fatalf("reading first_mb_in_slice: %v", err)
		}
		st, err := r.ReadUE()
		if err != nil {
			t.Fatalf("reading slice_type: %v", err)
		}
		if first != 0 {
			continue
		}
		out = append(out, syntax.SliceType(st).Base())
	}
	return out
}

func TestBFramesAppearInTheExpectedOrder(t *testing.T) {
	cfg := bConfig(176, 144, 26, 2, 6, false)
	var frames [][]byte
	for i := 0; i < 7; i++ {
		frames = append(frames, syntheticFrame(cfg.Width, cfg.Height, i))
	}
	stream, _ := encodeBStream(t, cfg, frames, nil)
	got := sliceTypesOf(t, stream)
	want := []syntax.SliceType{
		syntax.SliceI, syntax.SliceP, syntax.SliceB, syntax.SliceB,
		syntax.SliceP, syntax.SliceP, syntax.SliceI,
	}
	if len(got) != len(want) {
		t.Fatalf("the stream carries %d pictures, want %d: %v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("picture %d in coding order is %s, want %s (got %v)", i, got[i], want[i], got)
		}
	}
}

func TestBFramesEmitNothingUntilTheAnchorArrives(t *testing.T) {
	cfg := bConfig(176, 144, 26, 2, 100, false)
	enc, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	sizes := make([]int, 0, 5)
	for i := 0; i < 5; i++ {
		pkt, err := enc.Encode(syntheticFrame(cfg.Width, cfg.Height, i))
		if err != nil {
			t.Fatalf("frame %d: %v", i, err)
		}
		sizes = append(sizes, len(pkt))
	}
	if sizes[0] == 0 {
		t.Fatal("the key frame produced no bytes")
	}
	if sizes[1] != 0 || sizes[2] != 0 {
		t.Fatalf("frames 1 and 2 produced %d and %d bytes, but they must wait for their anchor",
			sizes[1], sizes[2])
	}
	if sizes[3] == 0 {
		t.Fatal("the anchor frame flushed no bytes")
	}
	tail, err := enc.Flush()
	if err != nil {
		t.Fatalf("Flush: %v", err)
	}
	if len(tail) == 0 {
		t.Fatal("Flush left the queued frame unencoded")
	}
}

func TestBFramesRoundTrip(t *testing.T) {
	for _, cabac := range []bool{false, true} {
		for _, bframes := range []int{1, 2, 3} {
			for _, qp := range []int{0, 26, 45} {
				cfg := bConfig(176, 144, qp, bframes, 8, cabac)
				var frames [][]byte
				for i := 0; i < 11; i++ {
					frames = append(frames, syntheticFrame(cfg.Width, cfg.Height, i))
				}
				stream, recons := encodeBStream(t, cfg, frames, nil)
				label := fmt.Sprintf("cabac %v, %d B frames, qp %d", cabac, bframes, qp)
				assertMatchesReconstruction(t, label, decodeUnits(t, [][]byte{stream}), recons)
			}
		}
	}
}

func TestBFramesRoundTripAcrossPictureSizes(t *testing.T) {
	for _, size := range [][2]int{{100, 60}, {176, 144}, {96, 96}} {
		cfg := bConfig(size[0], size[1], 28, 2, 7, true)
		var frames [][]byte
		for i := 0; i < 9; i++ {
			frames = append(frames, syntheticFrame(cfg.Width, cfg.Height, i))
		}
		stream, recons := encodeBStream(t, cfg, frames, nil)
		label := fmt.Sprintf("%dx%d with B frames", size[0], size[1])
		assertMatchesReconstruction(t, label, decodeUnits(t, [][]byte{stream}), recons)
	}
}

func TestBFramesRoundTripWithoutMotionSearch(t *testing.T) {
	for _, cabac := range []bool{false, true} {
		cfg := bConfig(176, 144, 28, 2, 8, cabac)
		cfg.MotionSearch = MotionSearchZero
		var frames [][]byte
		for i := 0; i < 9; i++ {
			frames = append(frames, syntheticFrame(cfg.Width, cfg.Height, i))
		}
		stream, recons := encodeBStream(t, cfg, frames, nil)
		label := fmt.Sprintf("cabac %v without motion search", cabac)
		assertMatchesReconstruction(t, label, decodeUnits(t, [][]byte{stream}), recons)
	}
}

func TestBFramesRoundTripWithSlices(t *testing.T) {
	for _, cabac := range []bool{false, true} {
		for _, slices := range []int{2, 4} {
			cfg := bConfig(320, 240, 27, 2, 9, cabac)
			cfg.Slices = slices
			var frames [][]byte
			for i := 0; i < 10; i++ {
				frames = append(frames, syntheticFrame(cfg.Width, cfg.Height, i))
			}
			stream, recons := encodeBStream(t, cfg, frames, nil)
			label := fmt.Sprintf("cabac %v, %d slices with B frames", cabac, slices)
			assertMatchesReconstruction(t, label, decodeUnits(t, [][]byte{stream}), recons)
		}
	}
}

func TestBFramesRoundTripWithHints(t *testing.T) {
	cfg := bConfig(176, 144, 30, 2, 12, true)
	frames := [][]byte{syntheticFrame(cfg.Width, cfg.Height, 0)}
	hints := []Hints{{}}
	for i := 1; i < 9; i++ {
		frames = append(frames, syntheticFrame(cfg.Width, cfg.Height, i))
		hints = append(hints, Hints{Changed: nil,
			Regions: []Region{{Rect: rectAt(i, cfg.Width), Kind: RegionText}}})
	}
	stream, recons := encodeBStream(t, cfg, frames, hints)
	assertMatchesReconstruction(t, "B frames with hints", decodeUnits(t, [][]byte{stream}), recons)
}

func TestForcedKeyFrameClosesTheBGroup(t *testing.T) {
	cfg := bConfig(176, 144, 26, 3, 100, false)
	enc, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	var records []reconRecord
	enc.onPicture = func(display int, rec *frame.Picture) {
		records = append(records, reconRecord{display: display, pic: snapshotOf(cfg, rec)})
	}
	var stream []byte
	for i := 0; i < 6; i++ {
		if i == 3 {
			enc.ForceKeyFrame()
		}
		pkt, err := enc.Encode(syntheticFrame(cfg.Width, cfg.Height, i))
		if err != nil {
			t.Fatalf("frame %d: %v", i, err)
		}
		stream = append(stream, pkt...)
	}
	tail, err := enc.Flush()
	if err != nil {
		t.Fatalf("Flush: %v", err)
	}
	stream = append(stream, tail...)

	if len(records) != 6 {
		t.Fatalf("the encoder produced %d pictures for 6 input frames", len(records))
	}
	sort.SliceStable(records, func(i, j int) bool { return records[i].display < records[j].display })
	recons := make([]*frame.Picture, 0, len(records))
	for _, r := range records {
		recons = append(recons, r.pic)
	}
	assertMatchesReconstruction(t, "forced key frame", decodeUnits(t, [][]byte{stream}), recons)

	types := sliceTypesOf(t, stream)
	want := []syntax.SliceType{
		syntax.SliceI, syntax.SliceP, syntax.SliceP,
		syntax.SliceI, syntax.SliceP, syntax.SliceP,
	}
	if len(types) != len(want) {
		t.Fatalf("the stream carries %d pictures, want %d: %v", len(types), len(want), types)
	}
	for i := range want {
		if types[i] != want[i] {
			t.Fatalf("the forced key frame did not close the group, coding order was %v", types)
		}
	}
}

func fadeFrames(t *testing.T) [][]byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "fade.yuv"))
	if err != nil {
		t.Skipf("fade.yuv is not available: %v", err)
	}
	size := 176 * 144 * 3 / 2
	var out [][]byte
	for i := 0; i+size <= len(data); i += size {
		out = append(out, data[i:i+size])
	}
	return out
}

func TestBFramesSaveBitsAgainstPOnly(t *testing.T) {
	frames := fadeFrames(t)
	for _, cabac := range []bool{false, true} {
		for _, qp := range []int{22, 30, 38} {
			base := bConfig(176, 144, qp, 0, 100, cabac)
			base.RefFrames = 2
			withB := bConfig(176, 144, qp, 2, 100, cabac)

			plain, _ := encodeBStream(t, base, frames, nil)
			bstream, _ := encodeBStream(t, withB, frames, nil)
			saving := 100 * float64(len(plain)-len(bstream)) / float64(len(plain))
			t.Logf("cabac %v, qp %d: P only %d bytes, with B frames %d bytes, %.1f%% saved",
				cabac, qp, len(plain), len(bstream), saving)
			if len(bstream) >= len(plain) {
				t.Errorf("cabac %v, qp %d: B frames cost %d bytes against %d for P only",
					cabac, qp, len(bstream), len(plain))
			}
		}
	}
}

func TestBFramesOnSyntheticMotionAreReported(t *testing.T) {
	var frames [][]byte
	for i := 0; i < 25; i++ {
		frames = append(frames, movingFrame(176, 144, i))
	}
	for _, qp := range []int{22, 30, 38} {
		base := bConfig(176, 144, qp, 0, 100, true)
		base.RefFrames = 2
		withB := bConfig(176, 144, qp, 2, 100, true)
		plain, _ := encodeBStream(t, base, frames, nil)
		bstream, _ := encodeBStream(t, withB, frames, nil)
		t.Logf("rigid translation, qp %d: P only %d bytes, with B frames %d bytes, %.1f%% saved",
			qp, len(plain), len(bstream),
			100*float64(len(plain)-len(bstream))/float64(len(plain)))
	}
}

func TestFFmpegDecodesOurBFrameStreams(t *testing.T) {
	for _, cabac := range []bool{false, true} {
		for _, bframes := range []int{1, 2, 3} {
			for _, qp := range []int{18, 30} {
				cfg := bConfig(176, 144, qp, bframes, 8, cabac)
				var frames [][]byte
				for i := 0; i < 11; i++ {
					frames = append(frames, movingFrame(cfg.Width, cfg.Height, i))
				}
				stream, recons := encodeBStream(t, cfg, frames, nil)
				ref := decodeWithFFmpeg(t, stream)
				frameSize := cfg.Width * cfg.Height * 3 / 2
				if len(ref) != frameSize*len(frames) {
					t.Fatalf("cabac %v, %d B frames, qp %d: ffmpeg produced %d bytes, want %d",
						cabac, bframes, qp, len(ref), frameSize*len(frames))
				}
				for i := range recons {
					got := make([]byte, recons[i].Size())
					recons[i].CopyOut(got)
					want := ref[i*frameSize : (i+1)*frameSize]
					for j := range got {
						if got[j] != want[j] {
							t.Fatalf("cabac %v, %d B frames, qp %d, frame %d: ffmpeg and our reconstruction disagree at sample %d, ffmpeg %d ours %d",
								cabac, bframes, qp, i, j, want[j], got[j])
						}
					}
				}
				t.Logf("cabac %v, %d B frames, qp %d: %d bytes decode identically in ffmpeg",
					cabac, bframes, qp, len(stream))
			}
		}
	}
}

func TestConfigRejectsTooManyBFrames(t *testing.T) {
	if _, err := New(Config{Width: 64, Height: 64, QP: 26, BFrames: 8}); err == nil {
		t.Fatal("BFrames 8 was accepted")
	}
	if _, err := New(Config{Width: 64, Height: 64, QP: 26, BFrames: -1}); err == nil {
		t.Fatal("a negative BFrames was accepted")
	}
}

func TestBFramesForceTwoReferenceFrames(t *testing.T) {
	enc, err := New(Config{Width: 64, Height: 64, QP: 26, BFrames: 2, RefFrames: 1})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if enc.cfg.RefFrames < 2 {
		t.Fatalf("RefFrames stayed at %d although B frames were asked for", enc.cfg.RefFrames)
	}
	if enc.sps.PicOrderCntType != 0 {
		t.Fatalf("pic_order_cnt_type is %d, want 0 so that pictures may be reordered",
			enc.sps.PicOrderCntType)
	}
	if enc.sps.Log2MaxPicOrderCntLsbMinus4 != 8 {
		t.Fatalf("log2_max_pic_order_cnt_lsb_minus4 is %d, want 8",
			enc.sps.Log2MaxPicOrderCntLsbMinus4)
	}
	if enc.sps.ProfileIDC != syntax.ProfileMain {
		t.Fatalf("profile_idc is %d, but B slices need Main", enc.sps.ProfileIDC)
	}
}
