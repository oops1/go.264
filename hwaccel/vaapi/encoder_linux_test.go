package vaapi

import (
	"errors"
	"testing"
	"time"
	"unsafe"

	"github.com/oops1/go.264/internal/level"
	"github.com/oops1/go.264/internal/syntax"
)

func requireAdapter(t *testing.T) {
	t.Helper()
	if !Available() {
		t.Skip("libva.so.2 or libva-drm.so.2 is not available on this machine")
	}
	if _, err := openDisplay(); err != nil {
		t.Skipf("no VA-API render node could be opened: %v", err)
	}
}

func TestConfigValidation(t *testing.T) {
	cases := []struct {
		name string
		cfg  Config
		ok   bool
	}{
		{"zero width", Config{Width: 0, Height: 144, FPSNum: 25, FPSDen: 1}, false},
		{"odd width", Config{Width: 177, Height: 144, FPSNum: 25, FPSDen: 1}, false},
		{"odd height", Config{Width: 176, Height: 145, FPSNum: 25, FPSDen: 1}, false},
		{"zero fps num", Config{Width: 176, Height: 144, FPSNum: 0, FPSDen: 1}, false},
		{"zero fps den", Config{Width: 176, Height: 144, FPSNum: 25, FPSDen: 0}, false},
		{"not mb aligned but even", Config{Width: 1920, Height: 1080, FPSNum: 25, FPSDen: 1}, true},
		{"mb aligned", Config{Width: 176, Height: 144, FPSNum: 25, FPSDen: 1}, true},
	}
	for _, c := range cases {
		err := c.cfg.valid()
		if c.ok && err != nil {
			t.Errorf("%s: valid() = %v, want nil", c.name, err)
		}
		if !c.ok && err == nil {
			t.Errorf("%s: valid() = nil, want an error", c.name)
		}
	}
}

