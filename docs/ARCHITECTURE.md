# go264 — Architecture

H.264/AVC encoder and decoder in Go. CPU-only, with SIMD acceleration via
Go assembly (generated with `avo`) and pure-Go fallbacks selected at runtime.

Consumer: winline 1.3.0. Distributed as a standalone library in its own
GitHub repository.

## Goals

- Encoder: Constrained Baseline profile (I/P slices, CAVLC), CQP first,
  then average-bitrate rate control. Input: YUV 4:2:0 planar frames.
  Output: Annex-B byte stream.
- Decoder: Baseline + Main profile bitstreams (CAVLC required; CABAC and
  B-slices staged after Baseline is complete). Input: Annex-B or
  length-prefixed NAL units. Output: YUV 4:2:0 frames.
- Strictly CGO-free (`CGO_ENABLED=0` must build and pass all tests).
  Works everywhere Go works; AVX2/SSE4 fast paths on amd64, NEON
  considered later for arm64.
- Hardware acceleration when available: probe platform decoders/encoders
  at runtime and prefer them; fall back to the CPU SIMD path
  transparently. Hardware backends load system libraries dynamically
  (Windows syscall / purego dlopen), never via cgo.
- Maximum practical test coverage: golden vectors, conformance streams,
  asm-vs-generic equivalence, round-trip PSNR, fuzzing.

## Non-goals (for 1.3.0)

- High profile tools (8x8 transform, monochrome, 4:2:2/4:4:4).
- Interlaced coding (MBAFF/PAFF), FMO/ASO, SP/SI slices.
- Multi-threaded slice/frame parallelism (single-threaded correctness
  first; parallelism is an additive later step).

## Package layout

    go264/
      go.mod                     module github.com/oops1/go.264
      go264.go                   public API: Encoder, Decoder, Frame, params
      internal/bits/             bit reader/writer, Exp-Golomb (ue/se/te)
      internal/nal/              NAL unit framing, RBSP emulation prevention
      internal/syntax/           SPS, PPS, slice header parse + write
      internal/cavlc/            CAVLC residual coding (read + write)
      internal/cabac/            CABAC (decoder side, staged)
      internal/pred/             intra 4x4 / 16x16 / chroma prediction modes
      internal/mc/               inter motion compensation, 6-tap luma
                                 interpolation, chroma bilinear, quarter-pel
      internal/transform/        4x4 integer transform, Hadamard, (de)quant
      internal/deblock/          in-loop deblocking filter
      internal/dpb/              decoded picture buffer, reference lists,
                                 POC computation, sliding window / MMCO
      internal/me/               motion estimation: SAD/SATD, diamond +
                                 hexagon search, sub-pel refinement
      internal/rc/               rate control: CQP, then ABR
      internal/hwaccel/          hardware codec backends + capability probe:
        hwaccel.go               backend interface, probe, selection order
        mf_windows.go            Windows Media Foundation MFTs via syscall
                                 (covers Intel QSV, NVDEC/NVENC, AMD VCN
                                 through vendor MFTs, D3D11 surfaces)
        vaapi_linux.go           VA-API via purego dlopen (libva)
        vt_darwin.go             VideoToolbox via purego (staged, optional)
      internal/simd/             dispatch table + kernels:
        dispatch.go              runtime CPUID selection (x/sys/cpu)
        generic.go               pure-Go reference implementations
        *_amd64.s, *_amd64.go    avo-generated AVX2/SSE4 kernels
      internal/simd/asmgen/      avo generator programs (go:generate)
      testdata/                  golden bitstreams, YUV vectors, conformance
      cmd/go264/                 CLI: encode/decode YUV <-> .264 for manual
                                 testing and benchmarking

## Data flow

Decoder:

    Annex-B -> nal (unescape RBSP) -> syntax (SPS/PPS/slice header)
      -> cavlc/cabac (residuals, MB layer) -> pred | mc (prediction)
      -> transform (dequant + inverse) -> reconstruct -> deblock
      -> dpb (reorder, output) -> Frame

Encoder:

    Frame -> me (motion search, P) / pred mode decision (I)
      -> transform (forward + quant) -> cavlc (entropy write)
      -> syntax + nal (escape RBSP, Annex-B) -> bytes
    reconstruction path mirrors the decoder (dequant -> inverse ->
    deblock -> dpb) so references match bit-exactly

