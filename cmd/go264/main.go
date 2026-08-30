package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/oops1/go264"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "go264:", err)
		os.Exit(1)
	}
}

func usage() string {
	return strings.Join([]string{
		"usage:",
		"  go264 encode -s WxH [-qp N | -b KBPS] [-gop N] [-fps N] [-i in.yuv] [-o out.264]",
		"  go264 decode [-i in.264] [-o out.yuv]",
		"  go264 backends",
	}, "\n")
}

func run(args []string) error {
	if len(args) == 0 {
		return errors.New(usage())
	}
	switch args[0] {
	case "encode":
		return runEncode(args[1:])
	case "decode":
		return runDecode(args[1:])
	case "backends":
		fmt.Println(strings.Join(go264.Backends(), "\n"))
		return nil
	}
	return fmt.Errorf("unknown command %q\n%s", args[0], usage())
}

func parseSize(s string) (int, int, error) {
	var w, h int
	if _, err := fmt.Sscanf(s, "%dx%d", &w, &h); err != nil {
		return 0, 0, fmt.Errorf("size %q must look like 1920x1080", s)
	}
	return w, h, nil
}

func openIn(path string) (io.ReadCloser, error) {
	if path == "" || path == "-" {
		return io.NopCloser(os.Stdin), nil
	}
	return os.Open(path)
}

func openOut(path string) (io.WriteCloser, error) {
	if path == "" || path == "-" {
		return nopWriteCloser{os.Stdout}, nil
	}
	return os.Create(path)
}

type nopWriteCloser struct{ io.Writer }

func (nopWriteCloser) Close() error { return nil }

func runEncode(args []string) error {
	fs := flag.NewFlagSet("encode", flag.ContinueOnError)
	size := fs.String("s", "", "frame size, for example 1280x720")
	qp := fs.Int("qp", 26, "constant quantiser, 0 to 51")
	bitrate := fs.Int("b", 0, "target bitrate in kbit/s, zero for constant quantiser")
	gop := fs.Int("gop", 30, "distance between IDR pictures")
	fps := fs.Int("fps", 25, "frame rate")
	in := fs.String("i", "-", "raw I420 input, - for standard input")
	out := fs.String("o", "-", "Annex B output, - for standard output")
	software := fs.Bool("software", false, "never use a hardware encoder")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *size == "" {
		return errors.New("encode requires -s WxH")
	}
	w, h, err := parseSize(*size)
	if err != nil {
		return err
	}

	enc, err := go264.NewEncoder(go264.EncoderConfig{
		Width: w, Height: h, FPSNum: *fps, FPSDen: 1,
		GOPSize: *gop, QP: *qp, BitrateKbps: *bitrate, ForceSoftware: *software,
	})
	if err != nil {
		return err
	}
	defer enc.Close()
	fmt.Fprintf(os.Stderr, "encoding %dx%d with the %s backend\n", w, h, enc.Backend())

	src, err := openIn(*in)
	if err != nil {
		return err
	}
	defer src.Close()
	dst, err := openOut(*out)
	if err != nil {
		return err
	}
	defer dst.Close()

	reader := bufio.NewReaderSize(src, 1<<20)
	writer := bufio.NewWriterSize(dst, 1<<20)
	defer writer.Flush()

	buf := make([]byte, w*h*3/2)
	frames := 0
	total := 0
	for {
		if _, err := io.ReadFull(reader, buf); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			if errors.Is(err, io.ErrUnexpectedEOF) {
				fmt.Fprintln(os.Stderr, "ignoring a trailing partial frame")
				break
			}
			return err
		}
		pkt, err := enc.Encode(buf)
		if err != nil {
			return err
		}
		if _, err := writer.Write(pkt); err != nil {
			return err
		}
		frames++
		total += len(pkt)
	}
	fmt.Fprintf(os.Stderr, "encoded %d frames, %d bytes\n", frames, total)
	return nil
}

func runDecode(args []string) error {
	fs := flag.NewFlagSet("decode", flag.ContinueOnError)
	in := fs.String("i", "-", "Annex B input, - for standard input")
	out := fs.String("o", "-", "raw I420 output, - for standard output")
	software := fs.Bool("software", false, "never use a hardware decoder")
	if err := fs.Parse(args); err != nil {
		return err
	}

	dec := go264.NewDecoderWithConfig(go264.DecoderConfig{ForceSoftware: *software})
	defer dec.Close()
	fmt.Fprintf(os.Stderr, "decoding with the %s backend\n", dec.Backend())

	src, err := openIn(*in)
	if err != nil {
		return err
	}
	defer src.Close()
	dst, err := openOut(*out)
	if err != nil {
		return err
	}
	defer dst.Close()

	reader := bufio.NewReaderSize(src, 1<<20)
	writer := bufio.NewWriterSize(dst, 1<<20)
	defer writer.Flush()

	chunk := make([]byte, 1<<16)
	var scratch []byte
	frames := 0
	emit := func(fs []*go264.Frame) error {
		for _, f := range fs {
			scratch = f.AppendI420(scratch[:0])
			if _, err := writer.Write(scratch); err != nil {
				return err
			}
			frames++
		}
		return nil
	}
	for {
		n, err := reader.Read(chunk)
		if n > 0 {
			got, derr := dec.Decode(chunk[:n])
			if eerr := emit(got); eerr != nil {
				return eerr
			}
			if derr != nil {
				return derr
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return err
		}
	}
	got, err := dec.Flush()
	if eerr := emit(got); eerr != nil {
		return eerr
	}
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "decoded %d frames\n", frames)
	return nil
}
