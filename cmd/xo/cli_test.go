package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

// cliEnv names another xo binary for this package's tests (decision 0178
// stage 2): when it is set, run execs `$XO_CLI <args>` in the current
// directory with the current environment and returns its exit status, so
// the same tests check the self hosted CLI (xo1) against the Go one. The
// value is split on spaces, so it may carry leading flags.
const cliEnv = "XO_CLI"

// run is the command line as the tests drive it: runXo in process, or the
// binary $XO_CLI names.
func run(args []string, stdout, stderr io.Writer) int {
	cli := strings.Fields(os.Getenv(cliEnv))
	if len(cli) == 0 {
		return runXo(args, stdout, stderr)
	}
	return execCLI(cli, args, stdout, stderr)
}

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

// skipComparisonDump skips a test of a hidden dump command (lexdump,
// astdump, typedump, ssadump, asmdump, gendump, llvmdump) when $XO_CLI
// names another binary: those commands print the Go implementation's
// output for the differential tests against selfhost/, so the self hosted
// CLI has none (decision 0178, switch point B: "the hidden dumps used by
// tests are not needed"; stage 7 replaces them with recorded goldens).
func skipComparisonDump(t interface{ Skipf(string, ...any) }, cmd string) {
	if strings.TrimSpace(os.Getenv(cliEnv)) != "" {
		t.Skipf("xo %s is a hidden comparison dump of the Go implementation, exempt from the %s gate (decision 0178 switch point B)", cmd, cliEnv)
	}
}
