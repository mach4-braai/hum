package main

import (
	"bytes"
	"strings"
	"testing"
)

// Usage goes to stderr; stdout stays reserved for output callers capture.
func TestRunWithoutArgumentsPrintsUsageToStderrAndExitsTwo(t *testing.T) {
	var stdout, stderr bytes.Buffer

	code := run(nil, &stdout, &stderr)

	if code != 2 {
		t.Errorf("exit code = %d, want 2", code)
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want empty so callers can capture output cleanly", stdout.String())
	}
	if got := stderr.String(); !strings.Contains(got, "usage:") {
		t.Errorf("stderr = %q, want it to contain %q", got, "usage:")
	}
}
