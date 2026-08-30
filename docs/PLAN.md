# go264 — Work Plan

Target: encoder + decoder ready for winline 1.3.0.

Every phase ends with green tests and a commit series in English with no
trailers. Status below reflects what is actually merged and measured.

## Done

### Phase 0 — Bootstrap
Module `github.com/oops1/go264`, BSD-2-Clause, GitHub Actions running
gofmt, vet, staticcheck, race tests, coverage, a cgo-import rejection
check and cross-target `CGO_ENABLED=0` builds.

### Phase 1 — Bitstream foundation
`internal/bits` (99% covered) and `internal/nal` (100%): bit reader and
writer, Exp-Golomb, RBSP trailing bits, emulation prevention, Annex B
splitting for whole buffers and for streamed chunks, AVCC framing.

### Phase 2 — Syntax
`internal/syntax` (87%): SPS, VUI, HRD, PPS and slice header, parse and
write, with byte-identical round trips and fuzz targets.

### Reference corpus
Five Constrained Baseline streams produced by libx264 plus the frames
ffmpeg decodes from them, checked in under `testdata/conformance`. This is
the ground truth every decoding stage is measured against.

### Phase 3 — Decoder, intra
`internal/transform` (100%), `internal/pred` (100%), `internal/cavlc`
(92%), `internal/deblock` (100%) and the macroblock layer. All ten frames
of both intra reference clips decode to exactly the bytes ffmpeg produces,
with and without the loop filter.

### Phase 4 — Decoder, inter
Decoded picture buffer with picture order count, reference lists, sliding
window and MMCO; six tap luma and bilinear chroma interpolation
(`internal/mc`, 100%); motion vector prediction; P_Skip; every P partition
including 8x8 sub-macroblocks. The whole corpus decodes bit-exactly.

### Phase 5 and 6 — Encoder
`internal/encoder` (96%): parameter set construction, Intra_4x4,
Intra_16x16 and chroma mode search, motion estimation with sub-sample
refinement, skip decision, run length coding of skipped macroblocks.
ffmpeg decodes our streams to exactly what our own decoder produces.

### Public interface
Root package `go264` with `Encoder`, `Decoder`, `Frame` and backend
reporting, plus the `go264` command line tool for encoding and decoding
raw I420.

## Remaining

### Phase 7 — Rate control
Average bitrate on top of the constant quantiser path: frame level
quantiser adaptation, a buffer model, scene cut detection for IDR
placement. Today only `QP` is honoured; `BitrateKbps` is not yet
implemented.

### Phase 8 — SIMD
`internal/simd` with a runtime dispatch table and pure Go fallbacks, and
avo generator programs under `internal/simd/asmgen`. Kernels to cover: SAD
and SATD, the six tap interpolation, forward and inverse transforms,
quantisation, the deblocking edge filters and plane copies. Every kernel
gets a randomised equivalence test against its generic twin, and CI checks
the generated assembly is in sync.

### Phase 9 — Encoder partitions and quality
16x8, 8x16 and 8x8 inter partitions in the encoder, multiple reference
frames, and a rate-distortion mode decision to replace the current
transformed-difference heuristic.

### Phase 10 — Decoder, Main profile
CABAC, B slices and weighted prediction as decoder side tools, measured
against a Main profile corpus generated the same way as the current one.

### Phase 11 — Hardware acceleration
The probe, backend registry and transparent fallback are in place in
`internal/hwaccel`; no backend is implemented yet. Next: Windows Media
Foundation through `golang.org/x/sys/windows` syscalls into the COM
interfaces, then Linux VA-API through `purego` dlopen. Both stay CGO-free.
Cross-validation: any stream a hardware encoder emits must decode with the
CPU decoder, and hardware decoder output is compared against CPU decoder
output on the same stream. CI has no GPU, so the registry and fallback get
mock-based tests and a `go264 selftest` command covers real hardware.

### Phase 12 — Release
API review and freeze, godoc pass, benchmarks in the README, coverage
audit, fuzz soak, tag v0.1.0 and integration notes for winline 1.3.0.

## Standing rules

- `CGO_ENABLED=0` everywhere, including tests and CI.
- No comments in code.
- Commits authored by the repository owner, English imperative messages,
  no trailers of any kind, one logical change per commit.
- New code lands with its tests in the same commit.
- Numeric tables from the specification are transcribed from an
  authoritative source and validated structurally, never written from
  memory. That practice caught seven wrong entries in one CAVLC table and
  a chroma table whose Kraft sum exceeded one.
