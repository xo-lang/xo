//go:build !windows && !darwin && !freebsd && !netbsd && !openbsd && !dragonfly && !(linux && (amd64 || arm64 || 386 || arm || riscv64 || s390x || loong64))

package rt

import "os"

// No terminal support on this target (wasip1, js, plan9, Linux on
// mips and ppc): the streams are never terminals.

func isTerminal(*os.File) bool { return false }
func termWidth(*os.File) int   { return 0 }
func echoOff(*os.File) func()  { return nil }
