package main

import (
	"bytes"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
)

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
	if err := cmd.Run(); err != nil || !strings.Contains(out.String(), "100000 faults") {
		t.Fatalf("run: %v\n%s", err, out.String())
	}
	ru, ok := cmd.ProcessState.SysUsage().(*syscall.Rusage)
	if !ok {
		t.Skip("no rusage")
	}
	rss := int64(ru.Maxrss)
	if runtime.GOOS != "darwin" {
		rss *= 1024 // Linux reports KiB, macOS bytes
	}
	// a leak of a trace block a fault is about 100 MB here
	if rss > 32<<20 {
		t.Errorf("max RSS %d MB after 100,000 isolated faults under XO_TRACEBACK=1, want under 32 MB", rss>>20)
	}
}
