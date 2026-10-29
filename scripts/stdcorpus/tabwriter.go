package main

import (
	"bytes"
	"fmt"
	"strings"
	"text/tabwriter"
)

// The std/tabwriter corpus: the table of Go's text/tabwriter tests
// (tabwriter_test.go, `tests`, copied below; Copyright 2009 The Go
// Authors, BSD-style license), each case run three ways as Go's check does
// (all at once, byte by byte, in Fibonacci slices), its expected text
// taken from Go's table and verified here against Go's own Writer.

var tabwriterTests = []struct {
	testname                    string
	minwidth, tabwidth, padding int
	padchar                     byte
	flags                       uint
	src, expected               string
}{
	{
		"1a",
		8, 0, 1, '.', 0,
		"",
		"",
	},

	{
		"1a debug",
		8, 0, 1, '.', tabwriter.Debug,
		"",
		"",
	},

	{
		"1b esc stripped",
		8, 0, 1, '.', tabwriter.StripEscape,
		"\xff\xff",
		"",
	},

	{
		"1b esc",
		8, 0, 1, '.', 0,
		"\xff\xff",
		"\xff\xff",
	},

	{
		"1c esc stripped",
		8, 0, 1, '.', tabwriter.StripEscape,
		"\xff\t\xff",
		"\t",
	},

	{
		"1c esc",
		8, 0, 1, '.', 0,
		"\xff\t\xff",
		"\xff\t\xff",
	},

	{
		"1d esc stripped",
		8, 0, 1, '.', tabwriter.StripEscape,
		"\xff\"foo\t\n\tbar\"\xff",
		"\"foo\t\n\tbar\"",
	},

	{
		"1d esc",
		8, 0, 1, '.', 0,
		"\xff\"foo\t\n\tbar\"\xff",
		"\xff\"foo\t\n\tbar\"\xff",
	},

	{
		"1e esc stripped",
		8, 0, 1, '.', tabwriter.StripEscape,
		"abc\xff\tdef", // unterminated escape
		"abc\tdef",
	},

	{
		"1e esc",
		8, 0, 1, '.', 0,
		"abc\xff\tdef", // unterminated escape
		"abc\xff\tdef",
	},

	{
		"2",
		8, 0, 1, '.', 0,
		"\n\n\n",
		"\n\n\n",
	},

	{
		"3",
		8, 0, 1, '.', 0,
		"a\nb\nc",
		"a\nb\nc",
	},

	{
		"4a",
		8, 0, 1, '.', 0,
		"\t", // '\t' terminates an empty cell on last line - nothing to print
		"",
	},

	{
		"4b",
		8, 0, 1, '.', tabwriter.AlignRight,
		"\t", // '\t' terminates an empty cell on last line - nothing to print
		"",
	},

	{
		"5",
		8, 0, 1, '.', 0,
		"*\t*",
		"*.......*",
	},

	{
		"5b",
		8, 0, 1, '.', 0,
		"*\t*\n",
		"*.......*\n",
	},

	{
		"5c",
		8, 0, 1, '.', 0,
		"*\t*\t",
		"*.......*",
	},

	{
		"5c debug",
		8, 0, 1, '.', tabwriter.Debug,
		"*\t*\t",
		"*.......|*",
	},

	{
		"5d",
		8, 0, 1, '.', tabwriter.AlignRight,
		"*\t*\t",
		".......**",
	},

	{
		"6",
		8, 0, 1, '.', 0,
		"\t\n",
		"........\n",
	},

	{
		"7a",
		8, 0, 1, '.', 0,
		"a) foo",
		"a) foo",
	},

	{
		"7b",
		8, 0, 1, ' ', 0,
		"b) foo\tbar",
		"b) foo  bar",
	},

	{
		"7c",
		8, 0, 1, '.', 0,
		"c) foo\tbar\t",
		"c) foo..bar",
	},

	{
		"7d",
		8, 0, 1, '.', 0,
		"d) foo\tbar\n",
		"d) foo..bar\n",
	},

	{
		"7e",
		8, 0, 1, '.', 0,
		"e) foo\tbar\t\n",
		"e) foo..bar.....\n",
	},

	{
		"7f",
		8, 0, 1, '.', tabwriter.FilterHTML,
		"f) f&lt;o\t<b>bar</b>\t\n",
		"f) f&lt;o..<b>bar</b>.....\n",
	},

	{
		"7g",
		8, 0, 1, '.', tabwriter.FilterHTML,
		"g) f&lt;o\t<b>bar</b>\t non-terminated entity &amp",
		"g) f&lt;o..<b>bar</b>..... non-terminated entity &amp",
	},

	{
		"7g debug",
		8, 0, 1, '.', tabwriter.FilterHTML | tabwriter.Debug,
		"g) f&lt;o\t<b>bar</b>\t non-terminated entity &amp",
		"g) f&lt;o..|<b>bar</b>.....| non-terminated entity &amp",
	},

	{
		"8",
		8, 0, 1, '*', 0,
		"Hello, world!\n",
		"Hello, world!\n",
	},

	{
		"9a",
		1, 0, 0, '.', 0,
		"1\t2\t3\t4\n" +
			"11\t222\t3333\t44444\n",

		"1.2..3...4\n" +
			"11222333344444\n",
	},

	{
		"9b",
		1, 0, 0, '.', tabwriter.FilterHTML,
		"1\t2<!---\f--->\t3\t4\n" + // \f inside HTML is ignored
			"11\t222\t3333\t44444\n",

		"1.2<!---\f--->..3...4\n" +
			"11222333344444\n",
	},

	{
		"9c",
		1, 0, 0, '.', 0,
		"1\t2\t3\t4\f" + // \f causes a newline and flush
			"11\t222\t3333\t44444\n",

		"1234\n" +
			"11222333344444\n",
	},

	{
		"9c debug",
		1, 0, 0, '.', tabwriter.Debug,
		"1\t2\t3\t4\f" + // \f causes a newline and flush
			"11\t222\t3333\t44444\n",

		"1|2|3|4\n" +
			"---\n" +
			"11|222|3333|44444\n",
	},

	{
		"10a",
		5, 0, 0, '.', 0,
		"1\t2\t3\t4\n",
		"1....2....3....4\n",
	},

	{
		"10b",
		5, 0, 0, '.', 0,
		"1\t2\t3\t4\t\n",
		"1....2....3....4....\n",
	},

	{
		"11",
		8, 0, 1, '.', 0,
		"本\tb\tc\n" +
			"aa\t\u672c\u672c\u672c\tcccc\tddddd\n" +
			"aaa\tbbbb\n",

		"本.......b.......c\n" +
			"aa......本本本.....cccc....ddddd\n" +
			"aaa.....bbbb\n",
	},

	{
		"12a",
		8, 0, 1, ' ', tabwriter.AlignRight,
		"a\tè\tc\t\n" +
			"aa\tèèè\tcccc\tddddd\t\n" +
			"aaa\tèèèè\t\n",

		"       a       è       c\n" +
			"      aa     èèè    cccc   ddddd\n" +
			"     aaa    èèèè\n",
	},

	{
		"12b",
		2, 0, 0, ' ', 0,
		"a\tb\tc\n" +
			"aa\tbbb\tcccc\n" +
			"aaa\tbbbb\n",

		"a  b  c\n" +
			"aa bbbcccc\n" +
			"aaabbbb\n",
	},

	{
		"12c",
		8, 0, 1, '_', 0,
		"a\tb\tc\n" +
			"aa\tbbb\tcccc\n" +
			"aaa\tbbbb\n",

		"a_______b_______c\n" +
			"aa______bbb_____cccc\n" +
			"aaa_____bbbb\n",
	},

	{
		"13a",
		4, 0, 1, '-', 0,
		"4444\t日本語\t22\t1\t333\n" +
			"999999999\t22\n" +
			"7\t22\n" +
			"\t\t\t88888888\n" +
			"\n" +
			"666666\t666666\t666666\t4444\n" +
			"1\t1\t999999999\t0000000000\n",

		"4444------日本語-22--1---333\n" +
			"999999999-22\n" +
			"7---------22\n" +
			"------------------88888888\n" +
			"\n" +
			"666666-666666-666666----4444\n" +
			"1------1------999999999-0000000000\n",
	},

	{
		"13b",
		4, 0, 3, '.', 0,
		"4444\t333\t22\t1\t333\n" +
			"999999999\t22\n" +
			"7\t22\n" +
			"\t\t\t88888888\n" +
			"\n" +
			"666666\t666666\t666666\t4444\n" +
			"1\t1\t999999999\t0000000000\n",

		"4444........333...22...1...333\n" +
			"999999999...22\n" +
			"7...........22\n" +
			"....................88888888\n" +
			"\n" +
			"666666...666666...666666......4444\n" +
			"1........1........999999999...0000000000\n",
	},

	{
		"13c",
		8, 8, 1, '\t', tabwriter.FilterHTML,
		"4444\t333\t22\t1\t333\n" +
			"999999999\t22\n" +
			"7\t22\n" +
			"\t\t\t88888888\n" +
			"\n" +
			"666666\t666666\t666666\t4444\n" +
			"1\t1\t<font color=red attr=日本語>999999999</font>\t0000000000\n",

		"4444\t\t333\t22\t1\t333\n" +
			"999999999\t22\n" +
			"7\t\t22\n" +
			"\t\t\t\t88888888\n" +
			"\n" +
			"666666\t666666\t666666\t\t4444\n" +
			"1\t1\t<font color=red attr=日本語>999999999</font>\t0000000000\n",
	},

	{
		"14",
		1, 0, 2, ' ', tabwriter.AlignRight,
		".0\t.3\t2.4\t-5.1\t\n" +
			"23.0\t12345678.9\t2.4\t-989.4\t\n" +
			"5.1\t12.0\t2.4\t-7.0\t\n" +
			".0\t0.0\t332.0\t8908.0\t\n" +
			".0\t-.3\t456.4\t22.1\t\n" +
			".0\t1.2\t44.4\t-13.3\t\t",

		"    .0          .3    2.4    -5.1\n" +
			"  23.0  12345678.9    2.4  -989.4\n" +
			"   5.1        12.0    2.4    -7.0\n" +
			"    .0         0.0  332.0  8908.0\n" +
			"    .0         -.3  456.4    22.1\n" +
			"    .0         1.2   44.4   -13.3",
	},

	{
		"14 debug",
		1, 0, 2, ' ', tabwriter.AlignRight | tabwriter.Debug,
		".0\t.3\t2.4\t-5.1\t\n" +
			"23.0\t12345678.9\t2.4\t-989.4\t\n" +
			"5.1\t12.0\t2.4\t-7.0\t\n" +
			".0\t0.0\t332.0\t8908.0\t\n" +
			".0\t-.3\t456.4\t22.1\t\n" +
			".0\t1.2\t44.4\t-13.3\t\t",

		"    .0|          .3|    2.4|    -5.1|\n" +
			"  23.0|  12345678.9|    2.4|  -989.4|\n" +
			"   5.1|        12.0|    2.4|    -7.0|\n" +
			"    .0|         0.0|  332.0|  8908.0|\n" +
			"    .0|         -.3|  456.4|    22.1|\n" +
			"    .0|         1.2|   44.4|   -13.3|",
	},

	{
		"15a",
		4, 0, 0, '.', 0,
		"a\t\tb",
		"a.......b",
	},

	{
		"15b",
		4, 0, 0, '.', tabwriter.DiscardEmptyColumns,
		"a\t\tb", // htabs - do not discard column
		"a.......b",
	},

	{
		"15c",
		4, 0, 0, '.', tabwriter.DiscardEmptyColumns,
		"a\v\vb",
		"a...b",
	},

	{
		"15d",
		4, 0, 0, '.', tabwriter.AlignRight | tabwriter.DiscardEmptyColumns,
		"a\v\vb",
		"...ab",
	},

	{
		"16a",
		100, 100, 0, '\t', 0,
		"a\tb\t\td\n" +
			"a\tb\t\td\te\n" +
			"a\n" +
			"a\tb\tc\td\n" +
			"a\tb\tc\td\te\n",

		"a\tb\t\td\n" +
			"a\tb\t\td\te\n" +
			"a\n" +
			"a\tb\tc\td\n" +
			"a\tb\tc\td\te\n",
	},

	{
		"16b",
		100, 100, 0, '\t', tabwriter.DiscardEmptyColumns,
		"a\vb\v\vd\n" +
			"a\vb\v\vd\ve\n" +
			"a\n" +
			"a\vb\vc\vd\n" +
			"a\vb\vc\vd\ve\n",

		"a\tb\td\n" +
			"a\tb\td\te\n" +
			"a\n" +
			"a\tb\tc\td\n" +
			"a\tb\tc\td\te\n",
	},

	{
		"16b debug",
		100, 100, 0, '\t', tabwriter.DiscardEmptyColumns | tabwriter.Debug,
		"a\vb\v\vd\n" +
			"a\vb\v\vd\ve\n" +
			"a\n" +
			"a\vb\vc\vd\n" +
			"a\vb\vc\vd\ve\n",

		"a\t|b\t||d\n" +
			"a\t|b\t||d\t|e\n" +
			"a\n" +
			"a\t|b\t|c\t|d\n" +
			"a\t|b\t|c\t|d\t|e\n",
	},

	{
		"16c",
		100, 100, 0, '\t', tabwriter.DiscardEmptyColumns,
		"a\tb\t\td\n" + // hard tabs - do not discard column
			"a\tb\t\td\te\n" +
			"a\n" +
			"a\tb\tc\td\n" +
			"a\tb\tc\td\te\n",

		"a\tb\t\td\n" +
			"a\tb\t\td\te\n" +
			"a\n" +
			"a\tb\tc\td\n" +
			"a\tb\tc\td\te\n",
	},

	{
		"16c debug",
		100, 100, 0, '\t', tabwriter.DiscardEmptyColumns | tabwriter.Debug,
		"a\tb\t\td\n" + // hard tabs - do not discard column
			"a\tb\t\td\te\n" +
			"a\n" +
			"a\tb\tc\td\n" +
			"a\tb\tc\td\te\n",

		"a\t|b\t|\t|d\n" +
			"a\t|b\t|\t|d\t|e\n" +
			"a\n" +
			"a\t|b\t|c\t|d\n" +
			"a\t|b\t|c\t|d\t|e\n",
	},
}

