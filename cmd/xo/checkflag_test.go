package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// leakProg leaves blocks live at exit through the one cycle Xo values
// can form (spec core 5.5): a Mutex storing a closure that captures it.
const leakProg = `fn main(os: Os) {
  let m: Mutex[List[fn() -> Int]] = Mutex.new([])
  m.with(fn(var l) { l.push(fn() { m.with_read(fn(x) { x.len() }) }) })
  os.stdio.println("stored ${m.with_read(fn(x) { x.len() })}")
}
`

const leakReport = "xo leakcheck: 4 live of "

// TestCheckFlag: --check (decision 0638). xo run --check=leak reports the
// live blocks and exits 1 where a plain run exits 0; a binary built with
// --check=leak does the same outside xo, and XO_LEAKCHECK=0 in its
// environment turns the check off; the backends that lack a check, and
// an unknown name, are usage errors (exit 2).
func TestCheckFlag(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("builds programs")
	}
	if runtime.GOARCH != "arm64" || runtime.GOOS == "windows" {
		t.Skip("the native arm64 backend runs on the host")
	}
	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "main.xo"), []byte(leakProg), 0o644); err != nil {
		t.Fatal(err)
	}
	xo := func(args ...string) (int, string, string) {
		t.Helper()
		var out, errOut bytes.Buffer
		code := run(t, args, &out, &errOut)
		return code, out.String(), errOut.String()
	}
	if code, out, errs := xo("run", src); code != 0 || out != "stored 1\n" || strings.Contains(errs, "leakcheck") {
		t.Fatalf("plain run: exit %d stdout %q stderr %q", code, out, errs)
	}
	for _, be := range []string{"arm64", "llvm"} {
		code, out, errs := xo("run", "--backend="+be, "--check=leak", src)
		if code != 1 || out != "stored 1\n" || !strings.Contains(errs, leakReport) {
			t.Errorf("--backend=%s --check=leak: exit %d (want 1) stdout %q stderr %q", be, code, out, errs)
		}
	}
	if code, _, errs := xo("run", "--check=all", src); code != 1 || !strings.Contains(errs, leakReport) {
		t.Errorf("--check=all: exit %d (want 1) stderr %q", code, errs)
	}

	bin := filepath.Join(t.TempDir(), "leak")
	if code, _, errs := xo("build", "--check=leak", "-o", bin, src); code != 0 {
		t.Fatalf("build --check=leak: exit %d: %s", code, errs)
	}
	runBin := func(env ...string) (int, string) {
		t.Helper()
		cmd := exec.Command(bin)
		cmd.Env = append(os.Environ(), env...)
		var errOut bytes.Buffer
		cmd.Stderr = &errOut
		err := cmd.Run()
		if ee, ok := err.(*exec.ExitError); ok {
			return ee.ExitCode(), errOut.String()
		} else if err != nil {
			t.Fatal(err)
		}
		return 0, errOut.String()
	}
	if code, errs := runBin(); code != 1 || !strings.Contains(errs, leakReport) {
		t.Errorf("built --check=leak: exit %d (want 1) stderr %q", code, errs)
	}
	if code, errs := runBin("XO_LEAKCHECK=0"); code != 0 || strings.Contains(errs, "leakcheck") {
		t.Errorf("built --check=leak with XO_LEAKCHECK=0: exit %d (want 0) stderr %q", code, errs)
	}
	if code, errs := runBin("XO_LEAKCHECK=1"); code != 0 || !strings.Contains(errs, leakReport) {
		t.Errorf("built --check=leak with XO_LEAKCHECK=1 (report only): exit %d (want 0) stderr %q", code, errs)
	}

	for _, c := range []struct {
		args []string
		want string
	}{
		{[]string{"--check=bogus"}, `invalid value "bogus" for flag -check`},
		{[]string{"--backend=go", "--check=leak"}, "--check=leak: native backends only; the Go backend has a garbage collector"},
		{[]string{"--backend=llvm", "--check=free"}, "--check=free: the arm64 and amd64 backends only for now"},
		{[]string{"--backend=llvm", "--check=guard"}, "--check=guard: the arm64 and amd64 backends only for now"},
		{[]string{"--race", "--check=free"}, "--check=free: not with --race"},
	} {
		code, _, errs := xo(append(append([]string{"run"}, c.args...), src)...)
		if code != 2 || !strings.Contains(errs, c.want) {
			t.Errorf("run %s: exit %d (want 2) stderr %q, want %q", strings.Join(c.args, " "), code, errs, c.want)
		}
	}
}

// TestCheckFlagTest: xo test --check=leak fails the test that leaves
// blocks live and passes the others (each test's process is checked).
func TestCheckFlagTest(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("builds programs")
	}
	if runtime.GOARCH != "arm64" || runtime.GOOS == "windows" {
		t.Skip("the native arm64 backend runs on the host")
	}
	src := t.TempDir()
	lib := `fn cycle() -> Int {
  let m: Mutex[List[fn() -> Int]] = Mutex.new([])
  m.with(fn(var l) { l.push(fn() { m.with_read(fn(x) { x.len() }) }) })
  m.with_read(fn(x) { x.len() })
}

fn plain() -> Int { [1, 2, 3].len() }
`
	tests := `test "plain" {
  expect(plain()) == 3
}

test "leaks a cycle" {
  expect(cycle()) == 1
}

test "plain again" {
  expect(plain()) == 3
}
`
	for name, body := range map[string]string{"lib.xo": lib, "lib_test.xo": tests} {
		if err := os.WriteFile(filepath.Join(src, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	var out, errOut bytes.Buffer
	if code := run(t, []string{"test", "--count=1", src}, &out, &errOut); code != 0 {
		t.Fatalf("plain xo test: exit %d\n%s%s", code, out.String(), errOut.String())
	}
	out.Reset()
	errOut.Reset()
	code := run(t, []string{"test", "--count=1", "--check=leak", src}, &out, &errOut)
	all := out.String() + errOut.String()
	if code != 1 || !strings.Contains(all, "--- FAIL: leaks a cycle") || !strings.Contains(all, "2 passed, 1 failed") || !strings.Contains(all, leakReport) {
		t.Fatalf("xo test --check=leak: exit %d (want 1)\n%s", code, all)
	}
}
