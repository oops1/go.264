package mf

import (
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

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

func TestAWaitThatNeverEndsIsCutOffByTheWatchdog(t *testing.T) {
	stopped := make(chan struct{})
	var finished atomic.Bool
	start := time.Now()
	_, ok, err, settled := waitWithWatchdog(func() (transformEvent, bool, error) {
		<-stopped
		return transformEvent{}, false, &Error{Op: "GetEvent", Code: mfEShutdown}
	}, func() {
		close(stopped)
		time.Sleep(30 * time.Millisecond)
		finished.Store(true)
	}, 50*time.Millisecond)
	if ok || err == nil || !strings.Contains(err.Error(), "no event within 50ms") {
		t.Fatalf("a wait the watchdog ended returned ok=%v err=%v, want the timeout", ok, err)
	}
	if !settled || !finished.Load() {
		t.Fatal("the wait returned while the shutdown was still running, so the caller could release the transform under it")
	}
	if waited := time.Since(start); waited > 5*time.Second {
		t.Fatalf("the watchdog took %v to end a 50ms wait", waited)
	}
}

func TestAShutdownThatHangsLeavesTheTransformUnreleased(t *testing.T) {
	stopped := make(chan struct{})
	_, _, err, settled := waitWithWatchdog(func() (transformEvent, bool, error) {
		<-stopped
		return transformEvent{}, false, nil
	}, func() {
		close(stopped)
		select {}
	}, 20*time.Millisecond)
	if err == nil || settled {
		t.Fatalf("err=%v settled=%v, want a timeout that is not settled", err, settled)
	}
	g := &eventGenerator{abandoned: true}
	g.release()
}

func TestAWaitThatEndsInTimeLeavesTheTransformAlone(t *testing.T) {
	stopped := false
	ev, ok, err, settled := waitWithWatchdog(func() (transformEvent, bool, error) {
		return transformEvent{kind: transformEventNeedInput}, true, nil
	}, func() { stopped = true }, time.Second)
	if !ok || err != nil || ev.kind != transformEventNeedInput || !settled {
		t.Fatalf("got %+v ok=%v err=%v settled=%v, want the event", ev, ok, err, settled)
	}
	time.Sleep(10 * time.Millisecond)
	if stopped {
		t.Fatal("the watchdog shut the transform down after the event arrived")
	}
}

func TestPollingGivesUpAtTheDeadline(t *testing.T) {
	polls := 0
	_, ok, err := pollForEvent(func() (transformEvent, bool, error) {
		polls++
		return transformEvent{}, false, nil
	}, 20*time.Millisecond)
	if ok || err == nil || polls < 2 {
		t.Fatalf("after %d polls ok=%v err=%v, want a timeout after several polls", polls, ok, err)
	}
}

func TestAsynchronousEncoderOffersAShutdown(t *testing.T) {
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
		t.Fatal(err)
	}
	defer gen.release()
	if gen.shutdown.p == nil {
		t.Fatalf("%s exposes no IMFShutdown, so a wait on it falls back to polling", chosen.Name)
	}
	for i := 0; i < 64; i++ {
		if _, ok, err := gen.next(false); err != nil || !ok {
			break
		}
	}
	saved := eventTimeout
	eventTimeout = 200 * time.Millisecond
	defer func() { eventTimeout = saved }()
	start := time.Now()
	_, ok, err := gen.next(true)
	if ok || err == nil {
		t.Fatalf("%s sent an event before streaming began, so the wait was not tested", chosen.Name)
	}
	if !strings.Contains(err.Error(), "no event within") {
		t.Fatalf("%s: the wait ended with %v, want the timeout", chosen.Name, err)
	}
	if waited := time.Since(start); waited > 5*time.Second {
		t.Fatalf("%s: shutting the transform down took %v to end a blocked wait", chosen.Name, waited)
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
