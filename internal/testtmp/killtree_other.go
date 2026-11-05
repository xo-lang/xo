//go:build !unix

package testtmp

import "os"

// killTree stops p (no process groups here).
func killTree(p *os.Process) { _ = p.Kill() }
