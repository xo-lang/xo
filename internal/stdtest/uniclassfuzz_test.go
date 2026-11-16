// A fuzz harness for std/uniclass, the Unicode class tables of std/regexp's
// \p{...}, against Go's regexp (decision 0616's fuzz kind). Each case is a
// class name (one of uniclass.class_of's names, read from std/uniclass.xo,
// or one spelled differently: lower or upper case, with _ - or spaces, cut
// short, with a letter added, or random letters), a form (\p{N}, \P{N},
// (?i)\p{N}, [^\p{N}], [\p{N}\p{Greek}], \pL style for a one letter
// name), and 24 runes (the ends of the class's ranges and their
// neighbours when Go's unicode package has the class, ASCII, Latin-1,
// the BMP, the astral planes and U+10FFFF); std/regexp must reject the
// pattern where Go's regexp does, and otherwise match each rune alone
// exactly where Go's matches.
//
// The cases from seed 1 are the committed corpus
// testdata/corpus/uniclassfuzz/uniclassfuzz_test.xo, which TestStdCorpora
// runs on every corpus backend; TestUniclassFuzzCorpusCurrent checks it is
// what this generator writes (XO_UNICLASSFUZZ_WRITE=1 rewrites it).
// TestUniclassFuzz runs a fresh seed (XO_UNICLASSFUZZ_SEED, default 2;
// XO_UNICLASSFUZZ_N cases, default 2000) on XO_UNICLASSFUZZ_BACKENDS
// (default arm64 and llvm).
package stdtest

import (
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"unicode"
)

// ucNames is the names class_of knows, from std/uniclass.xo.
var ucNames = sync.OnceValues(func() ([]string, error) {
	b, err := os.ReadFile(filepath.Join("..", "..", "std", "uniclass.xo"))
	if err != nil {
		return nil, err
	}
	var names []string
	for _, m := range regexp.MustCompile(`(?m)^    "([A-Za-z0-9_]+)" => \(`).FindAllStringSubmatch(string(b), -1) {
		names = append(names, m[1])
	}
	if len(names) < 190 {
		return nil, fmt.Errorf("std/uniclass.xo: %d class names read, it has over 190", len(names))
	}
	return names, nil
})

type ucCase struct {
	pattern string
	runes   []rune
	want    string // "err", or one 0 or 1 a rune
}

func ucSpell(r *rand.Rand, n string) string {
	switch r.Intn(9) {
	case 0:
		return strings.ToLower(n)
	case 1:
		return strings.ToUpper(n)
	case 2:
		i := r.Intn(len(n) + 1)
		return n[:i] + []string{"_", "-", " "}[r.Intn(3)] + n[i:]
	case 3:
		if len(n) > 1 {
			return n[:1+r.Intn(len(n)-1)]
		}
	case 4:
		return n + string("aZsx"[r.Intn(4)])
	case 5:
		var b strings.Builder
		for k := 1 + r.Intn(6); k > 0; k-- {
			b.WriteByte("aeiouLNPSZcklmnrst"[r.Intn(18)])
		}
		return b.String()
	}
	return n
}

// ucTable is Go's unicode table of a name, or nil.
func ucTable(n string) *unicode.RangeTable {
	for _, m := range []map[string]*unicode.RangeTable{unicode.Categories, unicode.Scripts, unicode.Properties} {
		if t, ok := m[n]; ok {
			return t
		}
	}
	return nil
}

func ucRunes(r *rand.Rand, t *unicode.RangeTable) []rune {
	var ends []rune
	if t != nil {
		for _, x := range t.R16 {
			ends = append(ends, rune(x.Lo), rune(x.Hi))
		}
		for _, x := range t.R32 {
			ends = append(ends, rune(x.Lo), rune(x.Hi))
		}
	}
	rs := make([]rune, 0, 24)
	for len(rs) < 24 {
		var c rune
		switch k := r.Intn(8); {
		case k < 3 && len(ends) > 0:
			c = ends[r.Intn(len(ends))] + rune(r.Intn(3)) - 1
		case k == 3:
			c = rune(r.Intn(128))
		case k == 4:
			c = rune(128 + r.Intn(128))
		case k == 5:
			c = rune(r.Intn(0x10000))
		case k == 6:
			c = []rune{0x10ffff, 0xe0001, 0x1f600, 0xfffd, 0x3000, 0x2028, 0x10000}[r.Intn(7)]
		default:
			c = rune(0x10000 + r.Intn(0x100000))
		}
		if c < 0 || c > 0x10ffff || (c >= 0xd800 && c <= 0xdfff) {
			continue
		}
		rs = append(rs, c)
	}
	return rs
}

func ucCaseOf(pattern string, rs []rune) ucCase {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return ucCase{pattern, rs, "err"}
	}
	var b strings.Builder
	for _, c := range rs {
		if re.MatchString(string(c)) {
			b.WriteByte('1')
		} else {
			b.WriteByte('0')
		}
	}
	return ucCase{pattern, rs, b.String()}
}

