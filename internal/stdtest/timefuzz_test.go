// A fuzz harness for std/time's own layouts against Go's time package
// (decision 0461: a parser of untrusted input is Early on testing until it
// is fuzzed; decision 0616's fuzz kind). std/timego has its own harness
// (timegofuzz_test.go) for Go's layout language; this one reaches
// std/time's format and parse, which read Xo's named fields
// (`{year}-{month}-{day}`, spec/std.md 13.3).
//
// Each case is a random Xo layout of date and time fields with its Go
// twin (the same fields in Go's reference spelling), a random instant
// from 1700 to 2200 and a zone offset:
//
//   - format: time.format of the instant must print what Go's
//     Time.Format prints with the twin layout.
//   - parse: Go's text, or that text changed the way a bad input would be
//     (a byte replaced, deleted or inserted, or the text cut), is parsed by
//     time.parse with the Xo layout and by time.Parse with the twin. Go's
//     own text must parse to Go's instant. A changed text that Xo accepts
//     must be one Go accepts, at the same instant. Xo may refuse a changed
//     text Go accepts: spec/std.md 13.3's parse "accepts exactly the
//     layout" where Go's is lenient (an hour of one digit, a fraction the
//     layout does not name, names in any case).
//
// The cases from seed 1 are the committed corpus
// testdata/corpus/timefuzz/timefuzz_test.xo, which TestStdCorpora runs on
// every corpus backend; TestTimeFuzzCorpusCurrent checks it is what this
// generator writes (XO_TIMEFUZZ_WRITE=1 rewrites it). TestTimeFuzz runs a
// fresh seed (XO_TIMEFUZZ_SEED, default 2; XO_TIMEFUZZ_N cases, default
// 2000) on XO_TIMEFUZZ_BACKENDS (default arm64 and llvm).
package stdtest

