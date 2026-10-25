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
// with RFC3339Nano, in UTC), Time.from_unix/from_unix_millis with
// unix/unix_millis, and std.md 13.2's Date arithmetic: weekday, add_days,
// add_months and days_until.
//
// The inputs are Go's: time_test.go's parseDurationTests, and
// format_test.go's parseTests (the RFC3339 rows), parseErrorTests (the
// RFC3339 rows) and longFractionalDigitsTests, plus rfc3339Formats' texts
// and seeded random durations and instants. The Date rows are calendar
// dates at the ends of years, months and leap rules, against Go's
// time.Time at midnight UTC (Weekday, AddDate, Sub), with add_months'
// clamping done explicitly since Go's AddDate normalizes instead. The
// expectations are Go's own results on them, except where Xo deliberately
// differs; those rows carry a comment naming the reason. Go's time tests
// are Copyright 2009 The Go Authors, BSD-style license.

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
	o.WriteString(header("time.ParseDuration, Duration.String and the RFC 3339 and Unix conversions of time.Time on the inputs of Go's time_test.go (parseDurationTests) and format_test.go (parseTests, parseErrorTests and longFractionalDigitsTests for RFC 3339, and rfc3339Formats), on seeded random durations and instants, and on the ends of Time's range from decision 0364, against Duration.parse, the Display of a Duration, Duration.from_nanos and Duration.from_millis, Time.parse_rfc3339 and format_rfc3339, and Time.from_unix, from_unix_millis, unix and unix_millis; and std.md 13.2's Date arithmetic (weekday, add_days, add_months, days_until) against Go's time.Time at midnight UTC (Go's time tests are Copyright 2009 The Go Authors, BSD-style license)"))
	o.WriteString("\nuse std/time\n")

	// Duration.parse and the Display of a Duration. The inputs are Go's
	// parseDurationTests; the expectation is Go's ParseDuration and
	// Duration.String on them, so both the accepted texts and the refused
	// ones are Go's answers.
	durs := timeDedup(goDurationInputs())
	// Xo trims white space before parsing (std.md 2.6), which Go does not:
	// a deliberate difference, so the expectation is Xo's own reading.
	durs = append(durs, "  1.5s  ", "\t2h\n", " 0 ")
	// fractions at the end of the range a Duration keeps: Go's
	// leadingFraction stops accumulating at the overflow and keeps the
	// digits it read, so a fraction of 19 digits that passes 2^63 keeps
	// eighteen of them and scales by what it read (2026-10-25: Xo kept the
	// wrapped value and read 1s where Go reads 1.9223372036854775s)
	durs = append(durs, "1.922337203685477580s", "1.9223372036854775807s",
		"1.9223372036854775808s", "1.9223372036854775809s",
		"1.9999999999999999999s", "-1.9223372036854775809s",
		"1.9223372036854775809ns", "1.12345678901234567890s",
		"1.922337203685477581999999999s",
		"9223372036854775.8079223372036854775809s")
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
	// Xo's reader is stricter than Go's in shape: std/native/prelude.xo's
	// time_parse_rfc3339 takes "two and four digit fields ... a '.' fraction
	// ... Go's parser alone also takes a one digit hour, a comma fraction and
	// offsets to +24:60". These two are Go's reading; Xo refuses them, so the
	// expectation is None (the range rows above are decision 0364; these are
	// shape, which the module comment states and no ADR decides).
	for _, s := range []string{
		"2010-02-04T21:00:57,012Z", // a comma fraction
		"2010-02-04T1:00:57Z",      // a one digit hour
	} {
		if _, _, _, ok := timeRFC3339(s); !ok {
			fmt.Fprintf(os.Stderr, "stdcorpus: Go refuses %q too, so it is not a difference to pin\n", s)
			os.Exit(1)
		}
		rows = append(rows, rfcRow{text: s, note: "Go reads it; Xo refuses it, as std/native/prelude.xo time_parse_rfc3339 says Go's parser alone takes a comma fraction or a one digit hour"})
	}
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

	// Date arithmetic (std.md 13.2). Go's time at midnight UTC is the
	// oracle: weekday is Time.Weekday, add_days is AddDate(0, 0, n) (the
	// same civil date), and days_until is the difference of the two
	// midnights in whole days. add_months is where Xo differs on purpose:
	// std.md 13.2 says it clamps the day (Jan 31 + 1 month is Feb 28 or
	// 29), where Go's AddDate normalizes (2024-01-31 + 1 month is
	// 2024-03-02), so the expectation clamps explicitly and every row where
	// Go's AddDate would give another date says so in its label.
	gdate := func(y, m, d int) time.Time { return time.Date(y, time.Month(m), d, 0, 0, 0, 0, time.UTC) }
	dateShown := func(t time.Time) string {
		y := t.Year()
		if y < 0 {
			return fmt.Sprintf("-%04d-%02d-%02d", -y, int(t.Month()), t.Day())
		}
		return fmt.Sprintf("%04d-%02d-%02d", y, int(t.Month()), t.Day())
	}
	validDate := func(y, m, d int) bool {
		if y < 1 || y > 9999 {
			return false
		}
		t := gdate(y, m, d)
		return t.Year() == y && int(t.Month()) == m && t.Day() == d
	}
	inRange := func(t time.Time) bool { return t.Year() >= 1 && t.Year() <= 9999 }
	daysIn := func(y, m int) int { return gdate(y, m+1, 0).Day() }
	normMonth := func(y, m, n int) (int, int) {
		total := y*12 + (m - 1) + n
		ny := total / 12
		nm := total % 12
		if nm < 0 {
			nm += 12
			ny--
		}
		return ny, nm + 1
	}

	// The dates: the epoch week, month ends and leap days in every century
	// rule, the Gregorian change, the ends of the four digit years, and
	// seeded dates. time.date takes years 1 to 9999 (its CivilErr for
	// anything else), so the dates stay inside that range, as do the
	// results of the arithmetic rows.
	dates := [][3]int{
		{1970, 1, 1}, {1970, 1, 2}, {1970, 1, 5}, {1969, 12, 31},
		{2000, 2, 29}, {2024, 2, 29}, {2023, 2, 28}, {2100, 2, 28},
		{2400, 2, 29}, {1900, 2, 28}, {1900, 3, 1}, {1600, 2, 29},
		{1582, 10, 15}, {1582, 10, 4},
		{1, 1, 1}, {4, 2, 29}, {100, 3, 1}, {400, 2, 29}, {9999, 12, 31},
		{2026, 1, 31}, {2026, 3, 31}, {2026, 5, 31}, {2026, 7, 31},
		{2026, 8, 31}, {2026, 10, 31}, {2026, 12, 31},
		{2026, 4, 30}, {2026, 6, 30}, {2026, 9, 30}, {2026, 11, 30},
		{2026, 10, 24}, {2024, 1, 31}, {2024, 3, 31}, {2024, 12, 31},
	}
	rdt := rand.New(rand.NewSource(83))
	for i := 0; i < 40; i++ {
		y := 1 + rdt.Intn(9999)
		m := 1 + rdt.Intn(12)
		d := 1 + rdt.Intn(28)
		dates = append(dates, [3]int{y, m, d})
	}

	const dateLab = "std.md 13.2 Date.weekday (Go's time.Weekday at midnight UTC)"
	o.WriteString("\n/// (year, month, day, Go's Weekday, the rule)\nconst DATE_WEEKDAY_CASES: List[(Int, Int, Int, Str, Str)] = [\n")
	for _, dt := range dates {
		y, m, d := dt[0], dt[1], dt[2]
		if !validDate(y, m, d) {
			fmt.Fprintf(os.Stderr, "stdcorpus: date %d-%d-%d is not a Go date\n", y, m, d)
			os.Exit(1)
		}
		fmt.Fprintf(&o, "  (%d, %d, %d, %q, %q),\n", y, m, d, gdate(y, m, d).Weekday().String(), dateLab)
	}
	o.WriteString("]\n")

	const daysLab = "std.md 13.2 Date.add_days (Go's AddDate(0, 0, n) at midnight UTC)"
	o.WriteString("\n/// (year, month, day, n, the date after n days, the rule)\nconst DATE_DAYS_CASES: List[(Int, Int, Int, Int, Str, Str)] = [\n")
	for _, n := range []int{-1461, -366, -365, -31, -1, 0, 1, 28, 31, 365, 366, 1461, 36525} {
		for _, dt := range dates {
			y, m, d := dt[0], dt[1], dt[2]
			if !validDate(y, m, d) {
				continue
			}
			add := gdate(y, m, d).AddDate(0, 0, n)
			if !inRange(add) {
				continue
			}
			fmt.Fprintf(&o, "  (%d, %d, %d, %d, %q, %q),\n", y, m, d, n,
				dateShown(add), daysLab)
		}
	}
	o.WriteString("]\n")

	o.WriteString("\n/// (year, month, day, n, the date after n months with the day clamped, the rule)\nconst DATE_MONTHS_CASES: List[(Int, Int, Int, Int, Str, Str)] = [\n")
	for _, n := range []int{-25, -24, -13, -12, -11, -2, -1, 0, 1, 2, 11, 12, 13, 24, 25} {
		for _, dt := range dates {
			y, m, d := dt[0], dt[1], dt[2]
			if !validDate(y, m, d) {
				continue
			}
			wy, wm := normMonth(y, m, n)
			wd := d
			if wd > daysIn(wy, wm) {
				wd = daysIn(wy, wm)
			}
			want := gdate(wy, wm, wd)
			if !inRange(want) {
				continue
			}
			lab := "std.md 13.2 Date.add_months (Go's AddDate gives the same date here)"
			if add := gdate(y, m, d).AddDate(0, n, 0); dateShown(add) != dateShown(want) {
				lab = "std.md 13.2: add_months clamps the day; Go's AddDate(0, n, 0) normalizes to " + dateShown(add)
			}
			fmt.Fprintf(&o, "  (%d, %d, %d, %d, %q, %q),\n", y, m, d, n, dateShown(want), lab)
		}
	}
	o.WriteString("]\n")

	const untilLab = "std.md 13.2 Date.days_until (whole days between the two midnights UTC)"
	o.WriteString("\n/// (from year, month, day, to year, month, day, days from the first to the second, the rule)\nconst DATE_UNTIL_CASES: List[(Int, Int, Int, Int, Int, Int, Int, Str)] = [\n")
	until := func(dt, ot [3]int) {
		if !validDate(dt[0], dt[1], dt[2]) || !validDate(ot[0], ot[1], ot[2]) {
			return
		}
		// Unix seconds, not Sub: a time.Duration holds about 292 years and
		// Sub saturates past that, while these dates span up to 9998.
		span := int((gdate(ot[0], ot[1], ot[2]).Unix() - gdate(dt[0], dt[1], dt[2]).Unix()) / 86400)
		fmt.Fprintf(&o, "  (%d, %d, %d, %d, %d, %d, %d, %q),\n", dt[0], dt[1], dt[2], ot[0], ot[1], ot[2], span, untilLab)
	}
	for _, dt := range dates {
		for _, k := range []int{1, 2, 7, 30, 365} {
			back := ((len(dates)-k)%len(dates) + len(dates)) % len(dates)
			until(dt, dates[back])
			until(dt, dates[k%len(dates)])
		}
	}
	ru := rand.New(rand.NewSource(89))
	for i := 0; i < 120; i++ {
		until(dates[ru.Intn(len(dates))], dates[ru.Intn(len(dates))])
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

test "Date.weekday is Go's Weekday" {
  var bad: List[(Str, Str, Str)] = []
  for (y, m, d, want, _) in DATE_WEEKDAY_CASES {
    let got = match time.date(y, m, d) {
      Ok(dt) => "${dt.weekday()}"
      Err(e) => "!error ${e.display()}"
    }
    if got != want {
      bad.push(("${y}-${m}-${d}", got, want))
    }
  }
  expect(show3(bad)) == []
}

test "Date.add_days is Go's AddDate(0, 0, n)" {
  var bad: List[(Str, Str, Str)] = []
  for (y, m, d, n, want, _) in DATE_DAYS_CASES {
    let got = match time.date(y, m, d) {
      Ok(dt) => "${dt.add_days(n)}"
      Err(e) => "!error ${e.display()}"
    }
    if got != want {
      bad.push(("${y}-${m}-${d} plus ${n} days", got, want))
    }
  }
  expect(show3(bad)) == []
}

test "Date.add_months clamps the day" {
  var bad: List[(Str, Str, Str)] = []
  for (y, m, d, n, want, _) in DATE_MONTHS_CASES {
    let got = match time.date(y, m, d) {
      Ok(dt) => "${dt.add_months(n)}"
      Err(e) => "!error ${e.display()}"
    }
    if got != want {
      bad.push(("${y}-${m}-${d} plus ${n} months", got, want))
    }
  }
  expect(show3(bad)) == []
}

test "Date.days_until is the whole days between the two dates" {
  var bad: List[(Str, Str, Str)] = []
  for (y1, m1, d1, y2, m2, d2, want, _) in DATE_UNTIL_CASES {
    let got = match time.date(y1, m1, d1) {
      Ok(a) => match time.date(y2, m2, d2) {
        Ok(b) => "${a.days_until(b)}"
        Err(e) => "!error ${e.display()}"
      }
      Err(e) => "!error ${e.display()}"
    }
    if got != "${want}" {
      bad.push(("${y1}-${m1}-${d1} to ${y2}-${m2}-${d2}", got, "${want}"))
    }
  }
  expect(show3(bad)) == []
}

test "local with a fake Env reads no TZ and gives utc" {
  let z = time.local(os.env)?
  expect(z.name()) == "UTC"
}
`)
	return o.String()
}
