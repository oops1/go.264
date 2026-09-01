# go264

H.264/AVC encoder and decoder in pure Go. No cgo, ever.

- **CGO-free.** Builds and passes its full test suite with `CGO_ENABLED=0` on
  every supported platform. No C toolchain, no shared-library link step.
- **Verified against a reference implementation.** The decoder reproduces
  ffmpeg's output bit for bit on the checked-in conformance streams, and
  ffmpeg decodes the encoder's output bit for bit to what our own decoder
  produces. Both directions are measured, not assumed.
- **Hardware acceleration when present.** The encoder and decoder probe for
  platform video engines at construction time and use them when available,
  falling back to the CPU path transparently. Bindings go through syscalls
  and `dlopen`, never cgo.

## Status

Working today:

| Area | State |
| --- | --- |
| Decoder, High profile | complete, bit-exact against ffmpeg on twenty-nine clips |
| Encoder, Main profile | complete, bit-exact against ffmpeg |
| Intra prediction | all block sizes, 4x4, 8x8 and 16x16; 8x8 decoded only |
| Inter prediction | all P partitions both directions, 8x8 sub-macroblocks, multiple references |
| B slices | both directions: bi-prediction, spatial direct, output reordered by picture order count |
| Weighted prediction | explicit for predictive slices, explicit and implicit for bi-predictive |
| CAVLC | complete, both directions |
| CABAC | complete in both directions; streams are 18 to 30 per cent smaller than CAVLC |
| In-loop deblocking filter | complete, shared by encoder and decoder |
| Reference picture management | sliding window and MMCO, up to sixteen references in the encoder |
| Slices | any count, encoded in parallel; ten times faster on twenty threads for 1.8 per cent more bits |
| Hardware acceleration | encoding on Windows through Media Foundation and on Linux through NVENC, chosen automatically, no cgo on either path |
| Bitrate targeted rate control | complete, within about a fifth of the request |
| Mode decision | rate-distortion, with an early skip test that pays for itself six times over on screen content |
| SIMD kernels | transformed differences, six-tap and bilinear interpolation, block matching, the 4x4 transform and quantisation |
| Scaling matrices | resolved and applied when decoding; the encoder writes flat matrices only |
| High profile encoding | not started; the 8x8 transform and matrices are read but not written |
| Lossless transform bypass | rejected explicitly, in both directions |

See [docs/ROADMAP.md](docs/ROADMAP.md) for what comes next and why in that
order, [docs/PLAN.md](docs/PLAN.md) for the phase breakdown and
[docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) for the design.

## Install

```
go get github.com/oops1/go.264
```

## Library

```go
enc, err := go264.NewEncoder(go264.EncoderConfig{
    Width:  1920,
    Height: 1080,
    FPSNum: 30, FPSDen: 1,
    GOPSize: 60,
    QP:      26,
})
if err != nil {
    return err
}
defer enc.Close()

packet, err := enc.Encode(i420Frame)
```

```go
dec := go264.NewDecoder()
defer dec.Close()

frames, err := dec.Decode(annexB)
for _, f := range frames {
    i420 = f.AppendI420(i420[:0])
}
```

`Encoder.Backend()` and `Decoder.Backend()` report which implementation is
actually in use, and `ForceSoftware` pins the CPU path for reproducibility.

## Command line

```bash
go install github.com/oops1/go.264/cmd/go264@latest
```

```bash
go264 encode -s 1280x720 -qp 24 -gop 30 -i input.yuv -o output.264
```

Or aim at a bitrate instead of a fixed quantiser:

```bash
go264 encode -s 1280x720 -b 2500 -gop 30 -i input.yuv -o output.264
```

```bash
go264 decode -i input.264 -o output.yuv
```

Input and output default to standard input and output, so the tool composes
with ffmpeg:

```bash
ffmpeg -i movie.mp4 -pix_fmt yuv420p -f rawvideo - | go264 encode -s 1280x720 -o out.264
```

## Verification

Every numeric table taken from the specification is transcribed from an
authoritative source and then validated structurally rather than trusted.
For the CAVLC tables that means asserting each is a prefix free code whose
Kraft sum is at or just below one, which caught seven wrong entries in one
table and a chroma table whose sum exceeded one, an arithmetic
impossibility for any prefix code.

The decoder is measured against frames ffmpeg produces from the same
streams, sample for sample. The encoder is measured the other way: ffmpeg
must decode its output to exactly what our own decoder produces. Assembly
kernels are compared against their pure Go twins on randomised inputs.

The arithmetic decoder is held to the same standard from the other side.
Its tests carry a CABAC encoder written from the specification and round
trip randomised bin sequences through it, along with every syntax element
the Main profile needs, so that a stream that fails to decode points at
the surrounding code rather than at the arithmetic.
Prediction and interpolation are compared against independent reference
implementations written from the specification formulas rather than from
the production code.

## Testing

```bash
go test ./...
```

The conformance suite decodes the streams under `testdata/conformance` and
compares every sample against frames produced by ffmpeg. `testdata/regen.sh`
regenerates that corpus. Tests that shell out to ffmpeg skip themselves when
it is not installed.

## License

BSD 2-Clause. See [LICENSE](LICENSE).
