//go:build linux && (amd64 || arm64)

package vaapi

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func withProbeCommand(t *testing.T, script string) *int {
	t.Helper()
	restore, restoreNodes := probeCommand, anyRenderNode
	t.Cleanup(func() { probeCommand, anyRenderNode = restore, restoreNodes })
	anyRenderNode = func() bool { return true }
	calls := new(int)
	probeCommand = func(ctx context.Context, spec string) *exec.Cmd {
		*calls++
		return exec.CommandContext(ctx, "sh", "-c", script)
	}
	return calls
}

func freshGuard(t *testing.T) *processGuard {
	t.Helper()
	return &processGuard{}
}

var guardCfg = Config{Width: 640, Height: 368, FPSNum: 30, FPSDen: 1, GOPLength: 30, QP: 22}

func TestAProbeThatSucceedsLetsEveryLaterOpenThrough(t *testing.T) {
	calls := withProbeCommand(t, "echo 'renderD128, Intel iHD, profile 6, VAEntrypointEncSliceLP'")
	g := freshGuard(t)
	for i := 0; i < 3; i++ {
		other := guardCfg
		other.Width += 16 * i
		if err := g.check(other); err != nil {
			t.Fatalf("check %d: %v", i, err)
		}
	}
	if *calls != 1 {
		t.Fatalf("a driver that passed once was probed %d times", *calls)
	}
}

func TestADriverThatKillsTheProbeIsNeverOpenedInProcess(t *testing.T) {
	calls := withProbeCommand(t, "echo 'i965_encoder.c:1692: intel_enc_hw_context_init: Assertion failed.' >&2; kill -ABRT $$")
	g := freshGuard(t)
	err := g.check(guardCfg)
	if err == nil {
		t.Fatal("a probe killed by SIGABRT was taken as a working driver")
	}
	var refused *probeRefusal
	if errors.As(err, &refused) {
		t.Fatalf("a crash was reported as a clean refusal: %v", err)
	}
	for _, want := range []string{"aborted", "Assertion failed"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("%q does not say %q", err, want)
		}
	}
	other := guardCfg
	other.Width = 1920
	if err := g.check(other); err == nil {
		t.Fatal("after a crash another picture size was allowed through")
	}
	if *calls != 1 {
		t.Fatalf("a driver that crashed was probed %d times; once is enough to know", *calls)
	}
}

func TestACleanRefusalIsRememberedForItsOwnPictureSizeOnly(t *testing.T) {
	calls := withProbeCommand(t, "echo 'vaapi: no H.264 encode entry point was found'; exit 3")
	g := freshGuard(t)
	err := g.check(guardCfg)
	var refused *probeRefusal
	if !errors.As(err, &refused) {
		t.Fatalf("a clean refusal came back as %v", err)
	}
	if !strings.Contains(err.Error(), "no H.264 encode entry point") {
		t.Fatalf("the reason was lost: %v", err)
	}
	if strings.Count(err.Error(), "vaapi:") != 1 {
		t.Fatalf("the prefix was doubled: %v", err)
	}
	g.check(guardCfg)
	if *calls != 1 {
		t.Fatalf("the same refused size was probed %d times", *calls)
	}
	other := guardCfg
	other.Width = 1920
	g.check(other)
	if *calls != 2 {
		t.Fatalf("a different size was not given its own probe; %d probes", *calls)
	}
}

func TestAProbeThatHangsIsTreatedAsAnUntrustedDriver(t *testing.T) {
	withProbeCommand(t, "sleep 30")
	restore := probeTimeout
	probeTimeout = 300 * time.Millisecond
	t.Cleanup(func() { probeTimeout = restore })
	start := time.Now()
	err := freshGuard(t).check(guardCfg)
	if err == nil || !strings.Contains(err.Error(), "still running") {
		t.Fatalf("a hung probe gave %v", err)
	}
	if took := time.Since(start); took > 5*time.Second {
		t.Fatalf("the hung probe was waited on for %v", took)
	}
}

func TestTheProbeSpecificationRoundTrips(t *testing.T) {
	got, err := parseProbeSpec(probeSpec(guardCfg))
	if err != nil || got != guardCfg {
		t.Fatalf("round trip gave %+v, %v", got, err)
	}
	for _, bad := range []string{"", "1,2,3", "a,b,c,d,e,f", "640,368,30,1,30"} {
		if _, err := parseProbeSpec(bad); err == nil {
			t.Fatalf("%q was accepted", bad)
		}
	}
}

func TestTheRealProbeProcessExitsBeforeMain(t *testing.T) {
	var bad Config
	err := probeInChild(probeSpec(bad))
	var refused *probeRefusal
	if !errors.As(err, &refused) {
		t.Fatalf("re-executing this binary as a probe gave %v, want a clean refusal", err)
	}
	if !strings.Contains(err.Error(), "even positive dimensions") {
		t.Fatalf("the child did not run the probe: %v", err)
	}
	cmd := probeCommand(context.Background(), probeSpec(bad))
	out, _ := cmd.CombinedOutput()
	if strings.Contains(string(out), "PASS") || strings.Contains(string(out), "=== RUN") {
		t.Fatalf("the probe process ran the program's main as well:\n%s", out)
	}
}

func TestAMachineWithoutARenderNodeStartsNoProbe(t *testing.T) {
	calls := withProbeCommand(t, "exit 0")
	anyRenderNode = func() bool { return false }
	err := freshGuard(t).check(guardCfg)
	var refused *probeRefusal
	if !errors.As(err, &refused) || !strings.Contains(err.Error(), "render node") {
		t.Fatalf("with no render node the guard said %v", err)
	}
	if *calls != 0 {
		t.Fatalf("a machine with no graphics device started %d probe processes", *calls)
	}
}
