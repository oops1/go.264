# go264

H.264/AVC encoder and decoder in pure Go. No cgo, ever.

- **CGO-free.** Builds and passes its full test suite with `CGO_ENABLED=0`
  on every supported platform. No C toolchain, no shared-library link step.
- **SIMD on the CPU path.** Hot kernels have AVX2/SSE4 implementations in
  Go assembly with pure-Go fallbacks selected at runtime.
- **Hardware acceleration when present.** The encoder and decoder probe for
  platform video engines at construction time and use them when available,
  falling back to the CPU path transparently. Bindings go through syscalls
  and `dlopen`, never cgo.

Status: under active development toward v0.1.0. See
[docs/PLAN.md](docs/PLAN.md) for the phase breakdown and
[docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) for the design.

## Install

```
go get github.com/oops1/go264
```

## Usage

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

pkt, err := enc.Encode(frame)
```

```go
dec := go264.NewDecoder()
defer dec.Close()

frames, err := dec.Decode(annexBBytes)
```

## License

BSD 2-Clause. See [LICENSE](LICENSE).
