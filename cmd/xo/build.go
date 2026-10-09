package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/xo-lang/xo/internal/build"
	"github.com/xo-lang/xo/internal/diag"
	"github.com/xo-lang/xo/internal/format"
	"github.com/xo-lang/xo/internal/gen"
	"github.com/xo-lang/xo/internal/mod"
	"github.com/xo-lang/xo/internal/vet"
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

// arenaFlags defines --arenas (the default since 2026-10-08, decision
// 0066, kept so older command lines work; --arenas=false turns it off) and
// --no-arenas, and returns a function reporting whether arenas are off.
func arenaFlags(fl *flag.FlagSet) func() bool {
	on := fl.Bool("arenas", true, "request scoped arenas for confined HTTP handlers and non escaping collection temporaries (default; --no-arenas turns it off)")
	no := fl.Bool("no-arenas", false, "no request scoped arenas: every request and response is heap memory")
	return func() bool { return *no || !*on }
}

// tzdataFlag defines --tzdata (decision 0094): embedded (the default:
// the zones the program uses are embedded) or system (the host's zone
// database at run time).
func tzdataFlag(fl *flag.FlagSet) *string {
	return fl.String("tzdata", "embedded", "where std/time finds zone data: embedded (the zones the program names, or the whole database for a name known only at run time) or system (the host's database)")
}

// printNotes prints build notes (such as the embedding of the whole zone
// database) on stderr.
func printNotes(notes []string, stderr io.Writer) {
	for _, n := range notes {
		fmt.Fprintln(stderr, n)
	}
}

// backendFlag defines --backend: go (P1, the default) or llvm (the P3
// prototype: LLVM IR compiled by clang, host only, a core subset).
//
// It also defines --no-gc (decision 0083): a build without a garbage
// collector on a native backend, arm64 on arm64 hosts and llvm otherwise
// unless --backend names one. The returned function, called after
// parsing, gives the backend and whether --no-gc was set.
func backendFlag(fl *flag.FlagSet) func() (string, bool) {
	backend := fl.String("backend", "go", "code generator: go (default), llvm (P3 prototype: a core subset compiled with clang, host only), or arm64 (native AArch64 assembly linked with cc: darwin/arm64, linux/arm64)")
	noGC := fl.Bool("no-gc", false, "build without a garbage collector on a native backend (arm64 on arm64 hosts, else llvm; --backend may name one); what it cannot build is reported as XO1201 before code generation")
	return func() (string, bool) {
		if !*noGC {
			return *backend, false
		}
		set := false
		fl.Visit(func(f *flag.Flag) {
			if f.Name == "backend" {
				set = true
			}
		})
		if !set {
			return build.NativeBackend(), true
		}
		return *backend, true
	}
}

