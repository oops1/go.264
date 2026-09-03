# go264 — the decoder on hostile input

The decoder is the only part of go264 that reads bytes it did not produce.
Everything in this document is about `internal/decoder` and the packages it
parses with: `internal/nal`, `internal/bits`, `internal/syntax`,
`internal/cavlc`, `internal/cabac`.

Spec references are to Rec. ITU-T H.264 (04/2017).

## What memory safety means here

The decoder is written in Go and neither it nor any package it depends on
imports `unsafe`. Out-of-bounds reads and writes are not possible: they are
panics, not corruption. So the threat model is availability, not integrity:

- a panic reachable from a stream is a remote crash;
- an allocation the stream chooses is a remote memory exhaustion;
- a loop the stream lengthens is a remote CPU burn.

The rest of this document is about those three.

## What it refuses

### Features it will not decode at all

`Decoder.checkSupported` rejects a sequence parameter set that asks for
chroma formats other than 4:2:0, bit depths above 8, field or MBAFF coding
(`frame_mbs_only_flag` 0), or lossless transform bypass. `ParsePPS` rejects
slice groups (`num_slice_groups_minus1` other than 0). The slice decoder
rejects SP and SI slices. These come back wrapped in
`decoder.ErrUnsupported` or `syntax.ErrUnsupported`.

### Values outside the ranges the specification gives

Every syntax element with a stated range in clause 7.4 is checked as it is
parsed, and a violation is `syntax.ErrInvalidValue`. The full list, by
clause:

| Element | Bound | Clause |
| --- | --- | --- |
| `seq_parameter_set_id` | ≤ 31 | 7.4.2.1.1 |
| `chroma_format_idc` | ≤ 3 | 7.4.2.1.1 |
| `bit_depth_luma_minus8`, `bit_depth_chroma_minus8` | ≤ 6 | 7.4.2.1.1 |
| `log2_max_frame_num_minus4` | ≤ 12 | 7.4.2.1.1 |
| `log2_max_pic_order_cnt_lsb_minus4` | ≤ 12 | 7.4.2.1.1 |
| `pic_order_cnt_type` | ≤ 2 | 7.4.2.1.1 |
| `num_ref_frames_in_pic_order_cnt_cycle` | ≤ 255 | 7.4.2.1.1 |
| `offset_for_non_ref_pic`, `offset_for_top_to_bottom_field`, `offset_for_ref_frame[i]` | ≥ −2³¹ + 1 | 7.4.2.1.1 |
| `max_num_ref_frames` | ≤ 16, and ≤ MaxDpbFrames | 7.4.2.1.1, A.3.1 h |
| `pic_width_in_mbs_minus1`, `pic_height_in_map_units_minus1` | ≤ 1023 | 7.4.2.1.1 |
| `frame_crop_left/right_offset` | `CropUnitX * (left + right) < PicWidthInSamplesL` | 7.4.2.1.1 |
| `frame_crop_top/bottom_offset` | `CropUnitY * (top + bottom) < 16 * FrameHeightInMbs` | 7.4.2.1.1 |
| `pic_parameter_set_id` | ≤ 255 | 7.4.2.2 |
| `num_ref_idx_l0/l1_default_active_minus1` | ≤ 31 | 7.4.2.2 |
| `pic_init_qp_minus26`, `pic_init_qs_minus26` | −26 … 25 | 7.4.2.2 |
| `chroma_qp_index_offset`, `second_chroma_qp_index_offset` | −12 … 12 | 7.4.2.2 |
| `cpb_cnt_minus1` | ≤ 31 | E.2.2 |
| `slice_type` | ≤ 9 | 7.4.3 |
| `idr_pic_id` | ≤ 65535 | 7.4.3 |
| `num_ref_idx_l0/l1_active_minus1` | ≤ 31 | 7.4.3 |
| `cabac_init_idc` | ≤ 2 | 7.4.3 |
| SliceQPY | 0 … 51 | 7.4.3 |
| `disable_deblocking_filter_idc` | ≤ 2 | 7.4.3 |
| `slice_alpha_c0_offset_div2`, `slice_beta_offset_div2` | −6 … 6 | 7.4.3 |
| `luma_log2_weight_denom`, `chroma_log2_weight_denom` | ≤ 7 | 7.4.3.2 |
| `luma_weight_l0/l1`, `luma_offset_l0/l1` | −128 … 127 | 7.4.3.2 |
| `chroma_weight_l0/l1`, `chroma_offset_l0/l1` | −128 … 127 | 7.4.3.2 |
| `modification_of_pic_nums_idc` | ≤ 3 | 7.4.3.1 |
| `memory_management_control_operation` | ≤ 6 | 7.4.3.3 |
| `mb_qp_delta` | −26 … 25 | 7.4.5 |
| `sub_mb_type` | ≤ 3 | 7.4.5.2 |
| `ref_idx_l0/l1` | ≤ `num_ref_idx_lX_active_minus1` | 7.4.5.1 |
| `mvd_l0/l1` components | fit in int16 | A.3.1/A.3.2 MaxVmvR ≤ 8192 luma samples |
| `total_coeff`, `total_zeros`, `run_before` | consistent with the block size | 9.2 |
| coefficient levels | fit in int16 | see below |

