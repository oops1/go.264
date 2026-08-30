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
| Decoder, Constrained Baseline | complete, bit-exact against ffmpeg |
| Encoder, Constrained Baseline | complete, bit-exact against ffmpeg |
| Intra prediction, all block sizes | complete |
| Inter prediction, all P partitions | complete in the decoder, 16x16 in the encoder |
| CAVLC | complete, both directions |
| In-loop deblocking filter | complete, shared by encoder and decoder |
| Reference picture management | sliding window and MMCO |
| Hardware acceleration | probe and fallback in place, no backend implemented yet |
| Bitrate targeted rate control | complete, within about a fifth of the request |
| SIMD kernels | sum of absolute differences on amd64, about thirty times faster |
| CABAC, B slices, High profile | not started |

See [docs/PLAN.md](docs/PLAN.md) for the phase breakdown and
[docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) for the design.

## Install

```
go get github.com/oops1/go264
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
go install github.com/oops1/go264/cmd/go264@latest
```

```bash
go264 encode -s 1280x720 -qp 24 -gop 30 -i input.yuv -o output.264
```\n
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
