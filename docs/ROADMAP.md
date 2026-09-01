# go264 — Roadmap

Where the codec stands after v1.1.0, what comes next, and why in that
order. [PLAN.md](PLAN.md) records what was built and how; this document
looks forward. Both are kept honest by the same rule: nothing is called
done until it is measured against an independent implementation.

winline is the customer. Its plan asks for an encoder for machines with
no usable video adapter, and that is what decides the order below. Work
the customer does not need today is still built, but it waits behind the
work it does need, and anything it does not want is switchable off rather
than absent.

## Where the codec stands

| Area | State |
| --- | --- |
| Decoder | High profile, bit-exact against ffmpeg on twenty-nine clips |
| Encoder | Main profile, bit-exact against ffmpeg, B slices included |
| High profile encoding | not started: the 8x8 transform and the matrices are read but not written |
| Hardware encoding | Media Foundation on Windows, NVENC on Linux, both without cgo |
| Hardware decoding | none; the platform transform is kept as an oracle, not a backend |
| Screen content interface | changed rectangles, typed regions, forced key frames, switchable motion search |
| Slices | any count on macroblock row boundaries, encoded in parallel |
| Lossless transform bypass | rejected explicitly, both directions |

Measured on the development machine, 20 threads, one frame per operation.
The desktop figure is a 1080p screen with one moving video window, which
is the picture winline actually sends. Every row is the same benchmark
run back to back in one worktree:

| Path | 176x144 | 1280x720 | 1080p desktop |
| --- | --- | --- | --- |
| Where the codec started this milestone | 48.1 f/s | 1.08 f/s | 0.37 f/s |
| Early skip in the mode decision | 74.9 f/s | 1.06 f/s | 2.28 f/s |
| Vector kernels as well | 105 f/s | 1.70 f/s | 3.99 f/s |
| One slice per macroblock row as well | | | **29.8 f/s** |
| Adapter, for comparison | 1914 f/s | 606 f/s | 324 f/s |

That is eighty times where the processor path started, and enough to
carry a 1080p desktop at thirty frames a second on a machine with no
usable video adapter, which is the case this codec exists for.

Compression, all measured at the same quantiser: CABAC costs 18 to 30
per cent fewer bits than CAVLC; B pictures save a further 4.9 to 17.5
per cent on real video, and **cost** 5 to 11 per cent on rigid synthetic
motion; sixty-eight slices cost 1.8 per cent; a still screen with change
hints costs about ten bytes a frame.

## The number that sets the order

The first version of this document opened on one frame a second at 720p
against six hundred on the adapter, and argued that speed had to come
before features because the machine winline cares about has no adapter.
That work is done, and the gap it described is closed for the desktop
case.

## 1.2 — Speed, done

**Stop paying for easy content.** The mode decision tested every
candidate for every macroblock, so a still desktop cost the same as
moving video. It now tries the skip candidate first and takes it when
the residual quantises to nothing: six times faster on the desktop
picture, switchable off through `ModeDecision`.

**Cut the picture into slices and encode them at once.** Slices split on
macroblock row boundaries; prediction stops at a boundary, which is what
makes them independent, and the neighbour lookup decides by macroblock
address rather than reading a structure another goroutine owns. Ten
times faster for 1.8 per cent more bits. `Slices` negative means one per
processor.

The scaling is not linear and the numbers say why: two and four slices
barely help, because the moving window falls inside one of them and that
slice does all the work. Static partitioning cannot fix that; more
slices than threads can, and that is what the automatic setting does.

**Vector kernels where the time actually was.** The transformed
difference is called for every candidate of every macroblock and was the
hottest function in the encoder; interpolation runs for every
compensated block in both directions. Both are vectorised: 1.8 times on
a single-threaded encode, 1.14 to 1.20 times on decoding. The gain falls
to 1.12 times once the picture is already spread over sixty-eight
slices, where the work is no longer starved of arithmetic — a useful
reminder that the two optimisations overlap rather than multiply.

Deblocking kernels were written, tested and thrown away: at the real
call granularity of one sixteen line edge, the copying needed to avoid a
read after write hazard between the two sides cost more than the
arithmetic saved, and the filter ran three and a half times slower than
the scalar code.

**What is left of this milestone** is the acceptance test itself: a
1080p desktop frame inside the budget winline's `GFX_PLAN.md` records
for its planar path. The number to beat is in that document, not this
one.

