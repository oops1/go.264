package encoder

import (
	"math/rand"
	"testing"

	"github.com/oops1/go.264/internal/syntax"
	"github.com/oops1/go.264/internal/testutil"
	"github.com/oops1/go.264/internal/transform"
)

func flatScale4x4() (*scale4x4, *scale4x4) {
	ls := buildLevelScale4x4(transform.FlatWeightScale4x4)
	qs := buildQuantScale4x4(transform.FlatWeightScale4x4)
	return &ls, &qs
}

func TestDerivedFlatTablesMatchTheTransformPackage(t *testing.T) {
	ls, qs := flatScale4x4()
	rng := rand.New(rand.NewSource(9))
	for qp := 0; qp <= 51; qp++ {
		for _, intra := range []bool{false, true} {
			for trial := 0; trial < 8; trial++ {
				var a, b transform.Block
				for i := range a {
					v := int32(rng.Intn(4001) - 2000)
					a[i], b[i] = v, v
				}
				transform.Quant4x4(&a, qp, intra)
				quant4x4(&b, qp, qs, intra)
				if a != b {
					t.Fatalf("qp %d intra %v: the derived quantiser differs from the transform package\n%v\n%v",
						qp, intra, a, b)
				}

				for i := range a {
					v := int32(rng.Intn(401) - 200)
					a[i], b[i] = v, v
				}
				var c, d transform.Block
				c, d = a, b
				transform.Dequant4x4(&a, qp, false)
				dequant4x4(&b, qp, ls, false)
				if a != b {
					t.Fatalf("qp %d: the derived dequantiser differs from the transform package\n%v\n%v", qp, a, b)
				}
				transform.Dequant4x4(&c, qp, true)
				dequant4x4(&d, qp, ls, true)
				if c != d {
					t.Fatalf("qp %d: the derived dequantiser differs when the DC is held\n%v\n%v", qp, c, d)
				}

				for i := range a {
					v := int32(rng.Intn(2001) - 1000)
					a[i], b[i] = v, v
				}
				transform.QuantLumaDC(&a, qp, intra)
				quantLumaDC(&b, qp, qs, intra)
				if a != b {
					t.Fatalf("qp %d intra %v: the derived luma DC quantiser differs\n%v\n%v", qp, intra, a, b)
				}
				transform.DequantLumaDC(&a, qp)
				dequantLumaDC(&b, qp, ls)
				if a != b {
					t.Fatalf("qp %d: the derived luma DC dequantiser differs\n%v\n%v", qp, a, b)
				}

				var ca, cb transform.ChromaDC
				for i := range ca {
					v := int32(rng.Intn(2001) - 1000)
					ca[i], cb[i] = v, v
				}
				transform.QuantChromaDC(&ca, qp, intra)
				quantChromaDC(&cb, qp, qs, intra)
				if ca != cb {
					t.Fatalf("qp %d intra %v: the derived chroma DC quantiser differs\n%v\n%v", qp, intra, ca, cb)
				}
				transform.DequantChromaDC(&ca, qp)
				dequantChromaDC(&cb, qp, ls)
				if ca != cb {
					t.Fatalf("qp %d: the derived chroma DC dequantiser differs\n%v\n%v", qp, ca, cb)
				}
			}
		}
	}
}

func TestFlatMatrixIsTheDefaultAndChangesNothing(t *testing.T) {
	cfg := Config{Width: 96, Height: 80, FPSNum: 25, FPSDen: 1, GOPSize: 4, QP: 27, CABAC: true}
	enc, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if enc.PPS().PicScalingMatrixPresent {
		t.Fatal("the picture parameter set carries a scaling matrix although none was asked for")
	}
	ls, qs := flatScale4x4()
	for i := 0; i < 6; i++ {
		if enc.level4x4[i] != *ls || enc.quant4x4[i] != *qs {
			t.Fatalf("list %d is not flat by default", i)
		}
	}
}

func TestJVTMatrixLandsInThePictureParameterSet(t *testing.T) {
	for _, eight := range []bool{false, true} {
		cfg := Config{Width: 176, Height: 144, FPSNum: 25, FPSDen: 1, GOPSize: 4, QP: 26,
			ScalingMatrix: ScalingMatrixJVT, Transform8x8: eight}
		enc, err := New(cfg)
		if err != nil {
			t.Fatal(err)
		}
		sps, pps := enc.SPS(), enc.PPS()
		if sps.ProfileIDC != syntax.ProfileHigh {
			t.Fatalf("profile_idc is %d, want High for a custom matrix", sps.ProfileIDC)
		}
		if sps.SeqScalingMatrixPresent {
			t.Fatal("the matrix went into the sequence parameter set, but x264 always writes it into the picture one")
		}
		if !pps.PicScalingMatrixPresent {
			t.Fatal("pic_scaling_matrix_present_flag is not set")
		}
		for i := 0; i < 6; i++ {
			if !pps.ScalingList4x4Present[i] || !pps.UseDefaultScaling4x4[i] {
				t.Fatalf("4x4 list %d is present %v with use-default %v",
					i, pps.ScalingList4x4Present[i], pps.UseDefaultScaling4x4[i])
			}
		}
		for i := 0; i < 2; i++ {
			if pps.ScalingList8x8Present[i] != eight {
				t.Fatalf("8x8 list %d present is %v with transform_8x8_mode_flag %v",
					i, pps.ScalingList8x8Present[i], eight)
			}
		}

		headers, err := enc.Headers()
		if err != nil {
			t.Fatal(err)
		}
		rt := parseHeaders(t, headers)
		list4, list8 := rt.pps.ResolvedScalingLists(rt.sps)
		for i := 0; i < 6; i++ {
			want := transform.DefaultScalingList4x4Inter
			if i < 3 {
				want = transform.DefaultScalingList4x4Intra
			}
			if rasterOrder4x4(list4[i]) != want {
				t.Fatalf("re-reading our own headers gave a different 4x4 list %d", i)
			}
		}
		if !eight {
			continue
		}
		for i := 0; i < 2; i++ {
			want := transform.DefaultScalingList8x8Inter
			if i == 0 {
				want = transform.DefaultScalingList8x8Intra
			}
			if rasterOrder8x8(list8[i]) != want {
				t.Fatalf("re-reading our own headers gave a different 8x8 list %d", i)
			}
		}
	}
}

