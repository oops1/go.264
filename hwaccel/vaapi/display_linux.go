package vaapi

import (
	"errors"
	"fmt"
	"os"
	"unsafe"
)

type display struct {
	node   *os.File
	handle uintptr
	major  int32
	minor  int32
}

func renderNodeCandidates() []string {
	nodes := make([]string, 0, 16)
	for i := 128; i < 144; i++ {
		nodes = append(nodes, fmt.Sprintf("/dev/dri/renderD%d", i))
	}
	return nodes
}

func openDisplay() (*display, error) {
	if err := loadLibrary(); err != nil {
		return nil, err
	}
	var lastErr error
	for _, path := range renderNodeCandidates() {
		d, err := openDisplayAt(path)
		if err == nil {
			return d, nil
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = errors.New("vaapi: no DRM render node was found")
	}
	return nil, lastErr
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
	return &display{node: f, handle: handle, major: major, minor: minor}, nil
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

func (d *display) findEncodeProfile() (profileChoice, error) {
	max := vaMaxNumEntrypoints(d.handle)
	if max <= 0 {
		max = 32
	}
	entrypoints := make([]int32, max)
	for _, cand := range candidateProfiles {
		n := int32(len(entrypoints))
		st := vaQueryConfigEntrypoints(d.handle, int32(cand.profile), unsafe.Pointer(&entrypoints[0]), &n)
		if Status(st) != StatusSuccess {
			continue
		}
		for _, e := range entrypoints[:n] {
			if Entrypoint(e) == EntrypointEncSlice {
				return cand, nil
			}
		}
	}
	return profileChoice{}, errors.New("vaapi: no H.264 encode entry point was found")
}

func (d *display) createConfig(profile Profile) (uint32, error) {
	attribs := [2]ConfigAttrib{
		{Type: ConfigAttribRTFormat, Value: RTFormatYUV420},
		{Type: ConfigAttribRateControl, Value: RCCQP},
	}
	var cfg uint32
	err := check("vaCreateConfig", vaCreateConfig(d.handle, int32(profile), int32(EntrypointEncSlice),
		unsafe.Pointer(&attribs[0]), int32(len(attribs)), &cfg))
	if err != nil {
		return 0, err
	}
	return cfg, nil
}
