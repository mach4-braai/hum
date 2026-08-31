package main

import (
	"errors"
	"fmt"
	"io/fs"
	"syscall"
)

const wsaeconnrefused syscall.Errno = 10061

func classifyDialError(err error, socket string) error {
	if errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("%w at %s", errSocketMissing, socket)
	}
	if errors.Is(err, wsaeconnrefused) || errors.Is(err, syscall.WSAECONNRESET) {
		return fmt.Errorf("%w at %s: socket file exists but nothing is accepting; a fresh daemon replaces it", errSocketStale, socket)
	}
	return fmt.Errorf("%w at %s; try a longer --timeout", errNoDaemon, socket)
}
