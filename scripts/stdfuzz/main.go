// Command stdfuzz compares the three backends on the same seeded random
// inputs through pure std functions: it generates one Xo program that
// prints a line per case, runs it with `xo run --backend=arm64`,
// `--backend=llvm` and `--backend=go`, and compares the outputs line by
// line. A difference is a finding, not a bug to fix here: the report names
// the case and its source, and -only re-runs just that case.
//
//	go run ./scripts/stdfuzz                  # seed 1, 200 cases per area
//	go run ./scripts/stdfuzz -seed=7 -n=500
//	go run ./scripts/stdfuzz -area=strings,json
//	go run ./scripts/stdfuzz -only=strings.0042
//	go run ./scripts/stdfuzz -keep=/tmp/fz    # keep the program for a repro
//
// The inputs are seeded, so a report reproduces exactly. The areas are
// strings, bytes, strconv, json, runes, path, url, base64/base32/hex,
// time, regexp, csv, netip, big, compress, archive, mime, multipart, xml,
// sql, http, template, cryptostream, x509, ecc and limits.
package main

import (
	"context"
	"flag"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
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
	for _, b := range backends {
		ctx, cancel := context.WithTimeout(context.Background(), *timeout)
		cmd := exec.CommandContext(ctx, xo, "run", "--backend="+b, dir)
		cmd.Env = append(os.Environ(), "XO_STD="+filepath.Join(root, "std"))
		out, err := cmd.Output()
		cancel()
		if err != nil {
			msg := err.Error()
			if ee, ok := err.(*exec.ExitError); ok {
				msg = firstLines(string(ee.Stderr), 4)
			}
			failed[b] = msg
		}
		outs[b] = lines(string(out))
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
		fmt.Printf("  rerun: go run ./scripts/stdfuzz -seed=%d -only=%s\n", *seed, name)
	}
	fmt.Printf("stdfuzz seed %d: %d cases over %d areas, backends %s\n",
		*seed, len(cases), len(areas), strings.Join(backends, ", "))
	for _, b := range backends {
		fmt.Printf("  %s: %d case lines\n", b, len(values[b]))
	}
	if diverged > 0 {
		fmt.Printf("%d divergence(s)\n", diverged)
		os.Exit(1)
	}
	fmt.Println("no divergence")
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
