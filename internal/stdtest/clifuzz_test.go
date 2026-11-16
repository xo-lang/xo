// A fuzz harness for std/cli against Go's flag package (decision 0616's
// fuzz kind), on the commands std/cli makes to behave as Go's flag:
// single_dash_long and stop_at_positional (decisions 0547 and 0556, for
// ports of Go tools), long flags only (bool, str, int, duration, float and
// a repeated str flag, a flag.Value that appends), and a rest argument for
// what Go's FlagSet.Args returns. Each case declares 1 to 6 flags and
// parses 0 to 10 arguments built from the flags' -name, --name, -name=v
// and -name v forms (values valid and not: words, values starting with -,
// empty, numbers, 1.5, 2e-3, inf, nan, 30s, 1m30s, true, false, 0, 1),
// unknown names, -h and -help, -- and -, and plain words. std/cli must
// succeed where FlagSet.Parse does with the same values and the same
// remaining arguments, ask for help where Go returns flag.ErrHelp, and
// fail where Go fails (the messages differ by design).
//
// The cases from seed 1 are the committed corpus
// testdata/corpus/clifuzz/clifuzz_test.xo, which TestStdCorpora runs on
// every corpus backend; TestCliFuzzCorpusCurrent checks it is what this
// generator writes (XO_CLIFUZZ_WRITE=1 rewrites it). TestCliFuzz runs a
// fresh seed (XO_CLIFUZZ_SEED, default 2; XO_CLIFUZZ_N cases, default 2000)
// on XO_CLIFUZZ_BACKENDS (default arm64 and llvm).
package stdtest

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"math/rand"
	"strings"
	"testing"
	"time"
)

type clFlag struct {
	name, kind string // kind: bool str int dur float strs
}

type clCase struct {
	flags []clFlag
	args  []string
	want  string
}

type clStrs []string

func (s *clStrs) String() string     { return strings.Join(*s, ",") }
func (s *clStrs) Set(v string) error { *s = append(*s, v); return nil }

var clKinds = []string{"bool", "str", "int", "dur", "float", "strs"}
var clNames = []string{"verbose", "out", "count", "timeout", "ratio", "tag", "x64", "name", "dry-run", "level"}

func clValue(r *rand.Rand, kind string) string {
	switch kind {
	case "bool":
		return []string{"true", "false", "1", "0", "yes", ""}[r.Intn(6)]
	case "int":
		return []string{"0", "5", "-3", "123456789", "two", "1.5", "", "-0", "9223372036854775807"}[r.Intn(9)]
	case "dur":
		return []string{"30s", "1m30s", "0", "-5ms", "1h", "5", "x", "", "1.5h", "300ms"}[r.Intn(10)]
	case "float":
		return []string{"1.5", "2e-3", "-0", "inf", "-Inf", "nan", "x", "", "1e10", "7"}[r.Intn(10)]
	}
	return []string{"a", "word", "-x", "--y", "", "a=b", "日本", "-", "--"}[r.Intn(9)]
}

func clArgs(r *rand.Rand, flags []clFlag) []string {
	var args []string
	for k := r.Intn(11); k > 0; k-- {
		switch c := r.Intn(12); {
		case c < 6:
			f := flags[r.Intn(len(flags))]
			dash := []string{"-", "--"}[r.Intn(2)]
			switch {
			case f.kind == "bool" && r.Intn(3) != 0:
				args = append(args, dash+f.name)
			case r.Intn(2) == 0:
				args = append(args, dash+f.name+"="+clValue(r, f.kind))
			default:
				args = append(args, dash+f.name, clValue(r, f.kind))
			}
		case c < 7:
			args = append(args, []string{"-nope", "--nope", "-v", "---x", "-=x", "-", "--"}[r.Intn(7)])
		case c < 8 && r.Intn(4) == 0:
			args = append(args, []string{"-h", "-help", "--help"}[r.Intn(3)])
		default:
			args = append(args, []string{"file", "a b", "x=1", "", "-"}[r.Intn(5)])
		}
	}
	return args
}

