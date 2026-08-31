package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestStopRedirectsToHumDaemonStop(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"stop"}, &stdout, &stderr)
	if code != exitUsage {
		t.Fatalf("exit %d, want %d (usage); stdout=%q stderr=%q", code, exitUsage, stdout.String(), stderr.String())
	}
	if stdout.String() != "" {
		t.Errorf("stdout=%q, want empty — redirect must not write to stdout", stdout.String())
	}
	if !strings.Contains(stderr.String(), "daemon stop") {
		t.Errorf("stderr=%q, want 'daemon stop' hint", stderr.String())
	}
}