func TestMbAlign(t *testing.T) {
	cases := []struct{ in, want int }{
		{0, 0}, {1, 16}, {15, 16}, {16, 16}, {17, 32}, {1080, 1088}, {1920, 1920},
	}
	for _, c := range cases {
		if got := mbAlign(c.in); got != c.want {
			t.Errorf("mbAlign(%d) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestClampQP(t *testing.T) {
	cases := []struct {
		in   int
		want uint8
	}{
		{0, 26}, {-5, 26}, {1, 1}, {51, 51}, {52, 51}, {26, 26},
	}
	for _, c := range cases {
		if got := clampQP(c.in); got != c.want {
			t.Errorf("clampQP(%d) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestOpenRejectsBadConfig(t *testing.T) {
	if _, err := Open(Config{Width: 177, Height: 144, FPSNum: 25, FPSDen: 1}); err == nil {
		t.Fatal("Open accepted an odd width")
	}
}

func TestEncoderProbeWithoutAdapter(t *testing.T) {
	if Available() {
		t.Skip("libva is available on this machine; this test only covers the no-adapter path")
	}
	if _, err := Open(Config{Width: 176, Height: 144, FPSNum: 25, FPSDen: 1}); err == nil {
		t.Fatal("Open succeeded without libva.so.2 present")
	}
}

const (
	probeWidth  = 176
	probeHeight = 144
	probeFrames = 12
)

func probeConfig() Config {
	return Config{Width: probeWidth, Height: probeHeight, FPSNum: 25, FPSDen: 1, GOPLength: 30, QP: 26}
}

func movingPattern(i int) []byte {
	f := make([]byte, I420Size(probeWidth, probeHeight))
	for y := 0; y < probeHeight; y++ {
		for x := 0; x < probeWidth; x++ {
			f[y*probeWidth+x] = byte((x*3 + y*2 + i*9) & 0xFF)
		}
	}
	cw, ch := probeWidth/2, probeHeight/2
	cb := probeWidth * probeHeight
	cr := cb + cw*ch
	for y := 0; y < ch; y++ {
		for x := 0; x < cw; x++ {
			f[cb+y*cw+x] = byte(96 + (x+i)%64)
			f[cr+y*cw+x] = byte(160 - (y+i)%64)
		}
	}
	return f
}

func TestEncoderProducesAStream(t *testing.T) {
	requireAdapter(t)
	enc, err := Open(probeConfig())
	if err != nil {
		t.Skipf("the encoder could not be opened: %v", err)
	}
	defer enc.Close()

	var stream []byte
	for i := 0; i < probeFrames; i++ {
		out, err := enc.Encode(movingPattern(i))
		if err != nil {
			t.Fatalf("%s: frame %d: %v", enc.Name(), i, err)
		}
		stream = append(stream, out...)
	}
	if len(stream) == 0 {
		t.Fatalf("%s produced no bytes from %d frames", enc.Name(), probeFrames)
	}
	if len(stream) < 4 || stream[0] != 0 || stream[1] != 0 || stream[2] != 0 || stream[3] != 1 {
		t.Fatalf("%s did not begin with a start code", enc.Name())
	}
}

func TestEncoderRefusesShortFrames(t *testing.T) {
	requireAdapter(t)
	enc, err := Open(probeConfig())
	if err != nil {
		t.Skipf("the encoder could not be opened: %v", err)
	}
	defer enc.Close()
	if _, err := enc.Encode(make([]byte, 16)); err == nil {
		t.Fatal("a frame far shorter than the configured picture was accepted")
	}
}

func TestCloseIsSafeTwiceAndStopsFurtherWork(t *testing.T) {
	requireAdapter(t)
	enc, err := Open(probeConfig())
	if err != nil {
		t.Skipf("the encoder could not be opened: %v", err)
	}
	if err := enc.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := enc.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
	if _, err := enc.Encode(movingPattern(0)); err == nil {
		t.Fatal("a closed encoder accepted a frame")
	}
}

func TestProfileIDCCoversEveryCandidate(t *testing.T) {
	want := map[Profile]uint8{
		ProfileH264ConstrainedBaseline: syntax.ProfileBaseline,
		ProfileH264Main:                syntax.ProfileMain,
		ProfileH264High:                syntax.ProfileHigh,
	}
	for _, cand := range candidateProfiles {
		if got := profileIDC(cand.profile); got != want[cand.profile] {
			t.Errorf("VA profile %d maps to profile_idc %d, want %d", cand.profile, got, want[cand.profile])
		}
	}
}

func TestPickLevelIDCFollowsThePicture(t *testing.T) {
	cases := []struct {
		name    string
		cfg     Config
		profile Profile
		want    uint8
	}{
		{"quarter CIF", Config{Width: 176, Height: 144, FPSNum: 25, FPSDen: 1}, ProfileH264Main, 11},
		{"CIF", Config{Width: 352, Height: 288, FPSNum: 25, FPSDen: 1}, ProfileH264Main, 13},
		{"standard definition", Config{Width: 640, Height: 480, FPSNum: 30, FPSDen: 1},
			ProfileH264ConstrainedBaseline, 30},
		{"720p", Config{Width: 1280, Height: 720, FPSNum: 30, FPSDen: 1}, ProfileH264Main, 31},
		{"1080p", Config{Width: 1920, Height: 1080, FPSNum: 30, FPSDen: 1}, ProfileH264Main, 40},
		{"1080p at 60 in High", Config{Width: 1920, Height: 1080, FPSNum: 60, FPSDen: 1}, ProfileH264High, 42},
		{"2160p", Config{Width: 3840, Height: 2160, FPSNum: 30, FPSDen: 1}, ProfileH264Main, 51},
	}
	for _, c := range cases {
		got, err := pickLevelIDC(c.cfg, c.profile)
		if err != nil {
			t.Errorf("%s: pickLevelIDC = %v", c.name, err)
			continue
		}
		if got != c.want {
			t.Errorf("%s: %dx%d at %d/%d frames per second announces level %d, want %d",
				c.name, c.cfg.Width, c.cfg.Height, c.cfg.FPSNum, c.cfg.FPSDen, got, c.want)
		}
	}
}

func TestPickLevelIDCCarriesWhatTheSequenceAnnounces(t *testing.T) {
	cases := []Config{
		{Width: 176, Height: 144, FPSNum: 25, FPSDen: 1},
		{Width: 1280, Height: 720, FPSNum: 30, FPSDen: 1},
		{Width: 1920, Height: 1080, FPSNum: 30, FPSDen: 1},
		{Width: 3840, Height: 2160, FPSNum: 30, FPSDen: 1},
	}
	for _, cfg := range cases {
		for _, cand := range candidateProfiles {
			idc, err := pickLevelIDC(cfg, cand.profile)
			if err != nil {
				t.Errorf("%dx%d on VA profile %d: pickLevelIDC = %v", cfg.Width, cfg.Height, cand.profile, err)
				continue
			}
			l, ok := level.Lookup(idc)
			if !ok {
				t.Errorf("%dx%d: level %d is not one the table defines", cfg.Width, cfg.Height, idc)
				continue
			}
			frameMBs := mbAlign(cfg.Width) / 16 * (mbAlign(cfg.Height) / 16)
			if frameMBs > l.MaxFS {
				t.Errorf("%dx%d: level %d allows %d macroblocks, the picture holds %d",
					cfg.Width, cfg.Height, idc, l.MaxFS, frameMBs)
			}
			if rate := frameMBs * cfg.FPSNum / cfg.FPSDen; rate > l.MaxMBPS {
				t.Errorf("%dx%d: level %d allows %d macroblocks per second, the stream runs %d",
					cfg.Width, cfg.Height, idc, l.MaxMBPS, rate)
			}
			if held := l.MaxDpbMbs / frameMBs; held < maxNumRefFrames {
				t.Errorf("%dx%d: level %d holds %d frames, the sequence announces %d references",
					cfg.Width, cfg.Height, idc, held, maxNumRefFrames)
			}
			announced := int64(roughBitsPerSecond(cfg.Width, cfg.Height, cfg.FPSNum, cfg.FPSDen))
			if peak := l.MaxBitsPerSecond(profileIDC(cand.profile)); announced > peak {
				t.Errorf("%dx%d: level %d allows %d bit/s, the sequence announces %d",
					cfg.Width, cfg.Height, idc, peak, announced)
			}
		}
	}
}

func TestPickLevelIDCLeavesTheCeilingForStreamsThatNeedIt(t *testing.T) {
	idc, err := pickLevelIDC(Config{Width: 1920, Height: 1080, FPSNum: 25, FPSDen: 1}, ProfileH264Main)
	if err != nil {
		t.Fatalf("pickLevelIDC = %v", err)
	}
	top := level.Table()[len(level.Table())-1]
	if idc >= top.IDC {
		t.Fatalf("1080p at 25 frames per second announces level %d, the table ends at level %d",
			idc, top.IDC)
	}
	t.Logf("1080p at 25 frames per second announces level %d", idc)
}

func TestPickLevelIDCRefusesAPictureNoLevelCarries(t *testing.T) {
	cfg := Config{Width: 16384, Height: 16384, FPSNum: 30, FPSDen: 1}
	idc, err := pickLevelIDC(cfg, ProfileH264Main)
	if err == nil {
		t.Fatalf("a picture above every level announced level %d", idc)
	}
	if !errors.Is(err, level.ErrNoLevel) {
		t.Fatalf("the error is %v, want one wrapping level.ErrNoLevel", err)
	}
	t.Logf("%v", err)
}

func TestReadCodedBufferRejectsASegmentSizeLargerThanTheAllocatedBuffer(t *testing.T) {
	e := &Encoder{disp: &display{handle: 0xDEAD}, mbWidth: 2, mbHeight: 2}
	capacity := e.codedBufferCapacity()

	payload := make([]byte, 4)
	seg := CodedBufferSegment{
		Size: uint32(capacity) + 1,
		Buf:  unsafe.Pointer(&payload[0]),
	}

	restoreMap, restoreUnmap := vaMapBuffer, vaUnmapBuffer
	defer func() { vaMapBuffer, vaUnmapBuffer = restoreMap, restoreUnmap }()

	unmapped := false
	vaMapBuffer = func(_ uintptr, _ uint32, out *unsafe.Pointer) int32 {
		*out = unsafe.Pointer(&seg)
		return int32(StatusSuccess)
	}
	vaUnmapBuffer = func(uintptr, uint32) int32 {
		unmapped = true
		return int32(StatusSuccess)
	}

	if _, err := e.readCodedBuffer(0); err == nil {
		t.Fatal("readCodedBuffer accepted a segment larger than the allocated buffer, want an error")
	} else {
		t.Logf("%v", err)
	}
	if !unmapped {
		t.Fatal("readCodedBuffer did not unmap the buffer on the rejected-segment path")
	}
}

func TestReadCodedBufferAcceptsASegmentWithinCapacity(t *testing.T) {
	e := &Encoder{disp: &display{handle: 0xDEAD}, mbWidth: 2, mbHeight: 2}

	want := []byte{1, 2, 3, 4, 5}
	seg := CodedBufferSegment{
		Size: uint32(len(want)),
		Buf:  unsafe.Pointer(&want[0]),
	}

	restoreMap, restoreUnmap := vaMapBuffer, vaUnmapBuffer
	defer func() { vaMapBuffer, vaUnmapBuffer = restoreMap, restoreUnmap }()

	vaMapBuffer = func(_ uintptr, _ uint32, out *unsafe.Pointer) int32 {
		*out = unsafe.Pointer(&seg)
		return int32(StatusSuccess)
	}
	vaUnmapBuffer = func(uintptr, uint32) int32 { return int32(StatusSuccess) }

	got, err := e.readCodedBuffer(0)
	if err != nil {
		t.Fatalf("readCodedBuffer: %v", err)
	}
	if string(got) != string(want) {
		t.Fatalf("readCodedBuffer returned %v, want %v", got, want)
	}
}

func TestReadCodedBufferRejectsASegmentChainLongerThanTheLimit(t *testing.T) {
	e := &Encoder{disp: &display{handle: 0xDEAD}, mbWidth: 2, mbHeight: 2}

	var seg CodedBufferSegment
	seg.Next = unsafe.Pointer(&seg)

	restoreMap, restoreUnmap := vaMapBuffer, vaUnmapBuffer
	defer func() { vaMapBuffer, vaUnmapBuffer = restoreMap, restoreUnmap }()

	vaMapBuffer = func(_ uintptr, _ uint32, out *unsafe.Pointer) int32 {
		*out = unsafe.Pointer(&seg)
		return int32(StatusSuccess)
	}
	vaUnmapBuffer = func(uintptr, uint32) int32 { return int32(StatusSuccess) }

	done := make(chan error, 1)
	go func() {
		_, err := e.readCodedBuffer(0)
		done <- err
	}()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("readCodedBuffer accepted a cyclic segment chain, want an error")
		}
		t.Logf("%v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("readCodedBuffer did not terminate on a cyclic segment chain")
	}
}
