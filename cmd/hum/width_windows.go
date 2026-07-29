//go:build windows

package main

import "io"

func statusWidth(io.Writer) int {
	return 0
}
