package vaapi

import (
	"os"
	"strings"
	"testing"

	go264 "github.com/oops1/go.264"
	"github.com/oops1/go.264/internal/bits"
	"github.com/oops1/go.264/internal/nal"
	"github.com/oops1/go.264/internal/syntax"
)

func TestPicturesAreNumberedFromEachIDR(t *testing.T) {
	e := &Encoder{gopLength: 30}
	for pos := 0; pos < 65; pos++ {
		isIDR := e.beginPicture()
		since := pos % 30
		if isIDR != (since == 0) {
			t.Fatalf("picture %d: IDR %v, want %v", pos, isIDR, since == 0)
		}
		if e.frameNum != uint32(since) {
			t.Fatalf("picture %d, %d after the IDR, has frame_num %d", pos, since, e.frameNum)
		}
		if got := e.picOrderCnt(); got != int32(since) {
			t.Fatalf("picture %d, %d after the IDR, has picture order count %d", pos, since, got)
		}
		if isIDR && e.idrPicID != uint16(pos/30) {
			t.Fatalf("IDR at %d has idr_pic_id %d, want %d", pos, e.idrPicID, pos/30)
		}
		e.endPicture(isIDR)
	}
}

func headerOnlyStream(t *testing.T, pictures [][2]uint32, idr func(i int) bool) []byte {
	t.Helper()
	e := headerTestEncoder(ProfileH264Main, false, 176, 144)
	stream, err := e.parameterSets()
	if err != nil {
		t.Fatal(err)
	}
	sps, pps := e.sequenceParameterSet(), e.pictureParameterSet()
	for i, p := range pictures {
		h := &syntax.SliceHeader{SliceType: syntax.SliceP, NalRefIDC: 1, FrameNum: p[0], PicOrderCntLsb: p[1]}
		typ := nal.TypeSliceNonIDR
		if idr(i) {
			h.SliceType, h.IDR, typ = syntax.SliceI, true, nal.TypeSliceIDR
		}
		w := bits.NewWriter()
		if err := syntax.WriteSliceHeader(w, h, sps, pps); err != nil {
			t.Fatal(err)
		}
		w.WriteRBSPTrailingBits()
		stream = nal.AppendAnnexB(stream, nal.Unit{Header: nal.Header{RefIDC: 1, Type: typ}, RBSP: w.Bytes()}, true)
	}
	return stream
}

func TestSliceNumberingAcceptsACorrectSequence(t *testing.T) {
	pics := [][2]uint32{{0, 0}, {1, 1}, {2, 2}, {3, 3}, {0, 0}, {1, 1}}
	stream := headerOnlyStream(t, pics, func(i int) bool { return i == 0 || i == 4 })
	if err := checkSliceNumbering(stream); err != nil {
		t.Fatalf("a correctly numbered sequence was refused: %v", err)
	}
}

func TestSliceNumberingRefusesWhatTheFirstBackendWrote(t *testing.T) {
	cases := []struct {
		name string
		pics [][2]uint32
		idr  func(i int) bool
		says string
	}{
		{"a P frame repeating the IDR's number", [][2]uint32{{0, 0}, {0, 1}}, func(i int) bool { return i == 0 },
			"picture 1 has frame_num 0 after a reference picture numbered 0"},
		{"an IDR carrying the running count", [][2]uint32{{0, 0}, {1, 1}, {2, 2}, {3, 3}}, func(i int) bool { return i == 0 || i == 3 },
			"picture 3 is an IDR with frame_num 3"},
		{"a skipped number", [][2]uint32{{0, 0}, {1, 1}, {3, 2}}, func(i int) bool { return i == 0 },
			"must be 2"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := checkSliceNumbering(headerOnlyStream(t, c.pics, c.idr))
			if err == nil || !strings.Contains(err.Error(), c.says) {
				t.Fatalf("got %v, want something saying %q", err, c.says)
			}
		})
	}
}

func TestSliceNumberingCatchesTheRealIHDStream(t *testing.T) {
	data, err := os.ReadFile("testdata/ihd-gen9-frame-num-off-by-one.264")
	if err != nil {
		t.Fatal(err)
	}
	err = checkSliceNumbering(data)
	if err == nil || !strings.Contains(err.Error(), "picture 1 has frame_num 0") {
		t.Fatalf("the stream iHD wrote on Gen9 through the first backend gave %v", err)
	}
}

func TestSliceNumberingAcceptsTheProcessorEncoder(t *testing.T) {
	enc, err := go264.NewEncoder(go264.EncoderConfig{Width: 176, Height: 144, FPSNum: 30, FPSDen: 1,
		GOPSize: 30, QP: 22, ForceSoftware: true})
	if err != nil {
		t.Fatal(err)
	}
	defer enc.Close()
	frame := make([]byte, 176*144*3/2)
	var stream []byte
	for i := 0; i < 65; i++ {
		for j := range frame[:176*144] {
			frame[j] = byte((j + 3*i) % 251)
		}
		pkt, err := enc.Encode(frame)
		if err != nil {
			t.Fatal(err)
		}
		stream = append(stream, pkt...)
	}
	if err := checkSliceNumbering(stream); err != nil {
		t.Fatalf("our own processor encoder's stream was refused: %v", err)
	}
}

func TestACrashIsExplainedByItsAssertionNotItsRegisters(t *testing.T) {
	trace := `libva info: VA-API version 1.22.0
libva info: User environment variable requested driver 'i965'
i965_encoder.c:1692: intel_enc_hw_context_init: Assertion ` + "`encoder_context->mfc_context'" + ` failed.
SIGABRT: abort
PC=0x7f0c3a8a9eec m=0 sigcode=18446744073709551610
signal arrived during cgo execution

goroutine 1 gp=0xc000002380 m=0 mp=0x7a4c20 [syscall]:
rax    0x0
gs     0x0
`
	if got := crashReason([]byte(trace)); !strings.Contains(got, "Assertion") || !strings.Contains(got, "i965_encoder.c:1692") {
		t.Fatalf("crashReason chose %q", got)
	}
	segv := "libva info: loading\nsome_driver.c:99: bad pointer\nSIGSEGV: segmentation violation\nrip 0x0\n"
	if got := crashReason([]byte(segv)); got != "some_driver.c:99: bad pointer" {
		t.Fatalf("without an assertion crashReason chose %q", got)
	}
}

func TestAForcedKeyFrameRestartsTheGOP(t *testing.T) {
	e := &Encoder{gopLength: 30}
	want := map[int]bool{0: true, 17: true, 47: true}
	last := 0
	for pos := 0; pos < 60; pos++ {
		if pos == 17 {
			e.ForceKeyFrame()
		}
		isIDR := e.beginPicture()
		if isIDR != want[pos] {
			t.Fatalf("picture %d: IDR %v, want %v", pos, isIDR, want[pos])
		}
		if isIDR {
			last = pos
		}
		if e.frameNum != uint32(pos-last) || e.picOrderCnt() != int32(pos-last) {
			t.Fatalf("picture %d, %d after the IDR at %d: frame_num %d, picture order count %d",
				pos, pos-last, last, e.frameNum, e.picOrderCnt())
		}
		e.endPicture(isIDR)
	}
}
