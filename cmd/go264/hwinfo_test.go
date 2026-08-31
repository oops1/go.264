package main

import (
	"strings"
	"testing"
)

func TestHardwareInfoNamesTheBackends(t *testing.T) {
	var out strings.Builder
	if err := runHardwareInfo(&out); err != nil {
		t.Fatalf("runHardwareInfo: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "backends:") {
		t.Fatalf("the report names no backends:\n%s", got)
	}
	if !strings.Contains(got, "cpu") {
		t.Fatalf("the report omits the processor backend:\n%s", got)
	}
	if strings.Count(got, "\n") < 2 {
		t.Fatalf("the report says nothing about platform transforms:\n%s", got)
	}
}

func TestHardwareInfoIsReachableFromTheCommandLine(t *testing.T) {
	if err := run([]string{"hwinfo"}); err != nil {
		t.Fatalf("go264 hwinfo: %v", err)
	}
}
