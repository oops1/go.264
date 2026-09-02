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
| Encoder | High profile: the 8x8 transform, Intra_8x8, the scaling matrices, B slices |
| Hardware encoding | Media Foundation on Windows, NVENC on Linux, both without cgo |
| Hardware decoding | none; the platform transform is kept as an oracle, not a backend |
| Screen content interface | changed rectangles, typed regions, forced key frames, switchable motion search |
| Slices | any count on macroblock row boundaries, encoded in parallel |
| Intra refresh | a sweeping band with motion constrained across the boundary, recovery point announced |
| Buffer model | a real coded picture buffer, constant bitrate, announced in the parameters and the messages |
| Deblocking control | off, on, or kept inside slices, with both offsets |
| Weighted prediction | applied when decoding; the encoder never writes a weight table |
| Temporal direct | derived when decoding; the encoder writes spatial only |
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

## 1.5 — High profile, done

Both directions. Decoding took the eight by eight transform, the scaling
matrices and Intra_8x8, with eleven clips joining the corpus, three of
them carrying matrices that are not flat, which is the only way to tell
whether the matrix work exists at all. Encoding writes the same, with
the transform chosen per macroblock by rate and distortion.

The eight by eight transform is not a bitrate win on every source. On
synthetic content it saves 2.6 to 6.9 per cent; on the corpus clips,
whose source has already been through a codec, the bits are a wash and
at coarse quantisers it costs up to 4.6 per cent, while distortion
improves nearly everywhere, by up to 0.88 decibels. The JVT matrices
save 8 to 18 per cent and lower the peak signal to noise ratio, which is
what they are for.

## 1.6 — What a remote desktop needs, done

Three things that had no implementation at all, all off by default.

**Intra refresh.** A band of intra macroblocks sweeps the picture so that
recovery costs no key frame. It halves the largest frame and costs eight
to ten per cent more bits. What makes it real rather than decorative is
that motion may not reach across the boundary into stale area; breaking
any one of those constraints makes the convergence test fail. A decoder
joining at a recovery point is bit exact one sweep later, verified
against ffmpeg as well as our own decoder.

The cost does not fall with a longer period, because the loop filter
smears the stale region into the last refreshed column and forces one
intra column per frame regardless. The useful range is a period no
longer than the picture is wide in macroblock columns.

**Deblocking control.** Off, on, or kept inside slices, with both
offsets. Keeping it inside slices is what makes the parallel slice path
independent all the way through.

**A buffer model.** A real coded picture buffer, announced in the video
usability information and in the buffering period and picture timing
messages, with a constant bitrate mode that pads. Walked frame by frame
from the announced parameters and the actual unit sizes, it never
underflows or overflows across ten configurations, and the long run rate
never exceeds the request.

## 1.7 — Wider hardware

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

## What is still missing

Smaller than a milestone each, listed so none of it is forgotten.

**Explicit weighting for bi-predictive slices.** Weighting is written for
predictive slices, where it saves 38 to 55 per cent on a fade, and
implicitly for bi-predictive ones. The explicit bi-predictive mode is
written but disabled, because our reconstruction and ffmpeg's disagree on
one to eighteen chroma samples per clip.

The weighting arithmetic is not the cause and is no longer a suspect: it
is checked against both the specification's formula and the reference
decoder's own arithmetic, which arranges the same expression differently,
across eight and a half million combinations of weight, offset,
denominator and sample value, and they agree everywhere.

The cause is narrower than that. Every differing sample instrumented so
far sits in a macroblock whose motion is derived rather than transmitted,
B_Direct_16x16 or B_Skip, and ffmpeg's value is exactly what the
single-list weighted formula produces from our own list zero prediction
and list zero weights, to the unit, on every sample checked. So the two
sides disagree about whether those blocks use the second list at all,
which is a question about the derivation of direct motion and not about
weighting. Weighting only makes the disagreement visible, by changing
which macroblocks the mode decision leaves to the derivation.

That points at a difference in the derived reference indices for a block
whose neighbours use only one list, which the conformance corpus does not
reach because x264 does not produce that arrangement. **If it is real it
is a decoder fault, not merely a missing encoder feature**, and it would
be worth more than the mode it currently blocks.

**Quantisation with rate and distortion (trellis).** Not present.
Usually five to ten per cent of the bitrate.

**Long term references and the memory management operations** in the
encoder. The decoder handles both; the encoder does sliding window only.

**Level limits against the bitrate.** The buffer model does not check
the announced level's maximum bitrate or buffer size, because those
tables were not transcribed from a verified source.

**Parameter sets alongside a recovery point.** A late joiner needs them
in band. The decoder no longer throws away its references when they are
repeated, which was the blocker; the encoder does not yet send them.

**An 8x8 transformed difference kernel.** The Intra_8x8 mode decision
scores with 4x4 sums, which under-rates the transform it is choosing.

**Rejected on purpose, both directions**: lossless transform bypass, the
4:2:2 and 4:4:4 chroma formats, bit depths above eight, interlaced and
macroblock adaptive coding, slice groups, and data partitioning. Each
has a test that feeds a real stream of that kind and requires the
refusal.

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
