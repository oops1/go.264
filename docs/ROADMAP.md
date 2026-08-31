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
| Slices | one per picture, single threaded |

Measured on the development machine, 20 threads, one frame per operation:

| Path | 176x144 | 1280x720 | 1920x1080 |
| --- | --- | --- | --- |
| Encoder, CPU | 48.1 f/s | 1.08 f/s | not measured |
| Encoder, adapter | 1914 f/s | 606 f/s | 324 f/s |

CABAC costs 18 to 30 per cent fewer bits than CAVLC at the same
quantiser. A still screen with change hints costs about ten bytes a
frame against two seconds of work without them.

## The number that sets the order

One frame a second at 720p on the processor, against six hundred on the
adapter. The whole reason winline commissioned this codec is the machine
that has no adapter, so on the machine that matters most the encoder is
three orders of magnitude short of real time, and no feature closes that
gap. Speed comes first.

The production machine sharpens the point: eighteen cores and thirty-six
threads, and one video card whose driver does not expose an encoder. The
processor there is wide and idle. The encoder does not use it.

## 1.2 — Speed

Three pieces, in this order.

**Stop paying for easy content.** The mode decision trial-encodes every
candidate for every macroblock, so a still desktop costs the same as a
moving picture. An early skip test — accept the skip when its distortion
is already below the threshold the other candidates would have to beat —
cuts the common case without touching the search that finds hard blocks.

**Cut the picture into slices and encode them at once.** One slice per
picture today, so one core. Slices are independent by construction:
prediction does not cross a slice boundary and the entropy coder restarts
at each one. The cost is a little compression, because context and
prediction are lost at every boundary; the count must therefore be a
setting, not a constant.

**Profile, and write the numbers here.** Before and after each change,
in this document, so a later change that undoes a gain is visible.

Acceptance: a typical 1080p desktop frame inside the budget winline's
`GFX_PLAN.md` records for its planar path, so go264 is no worse than what
that project already has; near-linear scaling to the core count on
1080p; ffmpeg still decodes every stream to exactly what our decoder
produces, multi-slice included.

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