func ucCases(t *testing.T, r *rand.Rand, n int) []ucCase {
	names, err := ucNames()
	if err != nil {
		t.Fatal(err)
	}
	cs := make([]ucCase, 0, n)
	for len(cs) < n {
		base := names[r.Intn(len(names))]
		name := base
		if r.Intn(3) == 0 {
			name = ucSpell(r, base)
		}
		var p string
		switch r.Intn(7) {
		case 0:
			p = `^\P{` + name + `}$`
		case 1:
			p = `(?i)^\p{` + name + `}$`
		case 2:
			p = `^[^\p{` + name + `}]$`
		case 3:
			p = `^[\p{` + name + `}\p{Greek}]$`
		case 4:
			if len(name) == 1 {
				p = `^\p` + name + `$`
				break
			}
			fallthrough
		default:
			p = `^\p{` + name + `}$`
		}
		cs = append(cs, ucCaseOf(p, ucRunes(r, ucTable(base))))
	}
	return cs
}

const ucCheckXo = `use std/regexp

fn answer(pattern: Str, runes: Str) -> Str {
  match regexp.compile(pattern) {
    Ok(re) => {
      var b = ""
      for r in runes.runes() {
        b = b + (if re.is_match("${r}") { "1" } else { "0" })
      }
      b
    }
    Err(_) => "err"
  }
}

test "std/regexp's Unicode classes (std/uniclass) match as Go's regexp" {
  var bad: List[(Str, Str, Str, Str)] = []
  for (pattern, runes, want) in CASES {
    let got = answer(pattern, runes)
    if got != want {
      bad.push((pattern, runes, got, want))
    }
  }
  expect(bad[0..(if bad.len() < 20 { bad.len() } else { 20 })]) == []
}
`

func ucXoFile(seed int64, cases []ucCase) string {
	var b strings.Builder
	fmt.Fprintf(&b, "// Generated by internal/stdtest/uniclassfuzz_test.go (seed %d, %d cases) from Go's\n", seed, len(cases))
	b.WriteString("// regexp and unicode; do not edit: XO_UNICLASSFUZZ_WRITE=1 go test ./internal/stdtest -run TestUniclassFuzzCorpusCurrent\n")
	b.WriteString("// rewrites it.\n")
	b.WriteString(ucCheckXo)
	b.WriteString("\n/// (pattern, the runes, Go's match of each rune alone as 0 or 1, or err)\nconst CASES: List[(Str, Str, Str)] = [\n")
	for _, c := range cases {
		fmt.Fprintf(&b, "  (%s, %s, %q),\n", xoStr(c.pattern), ucRuneStr(c.runes), c.want)
	}
	b.WriteString("]\n")
	return b.String()
}

// ucRuneStr is the runes as an Xo string literal, every rune outside
// printable ASCII as \u{...}.
func ucRuneStr(rs []rune) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, c := range rs {
		if c >= 0x20 && c < 0x7f && c != '"' && c != '\\' && c != '$' {
			b.WriteRune(c)
		} else {
			fmt.Fprintf(&b, `\u{%x}`, c)
		}
	}
	b.WriteByte('"')
	return b.String()
}

// ucCoverage fails when the cases stop reaching rejected names, and runes
// in and out of their classes.
func ucCoverage(t *testing.T, cases []ucCase) {
	t.Helper()
	errs, ones, zeros := 0, 0, 0
	for _, c := range cases {
		if c.want == "err" {
			errs++
			continue
		}
		ones += strings.Count(c.want, "1")
		zeros += strings.Count(c.want, "0")
	}
	n := len(cases)
	if errs < n/50 || errs > n/3 || ones < n*3 || zeros < n*3 {
		t.Fatalf("coverage fell: %d rejected patterns of %d, %d runes matched, %d not", errs, n, ones, zeros)
	}
}

const ucCorpus = "testdata/corpus/uniclassfuzz/uniclassfuzz_test.xo"

// TestUniclassFuzzCorpusCurrent: the committed corpus is what seed 1 writes.
func TestUniclassFuzzCorpusCurrent(t *testing.T) {
	cases := ucCases(t, rand.New(rand.NewSource(1)), 400)
	ucCoverage(t, cases)
	bxCorpusCurrent(t, ucCorpus, "XO_UNICLASSFUZZ_WRITE", ucXoFile(1, cases))
}

// TestUniclassFuzz runs a fresh seed's cases.
func TestUniclassFuzz(t *testing.T) {
	if testing.Short() {
		t.Skip("the uniclass fuzz is a heavy test; its seed 1 corpus runs in TestStdCorpora")
	}
	seed := int64(envInt(t, "XO_UNICLASSFUZZ_SEED", 2))
	n := envInt(t, "XO_UNICLASSFUZZ_N", 2000)
	cases := ucCases(t, rand.New(rand.NewSource(seed)), n)
	runFuzzDir(t, "uniclassfuzz_test.xo", ucXoFile(seed, cases), envBackends("XO_UNICLASSFUZZ_BACKENDS", []string{"arm64", "llvm"}), len(cases))
}
