package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// The API this corpus covers, as std/prelude.xo and std.md 2.6 declare it:
// Duration.parse (Go's time.ParseDuration), the Display of a Duration (Go's
// Duration.String), Duration.from_nanos/from_millis and nanos/millis,
// Time.parse_rfc3339 and format_rfc3339 (Go's time.Parse and Time.Format
// with RFC3339Nano, in UTC), and Time.from_unix/from_unix_millis with
// unix/unix_millis.
//
// The inputs are Go's: time_test.go's parseDurationTests, and
// format_test.go's parseTests (the RFC3339 rows), parseErrorTests (the
// RFC3339 rows) and longFractionalDigitsTests, plus rfc3339Formats' texts
// and seeded random durations and instants. The expectations are Go's own
// results on them, except where Xo deliberately differs; those rows carry a
// comment naming the reason. Go's time tests are Copyright 2009 The Go
// Authors, BSD-style license.

// goTimeTable returns the elements of the composite literal a test file's
// var name holds.
func goTimeTable(file, name string) []*ast.CompositeLit {
	path := filepath.Join(runtime.GOROOT(), "src", "time", file)
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		fmt.Fprintln(os.Stderr, "stdcorpus: reading Go's time tests:", err)
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
			var out []*ast.CompositeLit
			for _, e := range lit.Elts {
				if c, ok := e.(*ast.CompositeLit); ok {
					out = append(out, c)
				}
			}
			return out
		}
	}
	fmt.Fprintf(os.Stderr, "stdcorpus: Go's %s has no table %s\n", file, name)
	os.Exit(1)
	return nil
}

// goTimeString is the string a table cell holds: a literal, or a named
// constant the table uses for a layout.
func goTimeString(e ast.Expr) string {
	switch v := e.(type) {
	case *ast.BasicLit:
		if v.Kind == token.STRING {
			s, err := strconv.Unquote(v.Value)
			if err == nil {
				return s
			}
		}
	case *ast.Ident:
		switch v.Name {
		case "RFC3339", "RFC3339Nano":
			return "2006-01-02T15:04:05Z07:00"
		}
	}
	return "\x00missing"
}

// goDurationInputs are the inputs of Go's parseDurationTests.
func goDurationInputs() []string {
	var out []string
	for _, c := range goTimeTable("time_test.go", "parseDurationTests") {
		if len(c.Elts) == 0 {
			continue
		}
		if s := goTimeString(c.Elts[0]); s != "\x00missing" {
			out = append(out, s)
		}
	}
	return out
}

// goRFC3339Tables are Go's RFC 3339 inputs: parseTests' RFC3339 rows, the
// texts rfc3339Formats formats, and longFractionalDigitsTests' values, then
// the texts parseErrorTests says Go refuses.
func goRFC3339Tables() (valid, invalid []string) {
	for _, c := range goTimeTable("format_test.go", "parseTests") {
		if len(c.Elts) < 3 || goTimeString(c.Elts[1]) != "2006-01-02T15:04:05Z07:00" {
			continue
		}
		if s := goTimeString(c.Elts[2]); s != "\x00missing" {
			valid = append(valid, s)
		}
	}
	for _, c := range goTimeTable("format_test.go", "rfc3339Formats") {
		if len(c.Elts) == 2 {
			if s := goTimeString(c.Elts[1]); s != "\x00missing" {
				valid = append(valid, s)
			}
		}
	}
	for _, c := range goTimeTable("format_test.go", "longFractionalDigitsTests") {
		if len(c.Elts) == 0 {
			continue
		}
		if s := goTimeString(c.Elts[0]); s != "\x00missing" {
			valid = append(valid, s)
		}
	}
	for _, c := range goTimeTable("format_test.go", "parseErrorTests") {
		if len(c.Elts) < 2 || goTimeString(c.Elts[0]) != "2006-01-02T15:04:05Z07:00" {
			continue
		}
		if s := goTimeString(c.Elts[1]); s != "\x00missing" {
			invalid = append(invalid, s)
		}
	}
	return valid, invalid
}

// timeDedup keeps the first of each repeated input.
func timeDedup(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}

// timeRFC3339 is Go's reading of s as RFC 3339 in UTC: the text of its
// value, its Unix second, and its Unix millisecond.
func timeRFC3339(s string) (string, int64, int64, bool) {
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return "", 0, 0, false
	}
	t = t.UTC()
	return t.Format(time.RFC3339Nano), t.Unix(), t.UnixMilli(), true
}