Coefficient levels deserve a note. CAVLC already rejected `|level|` above
32767 (`cavlc.ErrLevelRange`); CABAC did not, and its UEG0 escape is capped
at k = 24, so a level could reach roughly 2²⁵ and wrap silently in the
int32 dequantisation multiply. That was a wrong-pixels bug, not a crash —
reconstruction clips to 0…255 and every write is bounds-checked — but the
two entropy coders now agree: `cabac.Decoder` flags a level above 32767 and
the slice is refused with `decoder.ErrCorrupt`.

An `se(v)` code word whose codeNum is 2³², which decodes to −2³¹, is
refused by `bits.Reader.ReadSE` with `bits.ErrRange`. No `se(v)` element in
H.264 has −2³¹ in its range: 7.4.2.1.1 and 7.4.3 give −2³¹ + 1 as the floor
for the widest ones.

The cropping bound replaces a check that could be defeated by an integer
wrap. It used to be `CroppedWidth() > 0`, where `CroppedWidth` computes
`Width() - CropUnitX*int(left+right)` with `left` and `right` both `uint32`
read from an unbounded `ue(v)`. Setting both to 2³¹ made the sum wrap to
zero and the crop vanish, and on a 32-bit `int` the subsequent conversion
and multiply could overflow into a `CropWidth` larger than the picture,
which `go264.go` hands straight to a copy loop. The offsets are now
compared in `uint64` against the bound 7.4.2.1.1 states.

### Streams that are longer than the syntax allows

Two loops read a count from the stream and then iterate. Both are capped at
64 entries: `ref_pic_list_modification` (7.3.3.1, terminated by
`modification_of_pic_nums_idc` 3) and `dec_ref_pic_marking` (7.3.3.3,
terminated by operation 0). 64 is well above what the other bounds permit,
since a reference list holds at most 32 entries and the buffer at most 16
frames, so the cap only fires on a stream that is already invalid.

`ue(v)` reading stops after 32 leading zeros (`bits.ErrInvalidCode`), which
bounds every Exp-Golomb read to 65 bits.

Inside a slice, `first_mb_in_slice` past the last macroblock, an
`mb_skip_run` that overruns the picture, and a slice that keeps decoding
past the last macroblock are all `decoder.ErrCorrupt`. Both entropy coders
therefore visit each macroblock address at most once per slice.

### Pictures the declared level cannot carry

This is the change that matters most for allocation. The syntax bound is
1024 by 1024 macroblocks — 16384 by 16384 pixels — which is 7.5 times the
frame size of the highest level in Table A-1. `internal/level.CheckCeiling`
refuses anything above that highest frame size, always and without being
asked, because no conformant stream is ever larger and the allocation is
the whole attack. `internal/level.CheckSPS` goes further and measures the
stream against the level it declares:

- `PicWidthInMbs * FrameHeightInMbs <= MaxFS` (A.3.1 e, A.3.2 c)
- `PicWidthInMbs <= Sqrt(MaxFS * 8)` (A.3.1 f, A.3.2 d)
- `FrameHeightInMbs <= Sqrt(MaxFS * 8)` (A.3.1 g, A.3.2 e)
- `max_num_ref_frames <= Min(MaxDpbMbs / (PicWidthInMbs * FrameHeightInMbs), 16)`
  (7.4.2.1.1 with A.3.1 h)

