//go:build !windows

package rt

import "os"

// killProcess sends SIGKILL (os.Process.Kill).
func killProcess(p *os.Process) error { return p.Kill() }
