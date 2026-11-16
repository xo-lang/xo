package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
)

// TestGoBackendColdRuntimeCompiledOnce: Go backend builds of programs
// with one runtime variant, started together on an empty Go cache,
// compile the generated runtime package (xoprog/rt) once: the others
// wait on the variant's lock and hit the cache (gobackend.xo
// cold_rt_lock; 2026-11-16, the hourly -short's 120 s build timeouts,
// when 26 builds compiled it at once). A go wrapper (XO_GO) adds a
// -toolexec logger that records each compile of the package.
// XO_GO_RT_LOCK=off shows the count without the lock (one a build).
func TestGoBackendColdRuntimeCompiledOnce(t *testing.T) {
	if testing.Short() {
		t.Skip("compiles the Go backend's runtime package cold (about 8 s, 1.2 GB)")
	}
	if runtime.GOOS == "windows" {
		t.Skip("the go wrapper and the logger are shell scripts")
	}
	goPath, err := exec.LookPath("go")
	if err != nil {
		t.Skip("no Go toolchain")
	}
	n := 4
	if v := os.Getenv("XO_TEST_RT_LOCK_OFF"); v == "1" {
		t.Log("XO_TEST_RT_LOCK_OFF=1: the lock is off (the count before the fix)")
	}
	dir := t.TempDir()
	log := filepath.Join(dir, "compiles.log")
	logger := filepath.Join(dir, "toolexec.sh")
	// a compile's arguments may be in an @file, one a line
	writeExec(t, logger, `#!/bin/sh
case "$1" in
*/compile)
	all="$*"
	for a in "$@"; do case "$a" in @*) all="$all $(tr '\n' ' ' < "${a#@}")";; esac; done
	case "$all" in *"-p xoprog/rt "*|*"-p xoprog/rt") echo compile >> `+shellQuote(log)+`;; esac
	;;
esac
exec "$@"
`)
	wrapper := filepath.Join(dir, "go.sh")
	writeExec(t, wrapper, `#!/bin/sh
# xo runs go -C TREE build ...
if [ "$1" = -C ] && [ "$3" = build ]; then d=$2; shift 3; exec `+shellQuote(goPath)+` -C "$d" build -toolexec=`+shellQuote(logger)+` "$@"; fi
exec `+shellQuote(goPath)+` "$@"
`)
	xo := xoBin(t)
	env := append(os.Environ(),
		"XO_GO="+wrapper,
		"XO_GOCACHE="+filepath.Join(dir, "gocache"),
		"XO_BIN_CACHE=off",
		"XO_STD="+filepath.Join(repoRoot, "std"),
	)
	if os.Getenv("XO_TEST_RT_LOCK_OFF") == "1" {
		env = append(env, "XO_GO_RT_LOCK=off")
	}
	var wg sync.WaitGroup
	errs := make([]string, n)
	for i := 0; i < n; i++ {
		p := filepath.Join(dir, "p"+string(rune('a'+i)))
		if err := os.MkdirAll(p, 0o755); err != nil {
			t.Fatal(err)
		}
		// one program, so one runtime variant, in a directory (and so a
		// build tree) of its own
		if err := os.WriteFile(filepath.Join(p, "main.xo"), []byte("fn main(os: Os) {\n  os.stdio.println(\"hi\")\n}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		wg.Add(1)
		go func(i int, p string) {
			defer wg.Done()
			cmd := exec.Command(xo, "build", "--backend=go", "-o", filepath.Join(p, "prog"), ".")
			cmd.Dir = p
			cmd.Env = append(env, "PWD="+p)
			var out bytes.Buffer
			cmd.Stdout = &out
			cmd.Stderr = &out
			if err := cmd.Run(); err != nil {
				errs[i] = p + ": " + err.Error() + "\n" + out.String()
			}
		}(i, p)
	}
	wg.Wait()
	for _, e := range errs {
		if e != "" {
			t.Fatal(e)
		}
	}
	data, _ := os.ReadFile(log)
	got := strings.Count(string(data), "compile")
	t.Logf("%d builds started together: %d compiles of xoprog/rt", n, got)
	if os.Getenv("XO_TEST_RT_LOCK_OFF") == "1" {
		return
	}
	if got != 1 {
		t.Errorf("%d concurrent cold builds of one runtime variant compiled it %d times, want 1", n, got)
	}
}

func writeExec(t *testing.T, path, text string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(text), 0o755); err != nil {
		t.Fatal(err)
	}
}

func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }
