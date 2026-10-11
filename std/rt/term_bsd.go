//go:build darwin || freebsd || netbsd || openbsd || dragonfly

package rt

import "syscall"

const (
	ioctlGetTermios = syscall.TIOCGETA
	ioctlSetTermios = syscall.TIOCSETA
	termEcho        = 0x8 // ECHO
)
