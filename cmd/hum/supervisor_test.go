package main

import (
	"errors"
	"strings"
	"testing"

	"github.com/mach4-braai/hum/internal/paths"
)

func stubProbe(t *testing.T, answer func(name string) error) {
	t.Helper()
	original := runQuietly
	t.Cleanup(func() { runQuietly = original })
	runQuietly = func(name string, _ ...string) error { return answer(name) }
}

func TestDetectSupervisorNamesLaunchd(t *testing.T) {
	stubProbe(t, func(name string) error {
		if name == "launchctl" {
			return nil
		}
		return errors.New("not loaded")
	})

	name, supervised := detectSupervisor()

	if !supervised || name != "launchd" {
		t.Errorf("detectSupervisor = %q, %v, want launchd, true", name, supervised)
	}
}

func TestDetectSupervisorNamesSystemd(t *testing.T) {
	stubProbe(t, func(name string) error {
		if name == "systemctl" {
			return nil
		}
		return errors.New("no such label")
	})

	name, supervised := detectSupervisor()

	if !supervised || name != "systemd" {
		t.Errorf("detectSupervisor = %q, %v, want systemd, true", name, supervised)
	}
}

func TestDetectSupervisorReportsNeither(t *testing.T) {
	stubProbe(t, func(string) error { return errors.New("absent") })

	if name, supervised := detectSupervisor(); supervised {
		t.Errorf("detectSupervisor = %q, %v, want no supervisor", name, supervised)
	}
}

func TestSupervisorCheckNamesTheSupervisorAndItsLog(t *testing.T) {
	stubProbe(t, func(name string) error {
		if name == "launchctl" {
			return nil
		}
		return errors.New("absent")
	})

	check := doctorSupervisorCheck(true)

	if check.Status != "pass" {
		t.Errorf("status = %q, want pass", check.Status)
	}
	if !strings.HasPrefix(check.Detail, "launchd, ") {
		t.Errorf("detail = %q, want it to name launchd first", check.Detail)
	}
	if !strings.HasSuffix(check.Detail, wantLog(t)) {
		t.Errorf("detail = %q, want it to end with the log location", check.Detail)
	}
}

func TestSupervisorCheckDoesNotFailAManualStart(t *testing.T) {
	stubProbe(t, func(string) error { return errors.New("absent") })

	check := doctorSupervisorCheck(true)

	if check.Status != "pass" {
		t.Errorf("status = %q, want pass: a foreground humd is a legitimate way to run it", check.Status)
	}
	if !strings.Contains(check.Detail, startedByHand) {
		t.Errorf("detail = %q, want %q", check.Detail, startedByHand)
	}
}

func TestSupervisorCheckStillReportsTheLogWithNoDaemon(t *testing.T) {
	stubProbe(t, func(string) error { return errors.New("absent") })

	check := doctorSupervisorCheck(false)

	if check.Status != "warn" {
		t.Errorf("status = %q, want warn", check.Status)
	}
	if !strings.Contains(check.Detail, "not running") {
		t.Errorf("detail = %q, want it to say the daemon is down", check.Detail)
	}
	if !strings.HasSuffix(check.Detail, wantLog(t)) {
		t.Errorf("detail = %q, want the log location so a crash can be inspected", check.Detail)
	}
}

func TestSupervisorCheckKeepsTheSupervisorNameWhenDown(t *testing.T) {
	stubProbe(t, func(name string) error {
		if name == "systemctl" {
			return nil
		}
		return errors.New("absent")
	})

	check := doctorSupervisorCheck(false)

	if !strings.HasPrefix(check.Detail, "systemd, not running") {
		t.Errorf("detail = %q, want the supervisor named before its state", check.Detail)
	}
}

func wantLog(t *testing.T) string {
	t.Helper()
	if log, ok := paths.LogFile(); ok {
		return log
	}
	return noLogFile
}
