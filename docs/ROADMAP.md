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

## Where v1.1.0 stands

| Area | State |
| --- | --- |
| Decoder, Main profile | complete, bit-exact against ffmpeg on all eighteen clips |
| Encoder, entropy coding | CAVLC and CABAC, both bit-exact through ffmpeg |
| Encoder, prediction | intra all sizes, every P partition to 8x8, multiple references |
| Encoder, B slices | not started; the decoder reads them |
| High profile | not started, rejected explicitly in both directions |
| Hardware encoding | Media Foundation on Windows, NVENC on Linux, both without cgo |
| Hardware decoding | none; the platform transform is kept as an oracle, not a backend |
| Screen content interface | changed rectangles, typed regions, forced key frames, switchable motion search |
| Slices | any count on macroblock row boundaries, encoded in parallel |

Measured on the development machine, 20 threads, one frame per operation.
The desktop figure is a 1080p screen with one moving video window, which
is the picture winline actually sends:

| Path | 176x144 | 1280x720 | 1080p desktop |
| --- | --- | --- | --- |
| CPU, exhaustive search, one slice | 48.1 f/s | 1.08 f/s | 0.37 f/s |
| CPU, early skip, one slice | 74.9 f/s | 1.06 f/s | 2.19 f/s |
| CPU, early skip, one slice per row | | | 19.9 f/s |
| Adapter | 1914 f/s | 606 f/s | 324 f/s |

CABAC costs 18 to 30 per cent fewer bits than CAVLC at the same
quantiser. Cutting the 1080p desktop into 68 slices costs 1.8 per cent
more bits. A still screen with change hints costs about ten bytes a
frame against two seconds of work without them.

## The number that sets the order

The first version of this document opened on one frame a second at 720p
against six hundred on the adapter, and argued that speed had to come
before features because the machine winline cares about has no adapter.
That argument still holds; the first two pieces of it are now done.

The production machine sharpens the point: eighteen cores and thirty-six
threads, and one video card whose driver does not expose an encoder. The
processor there is wide. Until this milestone the encoder used one lane
of it.

## 1.2 — Speed

**Done: stop paying for easy content.** The mode decision tested every
candidate for every macroblock, so a still desktop cost the same as
moving video. It now tries the skip candidate first and takes it when
the residual quantises to nothing. On the 1080p desktop that is 0.37
frames a second against 2.19, a factor of six, and it is switchable off
through `ModeDecision`.

**Done: cut the picture into slices and encode them at once.** Slices
split on macroblock row boundaries; prediction stops at a boundary,
which is what makes them independent, and the neighbour lookup decides
by macroblock address rather than reading a structure another goroutine
owns. One slice per macroblock row reaches 19.9 frames a second on the
same picture with twenty threads, ten times the single slice figure, for
1.8 per cent more bits. `Slices` negative means one per processor.

The scaling is not linear and the numbers say why: two and four slices
barely help, because the moving window falls inside one of them and that
slice does all the work. Static partitioning cannot fix that; more
slices than threads can, and that is what the automatic setting does.

**Still to do: profile what is left and write the numbers here.** The
next measurement to take is where the remaining time goes now that the
easy macroblocks are free — motion search, the intra mode search, or the
entropy coder — and whether the SIMD kernels reach the functions that
now dominate.

Acceptance for the milestone: a typical 1080p desktop frame inside the
budget winline's `GFX_PLAN.md` records for its planar path; ffmpeg
decodes every stream to exactly what our decoder produces, multi-slice
and both entropy coders included. The second half is measured and holds
today.

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

## 1.4 — B slices in the encoder

The decoder reads them; the encoder does not write them. This closes the
symmetry and is worth real bitrate on the video region winline encodes.

Needed: coding order separated from display order, list construction for
both directions, the direct modes on the writing side, and a bit cost
model that prices a bi-predicted candidate honestly against a predicted
one.

Acceptance: ffmpeg decodes the streams bit-exactly, and the saving over
a P-only encode at the same quantiser is measured and recorded.

## 1.5 — High profile

The 8x8 transform and scaling matrices, in both directions. Both are
rejected explicitly today, which is the correct behaviour for a codec
that cannot do them and the wrong one for a codec meant to be complete.

Acceptance: the conformance corpus grows High profile clips, and they
decode bit-exactly; our own High profile streams survive the same
three-way check.

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