// clGo is Go's answer: help, usage, or ok with each flag's value and the
// remaining arguments, as the Xo side writes them.
func clGo(flags []clFlag, args []string) string {
	fs := flag.NewFlagSet("t", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	vals := map[string]any{}
	for _, f := range flags {
		switch f.kind {
		case "bool":
			vals[f.name] = fs.Bool(f.name, false, "")
		case "str":
			vals[f.name] = fs.String(f.name, "", "")
		case "int":
			vals[f.name] = fs.Int(f.name, 0, "")
		case "dur":
			vals[f.name] = fs.Duration(f.name, 0, "")
		case "float":
			vals[f.name] = fs.Float64(f.name, 0, "")
		default:
			s := &clStrs{}
			fs.Var(s, f.name, "")
			vals[f.name] = s
		}
	}
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return "help"
		}
		return "usage"
	}
	parts := []string{"ok"}
	for _, f := range flags {
		var v string
		switch p := vals[f.name].(type) {
		case *bool:
			v = fmt.Sprint(*p)
		case *string:
			v = *p
		case *int:
			v = fmt.Sprint(*p)
		case *time.Duration:
			v = fmt.Sprint(int64(*p))
		case *float64:
			// a NaN as nan: Go's ParseFloat sets a payload bit Float.parse does not
			v = fmt.Sprint(int64(math.Float64bits(*p)))
			if math.IsNaN(*p) {
				v = "nan"
			}
		case *clStrs:
			v = strings.Join(*p, "\x1e")
		}
		parts = append(parts, f.name+"="+v)
	}
	parts = append(parts, "rest="+strings.Join(fs.Args(), "\x1e"))
	return strings.Join(parts, "\x1f")
}

func clCases(r *rand.Rand, n int) []clCase {
	cs := make([]clCase, 0, n)
	for len(cs) < n {
		var flags []clFlag
		seen := map[string]bool{}
		for k := 1 + r.Intn(6); k > 0; k-- {
			name := clNames[r.Intn(len(clNames))]
			if seen[name] {
				continue
			}
			seen[name] = true
			flags = append(flags, clFlag{name, clKinds[r.Intn(len(clKinds))]})
		}
		args := clArgs(r, flags)
		cs = append(cs, clCase{flags, args, clGo(flags, args)})
	}
	return cs
}

const clCheckXo = `use std/cli

fn flag_of(d: Str) -> cli.Flag {
  let (name, kind) = d.cut(":") ?? fault("flag ${d}")
  match kind {
    "bool" => cli.bool_flag(name, "b")
    "str" => cli.str_flag(name, "s")
    "int" => cli.int_flag(name, "i")
    "dur" => cli.duration_flag(name, "d")
    "float" => cli.float_flag(name, "f")
    _ => cli.strs_flag(name, "r")
  }
}

fn value_of(p: cli.Parsed, d: Str) -> Str {
  let (name, kind) = d.cut(":") ?? fault("flag ${d}")
  let v = match kind {
    "bool" => "${p.bool(name)}"
    "str" => p.str(name)
    "int" => "${p.int(name)}"
    "dur" => "${p.duration(name).nanos()}"
    "float" => if p.float(name).is_nan() { "nan" } else { "${p.float(name).to_bits()}" }
    _ => p.strs(name).join("\u{1e}")
  }
  "${name}=${v}"
}

fn answer(decl: Str, args: Str, env: Env) -> Str uses env {
  let ds = decl.split(" ")
  let cmd = cli.command("t", "a tool", flags: ds.map(flag_of), args: [cli.rest("args", "the rest")], single_dash_long: true, stop_at_positional: true)
  let av = if args == "" { [] } else { args.split("\u{1f}").map(fn(a: Str) -> Str { if a == "\u{1d}" { "" } else { a } }) }
  match cli.parse(cmd, av, env) {
    Ok(p) => {
      var parts = ["ok"]
      for d in ds {
        parts.push(value_of(p, d))
      }
      parts.push("rest=${p.rest("args").join("\u{1e}")}")
      parts.join("\u{1f}")
    }
    Err(cli.CliErr.Help(_)) => "help"
    Err(_) => "usage"
  }
}

test "std/cli with single_dash_long and stop_at_positional parses as Go's flag" {
  var bad: List[(Str, Str, Str, Str)] = []
  for (decl, args, want) in CASES {
    let got = answer(decl, args, os.env)
    if got != want {
      bad.push((decl, args, got, want))
    }
  }
  expect(bad[0..(if bad.len() < 10 { bad.len() } else { 10 })]) == []
}
`