func runBuild(args []string, stdout, stderr io.Writer) int {
	fl := flag.NewFlagSet("build", flag.ContinueOnError)
	fl.SetOutput(stderr)
	targets := fl.String("targets", "", "comma separated os/arch list, or all")
	out := fl.String("o", "", "output binary (one target) or directory (several targets)")
	asJSON := fl.Bool("json", false, "print the result as JSON")
	ci := fl.Bool("ci", false, "CI mode: fail before building when a .xo file is not formatted (core 10.5) or `dbg(...)` is left in code (core 11.4)")
	release := fl.Bool("release", false, "release build (core 8): contracts marked requires(debug) or ensures(debug) are compiled out; fails while an xo.work workspace overrides a required module (XO1308). --ci does not imply it")
	errorArgs := fl.Bool("error-args", true, "record parameter values in error frames (core 11.1, decision 0092); --error-args=false needs --release")
	hybrid := fl.Bool("hybrid", false, "compile the hot leaf functions of the main module (loops over scalars and Lists of flat structs, no allocation) to Go assembler with the arm64 backend, keeping the Go runtime (Go backend, decision 0084); the tag xo_pure builds the Go versions")
	noMono := monoFlags(fl)
	noArenas := arenaFlags(fl)
	backendOf := backendFlag(fl)
	tzdata := tzdataFlag(fl)
	if err := fl.Parse(args); err != nil {
		return 2
	}
	backend, noGC := backendOf()
	dirs := fl.Args()
	if len(dirs) == 0 {
		dirs = []string{"."}
	}
	ts, err := build.ParseTargets(*targets)
	if err != nil {
		fmt.Fprintf(stderr, "xo build: %v\n", err)
		return 2
	}
	if *out != "" {
		// the compilers run in the build directory, where a relative
		// path would land (gotchas/xo-build-relative-out.md)
		abs, err := filepath.Abs(*out)
		if err != nil {
			fmt.Fprintf(stderr, "xo build: %v\n", err)
			return 2
		}
		*out = abs
	}
	if len(ts) > 1 && *out != "" {
		if err := os.MkdirAll(*out, 0o755); err != nil {
			fmt.Fprintf(stderr, "xo build: %v\n", err)
			return 2
		}
	}
	if !*errorArgs && !*release {
		fmt.Fprintln(stderr, "xo build: --error-args=false needs --release (debug builds always record parameters)")
		return 2
	}
	if *hybrid && backend != "go" {
		fmt.Fprintln(stderr, "xo build: --hybrid needs the Go backend")
		return 2
	}
	if *ci || *release {
		if ds := workspaceRelease(dirs); len(ds) > 0 {
			if *asJSON {
				encodeJSON(stdout, struct {
					Ok          bool              `json:"ok"`
					Binaries    map[string]string `json:"binaries"`
					Diagnostics []diag.Diagnostic `json:"diagnostics"`
				}{false, map[string]string{}, ds})
			} else {
				printDiags(ds, stderr)
				flag := "--ci"
				if *release {
					flag = "--release"
				}
				fmt.Fprintf(stderr, "xo build: %s: nothing built\n", flag)
			}
			return 1
		}
	}
	if *ci {
		if code := ciCheck(dirs, *asJSON, stdout, stderr); code != 0 {
			return code
		}
	}
	res, err := build.Build(dirs, build.Options{Mode: gen.Build, Targets: ts, Out: *out, NoMono: noMono(), NoArenas: noArenas(), Backend: backend, NoGC: noGC, Hybrid: *hybrid, TZData: *tzdata, Release: *release, NoErrorArgs: !*errorArgs})
	if err != nil {
		fmt.Fprintf(stderr, "xo build: %v\n", err)
		return 1
	}
	if !*asJSON {
		printNotes(res.Notes, stderr)
	}
	if h := res.Hybrid; h != nil && !*asJSON {
		switch {
		case h.Err != "":
			fmt.Fprintf(stderr, "hybrid: no assembly (%s)\n", h.Err)
		case len(h.Asm) == 0:
			fmt.Fprintln(stderr, "hybrid: no hot function is eligible (a loop over scalars and Lists of flat structs, no allocation)")
		default:
			fmt.Fprintf(stderr, "hybrid: %s in Go assembler\n", strings.Join(h.Asm, ", "))
		}
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

// ciCheck is the CI mode of `xo build --ci`: every .xo file under dirs
// must be in the canonical format (core 10.5) and no `dbg(...)` may be
// left in code (core 11.4; XO0901, an error here). It returns 0 when the
// build may go on, 1 when it must fail, and 2 on a usage error.
func ciCheck(dirs []string, asJSON bool, stdout, stderr io.Writer) int {
	files, err := collectFmt(dirs)
	if err != nil {
		fmt.Fprintf(stderr, "xo build: %v\n", err)
		return 2
	}
	unformatted := []string{}
	ds := []diag.Diagnostic{}
	for _, path := range files {
		src, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintf(stderr, "xo build: %v\n", err)
			return 2
		}
		out, err := format.Source(path, src)
		if err != nil {
			// syntax errors are reported by the build itself
			continue
		}
		if string(out) != string(src) {
			unformatted = append(unformatted, path)
		}
	}
	r, err := vet.Vet(dirs)
	if err != nil {
		fmt.Fprintf(stderr, "xo build: %v\n", err)
		return 2
	}
	for _, d := range r.Diagnostics {
		if d.Code == string(diag.DebugLeft) {
			d.Severity = diag.Error
			ds = append(ds, d)
		}
	}
	if len(unformatted) == 0 && len(ds) == 0 {
		return 0
	}
	if asJSON {
		writeJSON(stdout, struct {
			Ok          bool              `json:"ok"`
			Binaries    map[string]string `json:"binaries"`
			Diagnostics []diag.Diagnostic `json:"diagnostics"`
			Unformatted []string          `json:"unformatted"`
		}{false, map[string]string{}, ds, unformatted})
		return 1
	}
	printDiags(ds, stderr)
	for _, p := range unformatted {
		fmt.Fprintf(stderr, "%s: not formatted; run `xo fmt` (core 10.5)\n", p)
	}
	fmt.Fprintln(stderr, "xo build: --ci: nothing built")
	return 1
}

// runRun builds for the host and runs the program with the arguments after
// the module directory (or after `--`).
func runRun(args []string, stdout, stderr io.Writer) int {
	fl := flag.NewFlagSet("run", flag.ContinueOnError)
	fl.SetOutput(stderr)
	holes := fl.Bool("holes", false, "allow holes (`???`), which fault when evaluated")
	seed := fl.Int64("seed", 0, "run every task on the deterministic scheduler with this seed (core 7.4; sets XO_SEED)")
	record := fl.String("record", "", "record every effect and scheduling decision into this .xotrace file for xo replay (core 11.3; sets XO_RECORD)")
	noMono := monoFlags(fl)
	noArenas := arenaFlags(fl)
	backendOf := backendFlag(fl)
	tzdata := tzdataFlag(fl)
	if err := fl.Parse(args); err != nil {
		return 2
	}
	backend, noGC := backendOf()
	if *seed != 0 && backend != "go" {
		// native builds run tasks on the real scheduler (decision 0068)
		fmt.Fprintln(stderr, "xo run: --seed needs the Go backend (native builds use the real scheduler)")
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
	res, err := build.Build([]string{dir}, build.Options{Mode: gen.Build, Holes: *holes, NoMono: noMono(), NoArenas: noArenas(), Backend: backend, NoGC: noGC, TZData: *tzdata})
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
	if *record != "" {
		if backend != "go" {
			fmt.Fprintln(stderr, "xo run: --record needs the Go backend")
			return 2
		}
		abs, err := filepath.Abs(*record)
		if err != nil {
			fmt.Fprintf(stderr, "xo run: %v\n", err)
			return 2
		}
		env = append(env, "XO_RECORD="+abs)
		code := execBinary(res.Binaries[build.Host()], progArgs, stdout, stderr, env...)
		if _, err := os.Stat(abs); err == nil {
			fmt.Fprintf(stderr, "xo run: recorded %s (xo replay %s)\n", *record, *record)
		}
		return code
	}
	return execBinary(res.Binaries[build.Host()], progArgs, stdout, stderr, env...)
}

func execBinary(bin string, args []string, stdout, stderr io.Writer, env ...string) int {
	return execBinaryIn(bin, args, os.Stdin, stdout, stderr, env...)
}

// execBinaryIn is execBinary with the given standard input (nil: none).
func execBinaryIn(bin string, args []string, stdin io.Reader, stdout, stderr io.Writer, env ...string) int {
	cmd := exec.Command(bin, args...)
	if len(env) > 0 {
		cmd.Env = append(os.Environ(), env...)
	}
	cmd.Stdin = stdin
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
	noArenas := arenaFlags(fl)
	backendOf := backendFlag(fl)
	tzdata := tzdataFlag(fl)
	if err := fl.Parse(args); err != nil {
		return 2
	}
	backend, noGC := backendOf()
	dirs := fl.Args()
	if len(dirs) == 0 {
		dirs = []string{"."}
	}
	// a workspace is one program: its modules' tests run together
	// (decision 0086)
	dirs = mod.WorkspaceDirs(dirs)
	res, err := build.Build(dirs, build.Options{Mode: gen.Test, Holes: *holes, Race: *race, NoMono: noMono(), NoArenas: noArenas(), Backend: backend, NoGC: noGC, TZData: *tzdata})
	if err != nil {
		fmt.Fprintf(stderr, "xo test: %v\n", err)
		return 1
	}
	printDiags(res.Diagnostics, stderr)
	if res.Failed {
		return 1
	}
	if backend != "go" && backend != "" {
		// a native test binary: one process per test (decision 0083)
		if *explore > 0 {
			fmt.Fprintln(stderr, "xo test: --explore needs the Go backend (native builds use the real scheduler)")
			return 2
		}
		return runNativeTests(res, nativeTestOpts{seed: *seed, run: *run, asJSON: *asJSON, verbose: *verbose, real: *real, timeout: 60 * time.Second}, stdout)
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