That stricter measurement is off by default and switched on with
`Limits.EnforceLevel`. Streams that decode perfectly well while declaring a
level too small for their own picture are common in the field — ffmpeg only
warns about them — and refusing them by default would cost more than the
bounded allocation it saves, since `MaxDpbFrames` is capped at 16 either
way. With it on, a `level_idc` that is not a level in Table A-1 is refused
as well. Everything here comes back as `decoder.ErrOverLimit`.

Level 1b is written two ways, and both are recognised: `level_idc` 11 with
`constraint_set3_flag` for the Baseline, Constrained Baseline, Main and
Extended profiles, and `level_idc` 9 for the rest. The profile matters,
because in the High family `constraint_set3_flag` means the Intra profile
instead — so High profile with `level_idc` 11 and that flag set is level
1.1, not 1b. Both spellings appear in ffmpeg's own Table A-1
(`libavcodec/h264_levels.c`) with identical MaxFS and MaxDpbMbs.

Effect, measured: a 30-byte stream declaring level 1.0 and a 1024 by 1024
macroblock picture allocated 1035 MiB before this check and is now refused
having allocated nothing.

The `max_num_ref_frames` check is the one most likely to reject a stream
that would otherwise have played. Encoders that write more reference frames
than the declared level's buffer holds are writing a non-conformant stream,
but they exist. It roughly halves the worst case — without it a stream can
ask for 16 reference frames at any picture size the level allows — and a
loud refusal is better than quietly evicting references the stream still
needs. It is the check to look at first if a real stream is turned away
with `EnforceLevel` on.

### CABAC decoding past the end of the slice

The arithmetic decoder feeds itself zero bits when the reader runs dry —
that is how every H.264 decoder works, because renormalisation may reach
into the trailing bits. Without a check it also means a slice header with
no slice data drives a complete picture's worth of macroblock decoding out
of padding. `cabac.Decoder` now counts reads that fell off the end and
`runCABAC` refuses to start another macroblock once the count is non-zero.
A conformant slice never trips this, because `end_of_slice_flag` is
decoded before the data runs out; all 29 conformance clips still decode
bit-exactly.

The CAVLC path already had the equivalent property for free, since it stops
at `more_rbsp_data()`.

## What it tolerates

These are deliberate. A decoder that refuses everything non-conformant is
useless on real streams, which are routinely sloppy.

- **A picture larger than its own declared level still decodes**, unless
  the caller sets `Limits.EnforceLevel`. Only the Table A-1 ceiling is
  unconditional.
- **SEI, access unit delimiters, filler and every reserved NAL type are
  skipped without being parsed.** `Decoder.handleUnit` only dispatches
  sequence parameter sets, picture parameter sets and coded slices.
  `syntax.ParseSEI` exists and is fuzzed, but it is not on the decoder's
  attack surface.
- **Reference list modifications that name a picture the buffer does not
  hold are ignored** rather than refused (8.2.4.3.1 assumes the picture is
  present). Likewise memory management operations naming an absent picture.
- **A reference index past the end of the actual reference list falls back
  to index 0.** The index is checked against `num_ref_idx_lX_active_minus1`,
  but that value may exceed the number of pictures the buffer really holds,
  for instance after a stream skips its own IDR.
- **Gaps in `frame_num` are not detected** and no frames are synthesised
  for them (8.2.5.2 is not implemented). `gaps_in_frame_num_value_allowed_flag`
  is parsed and ignored.
- **Parameter sets may be replaced mid-stream, including with different
  dimensions.** A new sequence parameter set takes effect at the next slice
  with `first_mb_in_slice` equal to 0, which drops the decoded picture
  buffer and reallocates the picture and the macroblock grid. Slices that
  arrive with a non-zero `first_mb_in_slice` after such a change continue
  into the picture and grid that are already open, whose geometry governs
  every bound, so nothing is indexed out of range.
- **A slice arriving with no picture open** (non-zero `first_mb_in_slice`
  and nothing to continue) returns `decoder.ErrNoParameters`. A slice
  naming an absent parameter set returns `syntax.ErrMissingSPS` or
  `syntax.ErrMissingPPS`. In both cases the decoder stays usable; the
  caller may keep feeding it.
- **An error inside a slice leaves the picture half decoded, and it is
  still emitted.** `Decode` returns at the first error with whatever it has
  produced so far; the next slice with `first_mb_in_slice` equal to 0
  finishes the open picture and hands it over. Macroblocks that were never
  decoded stay at whatever the freshly allocated picture held, which is
  zero. This is error concealment, not a correctness claim: a caller that
  needs to know a picture is incomplete must watch the error from `Decode`.
