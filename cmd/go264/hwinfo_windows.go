package main

import (
	"fmt"
	"io"

	"github.com/oops1/go.264/internal/hwaccel/mf"
)

func reportPlatformTransforms(w io.Writer) error {
	if !mf.Loaded() {
		fmt.Fprintln(w, "media foundation: not present")
		return nil
	}
	encoders, err := mf.ListH264Encoders()
	if err != nil {
		return err
	}
	decoders, err := mf.ListH264Decoders()
	if err != nil {
		return err
	}
	printTransforms(w, "encoder", encoders)
	printTransforms(w, "decoder", decoders)
	return nil
}

func printTransforms(w io.Writer, role string, list []mf.TransformDescription) {
	if len(list) == 0 {
		fmt.Fprintf(w, "media foundation %s: none\n", role)
		return
	}
	for _, d := range list {
		kind := "software"
		if d.Hardware {
			kind = "hardware"
		}
		mode := "synchronous"
		if d.Async {
			mode = "asynchronous"
		}
		fmt.Fprintf(w, "media foundation %s: %-8s %-12s %s\n", role, kind, mode, d.Name)
	}
}
