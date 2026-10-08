package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"strconv"

	"github.com/xo-lang/xo/internal/build"
	"github.com/xo-lang/xo/internal/diag"
	"github.com/xo-lang/xo/internal/gen"
)

// printDiags prints diagnostics in the human form and a summary.
func printDiags(ds []diag.Diagnostic, stderr io.Writer) {
	for _, d := range ds {
		fmt.Fprintln(stderr, diag.Human(d))
	}
}

// monoFlags defines --mono (the default since 2026-10-08, kept so older
// command lines work; --mono=false turns it off) and --no-mono, and
// returns a function reporting whether specialization is off.
func monoFlags(fl *flag.FlagSet) func() bool {
	mono := fl.Bool("mono", true, "specialize generic code per instantiation (default; --no-mono turns it off)")
	no := fl.Bool("no-mono", false, "do not specialize generic code: one Go generic function per Xo generic function")
	return func() bool { return *no || !*mono }
}

// arenaFlag defines --arenas: request scoped arenas for HTTP handlers and
// collection temporaries (std/rt/arena.go, decision 0040 experiment).
func arenaFlag(fl *flag.FlagSet) *bool {
	return fl.Bool("arenas", false, "request scoped arenas: confined HTTP handlers and non escaping collection temporaries reuse per request memory")
}

// backendFlag defines --backend: go (P1, the default) or llvm (the P3
// prototype: LLVM IR compiled by clang, host only, a core subset).
func backendFlag(fl *flag.FlagSet) *string {
	return fl.String("backend", "go", "code generator: go (default) or llvm (P3 prototype: a core subset compiled with clang, host only)")
}

func runBuild(args []string, stdout, stderr io.Writer) int {
	fl := flag.NewFlagSet("build", flag.ContinueOnError)
	fl.SetOutput(stderr)
	targets := fl.String("targets", "", "comma separated os/arch list, or all")
	out := fl.String("o", "", "output binary (one target) or directory (several targets)")
	asJSON := fl.Bool("json", false, "print the result as JSON")
	noMono := monoFlags(fl)
	arenas := arenaFlag(fl)
	backend := backendFlag(fl)
	if err := fl.Parse(args); err != nil {
		return 2
	}
	dirs := fl.Args()
	if len(dirs) == 0 {
		dirs = []string{"."}
	}
	ts, err := build.ParseTargets(*targets)
	if err != nil {
		fmt.Fprintf(stderr, "xo build: %v\n", err)
		return 2
	}
	if len(ts) > 1 && *out != "" {
		if err := os.MkdirAll(*out, 0o755); err != nil {
			fmt.Fprintf(stderr, "xo build: %v\n", err)
			return 2
		}
	}
	res, err := build.Build(dirs, build.Options{Mode: gen.Build, Targets: ts, Out: *out, NoMono: noMono(), Arenas: *arenas, Backend: *backend})
	if err != nil {
		fmt.Fprintf(stderr, "xo build: %v\n", err)
		return 1
	}
	if *asJSON {
		bins := map[string]string{}
		for t, p := range res.Binaries {
			bins[t.String()] = p
		}
		ds := res.Diagnostics
		if ds == nil {
			ds = []diag.Diagnostic{}
		}
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		enc.Encode(struct {
			Ok          bool              `json:"ok"`
			Binaries    map[string]string `json:"binaries"`
			Diagnostics []diag.Diagnostic `json:"diagnostics"`
		}{!res.Failed, bins, ds})
	} else {
		printDiags(res.Diagnostics, stderr)
		var names []string
		for t, p := range res.Binaries {
			names = append(names, t.String()+": "+p)
		}
		sort.Strings(names)
		for _, n := range names {
			fmt.Fprintln(stdout, n)
		}
	}
	if res.Failed {
		return 1
	}
	return 0
}

