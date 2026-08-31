package mf

import "testing"

func TestGUIDString(t *testing.T) {
	g := GUID{0x73646976, 0x0000, 0x0010, [8]byte{0x80, 0x00, 0x00, 0xaa, 0x00, 0x38, 0x9b, 0x71}}
	const want = "{73646976-0000-0010-8000-00AA00389B71}"
	if got := g.String(); got != want {
		t.Fatalf("String() = %s, want %s", got, want)
	}
}

func TestGUIDStringPadsShortFields(t *testing.T) {
	g := GUID{0x00000001, 0x0002, 0x0003, [8]byte{4, 5, 6, 7, 8, 9, 10, 11}}
	const want = "{00000001-0002-0003-0405-060708090A0B}"
	if got := g.String(); got != want {
		t.Fatalf("String() = %s, want %s", got, want)
	}
}

func TestMediaGUIDsMatchTheHeaderValues(t *testing.T) {
	cases := []struct {
		name string
		got  GUID
		want string
	}{
		{"MFMediaType_Video", MFMediaTypeVideo, "{73646976-0000-0010-8000-00AA00389B71}"},
		{"MFVideoFormat_H264", MFVideoFormatH264, "{34363248-0000-0010-8000-00AA00389B71}"},
		{"MFVideoFormat_NV12", MFVideoFormatNV12, "{3231564E-0000-0010-8000-00AA00389B71}"},
		{"MFVideoFormat_IYUV", MFVideoFormatIYUV, "{56555949-0000-0010-8000-00AA00389B71}"},
	}
	for _, c := range cases {
		if got := c.got.String(); got != c.want {
			t.Fatalf("%s = %s, want %s", c.name, got, c.want)
		}
	}
}

func TestFourCCGUIDSharesTheMediaSubtypeBase(t *testing.T) {
	base := fourCCGUID("")
	if base.Data1 != 0 {
		t.Fatalf("an empty code produced %08X, want 0", base.Data1)
	}
	for _, cc := range []string{"H264", "NV12", "IYUV", "vids"} {
		g := fourCCGUID(cc)
		if g.Data2 != 0x0000 || g.Data3 != 0x0010 || g.Data4 != mediaSubtypeBase {
			t.Fatalf("%s does not carry the media subtype base: %s", cc, g)
		}
		for i := 0; i < 4; i++ {
			if byte(g.Data1>>uint(8*i)) != cc[i] {
				t.Fatalf("%s encodes byte %d as %02X", cc, i, byte(g.Data1>>uint(8*i)))
			}
		}
	}
}
