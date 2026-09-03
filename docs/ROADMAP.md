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
slices than threads can.

That observation sat in this document for a while before it reached the
setting it describes. `Slices` negative first meant one slice per
processor, which is exactly the choice the paragraph above argues
against, and a probe built to be run on the production machine measured
it: twenty slices on twenty processors gave 21.4 frames a second where
thirty-four gave 31.5. It now means twice the processor count, capped at
the number of macroblock rows.

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

**VA-API for Intel and AMD** is written, in its own nested module
loading libva at run time, but has never met a driver. The structures
are transcribed from the installed headers with their sizes and offsets
measured by compiling C against those same headers, and the library
loads and marshals arguments against a live libva; what is unproven is
everything a driver would answer.

**Direct3D 11 for decoding on Windows** is done. Handing the decoder
transform a Direct3D 11 device makes it bind the adapter, and the
textures are copied back through a staging surface. Registered as
`mediafoundation-d3d11`, but only above 640x480, because below that our
own decoder is faster and registering it there would repeat the mistake
described next.

## A measurement that was wrong, and what it cost

Worth keeping because the shape of the mistake is general.

The decoder transform on Windows was recorded as four times slower than
our own decoder, and that number decided against registering it. The
benchmark that produced it ran only on the conformance corpus, which is
entirely small pictures, and charged every iteration with about one and
a third seconds of opening and closing the transform.

Measured properly, in frames a second: at 176x144 our decoder does 2723
against the transform's 643, and at 1920x1080 it does 74 against the
transform's 321. The transform was never slow. Our decoder is very fast
on tiny pictures, and the fixed cost of a decode cycle dominated
everything else at that size. Bound to the adapter through Direct3D the
same transform reaches 733 frames a second at 1080p, nine times ours.

The lesson is not about video: a benchmark that measures one size, with
setup inside the loop, produced a number that stood in the documents as
a fact for weeks and shaped a design decision.

## What is still missing

Smaller than a milestone each, listed so none of it is forgotten.

**Long-term references do not compose with intra refresh.** A long-term
picture is unusable under a refresh until it is promoted again after a
sweep, so the two features do not help each other. They now work with
bi-predicted pictures, which they did not before.

**Rejected on purpose, both directions**: lossless transform bypass, the
4:2:2 and 4:4:4 chroma formats, bit depths above eight, interlaced and
macroblock adaptive coding, slice groups, and data partitioning. Each
has a test that feeds a real stream of that kind and requires the
refusal.

**Unproven rather than missing**: NVENC and VA-API have never run
against real silicon, and no remote desktop client has yet accepted a
stream. Neither can be settled on the machine this was written on.

## Hostile input, and a leak the hardware found

The decoder was read through as if every byte came from an attacker, and
the result is written up in [DECODER-HARDENING.md](DECODER-HARDENING.md):
what it refuses, what it deliberately tolerates, what it allocates in the
worst case, and the ceilings a caller can set. The largest single finding
was allocation: a thirty byte stream declaring a picture of 1024 by 1024
macroblocks — seven and a half times the largest frame size in table A-1 —
took 1035 MiB before any of it was checked. It is now refused having
allocated nothing.

Two of the fixes are worth naming because both were unbounded, not merely
generous. `max_num_reorder_frames` is a `ue(v)` and was used raw as the
reorder queue depth, so a stream could ask the decoder to hold four
billion pictures before emitting one. And adaptive reference marking
appended without ever running the sliding window, so a stream that marked
on every picture and evicted on none grew the reference list forever, each
entry a whole picture.

Two things were tightened and then deliberately loosened again, because
strictness that refuses real streams is not security. Holding a stream to
the level it declares is now behind `Limits.EnforceLevel` rather than
unconditional: encoders that understate their level are common, ffmpeg
only warns, and the table A-1 ceiling — which is unconditional — is what
actually bounds the allocation. And an unrecognised `level_idc` still
falls back to the largest buffer in the table, because `MaxDpbFrames` caps
at sixteen regardless, so the strict fallback bought no bound and cost
every frame of reordering on a level the table does not yet list.

Level 1b is written two ways and both are now recognised. The first
attempt accepted only `level_idc` 11 with `constraint_set3_flag`, which is
the Baseline and Main spelling; for the High family the spelling is
`level_idc` 9, and rejecting it collapsed the buffer to a single frame.
ffmpeg's own transcription of table A-1 carries both rows.

