// Command stdfuzz compares the three backends on the same seeded random
// inputs through pure std functions: it generates one Xo program that
// prints a line per case, builds and runs it on arm64, llvm and go (all
// three at once, at most three at a time: 2026-10-25, they were one after
// another and a seed took about nine minutes), and compares the outputs
// line by line. A difference is a finding, not a bug to fix here: the
// report names the case and its source, and -only re-runs just that case.
//
//	go run ./scripts/stdfuzz                  # seed 1, 200 cases per area
//	go run ./scripts/stdfuzz -seed=7 -n=500
//	go run ./scripts/stdfuzz -area=strings,json
//	go run ./scripts/stdfuzz -only=strings.0042
//	go run ./scripts/stdfuzz -keep=/tmp/fz    # keep the program for a repro
//	go run ./scripts/stdfuzz -release         # arm64 and llvm built --release
//
// -release builds the two native backends with `xo build --release` and runs
// the binaries, so the release driver's differences are compared too (the
// DWARF readers left out, and the hollow pass that empties unreached
// functions); the Go backend stays an oracle run.
//
// The inputs are seeded, so a report reproduces exactly. The areas are
// strings, bytes, strconv, json, runes, path, url, base64/base32/hex,
// time, regexp, csv, netip, big, compress, archive, mime, multipart, xml,
// sql, http, template, cryptostream, x509, ecc, limits, runes and maps.
package main

import (
	"context"
	"flag"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/xo-lang/xo/internal/xotool"
)

// preamble is the Xo support every generated program carries: each case
// renders its value as one Str, hex for text, so a newline in a result
// cannot break the line-per-case contract.
const preamble = `
fn fz_emit(out: Stdio, name: Str, value: Str) { out.println("${name}|${value}") }
fn fz_hx(s: Str) -> Str { s.bytes().to_hex() }
fn fz_i(n: Int) -> Str { "${n}" }
fn fz_f(f: Float) -> Str { "${f}" }
fn fz_bool(b: Bool) -> Str { if b { "1" } else { "0" } }
fn fz_bools(a: Bool, b: Bool, c: Bool, d: Bool, e: Bool) -> Str {
  var out = ""
  for x in [a, b, c, d, e] {
    out = if x { out + "1" } else { out + "0" }
  }
  out
}
fn fz_opt_s(o: Str?) -> Str {
  match o {
    Some(v) => fz_hx(v)
    None => "none"
  }
}
fn fz_opt_b(o: Bytes?) -> Str {
  match o {
    Some(v) => v.to_hex()
    None => "none"
  }
}
fn fz_opt_i(o: Int?) -> Str {
  match o {
    Some(v) => "${v}"
    None => "none"
  }
}
fn fz_opt_f(o: Float?) -> Str {
  match o {
    Some(v) => "${v}"
    None => "none"
  }
}
fn fz_opt_bool(o: Bool?) -> Str {
  match o {
    Some(v) => "${v}"
    None => "none"
  }
}
fn fz_ls(xs: List[Str]) -> Str {
  var out = ""
  for x in xs {
    if out != "" { out = out + "," }
    out = out + fz_hx(x)
  }
  "${xs.len()}/${out}"
}
fn fz_is_letter(r: Rune) -> Bool { r.is_letter() }
fn fz_is_digit(r: Rune) -> Bool { r.is_digit() }
fn fz_upper_keep(r: Rune) -> Rune? {
  if r.is_digit() { None } else { Some(r.to_upper()) }
}
fn fz_lss(rows: List[List[Str]]) -> Str {
  var out = ""
  for row in rows {
    if out != "" { out = out + ";" }
    var one = ""
    for f in row {
      if one != "" { one = one + "," }
      one = one + fz_hx(f)
    }
    out = out + one
  }
  "${rows.len()}/${out}"
}
fn fz_lb(xs: List[Bytes]) -> Str {
  var out = ""
  for x in xs {
    if out != "" { out = out + "," }
    out = out + x.to_hex()
  }
  "${xs.len()}/${out}"
}
`

