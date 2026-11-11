//go:build !unix

package main

import "os"

// stopGroup stops p and the processes it started (stopgroup_unix_test.go;
// on Windows killTree's taskkill /T).
func stopGroup(p *os.Process) { killTree(p) }
