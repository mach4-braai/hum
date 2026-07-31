//go:build !windows

package transport

import (
	"errors"
	"io/fs"
	"syscall"
)

func notListening(err error) bool {
	return errors.Is(err, syscall.ECONNREFUSED) || errors.Is(err, fs.ErrNotExist)
}