func main() {
	seed := flag.Int64("seed", 1, "the seed; a report reproduces with the same seed")
	n := flag.Int("n", 200, "cases per area")
	areaFlag := flag.String("area", "", "comma separated areas (default all)")
	only := flag.String("only", "", "emit only this case name, for a repro")
	backendFlag := flag.String("backend", "arm64,llvm,go", "the backends to compare")
	release := flag.Bool("release", false, "build arm64 and llvm with xo build --release and run the binaries; go stays an oracle run")
	keep := flag.String("keep", "", "write the program to this directory and keep it")
	timeout := flag.Duration("timeout", 15*time.Minute, "per backend run")
	flag.Parse()

	areas := allAreas()
	if *areaFlag != "" {
		want := map[string]bool{}
		for _, a := range strings.Split(*areaFlag, ",") {
			want[strings.TrimSpace(a)] = true
		}
		var keepAreas []area
		for _, a := range areas {
			if want[a.Name] {
				keepAreas = append(keepAreas, a)
				delete(want, a.Name)
			}
		}
		for name := range want {
			fmt.Fprintf(os.Stderr, "stdfuzz: no area %q\n", name)
			os.Exit(2)
		}
		areas = keepAreas
	}

	// Cases are seeded per area, so a case name means the same input for
	// every run of the tool with the same seed.
	var cases []fuzzCase
	var helpers []string
	uses := map[string]bool{}
	areaOf := map[string]string{}
	for i, a := range areas {
		r := rand.New(rand.NewSource(*seed*1000 + int64(i)))
		cs := a.Gen(r, *n)
		for _, c := range cs {
			cases = append(cases, c)
			areaOf[c.Name] = a.Name
		}
		for _, u := range a.Uses {
			uses[u] = true
		}
		if a.Helpers != "" {
			helpers = append(helpers, a.Helpers)
		}
	}
	if *only != "" {
		var one []fuzzCase
		for _, c := range cases {
			if c.Name == *only {
				one = append(one, c)
			}
		}
		if len(one) == 0 {
			fmt.Fprintf(os.Stderr, "stdfuzz: no case %q for seed %d\n", *only, *seed)
			os.Exit(2)
		}
		cases = one
	}

	src := program(uses, helpers, cases)
	dir := *keep
	if dir == "" {
		d, err := os.MkdirTemp("", "stdfuzz")
		if err != nil {
			fatal(err)
		}
		defer os.RemoveAll(d)
		dir = d
	} else if err := os.MkdirAll(dir, 0o755); err != nil {
		fatal(err)
	}
	prog := filepath.Join(dir, "main.xo")
	if err := os.WriteFile(prog, []byte(src), 0o644); err != nil {
		fatal(err)
	}
	// std/internal is importable only from modules under
	// github.com/xo-lang/xo (std/internal.xo's header), which the crypto
	// area needs for internal.crypto_op.
	mod := filepath.Join(dir, "xo.mod")
	if err := os.WriteFile(mod, []byte("module github.com/xo-lang/xo/fuzz\nedition preview\n"), 0o644); err != nil {
		fatal(err)
	}

	xo, err := xotool.Path()
	if err != nil {
		fatal(err)
	}
	root, err := xotool.Root()
	if err != nil {
		fatal(err)
	}
	backends := splitList(*backendFlag)
	outs := make(map[string][]string, len(backends))
	failed := map[string]string{}
	// The backends build and run concurrently, at most three at a time
	// (2026-10-25: one after another, a seed of all areas took about ten
	// minutes on the 0372 critical path). They share only the generated
	// program, which is written before any of them starts, and their
	// outputs are compared after all finish, so a report and its exit
	// status are the same as before.
	runs := runConcurrently(backends, func(_ int, b string) backendRun {
		ctx, cancel := context.WithTimeout(context.Background(), *timeout)
		defer cancel()
		return runBackend(ctx, xo, root, dir, b, *release)
	})
	for i, b := range backends {
		outs[b] = runs[i].out
		if runs[i].fail != "" {
			failed[b] = runs[i].fail
		}
	}

	if len(failed) > 0 {
		for _, b := range backends {
			if msg, ok := failed[b]; ok {
				fmt.Printf("FAILED %s: %s\n", b, msg)
			}
		}
		fmt.Printf("stdfuzz seed %d: %d cases over %d areas, the run is invalid: a backend did not finish\n",
			*seed, len(cases), len(areas))
		os.Exit(1)
	}
	byName := map[string]string{}
	for _, c := range cases {
		byName[c.Name] = c.stmt()
	}
	values := parseOut(outs)
	diverged := 0
	for _, name := range order(cases) {
		vals, same := compare(backends, values, name)
		if same {
			continue
		}
		diverged++
		fmt.Printf("DIVERGENCE %s (%s)\n", name, areaOf[name])
		for _, b := range backends {
			if v, ok := vals[b]; ok {
				fmt.Printf("  %-6s %s\n", b, v)
			} else {
				fmt.Printf("  %-6s (no line)\n", b)
			}
		}
		fmt.Printf("  case: %s\n", byName[name])
		fmt.Printf("  rerun: go run ./scripts/stdfuzz -seed=%d%s -only=%s\n", *seed, releaseFlag(*release), name)
	}
	fmt.Printf("stdfuzz seed %d: %d cases over %d areas, backends %s%s\n",
		*seed, len(cases), len(areas), strings.Join(backends, ", "), releaseNote(*release))
	for _, b := range backends {
		fmt.Printf("  %s: %d case lines\n", b, len(values[b]))
	}
	if diverged > 0 {
		fmt.Printf("%d divergence(s)\n", diverged)
		os.Exit(1)
	}
	fmt.Println("no divergence")
}