func tabwriterCorpus() string {
	var o strings.Builder
	o.WriteString(header("text/tabwriter's own tests (tabwriter_test.go)"))
	o.WriteString("\nuse std/tabwriter\n\n")
	o.WriteString("/// (name, min_width, tab_width, padding, pad_char, flags, src, expected)\n")
	o.WriteString("const CASES: List[(Str, Int, Int, Int, Int, Int, Bytes, Bytes)] = [\n")
	for _, e := range tabwriterTests {
		var b bytes.Buffer
		w := tabwriter.NewWriter(&b, e.minwidth, e.tabwidth, e.padding, e.padchar, e.flags)
		w.Write([]byte(e.src))
		w.Flush()
		if b.String() != e.expected {
			panic(fmt.Sprintf("tabwriter corpus: %s: Go wrote %q, its table says %q", e.testname, b.String(), e.expected))
		}
		fmt.Fprintf(&o, "  (%s, %d, %d, %d, %d, %d, %s, %s),\n", xq(e.testname), e.minwidth, e.tabwidth, e.padding, e.padchar, e.flags, xbytes([]byte(e.src)), xbytes([]byte(e.expected)))
	}
	o.WriteString("]\n\n")
	o.WriteString(`
fn make(c: (Str, Int, Int, Int, Int, Int, Bytes, Bytes)) -> tabwriter.TabWriter {
  tabwriter.writer(c.1, c.2, c.3, c.4, c.5)
}

/// Go's check: the source written all at once, byte by byte, and in
/// Fibonacci slices; each must flush to the expected text.
test "tabwriter writes Go's table the same three ways" {
  var bad: List[Str] = []
  for c in CASES {
    var w = make(c)
    w.write(c.6)
    let all = w.flush()
    if all != c.7 {
      bad.push("${c.0} (all at once): got ${all.debug()} want ${c.7.debug()}")
    }
    var w2 = make(c)
    for i in 0..c.6.len() {
      w2.write(c.6[i..i + 1])
    }
    let each = w2.flush()
    if each != c.7 {
      bad.push("${c.0} (byte by byte): got ${each.debug()} want ${c.7.debug()}")
    }
    var w3 = make(c)
    var i = 0
    var d = 0
    while i < c.6.len() {
      w3.write(c.6[i..i + d])
      i += d
      d += 1
      if i + d > c.6.len() {
        d = c.6.len() - i
      }
    }
    let fib = w3.flush()
    if fib != c.7 {
      bad.push("${c.0} (fibonacci): got ${fib.debug()} want ${c.7.debug()}")
    }
  }
  expect(bad) == []
}
`)
	return o.String()
}