- **Weighted bi-prediction with a missing weight table entry falls back to
  an unweighted average** rather than failing. This needs a `ref_idx` that
  is inside `num_ref_idx_lX_active_minus1` but outside the prediction weight
  table, which the slice header layout makes hard to construct; the
  fallback is there so it cannot become a nil dereference.
- **Motion vectors are clamped, not rejected.** A decoded vector that
  points outside the reference picture is clamped by `clampComponent` into
  the padded plane before any sample is read, so `MaxVmvR` (Table A-1) is
  not enforced as a conformance bound.

## What it allocates

Per decoded picture, at `w` by `h` macroblocks (`internal/frame`):

- luma plane `(16w + 64) * (16h + 64)` bytes, chroma planes
  `2 * (8w + 32) * (8h + 32)` — a 32 sample luma border and 16 sample
  chroma border for unclamped motion compensation;
- a motion field of `2 * 16wh * 9` bytes, allocated when the picture is
  finished, kept for temporal direct prediction;
- a macroblock grid of `wh * 648` bytes, one per picture in progress, not
  retained.

The decoder retains at most `MaxDpbFrames` reference pictures (Table A-1,
capped at 16) plus `max_num_reorder_frames` pictures awaiting output, plus
the picture being decoded.

Worst case with no caller ceiling, computed from those formulas:

| Declared level | Largest picture | Per retained picture | Retained | Total |
| --- | --- | --- | --- | --- |
| 1.0 | 176x144 (99 MBs) | 0.1 MiB | 9 | 1 MiB |
| 4.0 | 1920x1088 (8160 MBs) | 5.5 MiB | 9 | 55 MiB |
| 5.1 | 4096x2304 (36864 MBs) | 24.2 MiB | 11 | 289 MiB |
| 6.2 | 8192x4352 (139264 MBs) | 90.4 MiB | 11 | 1081 MiB |

So level 6.2 still costs about a gigabyte. That is what level 6.2 means;
the only way to spend less is to refuse it.

### Bounds that were missing

Two of these were unbounded before this work and are worth naming:

- `max_num_reorder_frames` is a `ue(v)` and was used raw as the reorder
  queue depth, so a stream could ask the decoder to hold four billion
  pictures before emitting one. E.2.1 puts it in the range 0 to
  `max_dec_frame_buffering`, which A.3.1 h bounds by `MaxDpbFrames`;
  `SPS.MaxNumReorder` now clamps to that, hence at most 16.
- Adaptive reference picture marking appended to the reference list without
  ever running the sliding window, so a stream that sets
  `adaptive_ref_pic_marking_mode_flag` on every picture and issues no
  eviction — or issues MMCO 6 with a fresh `long_term_frame_idx` each time —
  grew the buffer without limit, each entry holding a whole picture.
  8.2.5.1 requires that after marking, the number of reference frames is at
  most `Max(max_num_ref_frames, 1)`; `dpb.enforceCapacity` now drops the
  oldest entries to hold that.

`SPS.maxDpbMbs` still falls back to the largest value in the table for a
`level_idc` it does not recognise. Yielding the smallest buffer instead was
tried and reverted: `MaxDpbFrames` caps at 16 whatever the table says, so
the strict fallback bought no bound, and it cost every frame of reordering
on a stream carrying a level this table does not yet list.

## What an attacker can still spend

### CPU proportional to the declared picture size

A slice header is a few dozen bytes and commits the decoder to a picture of
the size the sequence parameter set declared. At level 6.2 that is 139264
macroblocks of prediction, transform and deblocking per coded picture. This
is not a defect — it is what decoding is — but the ratio between the bytes
the attacker sends and the work they cause is bounded only by the level.
Set `Limits.MaxFrameMBs` if the ratio matters.

### Output held from one `Decode` call

`Decode` returns every picture produced from the buffer it is handed. Feed
it a large buffer and it hands back everything at once. Measured on 200
concatenated copies of `base_intra_qp26` — 8.5 MiB of stream, 2000 coded
QCIF pictures:

- one `Decode` call: 2000 pictures returned, 263 MiB live;
- the same stream in 4 KiB chunks with each batch consumed and dropped:
  peak heap 10 MiB.

