package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

// The API this corpus covers: the prelude methods no other corpus reaches.
// Float.is_inf (Go's math.IsInf), Str.unquote (Go's strconv.Unquote, with
// decision 0287's None when the result is not UTF-8), Option.or_err and
// Option.and_then (the rules std.md 2.5 states), and Duration.ceil_seconds
// (the whole seconds, one more when a positive remainder is left).
// Cmd.inherit is the one left out: it hands the child the program's own
// standard streams, which a corpus cannot observe without a terminal.
// BigInt.cmp_abs lives in the big corpus, next to cmp.
//
// The inputs are Go's: quote_test.go's unquotetests and misquoted, plus
// seeded texts, Float bit patterns and nanosecond counts. The expectations
// are Go's for is_inf and unquote; the Option and Duration rows carry the
// rule they follow in a label, since Go has no twin of either. Go's strconv
// tests are Copyright 2009 The Go Authors, BSD-style license.

// strconvQuoteTable returns the rows of a table in Go's strconv/quote_test.go:
// each row is the strings its composite literal holds, or the one string of
// a plain element.
func strconvQuoteTable(name string) [][]string {
	path := filepath.Join(runtime.GOROOT(), "src", "strconv", "quote_test.go")
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		fmt.Fprintln(os.Stderr, "stdcorpus: reading Go's strconv quote tests:", err)
		os.Exit(1)
	}
	for _, d := range f.Decls {
		g, ok := d.(*ast.GenDecl)
		if !ok || g.Tok != token.VAR {
			continue
		}
		for _, s := range g.Specs {
			vs, ok := s.(*ast.ValueSpec)
			if !ok || len(vs.Names) != 1 || vs.Names[0].Name != name || len(vs.Values) != 1 {
				continue
			}
			lit, ok := vs.Values[0].(*ast.CompositeLit)
			if !ok {
				continue
			}
			var out [][]string
			for _, e := range lit.Elts {
				if c, ok := e.(*ast.CompositeLit); ok {
					var row []string
					for _, cell := range c.Elts {
						if v, ok := strconvString(cell); ok {
							row = append(row, v)
						}
					}
					out = append(out, row)
					continue
				}
				if v, ok := strconvString(e); ok {
					out = append(out, []string{v})
				}
			}
			return out
		}
	}
	fmt.Fprintf(os.Stderr, "stdcorpus: Go's quote_test.go has no table %s\n", name)
	os.Exit(1)
	return nil
}

// sortShape names a List.sort row: its length, and whether it is all one
// key, in nondecreasing order, or in decreasing order.
func sortShape(keys []string) string {
	if len(keys) == 0 {
		return "no keys"
	}
	same, up, down := true, true, true
	for i := 1; i < len(keys); i++ {
		if keys[i] != keys[0] {
			same = false
		}
		if keys[i] < keys[i-1] {
			up = false
		}
		if keys[i] > keys[i-1] {
			down = false
		}
	}
	switch {
	case same && len(keys) > 1:
		return fmt.Sprintf("%d equal keys", len(keys))
	case up:
		return fmt.Sprintf("%d keys in order", len(keys))
	case down:
		return fmt.Sprintf("%d keys reversed", len(keys))
	}
	return fmt.Sprintf("%d keys", len(keys))
}

