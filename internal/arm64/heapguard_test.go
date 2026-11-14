package arm64

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/xo-lang/xo/internal/testtmp"
	"github.com/xo-lang/xo/internal/xotool"
)

// TestHeapGuard runs native/llrt/libcmem's tests built with
// XO_HEAP_GUARD=1: a runtime record (rt.heap_large) ends against an
// inaccessible page, so the SV_ROOTS overrun (a slot index taken for a
// byte offset, gotchas/svc-roots-slot-index.md) faults at its first
// access, and an rt.slot_* past a List's length faults with its index.
func TestHeapGuard(t *testing.T) {
	t.Parallel()
	if !native() {
		t.Skip("the host is not arm64")
	}
	if testing.Short() {
		t.Skip("builds and runs programs")
	}
	root, err := xotool.Root()
	if err != nil {
		t.Fatal(err)
	}
	xo, err := xotool.Path()
	if err != nil {
		t.Fatal(err)
	}
	run := func(sub string) (string, error) {
		cmd := testtmp.Guard(exec.Command(xo, "test", "--std-module=native/llrt/libcmem", "--backend=arm64", "--run="+sub))
		cmd.Dir = root
		cmd.Env = append(os.Environ(), "XO_STD="+filepath.Join(root, "std"), "PWD="+root, "XO_HEAP_GUARD=1")
		out, err := cmd.CombinedOutput()
		return string(out), err
	}
	if out, err := run("record"); err != nil {
		t.Fatalf("the guarded records' tests: %v\n%s", err, out)
	}
	if out, err := run("an overrun"); err == nil {
		t.Errorf("an overrun of a guarded record ran to the end:\n%s", out)
	}
	out, err := run("past a List")
	if err == nil {
		t.Errorf("rt.slot_add past a List's length ran to the end:\n%s", out)
	} else if !strings.Contains(out, "rt.slot_add: slot 4 out of range for List of length 4") {
		t.Errorf("rt.slot_add past a List's length: no range fault in the output:\n%s", out)
	}
}

// TestCheckFlagPlants: xo test --check=guard and --check=free (decision
// 0638) find the bugs native/llrt's tests plant for them: the overrun of
// a guarded record (libcmem) and a block written after its free
// (heapos), which pass without the flag. Not parallel: TestHeapGuard
// builds libcmem's tests with other settings into the same binary.
func TestCheckFlagPlants(t *testing.T) {
	if !native() {
		t.Skip("the host is not arm64")
	}
	if testing.Short() {
		t.Skip("builds and runs programs")
	}
	root, err := xotool.Root()
	if err != nil {
		t.Fatal(err)
	}
	xo, err := xotool.Path()
	if err != nil {
		t.Fatal(err)
	}
	run := func(module, sub string, flags ...string) (string, error) {
		args := append([]string{"test", "--std-module=" + module, "--backend=arm64", "--run=" + sub}, flags...)
		cmd := testtmp.Guard(exec.Command(xo, args...))
		cmd.Dir = root
		cmd.Env = append(os.Environ(), "XO_STD="+filepath.Join(root, "std"), "PWD="+root)
		out, err := cmd.CombinedOutput()
		return string(out), err
	}
	for _, c := range []struct{ module, sub, flag, want string }{
		{"native/llrt/libcmem", "an overrun", "--check=guard", ""},
		{"native/llrt/heapos", "write after free", "--check=free", "fault: XO_FREE_CHECK: a freed block was written after its free"},
	} {
		if out, err := run(c.module, c.sub); err != nil {
			t.Errorf("%s %q without %s: %v\n%s", c.module, c.sub, c.flag, err, out)
		}
		out, err := run(c.module, c.sub, c.flag)
		if err == nil {
			t.Errorf("%s %q with %s ran to the end:\n%s", c.module, c.sub, c.flag, out)
		} else if !strings.Contains(out, c.want) {
			t.Errorf("%s %q with %s: no %q in the output:\n%s", c.module, c.sub, c.flag, c.want, out)
		}
	}
}

// TestGoldenRunHeapGuard runs golden programs built with
// XO_HEAP_GUARD=1 (every runtime record against a guard page, the
// rt.slot_* on a List checked): a correct runtime never reads or writes
// past a record, so the output is the same. Every golden in the full
// tier (XO_SELFHOST_TIER=full), else those that use the runtime's
// records most (tasks, channels, contexts, servers).
func TestGoldenRunHeapGuard(t *testing.T) {
	t.Parallel()
	if !native() {
		t.Skip("the host is not arm64")
	}
	if testing.Short() {
		t.Skip("builds and runs programs")
	}
	var dirs []string
	if os.Getenv("XO_SELFHOST_TIER") == "full" {
		dirs = goldenDirs(t)
	} else {
		// http_tls, http_stream, http_conn_fault and http_tls_keys grow
		// raw.c_resize blocks, whose prefix word is data (2026-10-20: the
		// guarded resize read it as the size)
		for _, name := range []string{"conc_scopes", "fault_unwind", "fault_tasks", "http_server", "tls_nistec", "json_text", "maps", "closures", "http_tls", "http_stream", "http_conn_fault", "http_tls_keys"} {
			dirs = append(dirs, filepath.Join("..", "llvm", "testdata", "golden", name))
		}
	}
	for _, dir := range dirs {
		if _, err := os.Stat(filepath.Join(dir, "want.txt")); err != nil {
			continue
		}
		t.Run(filepath.Base(dir), func(t *testing.T) {
			t.Parallel()
			got, err := buildRunWith(t, dir, xotool.Options{Backend: xotool.BackendArm64, Env: []string{"XO_HEAP_GUARD=1"}})
			if err != nil {
				t.Fatalf("build: %v", err)
			}
			want, err := os.ReadFile(filepath.Join(dir, "want.txt"))
			if err != nil {
				t.Fatal(err)
			}
			if string(want) != got.String() {
				t.Errorf("output differs from want.txt with XO_HEAP_GUARD=1\n--- want\n%s\n--- got\n%s", want, got)
			}
		})
	}
}

// TestHeapGuardMainInlinePark repeats conc_reader_yield, built with
// XO_HEAP_GUARD=1, while busy loops take the CPUs: main's idle thread
// (pool.xo count_main) locked the park of a task main's fiber ran inline,
// after the task ended and its context, here unmapped, was gone. Before the
// fix 366 of 500 such runs ended in SIGSEGV on linux/arm64, and 0 of 200 with
// the CPUs free, so the load is the test (gotchas/park-pointer-outlives-inline-task.md).
// Not parallel: other tests' builds would change the load.
func TestHeapGuardMainInlinePark(t *testing.T) {
	if !native() {
		t.Skip("the host is not arm64")
	}
	if testing.Short() {
		t.Skip("builds and runs a program")
	}
	dir := filepath.Join("..", "llvm", "testdata", "golden", "conc_reader_yield")
	res, err := sharedBuild(dir, xotool.Options{Backend: xotool.BackendArm64, Env: []string{"XO_HEAP_GUARD=1"}})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if res.Failed {
		t.Fatalf("build failed:\n%s", res.Output)
	}
	var stop atomic.Bool
	defer stop.Store(true)
	for range 2 * runtime.NumCPU() {
		go func() {
			for !stop.Load() {
			}
		}()
	}
	for i := range 30 {
		cmd := testtmp.Guard(exec.Command(res.Binaries[xotool.Host()]))
		cmd.Dir = dir
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			text := stderr.String()
			if len(text) > 1500 {
				text = text[:1500]
			}
			t.Fatalf("run %d of 30: %v\n%s", i+1, err, text)
		}
	}
}