// timeRandomDuration makes a duration text out of one to three parts, in
// Go's ParseDuration syntax.
func timeRandomDuration(r *rand.Rand) string {
	units := []string{"h", "m", "s", "ms", "us", "µs", "ns"}
	var b strings.Builder
	if r.Intn(3) == 0 {
		b.WriteString("-")
	} else if r.Intn(8) == 0 {
		b.WriteByte('+')
	}
	n := 1 + r.Intn(3)
	for i := 0; i < n; i++ {
		if i > 0 {
			// a whole number, so the parts add up to what the text says
			fmt.Fprintf(&b, "%d%s", r.Intn(1000), units[r.Intn(len(units))])
			continue
		}
		fmt.Fprintf(&b, "%d", r.Intn(100000))
		if r.Intn(2) == 0 {
			fmt.Fprintf(&b, ".%d", r.Intn(1000000000))
		}
		b.WriteString(units[r.Intn(len(units))])
	}
	return b.String()
}

func timeCorpus() string {
	var o strings.Builder
	o.WriteString(header("time.ParseDuration, Duration.String and the RFC 3339 and Unix conversions of time.Time on the inputs of Go's time_test.go (parseDurationTests) and format_test.go (parseTests, parseErrorTests and longFractionalDigitsTests for RFC 3339, and rfc3339Formats), on seeded random durations and instants, and on the ends of Time's range from decision 0364, against Duration.parse, the Display of a Duration, Duration.from_nanos and Duration.from_millis, Time.parse_rfc3339 and format_rfc3339, and Time.from_unix, from_unix_millis, unix and unix_millis (Go's time tests are Copyright 2009 The Go Authors, BSD-style license)"))

	// Duration.parse and the Display of a Duration. The inputs are Go's
	// parseDurationTests; the expectation is Go's ParseDuration and
	// Duration.String on them, so both the accepted texts and the refused
	// ones are Go's answers.
	durs := timeDedup(goDurationInputs())
	// Xo trims white space before parsing (std.md 2.6), which Go does not:
	// a deliberate difference, so the expectation is Xo's own reading.
	durs = append(durs, "  1.5s  ", "\t2h\n", " 0 ")
	rd := rand.New(rand.NewSource(71))
	for i := 0; i < 500; i++ {
		durs = append(durs, timeRandomDuration(rd))
	}
	o.WriteString("\n/// (text, whether Go's ParseDuration accepts it, Go's Duration.String; empty on an error. The texts with surrounding white space are trimmed before parsing, as std.md 2.6 says and Go does not)\nconst DUR_CASES: List[(Str, Bool, Str)] = [\n")
	for _, in := range durs {
		// Xo trims white space first (std.md 2.6); Go does not, so the
		// expectation is Go's answer for the trimmed text
		d, err := time.ParseDuration(strings.TrimSpace(in))
		if err != nil {
			fmt.Fprintf(&o, "  (%s, false, \"\"),\n", xq(in))
			continue
		}
		fmt.Fprintf(&o, "  (%s, true, %s),\n", xq(in), xq(d.String()))
	}
	o.WriteString("]\n")

	// Duration.from_nanos, from_millis, millis and nanos.
	nanos := []int64{0, 1, -1, 999, 1000, -1000, 1000000, -1500000, 1500000,
		999999999, 1000000000, -999999999,
		9223372036854775807, -9223372036854775808}
	rn := rand.New(rand.NewSource(73))
	for i := 0; i < 200; i++ {
		nanos = append(nanos, rn.Int63()-rn.Int63())
	}
	o.WriteString("\n/// (nanoseconds, Go's Duration.String, Go's Duration.Milliseconds)\nconst DUR_NANO_CASES: List[(Int, Str, Int)] = [\n")
	for _, ns := range nanos {
		d := time.Duration(ns)
		fmt.Fprintf(&o, "  (%d, %s, %d),\n", ns, xq(d.String()), d.Milliseconds())
	}
	o.WriteString("]\n")

	// every one of these times a millisecond fits in an int64 nanosecond count
	millis := []int64{0, 1, -1, 999, -999, 1000, -1000, 1499, -1499, 60000,
		9223372036854, -9223372036854}
	o.WriteString("\n/// (milliseconds, Go's Duration.String, Go's Duration.Nanoseconds)\nconst DUR_MILLI_CASES: List[(Int, Str, Int)] = [\n")
	for _, ms := range millis {
		d := time.Duration(ms) * time.Millisecond
		fmt.Fprintf(&o, "  (%d, %s, %d),\n", ms, xq(d.String()), d.Nanoseconds())
	}
	o.WriteString("]\n")

	// RFC 3339: Go's inputs, then the ends of Time's range (decision 0364),
	// which Go reads and Xo refuses outside the ends.
	rfcValid, rfcInvalid := goRFC3339Tables()
	type rfcRow struct {
		text string
		ok   bool
		can  string
		secs int64
		ms   int64
		note string
	}
	var rows []rfcRow
	for _, s := range timeDedup(rfcValid) {
		can, secs, ms, ok := timeRFC3339(s)
		rows = append(rows, rfcRow{text: s, ok: ok, can: can, secs: secs, ms: ms})
	}
	for _, s := range timeDedup(rfcInvalid) {
		rows = append(rows, rfcRow{text: s})
	}
	// The ends of Time's range, to the nanosecond, and just past them.
	// Go parses all six (its Time is not limited to int64 nanoseconds), so
	// the two refusals are Xo's: decision 0364 says a time outside the range
	// of Time is None, and 2300 is the year that used to come back in the
	// 1700s.
	lo := time.Unix(0, -9223372036854775808).UTC()
	hi := time.Unix(0, 9223372036854775807).UTC()
	rows = append(rows,
		rfcRow{lo.Format(time.RFC3339Nano), true, lo.Format(time.RFC3339Nano), lo.Unix(), lo.UnixMilli(),
			"the first instant Time holds, to the nanosecond (decision 0364); Go reads it too"},
		rfcRow{hi.Format(time.RFC3339Nano), true, hi.Format(time.RFC3339Nano), hi.Unix(), hi.UnixMilli(),
			"the last instant Time holds, to the nanosecond (decision 0364); Go reads it too"},
		rfcRow{text: "1677-09-21T00:12:43.145224191Z",
			note: "one nanosecond before the first instant Time holds: None (decision 0364); Go reads it"},
		rfcRow{text: "2262-04-11T23:47:16.854775808Z",
			note: "one nanosecond past the last instant Time holds: None (decision 0364); Go reads it"},
		rfcRow{text: "2300-01-01T00:00:00Z",
			note: "outside Time's range, so None (decision 0364); it used to come back as a time in the 1700s"},
		rfcRow{text: "1600-01-01T00:00:00Z",
			note: "outside Time's range, so None (decision 0364)"},
	)
	// Seeded instants from inside the range, every second and nanosecond
	// shape: the text is Go's format of the same instant.
	rr := rand.New(rand.NewSource(79))
	for i := 0; i < 300; i++ {
		secs := rr.Int63n(2*9223372036) - 9223372036
		ns := rr.Int63n(1000000000)
		if secs == 9223372035 {
			// the last second Time holds ends before the next one's 1e9
			ns = rr.Int63n(854775808)
		}
		t := time.Unix(secs, ns).UTC()
		rows = append(rows, rfcRow{text: t.Format(time.RFC3339Nano), ok: true, can: t.Format(time.RFC3339Nano), secs: t.Unix(), ms: t.UnixMilli()})
	}
	o.WriteString("\n/// (text, whether it is a Time, the text in UTC, its Unix second, its Unix millisecond. The four texts at and just past the ends of Time's range are decision 0364: Go reads them all, Xo refuses the ones outside)\nconst RFC_CASES: List[(Str, Bool, Str, Int, Int)] = [\n")
	for _, r := range rows {
		if r.note != "" {
			fmt.Fprintf(&o, "  // %s\n", r.note)
		}
		fmt.Fprintf(&o, "  (%s, %v, %s, %d, %d),\n", xq(r.text), r.ok, xq(r.can), r.secs, r.ms)
	}
	o.WriteString("]\n")

	// Unix seconds and milliseconds: the instants Time.from_unix and
	// from_unix_millis build, and what they read back.
	secs := []int64{0, 1, -1, 60, -60, 951782400, -2208988800, 1700000000,
		-1700000000, 9223372036, -9223372036, 9223372035, 1}
	o.WriteString("\n/// (Unix second, its Unix millisecond, Go's text in UTC)\nconst UNIX_CASES: List[(Int, Int, Str)] = [\n")
	for _, s := range secs {
		t := time.Unix(s, 0).UTC()
		fmt.Fprintf(&o, "  (%d, %d, %s),\n", s, t.UnixMilli(), xq(t.Format(time.RFC3339Nano)))
	}
	o.WriteString("]\n")

	ms := []int64{0, 1, -1, 999, -999, 1000, -1000, 1499, -1499, 1700000000123,
		-1700000000123, 9223372036854, -9223372036854}
	o.WriteString("\n/// (Unix millisecond, its Unix second, Go's text in UTC)\nconst MILLI_CASES: List[(Int, Int, Str)] = [\n")
	for _, m := range ms {
		t := time.UnixMilli(m).UTC()
		fmt.Fprintf(&o, "  (%d, %d, %s),\n", m, t.Unix(), xq(t.Format(time.RFC3339Nano)))
	}
	o.WriteString("]\n")

	o.WriteString(`
fn show3(bad: List[(Str, Str, Str)]) -> List[(Str, Str, Str)] {
  bad[0..(if bad.len() < 20 { bad.len() } else { 20 })]
}

test "Duration.parse is Go's ParseDuration and Duration displays as Go's String" {
  var bad: List[(Str, Str, Str)] = []
  for (text, ok, want) in DUR_CASES {
    let got = match Duration.parse(text) {
      Ok(d) => "${d}"
      Err(_) => "!error"
    }
    let want_text = if ok { want } else { "!error" }
    if got != want_text {
      bad.push((text, got, want_text))
    }
  }
  expect(show3(bad)) == []
}

test "Duration.parse reads back what a Duration displays" {
  var bad: List[(Str, Str, Str)] = []
  for (text, ok, want) in DUR_CASES {
    if !ok {
      continue
    }
    let got = match Duration.parse(want) {
      Ok(d) => "${d}"
      Err(_) => "!error"
    }
    if got != want {
      bad.push((text, got, want))
    }
  }
  expect(show3(bad)) == []
}

test "from_nanos, from_millis, millis and nanos are Go's" {
  var bad: List[(Str, Str, Str)] = []
  for (ns, want, want_ms) in DUR_NANO_CASES {
    let d = Duration.from_nanos(ns)
    let got = "${d} ${d.millis()} ${d.nanos()}"
    let want_all = "${want} ${want_ms} ${ns}"
    if got != want_all {
      bad.push(("nanos ${ns}", got, want_all))
    }
  }
  for (ms, want, want_ns) in DUR_MILLI_CASES {
    let d = Duration.from_millis(ms)
    let got = "${d} ${d.nanos()}"
    let want_all = "${want} ${want_ns}"
    if got != want_all {
      bad.push(("millis ${ms}", got, want_all))
    }
  }
  expect(show3(bad)) == []
}

test "Time.parse_rfc3339 and format_rfc3339 are Go's RFC 3339 in UTC" {
  var bad: List[(Str, Str, Str)] = []
  for (text, ok, want_text, want_secs, want_ms) in RFC_CASES {
    let got = match Time.parse_rfc3339(text) {
      Some(t) => "${t.format_rfc3339()} ${t.unix()} ${t.unix_millis()}"
      None => "!none"
    }
    let want = if ok {
      "${want_text} ${want_secs} ${want_ms}"
    } else {
      "!none"
    }
    if got != want {
      bad.push((text, got, want))
    }
  }
  expect(show3(bad)) == []
}

test "from_unix and unix agree with Go" {
  var bad: List[(Str, Str, Str)] = []
  for (secs, want_ms, want_text) in UNIX_CASES {
    let t = Time.from_unix(secs)
    let got = "${t.unix()} ${t.unix_millis()} ${t.format_rfc3339()}"
    let want = "${secs} ${want_ms} ${want_text}"
    if got != want {
      bad.push(("unix ${secs}", got, want))
    }
  }
  expect(show3(bad)) == []
}

test "from_unix_millis and unix_millis agree with Go" {
  var bad: List[(Str, Str, Str)] = []
  for (ms, want_secs, want_text) in MILLI_CASES {
    let t = Time.from_unix_millis(ms)
    let got = "${t.unix()} ${t.unix_millis()} ${t.format_rfc3339()}"
    let want = "${want_secs} ${ms} ${want_text}"
    if got != want {
      bad.push(("millis ${ms}", got, want))
    }
  }
  expect(show3(bad)) == []
}

test "UNIX_EPOCH is from_unix 0" {
  expect(Time.UNIX_EPOCH.unix()) == 0
  expect(Time.UNIX_EPOCH.unix_millis()) == 0
  expect("${Time.UNIX_EPOCH}") == "1970-01-01T00:00:00Z"
}
`)
	return o.String()
}