// backendRun is one backend's case lines, or the message that it did not
// finish.
type backendRun struct {
	out  []string
	fail string
}

// runBackend builds the generated program with -release when b is a native
// backend and release is on, else runs it through `xo run`, and returns the
// lines it printed.
func runBackend(ctx context.Context, xo, root, dir, b string, release bool) backendRun {
	env := append(os.Environ(), "XO_STD="+filepath.Join(root, "std"))
	if release && isReleaseBackend(b) {
		bin := filepath.Join(dir, "prog-"+b)
		if runtime.GOOS == "windows" {
			bin += ".exe"
		}
		build := exec.CommandContext(ctx, xo, buildArgs(b, bin, dir)...)
		build.Env = env
		bout, err := build.CombinedOutput()
		if err != nil {
			return backendRun{fail: "build: " + firstLines(string(bout), 4)}
		}
		run := exec.CommandContext(ctx, bin)
		run.Env = env
		out, err := run.Output()
		if err != nil {
			return backendRun{fail: exitMessage(err)}
		}
		return backendRun{out: lines(string(out))}
	}
	cmd := exec.CommandContext(ctx, xo, "run", "--backend="+b, dir)
	cmd.Env = env
	out, err := cmd.Output()
	if err != nil {
		return backendRun{fail: exitMessage(err)}
	}
	return backendRun{out: lines(string(out))}
}

// exitMessage is a run's failure as a report line: the error, or the first
// lines a program that exited nonzero wrote to stderr.
func exitMessage(err error) string {
	if ee, ok := err.(*exec.ExitError); ok {
		return firstLines(string(ee.Stderr), 4)
	}
	return err.Error()
}

// runConcurrently runs run for every backend, at most three at a time, and
// returns the results in backend order. Three is the cap: the two native
// builds and the Go run, which is what a seed starts.
func runConcurrently(backends []string, run func(i int, b string) backendRun) []backendRun {
	results := make([]backendRun, len(backends))
	sem := make(chan struct{}, 3)
	var wg sync.WaitGroup
	for i, b := range backends {
		wg.Add(1)
		go func(i int, b string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			results[i] = run(i, b)
		}(i, b)
	}
	wg.Wait()
	return results
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "stdfuzz:", err)
	os.Exit(2)
}