func preludeCorpus() string {
	var o strings.Builder
	o.WriteString(header("Go's math.IsInf and strconv.Unquote (quote_test.go's unquotetests and misquoted), std.md 2.5's Option.or_err and Option.and_then, std.md 2.6's Duration.ceil_seconds, and List.sort of Strs (Go's sort.Strings order) on Go's tables and seeded texts, Float bit patterns, nanosecond counts and key shapes: the prelude methods no other corpus reaches (Go's strconv tests are Copyright 2009 The Go Authors, BSD-style license)"))

	// Float.is_inf: Go's math.IsInf(f, 0). Both infinities are true, a NaN is
	// not, and no finite Float is, the largest and the subnormals included.
	bits := []uint64{
		0x7FF0000000000000, 0xFFF0000000000000, // +Inf, -Inf
		0x7FF8000000000000, 0xFFF8000000000000, // quiet NaNs
		0x7FF0000000000001, 0xFFF0000000000001, // the smallest NaN payloads
		0x7FEFFFFFFFFFFFFF, 0xFFEFFFFFFFFFFFFF, // +-MaxFloat64
		0x0000000000000001, 0x8000000000000001, // the smallest +-subnormals
		0x0000000000000000, 0x8000000000000000, // +-0
		0x3FF0000000000000, 0xBFF0000000000000, // +-1
		0x7FE0000000000000, // 1e308
	}
	rf := rand.New(rand.NewSource(97))
	for i := 0; i < 60; i++ {
		bits = append(bits, rf.Uint64())
	}
	const infLab = "std.md 2.1 Float.is_inf (Go's math.IsInf(f, 0))"
	o.WriteString("\n/// (bits, Go's math.IsInf of that Float, the rule)\nconst IS_INF_CASES: List[(Int, Bool, Str)] = [\n")
	for _, b := range bits {
		f := math.Float64frombits(b)
		fmt.Fprintf(&o, "  (%d, %v, %q),\n", int64(b), math.IsInf(f, 0), infLab)
	}
	o.WriteString("]\n")

	// Str.unquote: Go's Unquote, then None when the result is not UTF-8
	// (decision 0287). Go's own tables give the accepted and the refused
	// texts; seeded texts mix escapes, quotes and stray characters.
	type uc struct {
		text, want, label string
		ok                bool
	}
	var ucs []uc
	for _, row := range strconvQuoteTable("unquotetests") {
		if len(row) != 2 {
			fmt.Fprintf(os.Stderr, "stdcorpus: an unquotetests row is not a pair: %q\n", row)
			os.Exit(1)
		}
		v, err := strconv.Unquote(row[0])
		if err != nil {
			fmt.Fprintf(os.Stderr, "stdcorpus: Go refuses its own unquotetests row %q\n", row[0])
			os.Exit(1)
		}
		lab := "std.md 2.1 Str.unquote (Go's strconv.Unquote)"
		if !utf8.ValidString(v) {
			lab = "decision 0287: None when the result is not UTF-8; Go's Unquote returns the bytes"
			ucs = append(ucs, uc{row[0], "", lab, false})
			continue
		}
		ucs = append(ucs, uc{row[0], v, lab, true})
	}
	for _, row := range strconvQuoteTable("misquoted") {
		if len(row) != 1 {
			continue
		}
		ucs = append(ucs, uc{row[0], "", "Go's quote_test.go misquoted list: Go refuses it, and so does Str.unquote", false})
	}
	ru := rand.New(rand.NewSource(101))
	alpha := []string{`"`, `'`, "`", `\`, "a", "0", "9", "x", "u", "n", "\n", " ", "\t", "☺", "\xff"}
	for i := 0; i < 60; i++ {
		var b strings.Builder
		for j := 0; j < ru.Intn(8); j++ {
			b.WriteString(alpha[ru.Intn(len(alpha))])
		}
		in := b.String()
		v, err := strconv.Unquote(in)
		if err != nil || !utf8.ValidString(v) {
			ucs = append(ucs, uc{in, "", "a seeded text: None, as Go's Unquote refuses it or its result is not UTF-8 (decision 0287)", false})
			continue
		}
		ucs = append(ucs, uc{in, v, "a seeded text: Go's strconv.Unquote", true})
	}
	o.WriteString("\n/// (text, whether Str.unquote answers a Str, the Str, the rule)\nconst UNQUOTE_CASES: List[(Str, Bool, Str, Str)] = [\n")
	for _, u := range ucs {
		fmt.Fprintf(&o, "  (%s, %v, %s, %q),\n", xq(u.text), u.ok, xq(u.want), u.label)
	}
	o.WriteString("]\n")

	// Duration.ceil_seconds: the whole seconds, and one more when the
	// remainder is positive; a negative remainder is left as it is (the
	// rule of std/native/prelude.xo duration_ceil_seconds, which truncates
	// toward zero and rounds up only for a positive remainder).
	nanos := []int64{0, 1, -1, 999999999, -999999999, 1000000000, -1000000000,
		1000000001, -1000000001, 1500000000, -1500000000, 1999999999, -1999999999,
		2000000000, -2000000000, 3600000000000, -3600000000000,
		math.MaxInt64, math.MinInt64}
	rn := rand.New(rand.NewSource(103))
	for i := 0; i < 120; i++ {
		nanos = append(nanos, rn.Int63()-rn.Int63())
	}
	const ceilLab = "std.md 2.6 Duration.ceil_seconds: the whole seconds, one more only when the remainder is positive"
	o.WriteString("\n/// (nanoseconds, the seconds rounded the way Duration.ceil_seconds rounds, the rule)\nconst CEIL_CASES: List[(Int, Int, Str)] = [\n")
	for _, n := range nanos {
		q := n / 1000000000
		if n%1000000000 > 0 {
			q++
		}
		fmt.Fprintf(&o, "  (%d, %d, %q),\n", n, q, ceilLab)
	}
	o.WriteString("]\n")

	// List.sort of Strs: the shapes a merge based sort has to get right,
	// the lengths around a half list (0, 1, 2, 12, 13 and 50), all equal,
	// already sorted and reversed, keys that share a prefix (a sort may
	// compare a seven byte prefix first and must still order the whole
	// string), and keys where one is a prefix of another. Go's sort.Strings
	// gives the order; it sorts unstably, which for equal strings is the
	// same output, so the row is an exact oracle, and the stability the spec
	// promises is not observable on Strs (two equal Strs are one text).
	rs := rand.New(rand.NewSource(107))
	var sortRows [][]string
	sortRows = append(sortRows, nil)
	sortRows = append(sortRows, []string{"b"})
	sortRows = append(sortRows, []string{"b", "a"})
	sortRows = append(sortRows, []string{"a", "b"})
	sortRows = append(sortRows, []string{"", "a", "ab", "b", "aa"})
	sortRows = append(sortRows, []string{"abcdefgh2", "abcdefgh1", "abcdefgh10", "abcdefgh0", "abcdefghi", "abcdefg"})
	for _, n := range []int{12, 13, 50} {
		var ks []string
		for i := 0; i < n; i++ {
			ks = append(ks, fmt.Sprintf("k%02d", rs.Intn(60)))
		}
		sortRows = append(sortRows, ks)
		srt := append([]string(nil), ks...)
		sort.Strings(srt)
		sortRows = append(sortRows, srt)
		var rev []string
		for i := len(srt) - 1; i >= 0; i-- {
			rev = append(rev, srt[i])
		}
		sortRows = append(sortRows, rev)
	}
	eq := make([]string, 13)
	for i := range eq {
		eq[i] = "same"
	}
	sortRows = append(sortRows, eq)
	o.WriteString("\n/// (the keys joined by a unit separator, the keys in Go's sort.Strings order, the shape)\nconst SORT_STR_CASES: List[(Str, Str, Str)] = [\n")
	for _, keys := range sortRows {
		want := append([]string(nil), keys...)
		sort.Strings(want)
		fmt.Fprintf(&o, "  (%q, %q, %q),\n", strings.Join(keys, "\x1f"), strings.Join(want, "\x1f"), sortShape(keys))
	}
	o.WriteString("]\n")

	o.WriteString(`
fn show3(bad: List[(Str, Str, Str)]) -> List[(Str, Str, Str)] {
  bad[0..(if bad.len() < 20 { bad.len() } else { 20 })]
}

fn opt_text(o: Int?) -> Str {
  match o {
    Some(n) => "some ${n}"
    None => "none"
  }
}

fn res_text(r: Result[Int, Str]) -> Str {
  match r {
    Ok(n) => "ok ${n}"
    Err(e) => "err ${e}"
  }
}

fn inc(n: Int) -> Int? { n.checked_add(1) }
fn nothing(_n: Int) -> Int? { None }

/// The unit separated keys of a SORT_STR_CASES row.
fn split_keys(s: Str) -> List[Str] {
  if s == "" {
    []
  } else {
    s.split("\u{1f}")
  }
}

test "Float.is_inf is Go's math.IsInf" {
  var bad: List[(Str, Str, Str)] = []
  for (bits, want, _) in IS_INF_CASES {
    let got = "${Float.from_bits(bits).is_inf()}"
    if got != "${want}" {
      bad.push(("bits ${bits}", got, "${want}"))
    }
  }
  expect(show3(bad)) == []
}

test "Str.unquote is Go's Unquote, and None when the result is not UTF-8" {
  var bad: List[(Str, Str, Str)] = []
  for (text, ok, want, _) in UNQUOTE_CASES {
    let got = match text.unquote() {
      Some(s) => "some ${s}"
      None => "none"
    }
    let want_text = if ok { "some ${want}" } else { "none" }
    if got != want_text {
      bad.push((text, got, want_text))
    }
  }
  expect(show3(bad)) == []
}

test "Option.or_err is Ok for Some and the error for None" {
  let some: Int? = Some(3)
  let none: Int? = None
  expect(res_text(some.or_err("e"))) == "ok 3"
  expect(res_text(none.or_err("e"))) == "err e"
  expect(res_text(Some(-7).or_err("e"))) == "ok -7"
}

test "Option.and_then runs its function on Some only" {
  let some: Int? = Some(3)
  let none: Int? = None
  expect(opt_text(some.and_then(inc))) == "some 4"
  expect(opt_text(none.and_then(inc))) == "none"
  expect(opt_text(some.and_then(nothing))) == "none"
  expect(opt_text(Some(9223372036854775807).and_then(inc))) == "none"
}

test "Duration.ceil_seconds rounds up only a positive remainder" {
  var bad: List[(Str, Str, Str)] = []
  for (nanos, want, _) in CEIL_CASES {
    let got = "${Duration.from_nanos(nanos).ceil_seconds()}"
    if got != "${want}" {
      bad.push(("nanos ${nanos}", got, "${want}"))
    }
  }
  expect(show3(bad)) == []
}

test "List.sort of Strs orders every shape Go's sort.Strings orders" {
  var bad: List[(Str, Str, Str)] = []
  for (keys, want, shape) in SORT_STR_CASES {
    let got = split_keys(keys).sort().join("\u{1f}")
    if got != want {
      bad.push((shape, got, want))
    }
  }
  expect(show3(bad)) == []
}

test "List.sort leaves the receiver in its order and returns a new list" {
  let xs = ["b", "a", "c"]
  let s = xs.sort()
  expect(xs.join(",")) == "b,a,c"
  expect(s.join(",")) == "a,b,c"
  let one = ["a", "a", "b", "a"]
  let one_s = one.sort()
  expect(one.join(",")) == "a,a,b,a"
  expect(one_s.join(",")) == "a,a,a,b"
}

test "List.sort_by keeps equal keys in input order" {
  let xs = [("b", 1), ("a", 2), ("b", 3), ("a", 4), ("b", 5)]
  let got = xs.sort_by(fn(p) { p.0 })
  expect(got.map(fn(p) { "${p.0}${p.1}" }).join(",")) == "a2,a4,b1,b3,b5"
  let same = [("k", 3), ("k", 1), ("k", 2)]
  expect(same.sort_by(fn(p) { p.0 }).map(fn(p) { "${p.1}" }).join(",")) == "3,1,2"
}
`)
	return o.String()
}
