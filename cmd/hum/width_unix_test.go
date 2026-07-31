//go:build !windows

package main

import (
	"os"
	"syscall"
	"testing"
)

func TestStatusWidthReportsTheTerminalWidth(t *testing.T) {
	t.Cleanup(func() { readTerminalColumns = terminalColumns })
	readTerminalColumns = func(uintptr) (uint16, syscall.Errno) { return 120, 0 }

	f, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatalf("open %s: %v", os.DevNull, err)
	}
	t.Cleanup(func() { f.Close() })

	if got := statusWidth(f); got != 120 {
		t.Errorf("statusWidth = %d, want 120: a real terminal reports its columns so titles can be truncated", got)
	}
}

func TestStatusWidthIsUnknownWhenTheIoctlFails(t *testing.T) {
	t.Cleanup(func() { readTerminalColumns = terminalColumns })
	readTerminalColumns = func(uintptr) (uint16, syscall.Errno) { return 0, syscall.ENOTTY }

	f, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatalf("open %s: %v", os.DevNull, err)
	}
	t.Cleanup(func() { f.Close() })

	if got := statusWidth(f); got != 0 {
		t.Errorf("statusWidth = %d, want 0: an unknown width disables truncation", got)
	}
}