// runRun builds for the host and runs the program with the arguments after
// the module directory (or after `--`).
func runRun(args []string, stdout, stderr io.Writer) int {
	fl := flag.NewFlagSet("run", flag.ContinueOnError)
	fl.SetOutput(stderr)
	holes := fl.Bool("holes", false, "allow holes (`???`), which fault when evaluated")
	seed := fl.Int64("seed", 0, "run every task on the deterministic scheduler with this seed (core 7.4; sets XO_SEED)")
	noMono := monoFlags(fl)
	arenas := arenaFlag(fl)
	backend := backendFlag(fl)
	if err := fl.Parse(args); err != nil {
		return 2
	}
	rest := fl.Args()
	dir := "."
	var progArgs []string
	if len(rest) > 0 {
		if rest[0] != "--" {
			dir = rest[0]
			rest = rest[1:]
		}
		if len(rest) > 0 && rest[0] == "--" {
			rest = rest[1:]
		}
		progArgs = rest
	}
	res, err := build.Build([]string{dir}, build.Options{Mode: gen.Build, Holes: *holes, NoMono: noMono(), Arenas: *arenas, Backend: *backend})
	if err != nil {
		fmt.Fprintf(stderr, "xo run: %v\n", err)
		return 1
	}
	printDiags(res.Diagnostics, stderr)
	if res.Failed {
		return 1
	}
	var env []string
	if *seed != 0 {
		env = append(env, "XO_SEED="+strconv.FormatInt(*seed, 10))
	}
	return execBinary(res.Binaries[build.Host()], progArgs, stdout, stderr, env...)
}

func execBinary(bin string, args []string, stdout, stderr io.Writer, env ...string) int {
	cmd := exec.Command(bin, args...)
	if len(env) > 0 {
		cmd.Env = append(os.Environ(), env...)
	}
	cmd.Stdin = os.Stdin
	cmd.Stdout, cmd.Stderr = stdout, stderr
	err := cmd.Run()
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

// runTest builds the test blocks of the modules under the directories and
// runs them (core 9.2).
func runTest(args []string, stdout, stderr io.Writer) int {
	fl := flag.NewFlagSet("test", flag.ContinueOnError)
	fl.SetOutput(stderr)
	seed := fl.Int64("seed", 0, "seed for the scheduler, the test rand, and generated values (default 1)")
	explore := fl.Int("explore", 0, "run each concurrent test under this many seeds and report the first failing one")
	run := fl.String("run", "", "run only tests whose name contains this text")
	holes := fl.Bool("holes", false, "allow holes (`???`), which fault when evaluated")
	race := fl.Bool("race", false, "build with the Go race detector")
	asJSON := fl.Bool("json", false, "print one JSON object per test")
	verbose := fl.Bool("v", false, "print every test, not only failures")
	real := fl.Bool("real", false, "give tests that declare `uses` real capabilities (core 9.2)")
	cases := fl.Int("cases", 0, "cases per property test and contract fuzz (default 100)")
	noMono := monoFlags(fl)
	arenas := arenaFlag(fl)
	if err := fl.Parse(args); err != nil {
		return 2
	}
	dirs := fl.Args()
	if len(dirs) == 0 {
		dirs = []string{"."}
	}
	res, err := build.Build(dirs, build.Options{Mode: gen.Test, Holes: *holes, Race: *race, NoMono: noMono(), Arenas: *arenas})
	if err != nil {
		fmt.Fprintf(stderr, "xo test: %v\n", err)
		return 1
	}
	printDiags(res.Diagnostics, stderr)
	if res.Failed {
		return 1
	}
	var targs []string
	if *seed != 0 {
		targs = append(targs, "-seed", strconv.FormatInt(*seed, 10))
	}
	if *explore > 0 {
		targs = append(targs, "-explore", strconv.Itoa(*explore))
	}
	if *run != "" {
		targs = append(targs, "-run", *run)
	}
	if *asJSON {
		targs = append(targs, "-json")
	}
	if *verbose {
		targs = append(targs, "-v")
	}
	if *real {
		targs = append(targs, "-real")
	}
	if *cases > 0 {
		targs = append(targs, "-cases", strconv.Itoa(*cases))
	}
	// shrunk contract fuzz failures become regression tests (core 9.2)
	rf, err := os.CreateTemp("", "xo-regressions-*.jsonl")
	if err == nil {
		rf.Close()
		defer os.Remove(rf.Name())
		os.Setenv("XO_REGRESSION_OUT", rf.Name())
		defer os.Unsetenv("XO_REGRESSION_OUT")
	}
	code := execBinary(res.Binaries[build.Host()], targs, stdout, stderr)
	if err == nil {
		writeRegressions(rf.Name(), stdout)
	}
	return code
}
