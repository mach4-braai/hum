package main

import (
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/mach4-braai/hum/internal/paths"
)

const (
	launchdLabel  = "homebrew.mxcl.hum"
	systemdUnit   = "humd"
	startedByHand = "started manually"
	noLogFile     = "no log file"
)

func runCommandQuietly(name string, args ...string) error {
	return exec.Command(name, args...).Run()
}

var runQuietly = runCommandQuietly

func detectSupervisor() (string, bool) {
	if runQuietly("launchctl", "print", "gui/"+strconv.Itoa(os.Getuid())+"/"+launchdLabel) == nil {
		return "launchd", true
	}
	if runQuietly("systemctl", "--user", "is-active", systemdUnit) == nil {
		return "systemd", true
	}
	return "", false
}

var logFile = paths.LogFile

func supervisorLog() string {
	if log, ok := logFile(); ok {
		return log
	}
	return noLogFile
}

func doctorSupervisorCheck(daemonOK bool) doctorCheck {
	name, supervised := detectSupervisor()

	var parts []string
	if supervised {
		parts = append(parts, name)
	} else if daemonOK {
		parts = append(parts, startedByHand)
	}
	if !daemonOK {
		parts = append(parts, "not running")
	}
	parts = append(parts, supervisorLog())

	return doctorCheck{doctorStatus(daemonOK), "supervisor", strings.Join(parts, ", ")}
}

func doctorStatus(ok bool) string {
	if ok {
		return "pass"
	}
	return "warn"
}
