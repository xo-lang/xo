// Command lifeline runs a test's child program so that it ends when the
// test process is gone (internal/testtmp Lifeline): a killed or timed
// out `go test` runs no deferred cleanup, and a network server child
// blocked in accept would stay behind (PPID 1).
//
//	lifeline PROGRAM ARGS...    (Unix; the lifeline is file descriptor 3)
//
// It starts a watcher (itself, with XO_LIFELINE_WATCH set) that holds the
// lifeline and an "alive" pipe, then execs PROGRAM in its own process, so
// the test's exec.Cmd is PROGRAM itself: its signals, exit status, and
// Kill are the program's. The watcher kills PROGRAM when the lifeline
// reaches end of file (the test closed it, or the test process died) and
// exits when PROGRAM does (the alive pipe's write end, which PROGRAM
// holds, closes).
package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"syscall"
)

func main() {
	if pid := os.Getenv("XO_LIFELINE_WATCH"); pid != "" {
		watch(pid)
		return
	}
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: lifeline PROGRAM ARGS...")
		os.Exit(2)
	}
	path, err := exec.LookPath(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, "lifeline:", err)
		os.Exit(127)
	}
	life := os.NewFile(3, "lifeline")
	ar, aw, err := os.Pipe()
	if err != nil {
		fmt.Fprintln(os.Stderr, "lifeline:", err)
		os.Exit(127)
	}
	w := exec.Command(os.Args[0])
	w.Env = append(os.Environ(), "XO_LIFELINE_WATCH="+strconv.Itoa(os.Getpid()))
	w.ExtraFiles = []*os.File{life, ar} // fds 3 and 4 in the watcher
	if err := w.Start(); err != nil {
		fmt.Fprintln(os.Stderr, "lifeline:", err)
		os.Exit(127)
	}
	life.Close()
	ar.Close()
	// PROGRAM keeps the alive pipe's write end open (not close on exec)
	if _, _, e := syscall.Syscall(syscall.SYS_FCNTL, aw.Fd(), syscall.F_SETFD, 0); e != 0 {
		fmt.Fprintln(os.Stderr, "lifeline:", e)
		os.Exit(127)
	}
	err = syscall.Exec(path, os.Args[1:], os.Environ())
	fmt.Fprintln(os.Stderr, "lifeline: exec:", err)
	os.Exit(127)
}

// watch kills process pid when the lifeline (fd 3) ends, and returns when
// the alive pipe (fd 4) ends: pid exited.
func watch(pid string) {
	p, _ := strconv.Atoi(pid)
	life := os.NewFile(3, "lifeline")
	alive := os.NewFile(4, "alive")
	gone := make(chan struct{})
	go func() {
		io.Copy(io.Discard, alive)
		close(gone)
	}()
	ended := make(chan struct{})
	go func() {
		io.Copy(io.Discard, life)
		close(ended)
	}()
	select {
	case <-gone:
	case <-ended:
		syscall.Kill(p, syscall.SIGKILL)
	}
}
