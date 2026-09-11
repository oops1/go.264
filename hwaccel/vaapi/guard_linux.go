package vaapi

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

const probeEnv = "GO264_VAAPI_PROBE"

const probeRefusedExit = 3

var probeTimeout = 20 * time.Second

var probeCommand = func(ctx context.Context, spec string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "/proc/self/exe")
	cmd.Env = append(os.Environ(), probeEnv+"="+spec)
	return cmd
}

func init() {
	if spec, ok := os.LookupEnv(probeEnv); ok {
		os.Exit(runProbeChild(spec))
	}
}

type probeRefusal struct {
	reason string
}

func (p *probeRefusal) Error() string { return "vaapi: " + p.reason }

type processGuard struct {
	mu       sync.Mutex
	passed   bool
	fatal    error
	refusals map[string]error
}

var guard processGuard

func (g *processGuard) check(cfg Config) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.passed {
		return nil
	}
	if g.fatal != nil {
		return g.fatal
	}
	spec := probeSpec(cfg)
	if err, seen := g.refusals[spec]; seen {
		return err
	}
	err := probeInChild(spec)
	var refused *probeRefusal
	switch {
	case err == nil:
		g.passed = true
	case errors.As(err, &refused):
		if g.refusals == nil {
			g.refusals = make(map[string]error)
		}
		g.refusals[spec] = err
	default:
		g.fatal = err
	}
	return err
}

func probeSpec(cfg Config) string {
	return fmt.Sprintf("%d,%d,%d,%d,%d,%d", cfg.Width, cfg.Height, cfg.FPSNum, cfg.FPSDen, cfg.GOPLength, cfg.QP)
}

func parseProbeSpec(spec string) (Config, error) {
	parts := strings.Split(spec, ",")
	if len(parts) != 6 {
		return Config{}, fmt.Errorf("malformed probe specification %q", spec)
	}
	var v [6]int
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return Config{}, fmt.Errorf("malformed probe specification %q", spec)
		}
		v[i] = n
	}
	return Config{Width: v[0], Height: v[1], FPSNum: v[2], FPSDen: v[3], GOPLength: v[4], QP: v[5]}, nil
}

func runProbeChild(spec string) int {
	cfg, err := parseProbeSpec(spec)
	if err == nil {
		err = cfg.valid()
	}
	if err != nil {
		fmt.Println(err)
		return probeRefusedExit
	}
	e, err := openUnguarded(cfg)
	if err != nil {
		fmt.Println(err)
		return probeRefusedExit
	}
	fmt.Println(e.Describe())
	e.Close()
	return 0
}

func lastLine(b []byte) string {
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	return strings.TrimSpace(lines[len(lines)-1])
}

func probeInChild(spec string) error {
	ctx, cancel := context.WithTimeout(context.Background(), probeTimeout)
	defer cancel()
	var stdout, stderr bytes.Buffer
	cmd := probeCommand(ctx, spec)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	cmd.WaitDelay = time.Second
	err := cmd.Run()
	if ctx.Err() != nil {
		return fmt.Errorf("vaapi: a probe process was still running after %v, so the driver is not trusted in this process", probeTimeout)
	}
	if err == nil {
		return nil
	}
	var exit *exec.ExitError
	if !errors.As(err, &exit) {
		return fmt.Errorf("vaapi: a probe process could not be started: %w", err)
	}
	if exit.ExitCode() == probeRefusedExit {
		reason := strings.TrimPrefix(lastLine(stdout.Bytes()), "vaapi: ")
		if reason == "" {
			reason = "a probe process refused without saying why"
		}
		return &probeRefusal{reason: reason}
	}
	detail := crashReason(stderr.Bytes())
	if detail == "" {
		detail = "no message"
	}
	return fmt.Errorf("vaapi: the driver brought down a probe process (%v), so it is not used in this process: %s", exit, detail)
}
