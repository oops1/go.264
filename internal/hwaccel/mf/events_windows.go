package mf

import "unsafe"

const (
	transformEventUnknown = 600 + iota
	transformEventNeedInput
	transformEventHaveOutput
	transformEventDrainComplete
	transformEventMarker
	transformEventInputStreamStateChanged
)

const eventFlagNoWait = 1

const (
	mfENoEventsAvailable   HRESULT = 0xc00d3e80
	mfETransformTypeNotSet HRESULT = 0xc00d6d60
)

type eventGenerator struct {
	obj unknown
}

func (t *Transform) eventGenerator() (*eventGenerator, error) {
	obj, err := t.obj.queryInterface(&IIDIMFMediaEventGenerator)
	if err != nil {
		return nil, err
	}
	return &eventGenerator{obj: obj}, nil
}

func (g *eventGenerator) release() {
	if g == nil {
		return
	}
	g.obj.release()
	g.obj = unknown{}
}

type transformEvent struct {
	kind   uint32
	status HRESULT
}

func (g *eventGenerator) next(wait bool) (transformEvent, bool, error) {
	flags := uintptr(eventFlagNoWait)
	if wait {
		flags = 0
	}
	var raw unsafe.Pointer
	code := hr(g.obj.p, eventGeneratorGetEvent, flags, uintptr(unsafe.Pointer(&raw)))
	if code == mfENoEventsAvailable {
		return transformEvent{}, false, nil
	}
	if err := check("IMFMediaEventGenerator::GetEvent", code); err != nil {
		return transformEvent{}, false, err
	}
	ev := unknown{raw}
	defer ev.release()

	var e transformEvent
	if code := hr(ev.p, mediaEventGetType, uintptr(unsafe.Pointer(&e.kind))); code.Failed() {
		return transformEvent{}, false, check("IMFMediaEvent::GetType", code)
	}
	if code := hr(ev.p, mediaEventGetStatus, uintptr(unsafe.Pointer(&e.status))); code.Failed() {
		return transformEvent{}, false, check("IMFMediaEvent::GetStatus", code)
	}
	return e, true, nil
}
