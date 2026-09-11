package vaapi

import (
	"math/rand"
	"strings"
	"testing"
	"unsafe"

	go264 "github.com/oops1/go.264"
)

func stubEntrypoints(t *testing.T, offered map[Profile][]Entrypoint) {
	t.Helper()
	restoreMax, restoreQuery := vaMaxNumEntrypoints, vaQueryConfigEntrypoints
	t.Cleanup(func() { vaMaxNumEntrypoints, vaQueryConfigEntrypoints = restoreMax, restoreQuery })
	vaMaxNumEntrypoints = func(uintptr) int32 { return 8 }
	vaQueryConfigEntrypoints = func(_ uintptr, profile int32, out unsafe.Pointer, n *int32) int32 {
		list, ok := offered[Profile(profile)]
		if !ok {
			return int32(StatusErrorOperationFailed)
		}
		dst := unsafe.Slice((*int32)(out), int(*n))
		for i, e := range list {
			dst[i] = int32(e)
		}
		*n = int32(len(list))
		return int32(StatusSuccess)
	}
}

func TestTheFullEncodeEntrypointIsPreferredAndLowPowerIsTheFallback(t *testing.T) {
	const vld Entrypoint = 1
	cases := []struct {
		name    string
		offered map[Profile][]Entrypoint
		profile Profile
		entry   Entrypoint
	}{
		{
			"i965 on Gen9 offers both",
			map[Profile][]Entrypoint{
				ProfileH264Main: {vld, EntrypointEncSlice, EntrypointEncSliceLP},
				ProfileH264High: {vld, EntrypointEncSlice, EntrypointEncSliceLP},
			},
			ProfileH264Main, EntrypointEncSlice,
		},
		{
			"the free iHD build on Gen9 offers only low power",
			map[Profile][]Entrypoint{
				ProfileH264Main:                {vld, EntrypointEncSliceLP},
				ProfileH264High:                {vld, EntrypointEncSliceLP},
				ProfileH264ConstrainedBaseline: {vld, EntrypointEncSliceLP},
			},
			ProfileH264Main, EntrypointEncSliceLP,
		},
		{
			"a full entry point on another profile beats low power on the first",
			map[Profile][]Entrypoint{
				ProfileH264Main: {vld, EntrypointEncSliceLP},
				ProfileH264High: {vld, EntrypointEncSlice},
			},
			ProfileH264High, EntrypointEncSlice,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			stubEntrypoints(t, c.offered)
			choice, entry, err := (&display{handle: 1}).findEncodeProfile()
			if err != nil {
				t.Fatalf("findEncodeProfile: %v", err)
			}
			if choice.profile != c.profile || entry != c.entry {
				t.Fatalf("chose profile %d with %s, want profile %d with %s",
					choice.profile, entry, c.profile, c.entry)
			}
		})
	}
}

func TestADecodeOnlyDriverOffersNoEncoder(t *testing.T) {
	stubEntrypoints(t, map[Profile][]Entrypoint{ProfileH264Main: {1}, ProfileH264High: {1}})
	if _, _, err := (&display{handle: 1}).findEncodeProfile(); err == nil {
		t.Fatal("a driver that only decodes was taken for an encoder")
	}
}

func encodedTestFrame(t *testing.T, width, height int, luma byte) []byte {
	t.Helper()
	enc, err := go264.NewEncoder(go264.EncoderConfig{Width: width, Height: height, FPSNum: 30, FPSDen: 1,
		GOPSize: 30, QP: 22, ForceSoftware: true})
	if err != nil {
		t.Fatal(err)
	}
	defer enc.Close()
	frame := make([]byte, width*height*3/2)
	for i := range frame {
		frame[i] = 128
	}
	for i := 0; i < width*height; i++ {
		frame[i] = luma
	}
	out, err := enc.Encode(frame)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestTheTestFrameCheckAcceptsAProperGreyFrame(t *testing.T) {
	if err := checkTestFrame(encodedTestFrame(t, 176, 144, 128), 176, 144); err != nil {
		t.Fatalf("a correct grey frame was refused: %v", err)
	}
}

func TestTheTestFrameCheckRefusesWhatADriverMightGetWrong(t *testing.T) {
	garbage := make([]byte, 4096)
	rand.New(rand.NewSource(1)).Read(garbage)
	cases := []struct {
		name   string
		stream []byte
		width  int
		height int
		says   string
	}{
		{"nothing at all", nil, 176, 144, "no coded data"},
		{"the wrong picture size", encodedTestFrame(t, 176, 144, 128), 320, 240, "decoded at 176x144"},
		{"the wrong picture", encodedTestFrame(t, 176, 144, 20), 176, 144, "mean luma"},
		{"bytes that are not a stream", garbage, 176, 144, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := checkTestFrame(c.stream, c.width, c.height)
			if err == nil {
				t.Fatal("accepted")
			}
			if c.says != "" && !strings.Contains(err.Error(), c.says) {
				t.Fatalf("refused, but with %q rather than something about %q", err, c.says)
			}
		})
	}
}
