// Package main holds the tests of the xo command line (decision 0215):
// since the Go implementation of the compiler was deleted, cmd/xo has no
// Go program, only these tests, which drive the shipped xo (xo1, built
// from selfhost/xo) as a process: $XO_CLI when it is set (a release's
// bin/xo, or any built xo), else the checkout's bin/xo, built from the
// current sources when they changed (internal/xotool).
package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sync"
	"testing"

	"github.com/xo-lang/xo/internal/xotool"
)

// cliEnv names the xo binary these tests run (its fields after the first
// are leading flags); unset, internal/xotool builds the checkout's.
const cliEnv = "XO_CLI"

// xoCommand is the xo command line the tests run (xotool.Command), or
// the reason there is none.
var xoCommand = sync.OnceValues(xotool.Command)

// run is the command line as the tests drive it: the xo binary with args
// in the current directory with the current environment, and its exit
// status.
func run(args []string, stdout, stderr io.Writer) int {
	cli, err := xoCommand()
	if err != nil {
		fmt.Fprintf(stderr, "no xo to run: %v\n", err)
		return -1
	}
	return execCLI(cli, args, stdout, stderr)
}

// runXo is run (the name the tests used for the in process Go command
// line before decision 0215).
func runXo(args []string, stdout, stderr io.Writer) int { return run(args, stdout, stderr) }

// execCLI runs cli[0] with cli[1:] and args, wiring standard input to this
// process's, and returns its exit status (-1 when it did not start or was
// killed by a signal, with the reason on stderr).
func execCLI(cli, args []string, stdout, stderr io.Writer) int {
	cmd := exec.Command(cli[0], append(append([]string{}, cli[1:]...), args...)...)
	if wd, err := os.Getwd(); err == nil {
		// Dir with a nil Env also sets PWD, which xo1 reads (t.Chdir).
		cmd.Dir = wd
	}
	cmd.Stdin = os.Stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	err := cmd.Run()
	if err == nil {
		return 0
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) && ee.ExitCode() >= 0 {
		return ee.ExitCode()
	}
	fmt.Fprintf(stderr, "%s %s: %v\n", cliEnv, cli[0], err)
	return -1
}

// xoBin is the path of the xo binary the tests run, for a test that
// starts it itself (a server).
func xoBin(t testing.TB) string {
	t.Helper()
	cli, err := xoCommand()
	if err != nil {
		t.Fatalf("no xo to run: %v", err)
	}
	return cli[0]
}

// repoRoot is the repository root, two levels up from this package.
var repoRoot = func() string {
	r, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		panic(err)
	}
	return r
}()

// version is the version xo prints: selfhost/cli/core's VERSION.
var version = func() string {
	data, err := os.ReadFile(filepath.Join(repoRoot, "selfhost", "cli", "core", "cli.xo"))
	if err != nil {
		panic(err)
	}
	m := regexp.MustCompile(`(?m)^pub const VERSION = "(.*)"$`).FindSubmatch(data)
	if m == nil {
		panic("no VERSION in selfhost/cli/core/cli.xo")
	}
	return string(m[1])
}()

// regressFile is the file `xo test` writes shrunk contract fuzz failures
// into (spec/core.md 9.2).
const regressFile = "regressions_test.xo"

// runTest is `xo test` of args.
func runTest(args []string, stdout, stderr io.Writer) int {
	return run(append([]string{"test"}, args...), stdout, stderr)
}