func splitList(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// isReleaseBackend reports whether -release builds backend b as a binary
// instead of running it through `xo run`: the two native backends. The Go
// backend stays an oracle run.
func isReleaseBackend(b string) bool {
	return b == "arm64" || b == "llvm"
}

// buildArgs is the `xo build` command line for a release run of backend b.
func buildArgs(b, bin, dir string) []string {
	return []string{"build", "--backend=" + b, "--release", "-o", bin, dir}
}

// releaseFlag is appended to a rerun hint when the run used -release.
func releaseFlag(on bool) string {
	if on {
		return " -release"
	}
	return ""
}

// releaseNote says in the summary that the native backends were built for
// release, so a log can be told apart from a plain `xo run` one.
func releaseNote(on bool) string {
	if on {
		return " (release builds for arm64 and llvm, go run for go)"
	}
	return ""
}

// program assembles the Xo source: the imports the areas need, the
// helpers, and one statement per case.
func program(uses map[string]bool, helpers []string, cases []fuzzCase) string {
	var b strings.Builder
	b.WriteString("// Generated by scripts/stdfuzz; do not edit.\n")
	var sorted []string
	for u := range uses {
		sorted = append(sorted, u)
	}
	sort.Strings(sorted)
	for _, u := range sorted {
		fmt.Fprintf(&b, "use %s\n", u)
	}
	b.WriteString(preamble)
	for _, h := range helpers {
		b.WriteString(h)
	}
	for _, pre := range preludes {
		b.WriteString(pre)
	}
	b.WriteString("\nfn main(os: Os) {\n")
	for _, c := range cases {
		b.WriteString(c.stmt())
		b.WriteByte('\n')
	}
	b.WriteString("}\n")
	return b.String()
}

// lines splits a run's stdout, keeping only the case lines.
func lines(out string) []string {
	var ls []string
	for _, l := range strings.Split(out, "\n") {
		if l != "" {
			ls = append(ls, l)
		}
	}
	return ls
}

// firstLines is the first n non-empty lines of a backend's stderr, where a
// compile error and its notes are.
func firstLines(s string, n int) string {
	var out []string
	for _, l := range strings.Split(s, "\n") {
		if strings.TrimSpace(l) == "" {
			continue
		}
		out = append(out, strings.TrimRight(l, " \t"))
		if len(out) == n {
			break
		}
	}
	return strings.Join(out, "\n")
}

// parseOut maps each backend's stdout to name to value, keeping only lines
// of the form name|value.
func parseOut(outs map[string][]string) map[string]map[string]string {
	values := make(map[string]map[string]string, len(outs))
	for b, ls := range outs {
		m := map[string]string{}
		for _, l := range ls {
			if i := strings.IndexByte(l, '|'); i > 0 {
				m[l[:i]] = l[i+1:]
			}
		}
		values[b] = m
	}
	return values
}

// compare returns one case's value per backend and whether every backend
// printed the same one. A backend with no line for the case is a difference.
func compare(backends []string, values map[string]map[string]string, name string) (map[string]string, bool) {
	vals := make(map[string]string, len(backends))
	same := true
	first := ""
	for _, b := range backends {
		v, ok := values[b][name]
		if !ok {
			same = false
			continue
		}
		vals[b] = v
		if first == "" {
			first = v
		} else if v != first {
			same = false
		}
	}
	return vals, same
}

// order returns the case names in generated order.
func order(cases []fuzzCase) []string {
	out := make([]string, len(cases))
	for i, c := range cases {
		out[i] = c.Name
	}
	return out
}

// xq is s as an Xo string literal.
func xq(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch {
		case r == '"':
			b.WriteString(`\"`)
		case r == '\\':
			b.WriteString(`\\`)
		case r == '$':
			b.WriteString(`\$`)
		case r == '\n':
			b.WriteString(`\n`)
		case r == '\r':
			b.WriteString(`\r`)
		case r == '\t':
			b.WriteString(`\t`)
		case r < 0x20 || r == 0x7f:
			fmt.Fprintf(&b, `\u{%x}`, r)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

// xbytes is b as an Xo Bytes literal.
func xbytes(b []byte) string {
	var s strings.Builder
	s.WriteString(`b"`)
	for _, c := range b {
		if c >= 0x20 && c < 0x7f && c != '"' && c != '\\' && c != '$' {
			s.WriteByte(c)
		} else {
			fmt.Fprintf(&s, `\x%02x`, c)
		}
	}
	s.WriteByte('"')
	return s.String()
}
