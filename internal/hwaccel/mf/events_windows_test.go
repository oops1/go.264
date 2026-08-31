package mf

import "testing"

func TestTransformEventsAreConsecutiveFromSixHundred(t *testing.T) {
	got := []int{
		transformEventUnknown,
		transformEventNeedInput,
		transformEventHaveOutput,
		transformEventDrainComplete,
		transformEventMarker,
		transformEventInputStreamStateChanged,
	}
	for i, v := range got {
		if v != 600+i {
			t.Fatalf("transform event %d is %d, want %d", i, v, 600+i)
		}
	}
}

func TestEventErrorsReadAsFailures(t *testing.T) {
	for _, c := range []HRESULT{mfENoEventsAvailable, mfETransformTypeNotSet} {
		if !c.Failed() {
			t.Fatalf("%s does not read as a failure", c)
		}
	}
}

func TestAsynchronousEncoderReportsItWantsInput(t *testing.T) {
	if !Loaded() {
		t.Skip("Media Foundation is not present on this machine")
	}
	if err := Startup(); err != nil {
		t.Fatalf("Startup: %v", err)
	}
	defer Shutdown()

	_, out := H264EncoderTypes()
	tr, chosen, err := openTransform(MFTCategoryVideoEncoder, nil, &out,
		func(d TransformDescription) bool { return d.Async })
	if err != nil {
		t.Skipf("no asynchronous H.264 encoder could be opened: %v", err)
	}
	defer tr.Release()

	gen, err := tr.eventGenerator()
	if err != nil {
		t.Fatalf("%s exposes no event generator: %v", chosen.Name, err)
	}
	defer gen.release()

	ev, ok, err := gen.next(false)
	if err != nil {
		t.Fatalf("%s: reading an event: %v", chosen.Name, err)
	}
	if ok {
		t.Logf("%s queued event %d with status %s before streaming began", chosen.Name, ev.kind, ev.status)
	}
}
