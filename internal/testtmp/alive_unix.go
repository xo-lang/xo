//go:build unix

package testtmp

import "syscall"

// alive reports whether process pid exists. Signal 0 delivers nothing;
// it only checks that the process exists.
func alive(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || err == syscall.EPERM
}
