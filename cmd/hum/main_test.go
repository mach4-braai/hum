package main

import (
	"bytes"
	"strings"
	"testing"
)

// The CLI must be usable from shell scripts, so exit codes and stream
// discipline are part of its contract, not incidental. Usage text belongs on
// stderr: stdout is reserved for machine-readable output that callers capture.
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
