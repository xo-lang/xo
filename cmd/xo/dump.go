package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
	"time"
)

// dumpPath is where an Xo program writes its task tree on SIGQUIT; it
// must agree with rt.DumpPath (std/rt/sched.go).
func dumpPath(pid int) string {
	dir := os.Getenv("XO_DUMP_DIR")
	if dir == "" {
		dir = os.TempDir()
	}
	return filepath.Join(dir, fmt.Sprintf("xo-dump-%d.txt", pid))
}

// runDump is `xo dump <pid>` (core 11.4): it asks a running Xo program for
// its task tree with SIGQUIT and prints the dump the program writes. Only
// the process the user names is signaled.
func runDump(args []string, stdout, stderr io.Writer) int {
	fl := flag.NewFlagSet("dump", flag.ContinueOnError)
	fl.SetOutput(stderr)
	timeout := fl.Duration("timeout", 5*time.Second, "how long to wait for the dump")
	if err := fl.Parse(args); err != nil {
		return 2
	}
	if fl.NArg() != 1 {
		fmt.Fprintln(stderr, "usage: xo dump [--timeout=5s] <pid>")
		return 2
	}
	pid, err := strconv.Atoi(fl.Arg(0))
	if err != nil || pid <= 0 {
		fmt.Fprintf(stderr, "xo dump: %q is not a process id\n", fl.Arg(0))
		return 2
	}
	path := dumpPath(pid)
	os.Remove(path)
	p, err := os.FindProcess(pid)
	if err == nil {
		err = p.Signal(syscall.SIGQUIT)
	}
	if err != nil {
		fmt.Fprintf(stderr, "xo dump: %v\n", err)
		return 1
	}
	deadline := time.Now().Add(*timeout)
	for {
		if data, err := os.ReadFile(path); err == nil {
			os.Remove(path)
			stdout.Write(data)
			return 0
		}
		if time.Now().After(deadline) {
			fmt.Fprintf(stderr, "xo dump: no dump from process %d within %s (is it an Xo program?)\n", pid, *timeout)
			return 1
		}
		time.Sleep(20 * time.Millisecond)
	}
}
