//go:build linux && (amd64 || arm64)

package vaapi

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"unsafe"
)

type display struct {
	node   *os.File
	handle uintptr
	major  int32
	minor  int32
	vendor string
}

func renderNodeCandidates() []string {
	nodes := make([]string, 0, 16)
	for i := 128; i < 144; i++ {
		nodes = append(nodes, fmt.Sprintf("/dev/dri/renderD%d", i))
	}
	return nodes
}

func openEncodeDisplay() (*display, profileChoice, Entrypoint, error) {
	if err := loadLibrary(); err != nil {
		return nil, profileChoice{}, 0, err
	}
	var lastErr error
	for _, path := range renderNodeCandidates() {
		d, err := openDisplayAt(path)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			lastErr = err
			continue
		}
		choice, entry, err := d.findEncodeProfile()
		if err != nil {
			lastErr = fmt.Errorf("vaapi: %s (%s): %w", path, d.vendor, err)
			d.close()
			continue
		}
		return d, choice, entry, nil
	}
	if lastErr == nil {
		lastErr = errors.New("vaapi: no DRM render node was found")
	}
	return nil, profileChoice{}, 0, lastErr
}

func openDisplayAt(path string) (*display, error) {
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return nil, err
	}
	fd := int32(f.Fd())
	handle := getDisplayDRM(fd)
	if handle == 0 {
		f.Close()
		return nil, fmt.Errorf("vaapi: vaGetDisplayDRM refused %s", path)
	}
	var major, minor int32
	if err := check("vaInitialize", vaInitialize(handle, &major, &minor)); err != nil {
		f.Close()
		return nil, fmt.Errorf("vaapi: %s: %w", path, err)
	}
	d := &display{node: f, handle: handle, major: major, minor: minor}
	if vaQueryVendorString != nil {
		d.vendor = vaQueryVendorString(handle)
	}
	return d, nil
}

func (d *display) close() {
	if d == nil {
		return
	}
	if d.handle != 0 {
		vaTerminate(d.handle)
		d.handle = 0
	}
	if d.node != nil {
		d.node.Close()
		d.node = nil
	}
}

type profileChoice struct {
	profile  Profile
	baseline bool
}

var candidateProfiles = []profileChoice{
	{ProfileH264Main, false},
	{ProfileH264High, false},
	{ProfileH264ConstrainedBaseline, true},
}

var encodeEntrypoints = []Entrypoint{EntrypointEncSlice, EntrypointEncSliceLP}

func (d *display) findEncodeProfile() (profileChoice, Entrypoint, error) {
	max := vaMaxNumEntrypoints(d.handle)
	if max <= 0 {
		max = 32
	}
	entrypoints := make([]int32, max)
	for _, want := range encodeEntrypoints {
		for _, cand := range candidateProfiles {
			n := int32(len(entrypoints))
			st := vaQueryConfigEntrypoints(d.handle, int32(cand.profile), unsafe.Pointer(&entrypoints[0]), &n)
			if Status(st) != StatusSuccess {
				continue
			}
			if n < 0 || int(n) > len(entrypoints) {
				n = int32(len(entrypoints))
			}
			for _, e := range entrypoints[:n] {
				if Entrypoint(e) == want {
					return cand, want, nil
				}
			}
		}
	}
	return profileChoice{}, 0, errors.New("no H.264 encode entry point was found")
}

func (d *display) createConfig(profile Profile, entry Entrypoint) (uint32, error) {
	attribs := [2]ConfigAttrib{
		{Type: ConfigAttribRTFormat, Value: RTFormatYUV420},
		{Type: ConfigAttribRateControl, Value: RCCQP},
	}
	var cfg uint32
	err := check("vaCreateConfig", vaCreateConfig(d.handle, int32(profile), int32(entry),
		unsafe.Pointer(&attribs[0]), int32(len(attribs)), &cfg))
	if err != nil {
		return 0, err
	}
	return cfg, nil
}
