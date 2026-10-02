package session

import (
	"errors"
	"syscall"
)

const (
	processQueryLimitedInformation = 0x1000
	stillActive                    = 259
)

var probeOwner = realProbeOwner

func realProbeOwner(pid int) ownerState {
	h, err := syscall.OpenProcess(processQueryLimitedInformation, false, uint32(pid))
	if errors.Is(err, syscall.ERROR_ACCESS_DENIED) {
		return ownerUnknown
	}
	if err != nil {
		return ownerExited
	}
	defer syscall.CloseHandle(h)
	var code uint32
	if err := syscall.GetExitCodeProcess(h, &code); err != nil {
		return ownerExited
	}
	if code == stillActive {
		return ownerRunning
	}
	return ownerExited
}
