//go:build unix

package xotool

import "syscall"

// reuseAddr sets SO_REUSEADDR on a socket before it binds (DialLoopback).
var reuseAddr = func(_, _ string, c syscall.RawConn) error {
	var err error
	if e := c.Control(func(fd uintptr) {
		err = syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1)
	}); e != nil {
		return e
	}
	return err
}
