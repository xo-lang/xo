package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"iter"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
	"slices"
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
	o.WriteString(header("Go's math.IsInf and strconv.Unquote (quote_test.go's unquotetests and misquoted), std.md 2.5's Option.or_err and Option.and_then, std.md 2.6's Duration.ceil_seconds, slices.Collect over ranges for Range.to_list, sort.Stable for List.sort_by's stability, and List.sort of Strs (Go's sort.Strings order, a seven byte prefix table included) on Go's tables and seeded texts, Float bit patterns, nanosecond counts, key shapes and range bounds: the prelude methods no other corpus reaches (Go's strconv tests are Copyright 2009 The Go Authors, BSD-style license)"))

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

	// The seven byte prefix path: rows where every key shares its first
	// seven bytes and differs after them, keys shorter than seven bytes
	// (their prefix is the whole key), and keys where one is the prefix of
	// another across the seven byte edge ("abcdefg" and "abcdefgh"). A sort
	// that orders by the prefix alone passes the shared prefix rows only by
	// accident, so the rows make the second comparison load bearing.
	var p7 [][]string
	rp7 := rand.New(rand.NewSource(113))
	var shared []string
	for i := 0; i < 40; i++ {
		shared = append(shared, fmt.Sprintf("abcdefg%03d", rp7.Intn(1000)))
	}
	p7 = append(p7, shared)
	p7 = append(p7, []string{"abcdefg", "abcdefgh", "abcdefghi", "abcdef", "abcde"})
	p7 = append(p7, []string{"abcdefg2", "abcdefg10", "abcdefg1", "abcdefg02", "abcdefg"})
	p7 = append(p7, []string{"", "a", "ab", "abc", "abcd", "abcde", "abcdef", "abcdefg"})
	p7 = append(p7, []string{"abcdefh", "abcdefg", "abcdef", "abcdefga", "abcdefg\u007f"})
	o.WriteString("\n/// (the keys joined by a unit separator, Go's sort.Strings order, the shape)\nconst SORT_PREFIX7_CASES: List[(Str, Str, Str)] = [\n")
	for _, keys := range p7 {
		want := append([]string(nil), keys...)
		sort.Strings(want)
		fmt.Fprintf(&o, "  (%q, %q, %q),\n", strings.Join(keys, "\x1f"), strings.Join(want, "\x1f"), sortShape(keys))
	}
	o.WriteString("]\n")

	// List.sort_by's stability against Go's sort.Stable: rows of keys with
	// duplicates, where the order of equal keys is what a stable sort
	// promises and an unstable one may change. The oracle is Go's
	// sort.SliceStable over the row's indices, so the expected text is the
	// tags in the order a stable sort leaves them.
	type stableRow struct{ keys, want, shape string }
	var stableRows []stableRow
	stableSeeds := [][]string{
		{"b", "a", "b", "a", "b"},
		{"k", "k", "k"},
		{"z", "a", "z", "b", "a", "z"},
		{"same", "same", "diff", "same", "diff"},
	}
	for _, keys := range stableSeeds {
		order := make([]int, len(keys))
		for i := range order {
			order[i] = i
		}
		sort.SliceStable(order, func(i, j int) bool { return keys[order[i]] < keys[order[j]] })
		var tags []string
		for _, oi := range order {
			tags = append(tags, fmt.Sprint(oi))
		}
		stableRows = append(stableRows, stableRow{strings.Join(keys, "\x1f"), strings.Join(tags, ","), sortShape(keys)})
	}
	rs2 := rand.New(rand.NewSource(109))
	for k := 0; k < 6; k++ {
		n := 20 + rs2.Intn(40)
		var keys []string
		for i := 0; i < n; i++ {
			keys = append(keys, fmt.Sprintf("k%d", rs2.Intn(5)))
		}
		order := make([]int, len(keys))
		for i := range order {
			order[i] = i
		}
		sort.SliceStable(order, func(i, j int) bool { return keys[order[i]] < keys[order[j]] })
		var tags []string
		for _, oi := range order {
			tags = append(tags, fmt.Sprint(oi))
		}
		stableRows = append(stableRows, stableRow{strings.Join(keys, "\x1f"), strings.Join(tags, ","), sortShape(keys)})
	}
	o.WriteString("\n/// (the keys joined by a unit separator, the tags in the order Go's sort.SliceStable leaves them, the shape)\nconst SORT_STABLE_CASES: List[(Str, Str, Str)] = [\n")
	for _, r := range stableRows {
		fmt.Fprintf(&o, "  (%q, %q, %q),\n", r.keys, r.want, r.shape)
	}
	o.WriteString("]\n")

	// Range.to_list: Go's slices.Collect over the same range. An exclusive
	// range stops before the upper bound, an inclusive one includes it, an
	// upper bound at or below the lower one is empty, and the bounds may be
	// negative or far apart.
	type rangeRow struct {
		lo, hi    int64
		inclusive bool
		want      string
		shape     string
	}
	var rangeRows []rangeRow
	addRange := func(lo, hi int64, inclusive bool, shape string) {
		var seq iter.Seq[int64]
		if inclusive {
			seq = func(yield func(int64) bool) {
				for i := lo; i <= hi; i++ {
					if !yield(i) {
						return
					}
					if i == math.MaxInt64 {
						break
					}
				}
			}
		} else {
			seq = func(yield func(int64) bool) {
				for i := lo; i < hi; i++ {
					if !yield(i) {
						return
					}
				}
			}
		}
		var got []string
		for _, v := range slices.Collect(seq) {
			got = append(got, fmt.Sprint(v))
		}
		rangeRows = append(rangeRows, rangeRow{lo, hi, inclusive, strings.Join(got, "\x1f"), shape})
	}
	addRange(5, 5, false, "an empty exclusive range")
	addRange(7, 3, false, "lo above hi, empty")
	addRange(2, 3, false, "one element")
	addRange(0, 5, false, "five elements from zero")
	addRange(-3, 2, false, "crossing zero")
	addRange(-4, -1, false, "negative bounds")
	addRange(0, 3, true, "an inclusive range")
	addRange(5, 5, true, "an inclusive range of one")
	addRange(-2, -2, true, "an inclusive negative range of one")
	addRange(-500, 500, false, "a thousand elements")
	addRange(1000000, 1000100, false, "a large start")
	addRange(math.MaxInt64-3, math.MaxInt64, false, "the top of Int")
	o.WriteString("\n/// (lo, hi, inclusive, the elements joined by a unit separator as Go's slices.Collect gives them, the shape)\nconst RANGE_CASES: List[(Int, Int, Bool, Str, Str)] = [\n")
	for _, r := range rangeRows {
		fmt.Fprintf(&o, "  (%d, %d, %v, %q, %q),\n", r.lo, r.hi, r.inclusive, r.want, r.shape)
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

test "List.sort_in_place orders every shape Go's sort.Strings orders" {
  var bad: List[(Str, Str, Str)] = []
  for (keys, want, shape) in SORT_STR_CASES {
    var xs = split_keys(keys)
    xs.sort_in_place()
    let got = xs.join("\u{1f}")
    if got != want {
      bad.push((shape, got, want))
    }
  }
  expect(show3(bad)) == []
}

test "List.sort_in_place leaves a shared list alone and sorts its own binding" {
  var a = ["b", "c", "a"]
  let b = a
  a.sort_in_place()
  expect(a.join(",")) == "a,b,c"
  expect(b.join(",")) == "b,c,a"
  var unique = ["d", "b"]
  unique.sort_in_place()
  expect(unique.join(",")) == "b,d"
}

test "List.sort_in_place_by keeps equal keys in input order" {
  var xs = [("b", 1), ("a", 2), ("b", 3), ("a", 4), ("b", 5)]
  xs.sort_in_place_by(fn(p) { p.0 })
  expect(xs.map(fn(p) { "${p.0}${p.1}" }).join(",")) == "a2,a4,b1,b3,b5"
  var same = [("k", 3), ("k", 1), ("k", 2)]
  same.sort_in_place_by(fn(p) { p.0 })
  expect(same.map(fn(p) { "${p.1}" }).join(",")) == "3,1,2"
}

test "List.sort_in_place_by orders every shape Go's sort.Strings orders" {
  var bad: List[(Str, Str, Str)] = []
  for (keys, want, shape) in SORT_STR_CASES {
    var xs = split_keys(keys)
    xs.sort_in_place_by(fn(k) { k })
    let got = xs.join("\u{1f}")
    if got != want {
      bad.push((shape, got, want))
    }
  }
  expect(show3(bad)) == []
}

test "List.sort orders keys that share a seven byte prefix" {
  var bad: List[(Str, Str, Str)] = []
  for (keys, want, shape) in SORT_PREFIX7_CASES {
    let got = split_keys(keys).sort().join("\u{1f}")
    if got != want {
      bad.push((shape, got, want))
    }
  }
  expect(show3(bad)) == []
}

test "List.sort_by keeps equal keys in Go's sort.Stable order" {
  var bad: List[(Str, Str, Str)] = []
  for (keys, want, shape) in SORT_STABLE_CASES {
    let ks = split_keys(keys)
    var pairs: List[(Str, Int)] = []
    for i in 0..ks.len() {
      pairs.push((ks[i], i))
    }
    let got = pairs.sort_by(fn(p) { p.0 }).map(fn(p) { "${p.1}" }).join(",")
    if got != want {
      bad.push((shape, got, want))
    }
  }
  expect(show3(bad)) == []
}

/// The first 80 bytes of a range's text, since a thousand elements are not
/// a readable failure.
fn head80(s: Str) -> Str {
  s[0..(if s.len() < 80 { s.len() } else { 80 })]
}

test "Range.to_list is Go's slices.Collect over the range" {
  var bad: List[(Str, Str, Str)] = []
  for (lo, hi, inclusive, want, shape) in RANGE_CASES {
    let got = if inclusive { (lo..=hi).to_list() } else { (lo..hi).to_list() }
    let text = got.map(fn(n) { "${n}" }).join("\u{1f}")
    if text != want {
      bad.push((shape, head80(text), head80(want)))
    }
  }
  expect(show3(bad)) == []
}
`)
	return o.String()
}
