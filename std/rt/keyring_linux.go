//go:build linux

package rt

import (
	"syscall"
	"unsafe"
)

// The kernel keyring calls of Keyring (decision 0584), raw syscalls as the
// native backends make them (std/native/llrt/linux.xo add_key, keyctl); the
// Go backend needs no golang.org/x/sys. Each returns the result and the
// errno (0 on success).

const (
	keyctlGetKeyringID  = 0
	keyctlLink          = 8
	keyctlUnlink        = 9
	keyctlSearch        = 10
	keyctlRead          = 11
	keyctlGetPersistent = 22
)

func keyctl(op int, a, b, c, d uintptr) (int64, syscall.Errno) {
	r, _, e := syscall.Syscall6(syscall.SYS_KEYCTL, uintptr(op), a, b, c, d, 0)
	if e != 0 {
		return -1, e
	}
	return int64(r), 0
}

func addKey(kind, desc string, payload []byte, ring int64) (int64, syscall.Errno) {
	k, err := syscall.BytePtrFromString(kind)
	if err != nil {
		return -1, syscall.EINVAL
	}
	d, err := syscall.BytePtrFromString(desc)
	if err != nil {
		return -1, syscall.EINVAL
	}
	var p uintptr
	if len(payload) > 0 {
		p = uintptr(unsafe.Pointer(&payload[0]))
	}
	r, _, e := syscall.Syscall6(syscall.SYS_ADD_KEY, uintptr(unsafe.Pointer(k)), uintptr(unsafe.Pointer(d)), p, uintptr(len(payload)), uintptr(ring), 0)
	if e != 0 {
		return -1, e
	}
	return int64(r), 0
}

func keyctlSearchKey(ring int64, kind, desc string) (int64, syscall.Errno) {
	k, err := syscall.BytePtrFromString(kind)
	if err != nil {
		return -1, syscall.EINVAL
	}
	d, err := syscall.BytePtrFromString(desc)
	if err != nil {
		return -1, syscall.EINVAL
	}
	return keyctl(keyctlSearch, uintptr(ring), uintptr(unsafe.Pointer(k)), uintptr(unsafe.Pointer(d)), 0)
}

func keyctlReadKey(id int64) ([]byte, syscall.Errno) {
	buf := make([]byte, 512)
	for {
		n, _, e := syscall.Syscall6(syscall.SYS_KEYCTL, keyctlRead, uintptr(id), uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)), 0, 0)
		if e != 0 {
			return nil, e
		}
		if int(n) <= len(buf) {
			return buf[:n], 0
		}
		buf = make([]byte, n)
	}
}

const keyringAvailable = true

// keyUserPersistent is userfs.xo's keyring.user_persistent: the persistent
// keyring linked to the user keyring, else the user keyring (the session
// keyring linked to it first, errors ignored).
func keyUserPersistent() (int64, syscall.Errno) {
	userKeyring, sessionKeyring := int64(-4), int64(-3)
	user, e := keyctl(keyctlGetKeyringID, uintptr(userKeyring), 1, 0, 0)
	if e != 0 {
		return -1, e
	}
	if sess, e := keyctl(keyctlGetKeyringID, uintptr(sessionKeyring), 1, 0, 0); e == 0 {
		keyctl(keyctlLink, uintptr(user), uintptr(sess), 0, 0)
	}
	if p, e := keyctl(keyctlGetPersistent, ^uintptr(0), uintptr(user), 0, 0); e == 0 {
		return p, 0
	}
	return user, 0
}