func clXoFile(seed int64, cases []clCase) string {
	var b strings.Builder
	fmt.Fprintf(&b, "// Generated by internal/stdtest/clifuzz_test.go (seed %d, %d cases) from Go's\n", seed, len(cases))
	b.WriteString("// flag; do not edit: XO_CLIFUZZ_WRITE=1 go test ./internal/stdtest -run TestCliFuzzCorpusCurrent\n")
	b.WriteString("// rewrites it.\n")
	b.WriteString(clCheckXo)
	b.WriteString("\n/// (the flags as name:kind, the arguments joined by U+001F (U+001D an empty one), Go's answer: help, usage, or ok and each value and the rest, joined by U+001F)\nconst CASES: List[(Str, Str, Str)] = [\n")
	for _, c := range cases {
		var ds []string
		for _, f := range c.flags {
			ds = append(ds, f.name+":"+f.kind)
		}
		as := make([]string, len(c.args))
		for i, a := range c.args {
			if a == "" {
				a = "\x1d"
			}
			as[i] = a
		}
		fmt.Fprintf(&b, "  (%s, %s, %s),\n", xoStr(strings.Join(ds, " ")), xoStr(strings.Join(as, "\x1f")), xoStr(c.want))
	}
	b.WriteString("]\n")
	return b.String()
}

// clCoverage fails when the cases stop reaching each outcome.
func clCoverage(t *testing.T, cases []clCase) {
	t.Helper()
	ok, help, usage, rest := 0, 0, 0, 0
	for _, c := range cases {
		switch {
		case c.want == "help":
			help++
		case c.want == "usage":
			usage++
		default:
			ok++
			if !strings.HasSuffix(c.want, "rest=") {
				rest++
			}
		}
	}
	n := len(cases)
	if ok < n/4 || help < n/100 || usage < n/10 || rest < n/10 {
		t.Fatalf("coverage fell: %d ok (%d with positionals), %d help, %d usage of %d", ok, rest, help, usage, n)
	}
}

const clCorpus = "testdata/corpus/clifuzz/clifuzz_test.xo"

// TestCliFuzzCorpusCurrent: the committed corpus is what seed 1 writes.
func TestCliFuzzCorpusCurrent(t *testing.T) {
	cases := clCases(rand.New(rand.NewSource(1)), 500)
	clCoverage(t, cases)
	bxCorpusCurrent(t, clCorpus, "XO_CLIFUZZ_WRITE", clXoFile(1, cases))
}

// TestCliFuzz runs a fresh seed's cases.
func TestCliFuzz(t *testing.T) {
	if testing.Short() {
		t.Skip("the cli fuzz is a heavy test; its seed 1 corpus runs in TestStdCorpora")
	}
	seed := int64(envInt(t, "XO_CLIFUZZ_SEED", 2))
	n := envInt(t, "XO_CLIFUZZ_N", 2000)
	cases := clCases(rand.New(rand.NewSource(seed)), n)
	runFuzzDir(t, "clifuzz_test.xo", clXoFile(seed, cases), envBackends("XO_CLIFUZZ_BACKENDS", []string{"arm64", "llvm"}), len(cases))
}
