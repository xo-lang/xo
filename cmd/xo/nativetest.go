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
	cases   int // per property test (0: 100)
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
		case t.Skip != "":
			out = nativeOutcome{status: "skip", msg: t.Skip}
		case len(t.Uses) > 0 && !o.real:
			out = nativeOutcome{status: "skip", msg: "uses " + strings.Join(t.Uses, ", ") + ": run with `xo test --real` for real capabilities"}
		case len(t.Uses) > 0:
			out = nativeOutcome{status: "skip", msg: "uses " + strings.Join(t.Uses, ", ") + ": real capabilities in tests are not available without a garbage collector (`--no-gc`) yet (support is planned)"}
		case t.Prop:
			cases := o.cases
			if cases <= 0 {
				cases = 100
			}
			out = runNativeProperty(bin, t, s, cases, o.timeout)
		default:
			out, _ = runNativeTestEnv(bin, t, s, o.timeout)
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

// propCase is what a property test's case reported before its body ran
// (decision 0120): the generator's choices and the inputs' `name = value`
// lines; notSmaller is a shrinking try whose choices were not smaller
// than the best so far (not run).
type propCase struct {
	choices    []uint64
	reported   bool
	input      string
	notSmaller bool
}

// runNativeTestEnv runs test t of the test binary bin with seed s and
// the extra environment env.
func runNativeTestEnv(bin string, t build.TestInfo, s int64, limit time.Duration, env ...string) (nativeOutcome, propCase) {
	ctx, cancel := context.WithTimeout(context.Background(), limit)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin)
	cmd.Dir = t.Dir
	cmd.Env = append(append(os.Environ(), "XO_TEST="+strconv.Itoa(t.Index), "XO_TEST_SEED="+strconv.FormatInt(s, 10)), env...)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	err := cmd.Run()
	var logs []string
	var failMsg []string
	var pc propCase
	var input strings.Builder
	failing := false
	for _, l := range strings.Split(strings.TrimRight(out.String(), "\n"), "\n") {
		switch {
		case failing:
			failMsg = append(failMsg, l)
		case l == "XO-TEST-FAIL":
			failing = true
		case strings.HasPrefix(l, "XO-TEST-LOG "):
			logs = append(logs, strings.TrimPrefix(l, "XO-TEST-LOG "))
		case strings.HasPrefix(l, "XO-PROP-CHOICES"):
			pc.reported = true
			for _, f := range strings.Split(strings.TrimSpace(strings.TrimPrefix(l, "XO-PROP-CHOICES")), ",") {
				if v, err := strconv.ParseUint(f, 10, 64); err == nil {
					pc.choices = append(pc.choices, v)
				}
			}
		case strings.HasPrefix(l, "XO-PROP-INPUT "):
			input.WriteString(strings.TrimPrefix(l, "XO-PROP-INPUT ") + "\n")
		case l == "XO-PROP-NOT-SMALLER":
			pc.notSmaller = true
		}
	}
	pc.input = input.String()
	if ctx.Err() != nil {
		return nativeOutcome{status: "fail", msg: fmt.Sprintf("the test did not finish within %s of real time", limit), logs: logs}, pc
	}
	code := 0
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		code = ee.ExitCode()
	} else if err != nil {
		return nativeOutcome{status: "fail", msg: err.Error(), logs: logs}, pc
	}
	switch {
	case code == 0:
		return nativeOutcome{status: "pass", logs: logs}, pc
	case code == 3 && pc.notSmaller:
		return nativeOutcome{status: "skip"}, pc
	case failing:
		return nativeOutcome{status: "fail", msg: strings.Join(failMsg, "\n"), logs: logs}, pc
	}
	msg := strings.TrimRight(errb.String(), "\n")
	if msg == "" {
		msg = fmt.Sprintf("the test exited with status %d", code)
	}
	return nativeOutcome{status: "fail", msg: msg, logs: logs}, pc
}

// runNativeProperty runs the cases of property test t, each in a process
// of its own, and reports the first failing input, shrunk: std/rt's
// runProperty (same case seeds, so the same inputs and report).
func runNativeProperty(bin string, t build.TestInfo, seed int64, cases int, limit time.Duration) nativeOutcome {
	for i := 0; i < cases; i++ {
		caseSeed := uint64(seed)*1000003 + uint64(i)
		out, pc := runNativeTestEnv(bin, t, seed, limit, "XO_PROP_SEED="+strconv.FormatUint(caseSeed, 10))
		if out.status != "fail" {
			continue
		}
		if pc.reported {
			if sIn, sOut, steps := shrinkNative(bin, t, seed, limit, pc.choices); steps > 0 {
				return nativeOutcome{status: "fail", msg: fmt.Sprintf("property failed on case %d; shrunk in %d steps to\n%s%s", i+1, steps, sIn, sOut.msg), logs: sOut.logs}
			}
		}
		out.msg = fmt.Sprintf("property failed on case %d with\n%s%s", i+1, pc.input, out.msg)
		return out
	}
	return nativeOutcome{status: "pass"}
}

func joinChoices(cs []uint64) string {
	parts := make([]string, len(cs))
	for i, c := range cs {
		parts[i] = strconv.FormatUint(c, 10)
	}
	return strings.Join(parts, ",")
}

// shrinkNative is std/rt's shrinkProperty: it edits the failing case's
// choices (deleting runs, then lowering each by binary search) and keeps
// every edit that still fails with smaller choices (the test binary
// replays them, XO_PROP_REPLAY, and skips a try that is not smaller than
// XO_PROP_BEST), within 2000 tries.
func shrinkNative(bin string, t build.TestInfo, seed int64, limit time.Duration, failing []uint64) (string, nativeOutcome, int) {
	best := append([]uint64{}, failing...)
	var bestIn string
	var bestOut nativeOutcome
	steps, budget := 0, 2000
	try := func(cand []uint64) bool {
		if budget <= 0 {
			return false
		}
		budget--
		out, pc := runNativeTestEnv(bin, t, seed, limit, "XO_PROP_REPLAY="+joinChoices(cand), "XO_PROP_BEST="+joinChoices(best))
		if pc.notSmaller || !pc.reported || out.status != "fail" {
			return false
		}
		best, bestIn, bestOut = append([]uint64{}, pc.choices...), pc.input, out
		steps++
		return true
	}
	for improved := true; improved && budget > 0; {
		improved = false
		for k := 8; k >= 1 && !improved; k /= 2 {
			for i := 0; i+k <= len(best) && !improved; i++ {
				cand := append(append([]uint64{}, best[:i]...), best[i+k:]...)
				improved = try(cand)
			}
		}
		for i := 0; i < len(best) && !improved; i++ {
			if best[i] == 0 {
				continue
			}
			lo, hi := uint64(0), best[i]
			for lo < hi && budget > 0 {
				mid := lo + (hi-lo)/2
				cand := append([]uint64{}, best...)
				if i >= len(cand) {
					break
				}
				cand[i] = mid
				if try(cand) {
					hi = mid
					improved = true
				} else {
					lo = mid + 1
				}
			}
		}
	}
	return bestIn, bestOut, steps
}
