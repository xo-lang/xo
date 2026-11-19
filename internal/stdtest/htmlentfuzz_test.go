// A fuzz harness for std/htmlent, the entity tables and lookups std/html
// and std/htmltok share, against Go's html package (decision 0616's fuzz
// kind). The names come from std/htmlent.xo's own ENTITY and ENTITY2
// tables (Go's are not exported); every value comes from Go:
//
//   - a name (a table name, or one changed: cut short, a letter's case
//     flipped, a byte added, its ; dropped or added, or random letters),
//     and entity_of and entity2_of of it, which must give Go's
//     UnescapeString("&" + name) for a name with ; that Go replaces whole,
//     and "" for a name not in the tables;
//   - text full of references (names as above, the legacy names without ;
//     followed by letters, digits or =, numeric references in decimal and
//     hex with leading zeros, past 0x10FFFF, surrogates, 0x80 to 0x9F and
//     0), which std/html's unescape_string, built on these lookups, must
//     unescape as Go's UnescapeString does, and escape_string as Go's
//     EscapeString.
//
// The cases from seed 1 are the committed corpus
// testdata/corpus/htmlentfuzz/htmlentfuzz_test.xo, which TestStdCorpora
// runs on every corpus backend; TestHtmlentFuzzCorpusCurrent checks it is
// what this generator writes (XO_HTMLENTFUZZ_WRITE=1 rewrites it).
// TestHtmlentFuzz runs a fresh seed (XO_HTMLENTFUZZ_SEED, default 2;
// XO_HTMLENTFUZZ_N cases, default 2000) on XO_HTMLENTFUZZ_BACKENDS (default
// arm64 and llvm).
package stdtest

import (
	"fmt"
	"html"
	"math/rand"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/xo-lang/xo/internal/hostile"
)

// heNames is the names of std/htmlent.xo's ENTITY and ENTITY2 tables.
var heNames = sync.OnceValues(func() ([]string, error) {
	b, err := os.ReadFile(filepath.Join("..", "..", "std", "htmlent.xo"))
	if err != nil {
		return nil, err
	}
	var names []string
	re := regexp.MustCompile(`(?m)^  \("([A-Za-z0-9;]+)", "`)
	for _, m := range re.FindAllStringSubmatch(string(b), -1) {
		names = append(names, m[1])
	}
	if len(names) < 2000 {
		return nil, fmt.Errorf("std/htmlent.xo: %d names read, the tables have over 2,000", len(names))
	}
	return names, nil
})

type heName struct {
	name, ent string // ent: what entity_of or entity2_of must give, "" for none
}

type heText struct {
	s, unesc, esc string
}

func heMutate(r *rand.Rand, n string) string {
	switch r.Intn(7) {
	case 0:
		if len(n) > 1 {
			return n[:1+r.Intn(len(n)-1)]
		}
	case 1:
		i := r.Intn(len(n))
		c := n[i]
		switch {
		case c >= 'a' && c <= 'z':
			c -= 32
		case c >= 'A' && c <= 'Z':
			c += 32
		}
		return n[:i] + string(c) + n[i+1:]
	case 2:
		i := r.Intn(len(n) + 1)
		return n[:i] + string("aZ9;x"[r.Intn(5)]) + n[i:]
	case 3:
		if strings.HasSuffix(n, ";") {
			return strings.TrimSuffix(n, ";")
		}
		return n + ";"
	case 4:
		var b strings.Builder
		for k := 1 + r.Intn(8); k > 0; k-- {
			b.WriteByte("abcdefghijklmnopqrstuvwxyzAELMNQ"[r.Intn(32)])
		}
		if r.Intn(2) == 0 {
			b.WriteByte(';')
		}
		return b.String()
	}
	return n
}

func heRef(r *rand.Rand, names []string) string {
	switch r.Intn(6) {
	case 0, 1:
		return "&" + heMutate(r, names[r.Intn(len(names))])
	case 2:
		n := strings.TrimSuffix(names[r.Intn(len(names))], ";")
		return "&" + n + []string{"", "x", "9", "=", ";", " "}[r.Intn(6)]
	case 3:
		v := []int{0, 9, 0x80, 0x85, 0x9f, 0xd800, 0xdfff, 0x10ffff, 0x110000, 0xfffffff, 65, 0x1f600}[r.Intn(12)]
		if r.Intn(3) == 0 {
			v = r.Intn(0x120000)
		}
		z := strings.Repeat("0", r.Intn(3))
		semi := []string{";", "", "x"}[r.Intn(3)]
		if r.Intn(2) == 0 {
			return fmt.Sprintf("&#%s%d%s", z, v, semi)
		}
		return fmt.Sprintf("&#%s%s%x%s", []string{"x", "X"}[r.Intn(2)], z, v, semi)
	case 4:
		// the hostile dictionary (task #352): decimal and hex references
		// past Int and at its edges, and text cut inside a rune
		switch r.Intn(4) {
		case 0:
			return "&#" + hostile.Pick(r, hostile.Digits()) + []string{";", ""}[r.Intn(2)]
		case 1:
			return "&#x" + hostile.Pick(r, hostile.Hex()) + []string{";", ""}[r.Intn(2)]
		}
		return []string{"&", "&#", "&#x", "&#;", "&;", "&&amp", "&#x;", "&#99999999999999999999;"}[r.Intn(8)]
	}
	if r.Intn(20) == 0 {
		return hostile.Pick(r, hostile.CutUTF8("é日😀"))
	}
	return []string{"a", " ", "<", ">", "\"", "'", "日本", "é", "\t"}[r.Intn(9)]
}