Separately, the Windows continuous integration went red for three runs
with a Media Foundation platform count of seven where two were expected.
Both `closeHere` paths left through an early return when the flush failed,
losing the platform reference and the transform object with it — and the
flush fails exactly when configuration did not finish, which is what
happens on a machine with no adapter. It could not reproduce on the
development machine for that reason. The regression tests build the
unconfigured state directly, so they fail with or without an adapter.

The same three runs also hit the forty minute test timeout, which was not
a fault: the encoder suite genuinely grew past it. Raised to ninety.

## What the allocation profile said

The performance pass started with a profile rather than a guess, and the
guess would have been wrong. In the decoder, the pictures themselves were
not the cost: the bookkeeping around them allocated four times as much.
Two structures were rebuilt where reuse is safe — the motion segment list,
which was a fresh slice for every inter macroblock, and the macroblock
grid, which was fresh for every picture although nothing retains it. Ten
frames of `base_ip_qp26` went from 3.19 ms, 2503 KB and 1096 objects to
2.41 ms, 1282 KB and 279. At 1080p the whole decode dropped from 86,398
allocations to 363.

Then fifteen and a half per cent of the decode turned out to be
`runtime.mapaccess2_fast32`. The CAVLC reader walked the bitstream a bit
at a time and consulted a map after each one, so a sixteen bit code cost
sixteen hash lookups. The same keys in a flat slice, 169 KB for all thirty
tables, took 1080p decoding from 26.1 to 29.3 frames per second.

The encoder had one allocation worth three and a half gigabytes per frame.
With CABAC, B pictures, the eight by eight transform and trellis together,
every trellis trial built its own bit estimate writer and its own trial
arithmetic coder. Both are now kept and reset, which is sound because
`Restore` already overwrites every field that affects encoding. That frame
now allocates eight megabytes and encodes sixteen per cent faster. The
encoder and cabac packages were run for twenty five minutes under the race
detector, parallel slices included, to establish that nothing shares the
buffers being reused.

Deblocking was a third of decode time with no accelerated kernel at all,
and both directions now have one. A horizontal edge needs no transpose:
the eight samples across it lie in eight rows of sixteen contiguous bytes,
so an edge is sixteen vectors and one filter pass. A vertical edge does
need one, and gets it - sixteen rows of eight bytes turned on their side
through two eight by eight byte transposes, filtered, and the four columns
the normal filter can change turned back and written four bytes to a line.

The filter had to be two kernels rather than one, for a reason worth
recording: as a single function the register allocator ran out of the
sixteen AVX2 registers and silently reached into Y16 and above, which is
AVX-512 and which the assembler refuses. Splitting it along the boundary
strength was legitimate because bS equal to four is a property of the
macroblock pair, so an edge is either strong throughout or normal
throughout; the Go side checks that and falls back to scalar for a
mixture.

Twelve 1080p frames went from 359 ms to 296, and deblocking from a third
of the profile to eight per cent. The oracle for both kernels is the
scalar filter, which is exhaustive: the filter is a pure function of eight
samples and four parameters, and 65,000 rounds of random samples,
strengths and index pairs match byte for byte, with the conformance clips
still bit exact on top.

Intra prediction has no kernel either, and does not need one: it is under
six per cent of either side.

## What the encoder was really spending

Motion search is sixty eight per cent of a 1080p encode and sub-pixel
refinement is forty seven, and more than half of that was the six tap
filter being run again for every candidate vector. It does not have to be.
The three half sample quantities the standard derives are samples of three
planes that depend only on the reference picture, so every one of the
sixteen fractional positions is a copy of one plane or an average of two.
The number that settled it: building all three planes for a padded 1080p
frame takes 0.75 ms, and the search had been spending 159 ms per frame on
the same filter.

Then the profile moved, three times, and each time the answer was smaller
than the last:

- averaging two planes is `(a+b+1)>>1` over bytes, which is exactly what
  PAVGB does sixteen at a time: eleven per cent of the encode became a
  kernel.
- the bit cost of a vector difference was a loop shifting a value down one
  bit at a time; it is `1 + 2*floor(log2(code+1))` and now uses
  math/bits.Len. Two lines, a seventh of the encode, because it runs for
  every candidate the search considers.
- block copying was Go's copy per row, a memmove call for each of sixteen
  rows; fixed shape kernels do it with one instruction per sixteen bytes.

Twelve 1080p frames went from 847 ms to 540, a third off, 1.18 to 1.85
frames per second. Nothing in the encoded output moved: the plane
derivation is checked against the direct filter over 16,875 predictions,
every fractional position and nine block shapes, byte for byte.

## Three measurements that said no

