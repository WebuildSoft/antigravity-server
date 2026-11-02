//go:build windows

package main

import (
	"os"

	"golang.org/x/sys/windows"
)

func init() {
	enableVirtualTerminal(os.Stdout.Fd())
	enableVirtualTerminal(os.Stderr.Fd())
}

func enableVirtualTerminal(fd uintptr) {
	handle := windows.Handle(fd)
	var mode uint32
	if err := windows.GetConsoleMode(handle, &mode); err == nil {
		mode |= windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING
		_ = windows.SetConsoleMode(handle, mode)
	}
}
