package main

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/build"
	"github.com/xo-lang/xo/internal/gen"
)

// nativeBackends lists the native backends this host can build tests
// with.
func nativeBackends(t *testing.T) []string {
	t.Helper()
	var out []string
	if runtime.GOARCH == "arm64" && (runtime.GOOS == "darwin" || runtime.GOOS == "linux") {
		if _, err := build.CC(); err == nil {
			out = append(out, build.BackendArm64)
		}
	}
	if _, err := build.Clang(); err == nil {
		out = append(out, build.BackendLLVM)
	}
	if len(out) == 0 {
		t.Skip("no native backend on this host")
	}
	return out
}

func xoTest(t *testing.T, args ...string) (int, string) {
	t.Helper()
	var out, errOut bytes.Buffer
	code := run(append([]string{"test"}, args...), &out, &errOut)
	return code, out.String() + errOut.String()
}

// TestNoGCTests runs native test binaries (`xo test --no-gc`, decision
// 0083): the report of testdata/nogc/same and of examples/notes-service
// is the Go backend's, line for line; testdata/nogc/native (error
// traces, fault frames, and deadlock details, which native builds do not
// record) matches its want.txt.
func TestNoGCTests(t *testing.T) {
	if testing.Short() {
		t.Skip("builds and runs test binaries on every backend")
	}
	backends := nativeBackends(t)
	t.Setenv("XO_BUILD_DIR", filepath.Join(t.TempDir(), "build"))
	for _, dir := range []string{filepath.Join("testdata", "nogc", "same"), filepath.Join("..", "..", "examples", "notes-service")} {
		goCode, goOut := xoTest(t, "-v", dir)
		for _, be := range backends {
			code, out := xoTest(t, "--no-gc", "--backend="+be, "-v", dir)
			if code != goCode || out != goOut {
				t.Errorf("%s on %s differs from the Go backend (exit %d, %d)\n--- go\n%s--- %s\n%s", dir, be, goCode, code, goOut, be, out)
			}
		}
	}
	dir := filepath.Join("testdata", "nogc", "native")
	want, err := os.ReadFile(filepath.Join(dir, "want.txt"))
	if err != nil {
		t.Fatal(err)
	}
	for _, be := range backends {
		code, out := xoTest(t, "--no-gc", "--backend="+be, "-v", dir)
		if code != 1 || out != string(want) {
			t.Errorf("%s on %s: exit %d\n--- want\n%s--- got\n%s", dir, be, code, want, out)
		}
	}
}

var leakLine = regexp.MustCompile(`xo leakcheck: (\d+) live of (\d+) allocations`)

// TestNoGCTestLeaks runs every passing test of the native test binaries
// with XO_LEAKCHECK=1: no allocation may be live at exit.
func TestNoGCTestLeaks(t *testing.T) {
	if testing.Short() {
		t.Skip("builds and runs test binaries")
	}
	backends := nativeBackends(t)
	t.Setenv("XO_BUILD_DIR", filepath.Join(t.TempDir(), "build"))
	for _, dir := range []string{filepath.Join("testdata", "nogc", "same"), filepath.Join("..", "..", "examples", "notes-service")} {
		for _, be := range backends {
			res, err := build.Build([]string{dir}, build.Options{Mode: gen.Test, NoGC: true, Backend: be})
			if err != nil || res.Failed {
				t.Fatalf("%s on %s: %v %v", dir, be, err, res.Diagnostics)
			}
			bin := res.Binaries[build.Host()]
			for _, ti := range res.Tests {
				cmd := exec.Command(bin)
				cmd.Dir = ti.Dir
				cmd.Env = append(os.Environ(), "XO_TEST="+strconv.Itoa(ti.Index), "XO_LEAKCHECK=1")
				var errb bytes.Buffer
				cmd.Stdout, cmd.Stderr = io.Discard, &errb
				if cmd.Run() != nil {
					continue // a failing test exits without the check
				}
				m := leakLine.FindStringSubmatch(errb.String())
				if m == nil {
					t.Errorf("%s %q on %s: no leak check line:\n%s", dir, ti.Name, be, errb.String())
					continue
				}
				if m[1] != "0" {
					t.Errorf("%s %q on %s: %s allocations live at exit (of %s)", dir, ti.Name, be, m[1], m[2])
				}
			}
		}
	}
}

var _ = strings.TrimSpace
