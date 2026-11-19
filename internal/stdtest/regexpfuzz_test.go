// A fuzz harness for std/regexp against Go's regexp (decision 0616's fuzz
// kind; task #108). Each case is a pattern built from RE2 pieces (literals
// with UTF-8, classes and negated classes, Perl and Unicode classes, anchors
// and word boundaries, the flags i, s, m and U, groups plain, non capturing
// and named, alternation, greedy and lazy repetition with counts, \Q...\E)
// with mistakes mixed in (an unclosed group or class, a stray ), a repeat
// of a repeat, {2,1}, a bad escape, an empty or bad group name), and four
// inputs. std/regexp must reject the pattern with Go's error code and
// expression (syntax.Error, "missing closing ): `(a`"), or answer each
// input as Go does: the leftmost match's indexes, every match's submatch
// indexes (FindAllStringSubmatchIndex), Split(s, -1) and
// ReplaceAllString(s, "<$1|${n}>").
//
// The cases from seed 1 are the committed corpus
// testdata/corpus/regexpfuzz/regexpfuzz_test.xo, which TestStdCorpora runs
// on every corpus backend; TestRegexpFuzzCorpusCurrent checks it is what
// this generator writes (XO_REGEXPFUZZ_WRITE=1 rewrites it). TestRegexpFuzz
// runs a fresh seed (XO_REGEXPFUZZ_SEED, default 2; XO_REGEXPFUZZ_N cases,
// default 2000) on XO_REGEXPFUZZ_BACKENDS (default arm64 and llvm).
package stdtest

import (
	"errors"
	"fmt"
	"math/rand"
	"regexp"
	"regexp/syntax"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/hostile"
)

type rxCase struct {
	pattern string
	inputs  []string
	want    string // "err:" and the message, or one answer per input joined by \x1f
}

var rxAtoms = []string{
	"a", "b", "ab", "é", "日", "x", "1", "-", " ", ".", `\.`, `\d`, `\D`, `\w`, `\W`, `\s`, `\S`,
	"[a-c]", "[^b]", "[a-zé]", "[[:alpha:]]", "[[:^digit:]]", `[\d_]`, `\pL`, `\PN`, `\p{Greek}`, `\p{Han}`,
	"^", "$", `\A`, `\z`, `\b`, `\B`, `\Q.*\E`, `\x41`, `\x{65e5}`, `\n`, `\t`,
}