## Hardware acceleration strategy

The public Encoder/Decoder are facades over two engines:

    NewEncoder/NewDecoder -> probe hwaccel backends -> hardware engine
                                       | (unavailable / init error /
                                       |  unsupported parameters)
                                       v
                              CPU engine (SIMD)

- Probing happens per instance, never at package init; a probe failure
  is silent and only downgrades to CPU. `Config.ForceSoftware` pins the
  CPU path for testing and reproducibility.
- All hardware bindings are CGO-free: on Windows through
  `golang.org/x/sys/windows` syscalls into Media Foundation COM; on
  Linux through `purego` dlopen of libva; missing libraries simply fail
  the probe.
- The CPU path is the reference: every stream an encoder backend emits
  must decode with the CPU decoder in tests, and hardware decoder output
  is PSNR-compared against CPU decoder output on the same stream.
- CI has no GPUs, so hardware backends are covered by mock-based unit
  tests (interface conformance, probe/fallback logic) plus a manual
  `go264 selftest` CLI command for real-hardware verification.

## SIMD strategy

Hot kernels behind a dispatch table, chosen once at init from CPU
features. Every kernel has a pure-Go generic twin; tests assert bit-exact
equivalence between generic and SIMD on randomized inputs.

Kernel set (amd64, AVX2 with SSE4 fallback):

- SAD 16x16/16x8/8x8/4x4, SATD (Hadamard-based)
- Luma 6-tap half-pel interpolation, quarter-pel averaging
- Chroma bilinear interpolation
- Forward/inverse 4x4 transform + quant/dequant
- Deblocking filter edges (bs<4 and bs=4 paths)
- Plane copy / pixel averaging

Assembly is not written by hand: `internal/simd/asmgen` holds avo
programs; `go generate ./...` regenerates the `.s` files, which are
committed. CI verifies generated files are in sync.

## Public API sketch

    type Frame struct { Y, Cb, Cr []byte; StrideY, StrideC, Width, Height int }

    type EncoderConfig struct {
        Width, Height int
        FPSNum, FPSDen int
        GOPSize int          // I-frame interval
        QP int               // CQP mode
        BitrateKbps int      // 0 = CQP, >0 = ABR
        ForceSoftware bool   // skip hardware probe, use CPU engine
    }

    enc.Backend()             // reports "cpu", "mediafoundation", "vaapi"

    enc, err := go264.NewEncoder(cfg)
    pkt, err := enc.Encode(frame)     // returns Annex-B access unit
    pkts, err := enc.Flush()

    dec := go264.NewDecoder()
    frames, err := dec.Decode(annexB) // zero or more decoded frames
    frames, err := dec.Flush()

## Testing

- Unit tests per package, table-driven, against values computed from the
  ITU-T H.264 spec text.
- Golden-vector tests: decode reference conformance bitstreams
  (JM/openh264 suites, Baseline set) and compare YUV output hashes.
- Round-trip: encode synthetic and natural YUV, decode with our decoder,
  assert PSNR floor per QP; also validate our streams with an external
  reference decoder in CI where available.
- Equivalence: generic vs SIMD kernels on randomized inputs, all sizes.
- Fuzzing: native Go fuzzing on NAL/syntax/CAVLC/decoder entry points;
  corpus seeded from conformance streams.
- Coverage gate in CI (target: >=90% overall, 100% on bits/nal/syntax).
- Hostile input: what the decoder refuses, what it tolerates and what it
  allocates in the worst case is written up in
  [DECODER-HARDENING.md](DECODER-HARDENING.md), along with the
  `decoder.Limits` ceilings a caller can set.

## Tooling and conventions

- License: BSD-2-Clause (same family as openh264; permissive, standard
  for codec libraries).
- No comments in code; names and tests carry the intent.
- CI: GitHub Actions — gofmt, go vet, staticcheck, race tests, coverage,
  asm regeneration check, fuzz smoke run.
- Commits: authored by the repository owner only, message in English,
  imperative mood, no trailers (no Co-Authored-By or similar tags).
