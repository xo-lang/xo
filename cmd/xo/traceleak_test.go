//go:build !windows

// Peak RSS through syscall.Rusage, which Windows does not have.

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

// peakRSS polls /proc/PID/status for VmHWM, the peak of the process's
// own address space, until stop is closed; the result is in bytes (0:
// unreadable). Linux's ru_maxrss of a child counts the memory of the
// process it was forked from until the exec (the test binary's, tens
// to hundreds of MB), so it cannot show a 32 MB bound there.
func peakRSS(pid int, stop <-chan struct{}) int64 {
	var peak int64
	for {
		if b, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/status"); err == nil {
			for _, ln := range strings.Split(string(b), "\n") {
				if v, ok := strings.CutPrefix(ln, "VmHWM:"); ok {
					if kb, err := strconv.ParseInt(strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(v), "kB")), 10, 64); err == nil && kb*1024 > peak {
						peak = kb * 1024
					}
				}
			}
		}
		select {
		case <-stop:
			return peak
		case <-time.After(10 * time.Millisecond):
		}
	}
}

// TestTracebackIsolatedNoLeak: a native program that catches 100,000
// faults with task.isolate under XO_TRACEBACK=1 keeps its memory flat.
// Each fault's trace block (fault.xo trace, from rt.heap_large) was
// released at the wrong address, so it stayed: 200,000 faults took
// 203 MB against 2.8 MB without XO_TRACEBACK (2026-10-22).
func TestTracebackIsolatedNoLeak(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("builds and runs a program")
	}
	be := nativeBackends(t)[0]
	dir := writeFiles(t, map[string]string{
		"app/xo.mod": "module app\n",
		"app/main.xo": `use std/task

fn deep(xs: List[Int], i: Int, d: Int) -> Int {
  if d == 0 { return xs[i] }
  deep(xs, i, d - 1) + 1
}

fn main(os: Os) {
  let xs = [1, 2, 3]
  var faults = 0
  for i in 0..100000 {
    match task.isolate(fn() { deep(xs, 3 + i % 2, 20) }) {
      Ok(_) => {}
      Err(_) => faults += 1
    }
  }
  os.stdio.println("${faults} faults")
}
`,
	})
	bin := filepath.Join(t.TempDir(), "app")
	if code, out := xoOut(t, "build", "--backend="+be, "-o", bin, filepath.Join(dir, "app")); code != 0 {
		t.Fatalf("build: exit %d\n%s", code, out)
	}
	cmd := command(t, bin)
	cmd.Env = append(cmd.Environ(), "XO_TRACEBACK=1")
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	var peak int64
	stop := make(chan struct{})
	var wg sync.WaitGroup
	if runtime.GOOS == "linux" {
		wg.Add(1)
		go func() {
			defer wg.Done()
			peak = peakRSS(cmd.Process.Pid, stop)
		}()
	}
	err := cmd.Wait()
	close(stop)
	wg.Wait()
	if err != nil || !strings.Contains(out.String(), "100000 faults") {
		t.Fatalf("run: %v\n%s", err, out.String())
	}
	rss := peak
	if runtime.GOOS != "linux" {
		ru, ok := cmd.ProcessState.SysUsage().(*syscall.Rusage)
		if !ok {
			t.Skip("no rusage")
		}
		rss = int64(ru.Maxrss) // macOS reports bytes
	} else if rss == 0 {
		t.Skip("no /proc VmHWM")
	}
	// a leak of a trace block a fault is about 100 MB here
	if rss > 32<<20 {
		t.Errorf("max RSS %d MB after 100,000 isolated faults under XO_TRACEBACK=1, want under 32 MB", rss>>20)
	}
}