import (
	"fmt"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// tzField is one Xo layout field and its spelling in Go's layouts.
type tzField struct{ xo, gol string }

var (
	tzYear    = tzField{"{year}", "2006"}
	tzMonths  = []tzField{{"{month}", "01"}, {"{month:1}", "1"}, {"{month:short}", "Jan"}, {"{month:name}", "January"}}
	tzDays    = []tzField{{"{day}", "02"}, {"{day:1}", "2"}}
	tzWeekday = []tzField{{"{weekday:short}", "Mon"}, {"{weekday:name}", "Monday"}}
	tzOffsets = []tzField{{"{offset}", "Z07:00"}, {"{offset:compact}", "-0700"}}
)

// tzCase is one layout pair, instant and offset with Go's answers.
type tzCase struct {
	xo, gol  string
	ns       int64
	off      int
	text     string // Go's Format
	ptext    string // the text parsed: Go's, or changed
	changed  bool
	pwant    string // Go's Parse: "ok UNIX_NS", "err", or "range" (outside a Time)
	zoneOnly bool   // a {zone} layout: formatted, not parsed (an abbreviation is not an offset)
}

func tzPick(r *rand.Rand, xs []tzField) tzField { return xs[r.Intn(len(xs))] }

// tzLayout is a random layout pair: a date in one of three orders with one
// separator, an optional weekday before it, a time in 24 or 12 hour form
// with an optional fraction, and an optional offset or zone.
func tzLayout(r *rand.Rand) (string, string, bool) {
	var xo, gol []string
	add := func(f tzField) { xo = append(xo, f.xo); gol = append(gol, f.gol) }
	lit := func(s string) { xo = append(xo, s); gol = append(gol, s) }
	if r.Intn(4) == 0 {
		add(tzPick(r, tzWeekday))
		lit(", ")
	}
	sep := []string{"-", "/", " ", "."}[r.Intn(4)]
	m, d := tzPick(r, tzMonths), tzPick(r, tzDays)
	switch r.Intn(3) {
	case 0:
		add(tzYear)
		lit(sep)
		add(m)
		lit(sep)
		add(d)
	case 1:
		add(d)
		lit(sep)
		add(m)
		lit(sep)
		add(tzYear)
	default:
		add(m)
		lit(sep)
		add(d)
		lit(sep)
		add(tzYear)
	}
	lit([]string{"T", " ", " at "}[r.Intn(3)])
	h12 := r.Intn(3) == 0
	if h12 {
		add(tzField{"{hour12}", "03"})
	} else {
		add(tzField{"{hour}", "15"})
	}
	lit(":")
	add(tzField{"{minute}", "04"})
	lit(":")
	add(tzField{"{second}", "05"})
	switch r.Intn(3) {
	case 0:
		add(tzField{"{frac}", ".999999999"})
	case 1:
		n := 1 + r.Intn(9)
		add(tzField{fmt.Sprintf("{frac:%d}", n), "." + strings.Repeat("0", n)})
	}
	if h12 {
		lit(" ")
		add(tzField{"{ampm}", "PM"})
	}
	zone := false
	switch r.Intn(4) {
	case 0, 1:
		add(tzPick(r, tzOffsets))
	case 2:
		lit(" ")
		add(tzField{"{zone}", "MST"})
		zone = true
	}
	return strings.Join(xo, ""), strings.Join(gol, ""), zone
}

// tzMutate changes a time text the way a bad input would be.
func tzMutate(r *rand.Rand, in string) string {
	b := []byte(in)
	if len(b) == 0 {
		return in
	}
	const alpha = "0123456789:-+/ ,.TZJanFebMonPMAMpmaUC"
	switch r.Intn(4) {
	case 0:
		b[r.Intn(len(b))] = alpha[r.Intn(len(alpha))]
	case 1:
		i := r.Intn(len(b))
		b = append(b[:i], b[i+1:]...)
	case 2:
		i := r.Intn(len(b) + 1)
		b = append(b[:i], append([]byte{alpha[r.Intn(len(alpha))]}, b[i:]...)...)
	default:
		b = b[:r.Intn(len(b))]
	}
	return string(b)
}

// tzParse is Go's answer for text in layout gol, read in UTC as Xo's
// parse reads it with time.utc().
func tzParse(gol, text string) string {
	t, err := time.Parse(gol, text)
	if err != nil {
		return "err"
	}
	if t.Before(time.Unix(0, math.MinInt64)) || t.After(time.Unix(0, math.MaxInt64)) {
		return "range"
	}
	return fmt.Sprintf("ok %d", t.UnixNano())
}

func tzCases(r *rand.Rand, n int) []tzCase {
	offs := []int{0, 0, 3600, -18000, 19800, -34200, 45900, 50400, -43200}
	var cs []tzCase
	for len(cs) < n {
		xo, gol, zone := tzLayout(r)
		// 1700 to 2200, any nanosecond
		secs := r.Int63n(500*365*24*3600) - 270*365*24*3600
		ns := secs*1e9 + r.Int63n(1e9)
		off := offs[r.Intn(len(offs))]
		z := time.FixedZone("ZZ", off)
		if off == 0 {
			z = time.UTC
		}
		text := time.Unix(0, ns).In(z).Format(gol)
		c := tzCase{xo: xo, gol: gol, ns: ns, off: off, text: text, ptext: text, zoneOnly: zone}
		if !zone {
			if r.Intn(2) == 0 {
				c.ptext = tzMutate(r, text)
				c.changed = c.ptext != text
			}
			c.pwant = tzParse(gol, c.ptext)
		}
		cs = append(cs, c)
	}
	return cs
}

const tzCheckXo = `use std/time

fn zone_for(off: Int) -> time.Zone {
  if off == 0 { time.utc() } else { time.fixed("ZZ", Duration.from_secs_int(off)) }
}

test "std/time formats what Go's Time.Format prints for the same fields" {
  var bad: List[Str] = []
  for (layout, ns, off, want) in FORMATS {
    let got = time.format(Time.from_unix_nanos(ns), zone_for(off), layout)
    if got != want {
      bad.push("${layout} ${ns} ${off}: got ${got} want ${want}")
    }
  }
  if !bad.is_empty() {
    fault("${bad.len()} of ${FORMATS.len()} formats differ, first: ${bad[0]}")
  }
}

test "std/time parses what Go's Time.Parse parses, and refuses no text of Go's own" {
  var bad: List[Str] = []
  for (layout, text, want, changed) in PARSES {
    let got = match time.parse(text, layout, time.utc()) {
      Ok(t) => "ok ${t.unix_nanos()}"
      Err(_) => "err"
    }
    // Go's own text: the same instant. A changed text: Xo may refuse what
    // Go accepts (exactly the layout), but what Xo accepts Go accepts,
    // at the same instant; a text outside a Time ("range") is refused.
    let fine = if !changed { got == want } else if got.starts_with("ok") { got == want } else { true }
    if !fine {
      bad.push("${layout} ${text} (changed ${changed}): got ${got} want ${want}")
    }
  }
  if !bad.is_empty() {
    fault("${bad.len()} of ${PARSES.len()} parses differ, first: ${bad[0]}")
  }
}
`

// tzXoFile is the generated test file.
func tzXoFile(seed int64, cases []tzCase) string {
	var b strings.Builder
	fmt.Fprintf(&b, "// Generated by internal/stdtest/timefuzz_test.go (seed %d, %d cases) from Go's\n", seed, len(cases))
	b.WriteString("// time package; do not edit: XO_TIMEFUZZ_WRITE=1 go test ./internal/stdtest -run TestTimeFuzzCorpusCurrent\n")
	b.WriteString("// rewrites it.\n")
	b.WriteString(tzCheckXo)
	b.WriteString("\n/// (Xo layout, Unix nanoseconds, zone offset in seconds, what Go's Format prints with the twin layout)\nconst FORMATS: List[(Str, Int, Int, Str)] = [\n")
	for _, c := range cases {
		fmt.Fprintf(&b, "  (%s, %d, %d, %s),\n", xoStr(c.xo), c.ns, c.off, xoStr(c.text))
	}
	b.WriteString("]\n\n/// (Xo layout, text, Go's Parse with the twin layout: `ok UNIX_NS`, `err` or `range`, whether the text was changed)\nconst PARSES: List[(Str, Str, Str, Bool)] = [\n")
	for _, c := range cases {
		if c.zoneOnly {
			continue
		}
		fmt.Fprintf(&b, "  (%s, %s, %s, %t),\n", xoStr(c.xo), xoStr(c.ptext), xoStr(c.pwant), c.changed)
	}
	b.WriteString("]\n")
	return b.String()
}

// tzCoverage fails when the cases stop reaching parse's outcomes.
func tzCoverage(t *testing.T, cases []tzCase) {
	t.Helper()
	ok, errs, changedOK, h12, frac, offs := 0, 0, 0, 0, 0, 0
	for _, c := range cases {
		switch {
		case strings.HasPrefix(c.pwant, "ok") && c.changed:
			changedOK++
			ok++
		case strings.HasPrefix(c.pwant, "ok"):
			ok++
		case c.pwant == "err":
			errs++
		}
		if strings.Contains(c.xo, "{hour12}") {
			h12++
		}
		if strings.Contains(c.xo, "{frac") {
			frac++
		}
		if strings.Contains(c.xo, "{offset") {
			offs++
		}
	}
	n := len(cases)
	if ok < n/3 || errs < n/10 || changedOK < n/100 || h12 < n/5 || frac < n/3 || offs < n/3 {
		t.Fatalf("coverage fell: %d ok (%d of them changed texts), %d errors, %d 12 hour, %d fraction, %d offset layouts of %d", ok, changedOK, errs, h12, frac, offs, n)
	}
}

const tzCorpus = "testdata/corpus/timefuzz/timefuzz_test.xo"

// TestTimeFuzzCorpusCurrent: the committed corpus is what seed 1 writes.
func TestTimeFuzzCorpusCurrent(t *testing.T) {
	cases := tzCases(rand.New(rand.NewSource(1)), 2000)
	tzCoverage(t, cases)
	text := tzXoFile(1, cases)
	if os.Getenv("XO_TIMEFUZZ_WRITE") == "1" {
		if err := os.MkdirAll(filepath.Dir(tzCorpus), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(tzCorpus, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	have, err := os.ReadFile(tzCorpus)
	if err != nil {
		t.Fatal(err)
	}
	if string(have) != text {
		t.Fatalf("%s is not what seed 1 writes (Go's time or the generator changed): XO_TIMEFUZZ_WRITE=1 go test ./internal/stdtest -run TestTimeFuzzCorpusCurrent", tzCorpus)
	}
}

// TestTimeFuzz runs a fresh seed's cases.
func TestTimeFuzz(t *testing.T) {
	if testing.Short() {
		t.Skip("the time fuzz is a heavy test; its seed 1 corpus runs in TestStdCorpora")
	}
	seed := int64(envInt(t, "XO_TIMEFUZZ_SEED", 2))
	n := envInt(t, "XO_TIMEFUZZ_N", 2000)
	cases := tzCases(rand.New(rand.NewSource(seed)), n)
	runFuzzDir(t, "timefuzz_test.xo", tzXoFile(seed, cases), envBackends("XO_TIMEFUZZ_BACKENDS", []string{"arm64", "llvm"}), len(cases))
}
