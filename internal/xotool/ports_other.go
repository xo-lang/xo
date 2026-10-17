//go:build !unix

package xotool

import "syscall"

// reuseAddr is nil without SO_REUSEADDR's BSD meaning (Windows lets it
// take a port another socket holds): DialLoopback dials as usual.
var reuseAddr func(_, _ string, c syscall.RawConn) error
