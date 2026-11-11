//go:build unix

package main

import (
	"errors"
	"os"
	"syscall"
	"time"
)

// stopGroup kills p and, when p leads a process group of its own (a
// testtmp.Guard'ed child), every process in it, then waits up to 10 s
// for the group to be gone: a git that `xo mod proxy serve` started for
// a fetch outlived the serve and wrote into the test's module cache
// while TempDir's RemoveAll ran ("directory not empty",
// TestChecksumLogFillRetry, tier 910, 2026-11-11). A p in this process's
// group is killed alone.
func stopGroup(p *os.Process) {
	g, err := syscall.Getpgid(p.Pid)
	if err != nil || g != p.Pid || g == syscall.Getpgrp() {
		p.Kill()
		return
	}
	syscall.Kill(-g, syscall.SIGKILL)
	p.Kill()
	for end := time.Now().Add(10 * time.Second); time.Now().Before(end); time.Sleep(20 * time.Millisecond) {
		if err := syscall.Kill(-g, 0); errors.Is(err, syscall.ESRCH) {
			return
		}
	}
}
