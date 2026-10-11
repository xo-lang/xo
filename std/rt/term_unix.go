//go:build darwin || freebsd || netbsd || openbsd || dragonfly || (linux && (amd64 || arm64 || 386 || arm || riscv64 || s390x || loong64))

package rt

import (
	"os"
	"syscall"
	"unsafe"
)

// termios reads f's terminal attributes; false when f is not a terminal.
func termios(f *os.File) (syscall.Termios, bool) {
	var t syscall.Termios
	_, _, e := syscall.Syscall(syscall.SYS_IOCTL, f.Fd(), ioctlGetTermios, uintptr(unsafe.Pointer(&t)))
	return t, e == 0
}

func isTerminal(f *os.File) bool {
	_, ok := termios(f)
	return ok
}

// termWidth is the columns of f's terminal, or 0.
func termWidth(f *os.File) int {
	var ws struct{ row, col, x, y uint16 }
	_, _, e := syscall.Syscall(syscall.SYS_IOCTL, f.Fd(), syscall.TIOCGWINSZ, uintptr(unsafe.Pointer(&ws)))
	if e != 0 {
		return 0
	}
	return int(ws.col)
}

// echoOff turns f's echo off and returns the function that turns it back
// on; nil when f is not a terminal.
func echoOff(f *os.File) func() {
	t, ok := termios(f)
	if !ok {
		return nil
	}
	old := t
	t.Lflag &^= termEcho
	if _, _, e := syscall.Syscall(syscall.SYS_IOCTL, f.Fd(), ioctlSetTermios, uintptr(unsafe.Pointer(&t))); e != 0 {
		return nil
	}
	return func() {
		syscall.Syscall(syscall.SYS_IOCTL, f.Fd(), ioctlSetTermios, uintptr(unsafe.Pointer(&old)))
	}
}
