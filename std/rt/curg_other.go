//go:build !amd64 && !arm64

package rt

import "runtime"

// curG identifies the running goroutine (see curg_asm.go). Without an
// assembly version for this architecture it parses the goroutine id from
// the first line of runtime.Stack ("goroutine 12 [running]:"): exact, but
// about a microsecond.
func curG() uintptr {
	var buf [64]byte
	n := runtime.Stack(buf[:], false)
	var id uintptr
	for _, c := range buf[len("goroutine "):n] {
		if c < '0' || c > '9' {
			break
		}
		id = id*10 + uintptr(c-'0')
	}
	return id
}
