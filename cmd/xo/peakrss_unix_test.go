//go:build !windows

package main

import (
	"os"
	"runtime"
	"syscall"
)

// procPeakRSS is a finished process's peak resident set in bytes.
func procPeakRSS(ps *os.ProcessState) int64 {
	rss := ps.SysUsage().(*syscall.Rusage).Maxrss
	if runtime.GOOS != "darwin" {
		rss *= 1024 // kilobytes on Linux
	}
	return rss
}

const hasProcPeakRSS = true
