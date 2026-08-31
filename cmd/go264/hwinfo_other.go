//go:build !windows

package main

import (
	"fmt"
	"io"
)

func reportPlatformTransforms(w io.Writer) error {
	fmt.Fprintln(w, "no platform video transforms are reachable on this system")
	return nil
}
