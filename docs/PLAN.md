# go264 — Work Plan

Target: encoder + decoder ready for winline 1.3.0. Order is strict:
each phase ends with green tests, coverage held at the gate, and a
commit series in English with no trailers.

## Phase 0 — Bootstrap

- Init repo, `go.mod` (module github.com/oops1/go264), BSD-2-Clause
  LICENSE, README skeleton, .gitignore.
- GitHub Actions: build with `CGO_ENABLED=0`, gofmt check, go vet,
  staticcheck, `go test -race -coverprofile`, coverage gate.
- Create GitHub repository, push initial commit.

Exit: CI green on empty-but-wired module.

## Phase 1 — Bitstream foundation

- `internal/bits`: bit reader/writer, ue(v)/se(v)/te(v) Exp-Golomb,
  byte alignment, more-RBSP-data, trailing bits.
- `internal/nal`: Annex-B start-code scanning, RBSP unescape/escape
  (emulation prevention), NAL header parse/write.
- Fuzz targets for reader and unescape; 100% coverage on both packages.

Exit: symmetric read/write property tests pass (write->read identity).

## Phase 2 — Syntax layer

- `internal/syntax`: SPS, PPS parse + write; slice header parse + write
  for I/P slices (Baseline fields; ref reordering + MMCO syntax).
- Golden tests against hex dumps of known-good SPS/PPS (from reference
  encoders); fuzz the parsers.

Exit: our written SPS/PPS re-parse to identical structs and match
reference hex dumps.

## Phase 3 — Decoder: intra path

- `internal/transform`: dequant, inverse 4x4 transform, DC Hadamard.
- `internal/pred`: intra 4x4 (9 modes), 16x16 (4 modes), chroma
  (4 modes), availability rules.
- `internal/cavlc`: coeff_token tables, residual decoding, nC context.
- Macroblock layer for I slices, reconstruction, `internal/deblock`.
- Decode I-only conformance streams; per-tool unit tests from spec
  worked examples.

Exit: bit-exact YUV vs reference decoder on I-only Baseline streams.

## Phase 4 — Decoder: inter path + DPB

- `internal/dpb`: POC, reference list init/reorder, sliding window,
  MMCO, output reordering.
- `internal/mc`: 6-tap luma half-pel, quarter-pel, chroma bilinear;
  P-skip, P_16x16..P_8x8 partitions, MV prediction.
- Decode full Baseline conformance suite.

Exit: bit-exact YUV on Baseline conformance set; fuzz corpus extended.

## Phase 5 — Encoder: intra

- Forward transform + quant; intra mode decision (SATD-based); CAVLC
  writing; MB encoding for I slices; CQP; reconstruction loop shared
  with decoder code.
- Round-trip tests: encode -> our decoder -> PSNR floor per QP;
  streams validated against an external reference decoder in CI.

Exit: valid I-frame streams at all QPs, monotonic rate/QP behavior.

## Phase 6 — Encoder: inter

- `internal/me`: SAD/SATD, integer search (diamond/hexagon), sub-pel
  refinement, MV cost; P slices, skip detection, mode decision I-vs-P.
- GOP structure (IDR interval), single reference to start.

Exit: P-frame streams decode bit-exact in our decoder and reference
decoder; compression vs I-only measured and recorded in benchmarks.

## Phase 7 — Rate control

- `internal/rc`: ABR over CQP base (frame-level QP adaptation, VBV-ish
  buffer model), scene-cut IDR insertion.

Exit: achieved bitrate within tolerance on test sequences.

## Phase 8 — SIMD

- `internal/simd`: dispatch table, generic kernels extracted from
  hot paths; avo generator programs; AVX2 + SSE4 kernels for SAD/SATD,
  interpolation, transforms, quant, deblock.
- Randomized equivalence tests generic-vs-asm; benchmarks; CI check
  that `go generate` output is committed.

Exit: measurable speedup, zero behavior change (bit-exact outputs).

## Phase 9 — Decoder: Main profile extras

- CABAC decoding, B-slices, weighted prediction as decoder-only tools.
- Extend conformance set to Main profile streams.

Exit: Main conformance streams pass.

## Phase 10 — Hardware acceleration

- `internal/hwaccel` interface + probe + fallback plumbing, CGO-free.
- Windows Media Foundation encoder+decoder backend (syscall COM).
- Linux VA-API decoder backend via purego; encoder if stable.
- `go264 selftest` CLI for on-hardware verification; mock-based tests
  for probe/fallback in CI; cross-validation hardware-vs-CPU output.

Exit: on machines with hardware codecs the facade selects them and
falls back cleanly; `Backend()` reporting correct.

## Phase 11 — Release

- API review and freeze, godoc pass, README with usage + benchmarks.
- Coverage audit to the gate (>=90% total), fuzz soak.
- Tag v0.1.0; integration notes for winline 1.3.0.

## Standing rules (every phase)

- `CGO_ENABLED=0` everywhere, including tests and CI.
- No comments in code.
- Commits: author is the repository owner, English imperative messages,
  no trailers of any kind (no Co-Authored-By etc.), one logical change
  per commit.
- New code lands with its tests in the same commit.