var rxBad = []string{"(", ")", "[", "[a", "**", "a{2,1}", `\8`, `\pX`, "(?P<>a)", "(?P<1n>a)", "(?z)", "[z-a]", "x{1001}", "(?i", `\`, "+", "?", "(?P<n>a)(?P<n>b)"}

func rxPiece(r *rand.Rand, depth int) string {
	var s string
	switch k := r.Intn(10); {
	case k < 5 || depth > 3:
		s = rxAtoms[r.Intn(len(rxAtoms))]
	case k == 5:
		s = "(" + rxSeq(r, depth+1) + ")"
	case k == 6:
		s = "(?:" + rxSeq(r, depth+1) + ")"
	case k == 7:
		s = fmt.Sprintf("(?P<%s>%s)", []string{"n", "word", "x1"}[r.Intn(3)], rxSeq(r, depth+1))
	case k == 8:
		s = rxSeq(r, depth+1) + "|" + rxSeq(r, depth+1)
	default:
		s = "(?" + []string{"i", "s", "m", "U", "is", "-i", "i:"}[r.Intn(7)]
		if strings.HasSuffix(s, ":") {
			s += rxSeq(r, depth+1) + ")"
		} else {
			s += ")"
		}
	}
	if r.Intn(3) == 0 {
		q := []string{"*", "+", "?", "{2}", "{1,3}", "{0,}", "*?", "+?", "??", "{2,4}?"}[r.Intn(10)]
		if q == "{2}" && len(s)%4 == 1 {
			// the hostile dictionary (task #352): repeat counts past
			// 1000 and past Int, chosen by the length so the cases after
			// it stay as they were
			ds := hostile.Digits()
			q = "{" + strings.TrimPrefix(ds[len(s)%len(ds)], "+") + "}"
		}
		s += q
	}
	return s
}

func rxSeq(r *rand.Rand, depth int) string {
	var b strings.Builder
	for k := 1 + r.Intn(3); k > 0; k-- {
		b.WriteString(rxPiece(r, depth))
	}
	return b.String()
}

func rxInput(r *rand.Rand) string {
	alpha := []string{"a", "b", "ab", "é", "日", "x", "1", "-", " ", "\n", "A", "B", ".", "_", "Ω"}
	var b strings.Builder
	for k := r.Intn(12); k > 0; k-- {
		b.WriteString(alpha[r.Intn(len(alpha))])
	}
	return b.String()
}

func rxIdx(xs []int) string {
	parts := make([]string, len(xs))
	for i, x := range xs {
		parts[i] = fmt.Sprint(x)
	}
	return strings.Join(parts, ",")
}

// rxAnswer is Go's answer for one input: the first match's indexes, every
// match's submatch indexes, the split, and the replacement, joined by ";".
func rxAnswer(re *regexp.Regexp, s string) string {
	first := "none"
	if m := re.FindStringIndex(s); m != nil {
		first = rxIdx(m)
	}
	var all []string
	for _, m := range re.FindAllStringSubmatchIndex(s, -1) {
		all = append(all, rxIdx(m))
	}
	return first + ";" + strings.Join(all, "/") + ";" + strings.Join(re.Split(s, -1), "\x1e") + ";" + re.ReplaceAllString(s, "<$1|${n}>")
}

func rxCases(r *rand.Rand, n int) []rxCase {
	cs := make([]rxCase, 0, n)
	for len(cs) < n {
		p := rxSeq(r, 0)
		if r.Intn(8) == 0 {
			// at a rune boundary: the pattern stays UTF-8 (a Str)
			rs := []rune(p)
			i := r.Intn(len(rs) + 1)
			p = string(rs[:i]) + rxBad[r.Intn(len(rxBad))] + string(rs[i:])
		}
		in := []string{rxInput(r), rxInput(r), rxInput(r), rxInput(r)}
		re, err := regexp.Compile(p)
		if err != nil {
			var se *syntax.Error
			if !errors.As(err, &se) || se.Code == syntax.ErrLarge || se.Code == syntax.ErrNestingDepth {
				continue // not ported (std/regexp has no size or depth limit)
			}
			cs = append(cs, rxCase{p, in, "err:" + string(se.Code) + ": `" + se.Expr + "`"})
			continue
		}
		var ans []string
		for _, s := range in {
			ans = append(ans, rxAnswer(re, s))
		}
		cs = append(cs, rxCase{p, in, strings.Join(ans, "\x1f")})
	}
	return cs
}

const rxCheckXo = `use std/regexp

fn idx(m: (Int, Int)) -> Str { "${m.0},${m.1}" }

fn sub_idx(ms: List[(Int, Int)?]) -> Str {
  ms.map(fn(m: (Int, Int)?) -> Str {
    match m {
      Some(p) => idx(p)
      None => "-1,-1"
    }
  }).join(",")
}

fn answer(re: regexp.Regex, s: Str) -> Str {
  let first = match re.find_index(s) {
    Some(m) => idx(m)
    None => "none"
  }
  let all = re.captures_all_index(s).map(sub_idx).join("/")
  "${first};${all};${re.split(s).join("\u{1e}")};${re.replace_all(s, "<$1|\u{24}{n}>")}"
}

fn answers(pattern: Str, inputs: List[Str]) -> Str {
  match regexp.compile(pattern) {
    Ok(re) => inputs.map(fn(s: Str) -> Str { answer(re, s) }).join("\u{1f}")
    Err(e) => "err:${e.msg}"
  }
}

test "std/regexp compiles, matches, splits and replaces as Go's regexp" {
  var bad: List[(Str, Str, Str)] = []
  for (pattern, inputs, want) in CASES {
    let got = answers(pattern, inputs)
    if got != want {
      bad.push((pattern, got, want))
    }
  }
  expect(bad[0..(if bad.len() < 10 { bad.len() } else { 10 })]) == []
}
`

func rxXoFile(seed int64, cases []rxCase) string {
	var b strings.Builder
	fmt.Fprintf(&b, "// Generated by internal/stdtest/regexpfuzz_test.go (seed %d, %d cases) from Go's\n", seed, len(cases))
	b.WriteString("// regexp; do not edit: XO_REGEXPFUZZ_WRITE=1 go test ./internal/stdtest -run TestRegexpFuzzCorpusCurrent\n")
	b.WriteString("// rewrites it.\n")
	b.WriteString(rxCheckXo)
	b.WriteString("\n/// (pattern, four inputs, Go's answer: err: and the message, or per input the first match, every match's submatch indexes, the split and the replacement)\nconst CASES: List[(Str, List[Str], Str)] = [\n")
	for _, c := range cases {
		in := make([]string, len(c.inputs))
		for i, s := range c.inputs {
			in[i] = xoStr(s)
		}
		fmt.Fprintf(&b, "  (%s, [%s], %s),\n", xoStr(c.pattern), strings.Join(in, ", "), xoStr(c.want))
	}
	b.WriteString("]\n")
	return b.String()
}

// rxCoverage fails when the cases stop reaching rejected patterns, matches
// and submatches.
func rxCoverage(t *testing.T, cases []rxCase) {
	t.Helper()
	errs, matched, subs := 0, 0, 0
	for _, c := range cases {
		if strings.HasPrefix(c.want, "err:") {
			errs++
			continue
		}
		for _, a := range strings.Split(c.want, "\x1f") {
			if !strings.HasPrefix(a, "none;") {
				matched++
			}
			if strings.Count(strings.SplitN(a, ";", 3)[1], ",") > 2 {
				subs++
			}
		}
	}
	n := len(cases)
	if errs < n/20 || errs > n/3 || matched < n || subs < n/2 {
		t.Fatalf("coverage fell: %d rejected of %d, %d inputs matched, %d with submatches", errs, n, matched, subs)
	}
}

const rxCorpus = "testdata/corpus/regexpfuzz/regexpfuzz_test.xo"

// TestRegexpFuzzCorpusCurrent: the committed corpus is what seed 1 writes.
func TestRegexpFuzzCorpusCurrent(t *testing.T) {
	cases := rxCases(rand.New(rand.NewSource(1)), 400)
	rxCoverage(t, cases)
	bxCorpusCurrent(t, rxCorpus, "XO_REGEXPFUZZ_WRITE", rxXoFile(1, cases))
}

// TestRegexpFuzz runs a fresh seed's cases.
func TestRegexpFuzz(t *testing.T) {
	if testing.Short() {
		t.Skip("the regexp fuzz is a heavy test; its seed 1 corpus runs in TestStdCorpora")
	}
	seed := int64(envInt(t, "XO_REGEXPFUZZ_SEED", 2))
	n := envInt(t, "XO_REGEXPFUZZ_N", 2000)
	cases := rxCases(rand.New(rand.NewSource(seed)), n)
	runFuzzDir(t, "regexpfuzz_test.xo", rxXoFile(seed, cases), envBackends("XO_REGEXPFUZZ_BACKENDS", []string{"arm64", "llvm"}), len(cases))
}