func heCases(t *testing.T, r *rand.Rand, n int) ([]heName, []heText) {
	names, err := heNames()
	if err != nil {
		t.Fatal(err)
	}
	in := map[string]bool{}
	for _, x := range names {
		in[x] = true
	}
	var ns []heName
	var ts []heText
	for len(ns)+len(ts) < n {
		if r.Intn(2) == 0 {
			x := names[r.Intn(len(names))]
			if r.Intn(2) == 0 {
				x = heMutate(r, x)
			}
			ent := ""
			if in[x] {
				// a name with ; matches whole; a legacy name without one is
				// the longest legacy prefix, this whole name when nothing follows
				ent = html.UnescapeString("&" + x)
				if ent == "&"+x {
					t.Fatalf("Go does not replace the table name %q", x)
				}
			}
			ns = append(ns, heName{x, ent})
			continue
		}
		var b strings.Builder
		for k := 1 + r.Intn(8); k > 0; k-- {
			b.WriteString(heRef(r, names))
		}
		s := b.String()
		ts = append(ts, heText{s, html.UnescapeString(s), html.EscapeString(s)})
	}
	return ns, ts
}

const heCheckXo = `use std/htmlent
use std/html

fn first_bad[T](bad: List[T]) -> List[T] { bad[0..(if bad.len() < 20 { bad.len() } else { 20 })] }

test "std/htmlent's lookups give Go's html entities" {
  var bad: List[(Str, Str, Str)] = []
  for (name, want) in NAMES {
    let one = htmlent.entity_of(name)
    let got = if one != "" { one } else { htmlent.entity2_of(name) }
    if got != want {
      bad.push((name, got, want))
    }
  }
  expect(first_bad(bad)) == []
}

test "std/html's unescape_string and escape_string over the tables are Go's" {
  var bad: List[(Str, Str, Str)] = []
  for (s, unesc, esc) in TEXTS {
    let u = html.unescape_string(s)
    if u != unesc {
      bad.push((s, u, unesc))
    }
    let e = html.escape_string(s)
    if e != esc {
      bad.push((s, e, esc))
    }
  }
  expect(first_bad(bad)) == []
}
`

func heXoFile(seed int64, ns []heName, ts []heText) string {
	var b strings.Builder
	fmt.Fprintf(&b, "// Generated by internal/stdtest/htmlentfuzz_test.go (seed %d, %d cases) from Go's\n", seed, len(ns)+len(ts))
	b.WriteString("// html; do not edit: XO_HTMLENTFUZZ_WRITE=1 go test ./internal/stdtest -run TestHtmlentFuzzCorpusCurrent\n")
	b.WriteString("// rewrites it.\n")
	b.WriteString(heCheckXo)
	b.WriteString("\n/// (name, Go's replacement of &name, \"\" for a name not in the tables)\nconst NAMES: List[(Str, Str)] = [\n")
	for _, c := range ns {
		fmt.Fprintf(&b, "  (%s, %s),\n", xoStr(c.name), xoStr(c.ent))
	}
	b.WriteString("]\n\n/// (text, Go's UnescapeString, Go's EscapeString)\nconst TEXTS: List[(Str, Str, Str)] = [\n")
	for _, c := range ts {
		fmt.Fprintf(&b, "  (%s, %s, %s),\n", xoStr(c.s), xoStr(c.unesc), xoStr(c.esc))
	}
	b.WriteString("]\n")
	return b.String()
}

// heCoverage fails when the cases stop reaching names in and out of the
// tables, two character entities, and texts Go changes and leaves.
func heCoverage(t *testing.T, ns []heName, ts []heText) {
	t.Helper()
	found, missing, two, changed, same := 0, 0, 0, 0, 0
	for _, c := range ns {
		switch {
		case c.ent == "":
			missing++
		case len([]rune(c.ent)) == 2:
			two++
			found++
		default:
			found++
		}
	}
	for _, c := range ts {
		if c.unesc != c.s {
			changed++
		} else {
			same++
		}
	}
	n := len(ns) + len(ts)
	if found < n/10 || missing < n/10 || two < n/500 || changed < n/10 || same < n/100 {
		t.Fatalf("coverage fell: %d names found, %d not, %d two character, %d texts changed, %d left of %d", found, missing, two, changed, same, n)
	}
}

const heCorpus = "testdata/corpus/htmlentfuzz/htmlentfuzz_test.xo"

// TestHtmlentFuzzCorpusCurrent: the committed corpus is what seed 1 writes.
func TestHtmlentFuzzCorpusCurrent(t *testing.T) {
	ns, ts := heCases(t, rand.New(rand.NewSource(1)), 600)
	heCoverage(t, ns, ts)
	bxCorpusCurrent(t, heCorpus, "XO_HTMLENTFUZZ_WRITE", heXoFile(1, ns, ts))
}

// TestHtmlentFuzz runs a fresh seed's cases.
func TestHtmlentFuzz(t *testing.T) {
	if testing.Short() {
		t.Skip("the htmlent fuzz is a heavy test; its seed 1 corpus runs in TestStdCorpora")
	}
	seed := int64(envInt(t, "XO_HTMLENTFUZZ_SEED", 2))
	n := envInt(t, "XO_HTMLENTFUZZ_N", 2000)
	ns, ts := heCases(t, rand.New(rand.NewSource(seed)), n)
	runFuzzDir(t, "htmlentfuzz_test.xo", heXoFile(seed, ns, ts), envBackends("XO_HTMLENTFUZZ_BACKENDS", []string{"arm64", "llvm"}), len(ns)+len(ts))
}