## 1.3 — Acceptance by a real client

AVC420 through MS-RDPEGFX in `mstsc`, full screen at 1080p. The wire
format constrains the encoder: an Annex B byte stream, YUV420p, picture
dimensions a multiple of sixteen, and the region actually sent named by
`regionRects` rather than cropped in the encoder. Those hold today and
are covered by tests, but no real client has yet accepted a stream.

The protocol also carries requirements of its own beyond H.264, and they
have to be read before the first connection rather than after it.

Acceptance: a browser playing video inside the session, no artefacts in
the moving region, text beside it readable, and a connection that
survives a long sitting.

## 1.4 — B slices in the encoder, done

The encoder writes bi-predictive slices: coding order separated from
display order, picture order count type 0, reference lists derived by
picture order count exactly as the decoder derives them, spatial direct
mirroring the decoder's derivation, explicit partitions down to 8x8 sub
macroblocks, under both entropy coders. ffmpeg decodes every one of them
to what our own decoder produces.

**They are not free, and for winline they may be a loss.** On real video
they save 4.9 to 17.5 per cent at the same quantiser, more at coarse
quantisers. On rigid synthetic motion they cost 5 to 11 per cent, because
with two B pictures the predicted anchors move from distance one to
distance three, and where a distance one prediction was already exact
nothing bi-prediction offers pays that back. Screen content is full of
exactly that. `BFrames` is zero by default and should stay zero for a
remote desktop; a test records the adverse number rather than asserting
it away.

Two things were left out deliberately: temporal direct, which needs the
distance scaling and the colocated picture order count mapping on top of
what spatial needed; and the list swap the decoder performs when both
reference lists come out identical, which this group structure cannot
produce. If B pyramids or open groups are ever added, that swap is where
the two sides would silently diverge.

## 1.5 — High profile, half done

**Decoding is done.** The eight by eight transform, the scaling matrices
and Intra_8x8 prediction, under both entropy coders. Eleven clips joined
the corpus, three of them carrying matrices that are not flat, which is
the only way to tell whether the matrix work exists at all rather than
being skipped over.

**Encoding is not started.** The encoder still writes flat matrices and
the four by four transform only. What it needs: the eight by eight
transform in the mode decision, `transform_size_8x8_flag` written at both
of its syntax positions, Intra_8x8 prediction on the search side, and the
matrices in the picture parameter set. The pieces underneath it —
transform, quantisation, matrix resolution — are built and tested.

Acceptance: our own High profile streams survive the same three-way
check the rest of the encoder does.

## 1.6 — Wider hardware

Three separate pieces, none blocking the others.

**NVENC on real silicon.** The Linux backend is written and its call
sequence is tested, but it has never run against a card that answers.
The adapter in production is a Kepler GT 710 on nouveau, which exposes
no encoder, and the proprietary branch that would has been dropped from
the distribution. Verification waits for the P40 or V100 planned for
that machine.

**VA-API for Intel and AMD.** Reached the same way as NVENC, through
`purego` dlopen, in its own nested module so the core `go.mod` stays
empty. This is what makes hardware encoding mean something on a Linux
box that is not NVIDIA.

**Direct3D 11 for decoding on Windows.** The platform's only decoder
transform is Microsoft's software one, and it is about four times slower
than our own decoder, which is why it is kept as a test oracle and not
registered as a backend. Real hardware decoding means handing that
transform a Direct3D device and copying textures back.

## Open questions that are not the codec's to answer

**Patents.** Section 1 of winline's plan puts the decision in front of
the owner and it is still unrecorded. The codec being pure Go changes
nothing about it; an open licence on source is not a patent licence.
This document notes the question and does not answer it.

**Hardware to test on.** Every claim about an adapter in this repository
is measured on the development machine. The production machine has
different silicon and a different driver, and until the card is fitted
the Linux path is written and tested but not proven.

## How a milestone is judged

The same rules that got the codec this far, unchanged.

- `CGO_ENABLED=0` everywhere, tests and CI included. The core module has
  no dependencies; anything that needs one lives in a nested module.
- Correctness is measured against an outside implementation, never
  asserted. Our decoder reads our stream, ffmpeg decodes it to the same
  bytes, and the encoder's own reconstruction matches both.
- Specification tables are transcribed from an authoritative source and
  checked structurally, never written from memory.
- New code lands with its tests in the same commit. No comments in code.
- Anything winline does not need is switchable, not missing.
