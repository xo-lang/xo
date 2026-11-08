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
	"strings"
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
// with its process group (SIGKILL; a Guard'ed child leads one, so the
// clang or linker it waits on goes too); the result is ErrHung. out is the buffer the child writes to; it is only
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
	killGroup(cmd.Process)
	<-done
	fmt.Fprintf(out, "\nxotool: %v (%v): pid %d hung; its state before the kill:\n%s", ErrHung, limit, cmd.Process.Pid, rep)
	return ErrHung
}

// HangReport is what to know about the hung xo process pid, while it
// still runs: its child processes (a build waiting on a clang that runs
// on looks like a hang of xo: 2026-11-07, pass_const_edges), `xo dump`
// (the task tree of a native program, core 11.4: which task waits on
// what), lsof (its descriptors), and where it can, sample (macOS) for 3
// seconds of stacks.
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
	if runtime.GOOS != "windows" {
		b.WriteString(childReport(pid))
	}
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

// childReport lists pid's child processes (ps: pid, elapsed time, CPU
// time, resident KiB, command line), or says it has none.
func childReport(pid int) string {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "ps", "-A", "-o", "pid=,ppid=,etime=,time=,rss=,command=").Output()
	if err != nil {
		return fmt.Sprintf("--- children (ps: %v)\n", err)
	}
	var b bytes.Buffer
	n := 0
	for _, line := range strings.Split(string(out), "\n") {
		f := strings.Fields(line)
		if len(f) < 6 || f[1] != strconv.Itoa(pid) {
			continue
		}
		if n == 0 {
			b.WriteString("--- children (pid, elapsed, cpu, rss KiB, command)\n")
		}
		n++
		fmt.Fprintf(&b, "%s %s %s %s %s\n", f[0], f[2], f[3], f[4], strings.Join(f[5:], " "))
	}
	if n == 0 {
		b.WriteString("--- children: none\n")
	}
	return b.String()
}
