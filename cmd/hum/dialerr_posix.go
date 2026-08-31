//go:build !windows

package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"syscall"
)

func classifyDialError(err error, socket string) error {
	if errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("%w at %s", errSocketMissing, socket)
	}
	if errors.Is(err, syscall.ECONNREFUSED) {
		return fmt.Errorf("%w at %s: socket file exists but nothing is accepting; a fresh daemon replaces it", errSocketStale, socket)
	}
	if errors.Is(err, syscall.EACCES) || errors.Is(err, syscall.EPERM) {
		return deniedError(socket)
	}
	return fmt.Errorf("%w at %s; try a longer --timeout", errNoDaemon, socket)
}

func deniedError(socket string) error {
	currentUID := os.Getuid()
	info, err := os.Stat(socket)
	if err != nil {
		return fmt.Errorf("%w: %s is not accessible to uid %d", errSocketDenied, socket, currentUID)
	}
	st := info.Sys().(*syscall.Stat_t)
	return fmt.Errorf("%w: %s is owned by uid %d (mode %04o), current uid %d",
		errSocketDenied, socket, st.Uid, info.Mode().Perm(), currentUID)
}
