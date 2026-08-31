//go:build !windows

package session

import (
	"errors"
	"syscall"
)

var pidAlive = realPidAlive

func realPidAlive(pid int) bool {
	err := syscall.Kill(pid, 0)
	if err == nil {
		return true
	}
	return errors.Is(err, syscall.EPERM)
}