Recorded because a profile is a hypothesis generator, not an answer, and
these three looked as convincing as the ones that worked.

**Reading variable length codes with one peek instead of bit by bit** was
1.2 per cent slower, reproducibly. The code words are short: most resolve
within a few bits, so preparing a window and skipping costs more than the
two or three bit reads it saves.

**Hoisting the bilinear chroma weights out of the pixel loop** - they were
being recomputed for every pixel - was 0.7 per cent slower. The blocks
that reach that path are two by two, from four by four luma partitions,
and the slice setup costs more than the arithmetic it removes.

**Using the AVX2 four by four cost kernel** was 1.8 per cent slower than
the SSE one. It was generated but never dispatched, which looked like an
oversight; on four rows the wider register is mostly idle and the state
transition is not free. The dispatch was right all along.

## Hostile input at the public boundary

The decoder hardening bounded what the decoder accepts, but the public API
reached past it. `Decoder.Decode` reads the picture size out of the first
sequence parameter set it sees and hands it to the hardware backend before
any check has run, so thirty bytes declaring 1024 by 1024 macroblocks asked
the driver for a 16384 by 16384 decoder. The size now passes the table A-1
ceiling before a backend is settled. `DecoderConfig` carries `Limits` so a
caller taking streams from somewhere it does not control can set the
ceilings the decoder already understands, and `ErrOverLimit` is exported so
that refusal can be told from any other.

On the VA-API side, three counts and a size come back from the driver into
buffers this package allocated, and each was used to slice or read without
being checked against the capacity it was given. A coded segment larger
than its buffer made `unsafe.Slice` read past the allocation, which is not
a panic but a read of whatever follows.

## A correction to the 1.5.0 notes

Two things that release says are wrong, and the record should say so
rather than quietly improve.

**The intra transform choice was not scored with an existing kernel.**
The commit and the tag both say the eight by eight candidate is now
scored with the matching kernel "which was already there". It was not.
`simd.SATD8x8` is four independent four by four Hadamard sums tiled over
the block — a saving in call shape, not a different measure, and blind
to the energy compaction in exactly the way the four by four metric is.
The change writes a new metric on `transform.Forward8x8`, the codec's
real eight by eight transform. Calibrating its scale against the old one
was a trap in itself: a naive halving overshot the cost by more than
twice and stopped Intra_8x8 being chosen at all.

**The measurement was narrower than the claim.** The release says the
stream is slightly smaller at slightly better quality at a coarse
quantiser and within noise at a fine one, which is what one content at
two quantisers showed. Across fifteen configurations spanning quantisers
eighteen to forty-two and three kinds of content the aggregate is a
wash: net 0.065 per cent fewer bytes and a hundredth of a decibel, with
individual points running from ten and a half per cent fewer bytes at
plus 0.29 decibels to four and a half per cent more at minus 0.26. The
mode decision is more principled; the bitrate is not decisively better.

**And the long-term work fixed three faults, not one.** Beyond letting
long-term pictures into the bi-predictive lists, the lists were being
built correctly and then truncated to their first entry, and the
bi-predictive motion search asked for reference index zero on both lists
regardless. A third was latent and would have bitten later: the final
motion store wrote a hardcoded reference index zero, which costs nothing
while only one entry is reachable and desynchronises the encoder from
the decoder the moment more than one is.

## What was missing and is not any more

Kept as a record of what each thing cost and what it bought.

**Quantisation by rate and distortion**: two to seven per cent of the
bitrate at equal quality, four to six and a half on a fade and on moving
pictures, essentially nothing on screen content. The first attempt was
thrown away for saving a third of the bits at the cost of twenty six
decibels, because distortion among the coefficients and lambda against
the error of pixels are not the same scale.

**Weighted prediction**, both directions and both forms: thirty eight to
fifty five per cent on a fade with predictive pictures, nineteen to
thirty with bi-predictive ones. The explicit bi-predictive form was held
back by a disagreement with ffmpeg that turned out to be our own
non-conforming weights, not its arithmetic.

**Long-term references and the memory management operations**: thirty
five per cent fewer bits on screen content, twenty six against an encode
given the same buffer size in short-term references instead.

**Level limits against the bitrate**: the level is now chosen from the
bitrate and the buffer as well as the picture size, so a stream can no
longer announce limits it exceeds. Transcribing table A-1 turned up that
level 4.1 had been missing entirely.

**Parameter sets alongside a recovery point**, which is what makes intra
refresh usable by a receiver that joins late, and which was blocked by
the decoder discarding its references whenever a set was repeated.

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