The decoder's own retention is bounded either way. The 263 MiB is output
the caller asked for. Feed the decoder in chunks and consume what it
returns.

### One unterminated NAL unit

Annex B frames a unit by the start of the next one, so a unit with no
following start code has no end, and the scanner buffers it. Memory is
proportional to what the caller feeds, but a stream can hold output
hostage indefinitely while the buffer grows. `Limits.MaxNALBytes` caps it,
checked once per `Decode` call — so it bounds growth across calls, not
within a single call whose buffer the caller chose.

Bytes that cannot begin a start code are now dropped as they are scanned,
so a stream containing no start code at all no longer accumulates: only the
last two bytes, which could still be the beginning of one, are kept.

### Scanning cost

The scanner used to compact its buffer on every unit, which is quadratic in
the number of units. Measured on a buffer of nothing but three-byte start
codes:

| Input | Before | After |
| --- | --- | --- |
| 64 KiB | 16.5 ms | 0.5 ms |
| 256 KiB | 205 ms | 1.5 ms |
| 1 MiB | 3.19 s | 5.5 ms |

It now carries a read offset and compacts only when the consumed prefix is
at least half the buffer, which is amortised linear.

## What a caller can do about it

```go
d := decoder.New()
d.SetLimits(decoder.Limits{
    MaxFrameMBs:  8192, // level 4.0, 1920x1088
    MaxNALBytes:  4 << 20,
    EnforceLevel: true,
})
```

`Limits` is empty by default, which means the highest frame size in Table
A-1 is the only ceiling.

- `MaxFrameMBs` — refuse a sequence parameter set whose picture is larger
  than this many macroblocks, whatever level it declares. Everything that
  scales with picture size — the pictures, the motion fields, the grid, and
  the per-picture decoding work — scales with this. `level.Table()` gives
  the `MaxFS` of every level if you want to express the ceiling as one.
- `EnforceLevel` — additionally hold the stream to the level it declares,
  and refuse a `level_idc` outside Table A-1. Off by default: a stream that
  understates its level is common and harmless once the ceiling is in
  place.
- `MaxNALBytes` — refuse to buffer more than this many bytes without
  reaching a NAL unit boundary.

Both report `decoder.ErrOverLimit`. Beyond that: feed `Decode` in chunks
rather than whole files, and release the pictures it returns.

Two things to know about reaching this from outside the module:

- `decoder.Limits` sits on the package-internal decoder. The public
  `go264.DecoderConfig` does not carry it yet, so an external caller gets
  the level-derived default and nothing else. Plumbing the two fields
  through `DecoderConfig` is a small change and the obvious next step;
  `go264.go` was outside the scope of this work.
- `go264.NewDecoderWithConfig` prefers a hardware decoder when one is
  available and only falls back to this code. Nothing in this document
  describes what a platform decoder does with hostile input.

## Fuzzing

Every fuzz target in the repository is seeded from the 29 conformance
clips: the decoder target from the whole streams and from each NAL unit on
its own, the syntax targets from the sequence parameter sets, picture
parameter sets, slice headers and SEI messages actually extracted from
them, the NAL targets from the streams and their units, and the bits and
CAVLC targets from real slice payloads. `internal/testutil/bitstream.go`
does the extraction, without importing any package under test.

The decoder target sets a caller ceiling of 1620 macroblocks and 1 MiB per
NAL unit, so that the fuzzer spends its time on decoding logic rather than
on memset. The level check and the allocation bounds are covered by
`internal/decoder/hardening_test.go` instead.

Checked-in regression inputs live under each package's
`testdata/fuzz/<Target>/`.

## Regression tests

- `internal/decoder/hardening_test.go` — the level and caller ceilings, the
  reorder clamp, the reference buffer cap under adaptive and long-term
  marking, the CABAC end-of-data check, the scanner cost and the scanner's
  buffer release.
- `internal/level/check_test.go` — `CheckSPS` at each level's own maximum
  and one macroblock beyond it, level 1b resolution, and the buffer size
  derivation.
- `internal/syntax/hardening_test.go` — the `se(v)` floor, the reorder
  clamp, the level 1b profile rule, and the prediction weight ranges.
- `internal/nal/bounds_test.go` — scanner cost, buffer release, and that
  the rewritten scanner splits identically to `SplitAnnexB` at every chunk
  size.
- `internal/bits/bits_test.go` — `TestReadSERejectsMinInt32`.
