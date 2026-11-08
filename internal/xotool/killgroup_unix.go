//go:build unix

package xotool

import (
	"os"
	"syscall"
)

// killGroup kills p and, when p leads a process group of its own (a
// testtmp.Guard'ed child: the lifeline wrapper makes it one), every
// process in that group: the clang an LLVM build was waiting on kept
// running at full CPU after the watchdog killed xo alone (2026-11-07,
// gotchas/watchdog-kill-orphans-children.md). A p in the caller's group
// is killed alone.
func killGroup(p *os.Process) {
	if g, err := syscall.Getpgid(p.Pid); err == nil && g == p.Pid && g != syscall.Getpgrp() {
		_ = syscall.Kill(-g, syscall.SIGKILL)
	}
	_ = p.Kill()
}
