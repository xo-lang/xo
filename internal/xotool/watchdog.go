package xotool

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"time"
)

// A build or check that a test starts and that does not return is a hang
// of xo itself (2026-10-23: an `xo build --backend=llvm` of a golden sat
// for 52 minutes with every worker idle and the main fiber in kevent; the
// sample said where the threads were and not which task waited on what).
// RunWatched is cmd.Run with a limit: when the child outlives it, the
// report below is taken while it still hangs (the task tree from
// `xo dump`, its open files, a stack sample), appended to the child's
// output, and the child is killed.

// BuildLimit is how long a build, check or asm run of the shared helpers
// may take before it counts as hung: $XO_BUILD_LIMIT (a Go duration, "0"
// for none), else 20 minutes (the slowest golden builds take a few
// minutes under the load of a full suite).
func BuildLimit() time.Duration {
	if v := os.Getenv("XO_BUILD_LIMIT"); v != "" {
		if v == "0" {
			return 0
		}
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return 20 * time.Minute
}

// ErrHung is RunWatched's error for a child killed at the limit.
var ErrHung = errors.New("xo did not finish within XO_BUILD_LIMIT")

// RunWatched runs cmd (an xo command line, xo being the xo command and its
// leading arguments as Command returns them) to the end, or for BuildLimit.
// A child still running then gets HangReport, written to out, and is killed
// (SIGKILL: the lifeline wrapper of testtmp.Guard ends its group); the
// result is ErrHung. out is the buffer the child writes to; it is only
// touched after the child has ended.
func RunWatched(cmd *exec.Cmd, xo []string, out *bytes.Buffer) error {
	limit := BuildLimit()
	if limit <= 0 {
		return cmd.Run()
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		return err
	case <-time.After(limit):
	}
	rep := HangReport(xo, cmd.Process.Pid)
	_ = cmd.Process.Kill()
	<-done
	fmt.Fprintf(out, "\nxotool: %v (%v): pid %d hung; its state before the kill:\n%s", ErrHung, limit, cmd.Process.Pid, rep)
	return ErrHung
}

// HangReport is what to know about the hung xo process pid, while it
// still runs: `xo dump` (the task tree of a native program, core 11.4:
// which task waits on what), lsof (its descriptors), and where it can,
// sample (macOS) for 3 seconds of stacks.
func HangReport(xo []string, pid int) string {
	var b bytes.Buffer
	run := func(name string, limit time.Duration, args ...string) {
		ctx, cancel := context.WithTimeout(context.Background(), limit)
		defer cancel()
		c := exec.CommandContext(ctx, args[0], args[1:]...)
		var o bytes.Buffer
		c.Stdout, c.Stderr = &o, &o
		err := c.Run()
		fmt.Fprintf(&b, "--- %s (%v)\n%s", name, err, o.String())
		if o.Len() > 0 && o.Bytes()[o.Len()-1] != '\n' {
			b.WriteByte('\n')
		}
	}
	p := strconv.Itoa(pid)
	if len(xo) > 0 && runtime.GOOS != "windows" {
		run("xo dump", 30*time.Second, append(append([]string{}, xo...), "dump", "--timeout=10s", p)...)
	}
	if path, err := exec.LookPath("lsof"); err == nil {
		run("lsof", 20*time.Second, path, "-p", p)
	}
	if runtime.GOOS == "darwin" {
		if path, err := exec.LookPath("sample"); err == nil {
			run("sample", 60*time.Second, path, p, "3")
		}
	}
	return b.String()
}
