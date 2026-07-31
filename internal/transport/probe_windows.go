package transport

import (
	"errors"
	"io/fs"
	"syscall"
)

const wsaeconnrefused syscall.Errno = 10061

func notListening(err error) bool {
	return errors.Is(err, wsaeconnrefused) ||
		errors.Is(err, syscall.WSAECONNRESET) ||
		errors.Is(err, fs.ErrNotExist)
}
