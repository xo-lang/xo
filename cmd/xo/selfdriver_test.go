package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// XO_SELF_DRIVER_ALL=1 runs the `build`, `run`, and `test` commands of
// cmd/xo's tests through the self hosted driver xo1 (selfhost/xo,
// decision 0178 stage 1's gate): $XO1 names the binary (default
// .tools/xo1 at the repository root). xo1 gets the repository's std
// (--std), the test's working directory (--wd), and --backend=go when
// the command line names no backend (cmd/xo's default; xo1's is arm64).
// Off by default: every test runs the Go CLI.
func init() {
	if os.Getenv("XO_SELF_DRIVER_ALL") != "1" {
		return
	}
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		panic(err)
	}
	bin := os.Getenv("XO1")
	if bin == "" {
		bin = filepath.Join(root, ".tools", "xo1")
	}
	selfDriver = func(args []string, stdout, stderr io.Writer) int {
		return runXo1(bin, filepath.Join(root, "std"), args, stdout, stderr)
	}
}

// selfArgs is the xo1 command line of an `xo build|run|test` one.
func selfArgs(std, wd string, args []string) []string {
	out := []string{args[0], "--std=" + std, "--wd=" + wd}
	named := false
	for _, a := range args[1:] {
		if a == "--" || !strings.HasPrefix(a, "-") {
			break
		}
		n := strings.TrimLeft(a, "-")
		if strings.HasPrefix(n, "backend") || strings.HasPrefix(n, "no-gc") {
			named = true
		}
	}
	if !named {
		out = append(out, "--backend=go")
	}
	return append(out, args[1:]...)
}

func runXo1(bin, std string, args []string, stdout, stderr io.Writer) int {
	wd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(stderr, "xo: %v\n", err)
		return 1
	}
	cmd := exec.Command(bin, selfArgs(std, wd, args)...)
	cmd.Env = append(os.Environ(), "PWD="+wd)
	cmd.Stdin = os.Stdin
	cmd.Stdout, cmd.Stderr = stdout, stderr
	err = cmd.Run()
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return ee.ExitCode()
	}
	if err != nil {
		fmt.Fprintf(stderr, "xo: %v\n", err)
		return 1
	}
	return 0
}

// TestSelfArgs pins the translation of a command line for xo1.
func TestSelfArgs(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"build -o b src", "build --std=S --wd=W --backend=go -o b src"},
		{"run --backend=llvm src -- a", "run --std=S --wd=W --backend=llvm src -- a"},
		{"test --no-gc src", "test --std=S --wd=W --no-gc src"},
		{"run src --backend=llvm", "run --std=S --wd=W --backend=go src --backend=llvm"},
	} {
		if got := strings.Join(selfArgs("S", "W", strings.Fields(c.in)), " "); got != c.want {
			t.Errorf("%s: got %s, want %s", c.in, got, c.want)
		}
	}
}