func TestJVTMatrixRoundTrips(t *testing.T) {
	for _, cabac := range []bool{false, true} {
		for _, eight := range []bool{false, true} {
			for _, qp := range []int{0, 20, 33, 51} {
				cfg := Config{Width: 176, Height: 144, FPSNum: 25, FPSDen: 1, GOPSize: 4, QP: qp,
					RefFrames: 2, CABAC: cabac, Transform8x8: eight, ScalingMatrix: ScalingMatrixJVT}
				var frames [][]byte
				for i := 0; i < 6; i++ {
					frames = append(frames, syntheticFrame(cfg.Width, cfg.Height, i))
				}
				pics, _, recons := encodeAndDecode(t, cfg, frames)
				assertMatchesReconstruction(t, label("jvt", cabac, qp), pics, recons)
			}
		}
	}
}

func TestJVTMatrixRoundTripsWithBFrames(t *testing.T) {
	for _, cabac := range []bool{false, true} {
		cfg := Config{Width: 176, Height: 144, FPSNum: 25, FPSDen: 1, GOPSize: 8, QP: 28,
			RefFrames: 2, BFrames: 2, CABAC: cabac, Transform8x8: true, ScalingMatrix: ScalingMatrixJVT}
		var frames [][]byte
		for i := 0; i < 9; i++ {
			frames = append(frames, syntheticFrame(cfg.Width, cfg.Height, i))
		}
		pics, recons := encodeAndDecodeReordered(t, cfg, frames)
		assertMatchesReconstruction(t, label("jvt bipredictive", cabac, 28), pics, recons)
	}
}

func TestFFmpegDecodesOurJVTMatrixStream(t *testing.T) {
	for _, cabac := range []bool{false, true} {
		for _, eight := range []bool{false, true} {
			for _, qp := range []int{0, 22, 36, 51} {
				cfg := Config{Width: 176, Height: 144, FPSNum: 25, FPSDen: 1, GOPSize: 4, QP: qp,
					RefFrames: 2, CABAC: cabac, Transform8x8: eight, ScalingMatrix: ScalingMatrixJVT}
				var frames [][]byte
				for i := 0; i < 5; i++ {
					frames = append(frames, syntheticFrame(cfg.Width, cfg.Height, i))
				}
				stream := encodeStream(t, cfg, frames)
				ref := decodeWithFFmpeg(t, stream)
				frameSize := cfg.Width * cfg.Height * 3 / 2
				if len(ref) != frameSize*len(frames) {
					t.Fatalf("cabac %v 8x8 %v qp %d: ffmpeg produced %d bytes, want %d",
						cabac, eight, qp, len(ref), frameSize*len(frames))
				}
				pics, _, _ := encodeAndDecode(t, cfg, frames)
				for i := range pics {
					got := make([]byte, pics[i].Size())
					pics[i].CopyOut(got)
					want := ref[i*frameSize : (i+1)*frameSize]
					for j := range got {
						if got[j] != want[j] {
							t.Fatalf("cabac %v 8x8 %v qp %d frame %d: ffmpeg and our decoder disagree at sample %d, ffmpeg %d ours %d",
								cabac, eight, qp, i, j, want[j], got[j])
						}
					}
				}
			}
		}
	}
}

func TestJVTMatrixShiftsBitsTowardsLowFrequencies(t *testing.T) {
	if testing.Short() {
		t.Skip("the measurement encodes several streams")
	}
	clip := testutil.Clip{Name: "main_cif_pyramid", Width: 352, Height: 288, Frames: 8}
	all := testutil.LoadReferenceYUV(t, clip)
	var frames [][]byte
	for i := 0; i < clip.Frames; i++ {
		frames = append(frames, clip.Frame(all, i))
	}
	for _, eight := range []bool{false, true} {
		for _, qp := range []int{22, 32} {
			flat := Config{Width: clip.Width, Height: clip.Height, FPSNum: 25, FPSDen: 1,
				GOPSize: clip.Frames, QP: qp, RefFrames: 2, CABAC: true, Transform8x8: eight}
			jvt := flat
			jvt.ScalingMatrix = ScalingMatrixJVT
			a := measure(t, flat, frames)
			b := measure(t, jvt, frames)

			pics, _, recons := encodeAndDecode(t, jvt, frames)
			assertMatchesReconstruction(t, "jvt real", pics, recons)

			t.Logf("8x8 %v qp %d: flat %d bytes %.2f dB, jvt %d bytes %.2f dB (%+.1f%% bits, %+.2f dB)",
				eight, qp, a.bytes, a.psnr, b.bytes, b.psnr,
				100*float64(b.bytes-a.bytes)/float64(a.bytes), b.psnr-a.psnr)
		}
	}
}
