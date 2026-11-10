//go:build !linux

package rt

import "syscall"

// The kernel keyring is Linux only (decision 0584): Secrets.keyring() is
// None elsewhere, so these are never reached.

const keyringAvailable = false

const keyctlUnlink = 9

func keyctl(op int, a, b, c, d uintptr) (int64, syscall.Errno) { return -1, syscall.ENOSYS }
func addKey(kind, desc string, payload []byte, ring int64) (int64, syscall.Errno) {
	return -1, syscall.ENOSYS
}
func keyctlSearchKey(ring int64, kind, desc string) (int64, syscall.Errno) {
	return -1, syscall.ENOSYS
}
func keyctlReadKey(id int64) ([]byte, syscall.Errno) { return nil, syscall.ENOSYS }
func keyUserPersistent() (int64, syscall.Errno)      { return -1, syscall.ENOSYS }
