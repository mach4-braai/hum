//go:build !windows

package session

import (
	"errors"
	"syscall"
)

var probeOwner = realProbeOwner

func realProbeOwner(pid int) ownerState {
	err := syscall.Kill(pid, 0)
	if err == nil {
		return ownerRunning
	}
	if errors.Is(err, syscall.EPERM) {
		return ownerUnknown
	}
	return ownerExited
}
