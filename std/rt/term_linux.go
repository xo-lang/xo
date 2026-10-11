//go:build linux && (amd64 || arm64 || 386 || arm || riscv64 || s390x || loong64)

package rt

const (
	ioctlGetTermios = 0x5401 // TCGETS
	ioctlSetTermios = 0x5402 // TCSETS
	termEcho        = 0x8    // ECHO
)
