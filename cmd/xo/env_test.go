package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/xo-lang/xo/internal/mod/modtest"
)

// The tests run in parallel (t.Parallel; a serial run of this package
// took about 25 s, bench/results/short-tests-2026-10-14.md), which
// t.Setenv and t.Chdir forbid because they change the whole test process.
// The xo the tests drive is a separate process, so a test's environment
// and working directory live here instead: setenv and chdir record them
// for the test and its subtests (by name prefix) until it ends, and
// command (run, execCLI) starts processes with them. Nothing here
// changes this process's environment or working directory.

// testEnv is what setenv and chdir recorded for one test.
type testEnv struct {
	vars map[string]string
	dir  string
}

var (
	testEnvMu sync.Mutex
	testEnvs  = map[string]*testEnv{} // by t.Name()
)

// envOf is t's own entry (testEnvMu held), created on first use and
// removed when t ends.
func envOf(t testing.TB) *testEnv {
	name := t.Name()
	e := testEnvs[name]
	if e == nil {
		e = &testEnv{vars: map[string]string{}}
		testEnvs[name] = e
		t.Cleanup(func() {
			testEnvMu.Lock()
			delete(testEnvs, name)
			testEnvMu.Unlock()
		})
	}
	return e
}

// setenv sets key to value for the processes t and its subtests start,
// as t.Setenv does for the whole test process.
func setenv(t testing.TB, key, value string) {
	testEnvMu.Lock()
	defer testEnvMu.Unlock()
	envOf(t).vars[key] = value
}

// chdir makes dir the working directory of the processes t and its
// subtests start, as t.Chdir does for the whole test process.
func chdir(t testing.TB, dir string) {
	t.Helper()
	abs, err := filepath.Abs(dir)
	if err != nil {
		t.Fatal(err)
	}
	testEnvMu.Lock()
	defer testEnvMu.Unlock()
	envOf(t).dir = abs
}

// resolved is the environment overrides and working directory of t: its
// own entry over its parents', outermost first (testEnvMu held).
func resolved(t testing.TB) (map[string]string, string) {
	vars := map[string]string{}
	dir := ""
	name := t.Name()
	for i := 0; i <= len(name); i++ {
		if i < len(name) && name[i] != '/' {
			continue
		}
		e := testEnvs[name[:i]]
		if e == nil {
			continue
		}
		for k, v := range e.vars {
			vars[k] = v
		}
		if e.dir != "" {
			dir = e.dir
		}
	}
	return vars, dir
}

// getenv is key as the processes t starts see it.
func getenv(t testing.TB, key string) string {
	testEnvMu.Lock()
	vars, _ := resolved(t)
	testEnvMu.Unlock()
	if v, ok := vars[key]; ok {
		return v
	}
	return os.Getenv(key)
}

// workdir is the working directory of the processes t starts: its last
// chdir, else this process's (the package directory).
func workdir(t testing.TB) string {
	testEnvMu.Lock()
	_, dir := resolved(t)
	testEnvMu.Unlock()
	if dir == "" {
		dir, _ = os.Getwd()
	}
	return dir
}

// environ is the environment of the processes t starts: this process's
// with t's setenv calls applied and PWD set to workdir(t) (exec sets PWD
// only when Env is nil; xo1 reads it).
func environ(t testing.TB) []string {
	testEnvMu.Lock()
	vars, _ := resolved(t)
	testEnvMu.Unlock()
	if _, ok := vars["XO_BUILD_DIR"]; !ok && os.Getenv("XO_BUILD_DIR") == "" {
		// a build directory of the test's own: tests run at the same
		// time, and builds of one program into the shared
		// <user cache>/xo/build overwrite each other's files
		// (gotchas/parallel-xo-builds-share-build-dir.md)
		dir := filepath.Join(t.TempDir(), "build")
		setenv(t, "XO_BUILD_DIR", dir)
		vars["XO_BUILD_DIR"] = dir
	}
	vars["PWD"] = workdir(t)
	var out []string
	for _, kv := range os.Environ() {
		k, _, _ := strings.Cut(kv, "=")
		if _, ok := vars[k]; !ok {
			out = append(out, kv)
		}
	}
	for k, v := range vars {
		out = append(out, k+"="+v)
	}
	return out
}

// modSetup is modtest.Setup with its variables set through setenv.
func modSetup(t testing.TB, src string) *modtest.Env {
	t.Helper()
	return modtest.SetupWith(t, src, modtest.Compare, func(k, v string) { setenv(t, k, v) })
}

// command is exec.Command in t's working directory with t's environment.
func command(t testing.TB, name string, args ...string) *exec.Cmd {
	cmd := exec.Command(name, args...)
	cmd.Dir, cmd.Env = workdir(t), environ(t)
	return cmd
}
