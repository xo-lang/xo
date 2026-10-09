package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/xo-lang/xo/internal/build"
)

// Native test binaries (`xo test --no-gc`, decision 0083): the binary
// runs one test per process (XO_TEST names it, XO_TEST_SEED seeds it),
// so a fault fails that test only; the results print as std/rt's
// RunTests prints them on the Go backend.

type nativeTestOpts struct {
	seed    int64
	run     string
	asJSON  bool
	verbose bool
	real    bool
	timeout time.Duration
}

type nativeOutcome struct {
	status string // pass, fail, skip
	msg    string
	logs   []string
}

func runNativeTests(res *build.Result, o nativeTestOpts, stdout io.Writer) int {
	bin := res.Binaries[build.Host()]
	pass, fail, skip := 0, 0, 0
	for _, t := range res.Tests {
		if o.run != "" && !strings.Contains(t.Name, o.run) {
			continue
		}
		s := t.Seed
		if s == 0 {
			s = o.seed
		}
		if s == 0 {
			s = 1
		}
		var out nativeOutcome
		switch {
		case len(t.Uses) > 0 && !o.real:
			out = nativeOutcome{status: "skip", msg: "uses " + strings.Join(t.Uses, ", ") + ": run with `xo test --real` for real capabilities"}
		case len(t.Uses) > 0:
			out = nativeOutcome{status: "skip", msg: "uses " + strings.Join(t.Uses, ", ") + ": real capabilities in tests are not available without a garbage collector (`--no-gc`) yet (support is planned)"}
		default:
			out = runNativeTest(bin, t, s, o.timeout)
		}
		switch out.status {
		case "pass":
			pass++
		case "fail":
			fail++
		default:
			skip++
		}
		if o.asJSON {
			rec := map[string]any{"name": t.Name, "module": t.Module, "loc": t.Loc, "status": out.status, "message": out.msg, "seed": s}
			if len(out.logs) > 0 && (out.status == "fail" || o.verbose) {
				rec["log"] = out.logs
			}
			data, _ := json.Marshal(rec)
			fmt.Fprintln(stdout, string(data))
			continue
		}
		switch {
		case out.status == "fail":
			fmt.Fprintf(stdout, "--- FAIL: %s (%s, seed %d)\n", t.Name, t.Loc, s)
			for _, l := range strings.Split(strings.TrimRight(out.msg, "\n"), "\n") {
				fmt.Fprintf(stdout, "    %s\n", l)
			}
			printNativeLogs(stdout, out.logs)
		case out.status == "skip" && o.verbose:
			fmt.Fprintf(stdout, "--- SKIP: %s (%s)\n", t.Name, out.msg)
		case out.status == "pass" && o.verbose:
			fmt.Fprintf(stdout, "--- PASS: %s\n", t.Name)
			printNativeLogs(stdout, out.logs)
		}
	}
	if !o.asJSON {
		status := "ok"
		if fail > 0 {
			status = "FAIL"
		}
		fmt.Fprintf(stdout, "%s: %d passed, %d failed, %d skipped\n", status, pass, fail, skip)
	}
	if fail > 0 {
		return 1
	}
	return 0
}

func printNativeLogs(w io.Writer, lines []string) {
	if len(lines) == 0 {
		return
	}
	fmt.Fprintf(w, "    log:\n")
	for _, l := range lines {
		fmt.Fprintf(w, "      %s\n", l)
	}
}

// runNativeTest runs test t of the test binary bin with seed s.
func runNativeTest(bin string, t build.TestInfo, s int64, limit time.Duration) nativeOutcome {
	ctx, cancel := context.WithTimeout(context.Background(), limit)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin)
	cmd.Dir = t.Dir
	cmd.Env = append(os.Environ(), "XO_TEST="+strconv.Itoa(t.Index), "XO_TEST_SEED="+strconv.FormatInt(s, 10))
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	err := cmd.Run()
	var logs []string
	var failMsg []string
	failing := false
	for _, l := range strings.Split(strings.TrimRight(out.String(), "\n"), "\n") {
		switch {
		case failing:
			failMsg = append(failMsg, l)
		case l == "XO-TEST-FAIL":
			failing = true
		case strings.HasPrefix(l, "XO-TEST-LOG "):
			logs = append(logs, strings.TrimPrefix(l, "XO-TEST-LOG "))
		}
	}
	if ctx.Err() != nil {
		return nativeOutcome{status: "fail", msg: fmt.Sprintf("the test did not finish within %s of real time", limit), logs: logs}
	}
	code := 0
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		code = ee.ExitCode()
	} else if err != nil {
		return nativeOutcome{status: "fail", msg: err.Error(), logs: logs}
	}
	switch {
	case code == 0:
		return nativeOutcome{status: "pass", logs: logs}
	case failing:
		return nativeOutcome{status: "fail", msg: strings.Join(failMsg, "\n"), logs: logs}
	}
	msg := strings.TrimRight(errb.String(), "\n")
	if msg == "" {
		msg = fmt.Sprintf("the test exited with status %d", code)
	}
	return nativeOutcome{status: "fail", msg: msg, logs: logs}
}
