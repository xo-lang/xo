//go:build !unix

package xotool

import "os"

// killGroup kills p (process groups are a Unix notion; killgroup_unix.go).
func killGroup(p *os.Process) { _ = p.Kill() }
