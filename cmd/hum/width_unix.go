//go:build !windows

package main

import (
	"io"
	"os"
	"syscall"
	"unsafe"
)

func terminalColumns(fd uintptr) (uint16, syscall.Errno) {
	var size struct{ rows, cols, xpixel, ypixel uint16 }
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, fd, syscall.TIOCGWINSZ, uintptr(unsafe.Pointer(&size)))
	return size.cols, errno
}

var readTerminalColumns = terminalColumns

func statusWidth(w io.Writer) int {
	file, ok := w.(*os.File)
	if !ok {
		return 0
	}
	cols, errno := readTerminalColumns(file.Fd())
	if errno != 0 || cols == 0 {
		return 0
	}
	return int(cols)
}
