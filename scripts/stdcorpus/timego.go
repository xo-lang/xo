package main

import (
	"bytes"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// timegoLayouts are Go layouts every element of which core time has a field for (std/timego translates them);
// elements without one are LayoutErr cases, checked by the native tests.
var timegoLayouts = []string{
	"2006-01-02", "2006-01-02T15:04:05Z07:00", "2006-01-02 15:04:05", "Mon Jan 2 15:04:05 MST 2006",
	"Monday, 02-Jan-2006", "Jan 2, 2006 at 03:04PM (MST)", "02/01/2006", "15:04:05.000", "15:04:05.000000000",
	"2006-01-02T15:04:05-0700", "2006-01-02T15:04:05Z07:00 MST", "Mon, 02 Jan 2006 15:04:05 MST", "03:04:05PM",
	"January 2, 2006", "1/2/2006", "20060102T150405", "2006-01-02 15:04:05.000 -0700",
}

// timegoRef asks the oracle (internal/stdtest/timeref, Go's time package) for every line of in.
func timegoOracle(in string) []string {
	cmd := exec.Command("go", "run", ".")
	cmd.Dir = filepath.Join("internal", "stdtest", "timeref")
	cmd.Stdin = strings.NewReader(in)
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		fmt.Fprintf(os.Stderr, "stdcorpus: the timego oracle (internal/stdtest/timeref) failed: %v\n", err)
		os.Exit(1)
	}
	return strings.Split(strings.TrimRight(string(out), "\n"), "\n")
}

// timegoNoParse is a layout whose Go output is not parsed back: a time of day alone has no date, and a zone
// abbreviation (MST) without an offset is made up by Go but looked up in the given zone by core time (decision 0537).
func timegoNoParse(l string) bool {
	return l == "15:04:05.000" || l == "15:04:05.000000000" || l == "03:04:05PM" || strings.Contains(l, "MST")
}

// timegoFuzzSeed is XO_TIMEGOFUZZ_SEED (default 0, the committed corpus), added to the seed so a fresh run
// (internal/stdtest TestTimegoFuzz) makes new cases.
func timegoFuzzSeed() int64 {
	n, _ := strconv.ParseInt(os.Getenv("XO_TIMEGOFUZZ_SEED"), 10, 64)
	return n
}

// timegoMutate changes a time text the way a bad input would be: a character replaced, deleted or inserted, or
// the text cut short.
func timegoMutate(r *rand.Rand, in string) string {
	b := []byte(in)
	if len(b) == 0 {
		return in
	}
	const alpha = "0123456789:-+ZTJanuMoeWdy,. "
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

func timegoCorpus() string {
	r := rand.New(rand.NewSource(537 + timegoFuzzSeed()))
	type fcase struct {
		layout string
		secs   int64
		nanos  int64
		off    int
	}
	var fc []fcase
	offs := []int{0, 3600, -18000, 19800, -34200, 45900}
	for _, l := range timegoLayouts {
		for i := 0; i < 24; i++ {
			// 1970 to 2100, any nanosecond
			secs := r.Int63n(130 * 365 * 24 * 3600)
			if i%3 == 0 {
				secs -= r.Int63n(40 * 365 * 24 * 3600)
			}
			fc = append(fc, fcase{l, secs, r.Int63n(1e9), offs[r.Intn(len(offs))]})
		}
	}
	var in bytes.Buffer
	for _, c := range fc {
		fmt.Fprintf(&in, "fmt\t%s\t%d\t%d\n", c.layout, c.secs*1e9+c.nanos, c.off)
	}
	want := timegoOracle(in.String())
	var o strings.Builder
	o.WriteString(header("Go's time.Format and time.Parse (the reference layouts of time/format.go)"))
	o.WriteString("use std/time\nuse std/timego\n\n")
	o.WriteString("/// (Go layout, Unix seconds, nanoseconds, zone offset in seconds, what Go's Format prints)\nconst FORMATS: List[(Str, Int, Int, Int, Str)] = [\n")
	for i, c := range fc {
		fmt.Fprintf(&o, "  (%s, %d, %d, %d, %s),\n", xq(c.layout), c.secs, c.nanos, c.off, xq(want[i]))
	}
	o.WriteString("]\n\n")
	// parse: Go's own output parsed back, offset and instant compared
	var pin bytes.Buffer
	ptext := make([]string, len(fc))
	pmut := make([]bool, len(fc))
	for i, c := range fc {
		if timegoNoParse(c.layout) {
			continue
		}
		ptext[i] = want[i]
		pmut[i] = false
		// a changed date under a weekday element would differ by design: Go ignores the weekday text, core time
		// requires it to match the date (spec/std.md 67)
		if r.Intn(3) == 0 && !strings.Contains(c.layout, "Mon") {
			ptext[i] = timegoMutate(r, want[i])
			pmut[i] = true
		}
		fmt.Fprintf(&pin, "parse\t%s\t%s\n", c.layout, ptext[i])
	}
	pwant := timegoOracle(pin.String())
	o.WriteString("/// (Go layout, text, what Go's Parse answers: `err`, or `ok UNIX_NANOS OFFSET`, and whether the text was changed)\nconst PARSES: List[(Str, Str, Str, Bool)] = [\n")
	k := 0
	for i, c := range fc {
		if timegoNoParse(c.layout) {
			continue
		}
		fmt.Fprintf(&o, "  (%s, %s, %s, %t),\n", xq(c.layout), xq(ptext[i]), xq(pwant[k]), pmut[i])
		k++
	}
	o.WriteString("]\n")
	o.WriteString(timegoCheckXo)
	return o.String()
}

const timegoCheckXo = `
fn zone_for(off: Int) -> time.Zone {
  if off == 0 { time.utc() } else { time.fixed("ZZ", Duration.from_secs_int(off)) }
}

test "std/timego formats what Go's Time.Format prints" {
  expect(timego.to_core("2006-01-02").or_fault("to_core")) == "{year}-{month}-{day}"
  expect(timego.to_core("2006-01-02 pm").is_err()) == true
  expect(timego.layout("2006-01-02").or_fault("layout").display()) == "{year}-{month}-{day}"
  var bad: List[Str] = []
  for (layout, secs, nanos, off, want) in FORMATS {
    let t = Time.from_unix(secs) + Duration.from_nanos(nanos)
    let got = match timego.format(t, zone_for(off), layout) {
      Ok(x) => x
      Err(_) => "layout error"
    }
    if got != want {
      bad.push("${layout} ${secs} ${nanos} ${off}: got ${got} want ${want}")
    }
  }
  if !bad.is_empty() {
    fault("${bad.len()} of ${FORMATS.len()} formats differ, first: ${bad[0]}")
  }
}

test "std/timego parses what Go's Time.Parse parses" {
  var bad: List[Str] = []
  for (layout, text, want, changed) in PARSES {
    let got = match timego.parse(text, layout, time.utc()) {
      Ok(t) => {
        let c = time.civil(t, time.utc())
        "ok ${t.unix() * 1000000000 + c.time.nanos}"
      }
      Err(_) => "err"
    }
    // Go's answer ends with the text's offset, which a Time does not keep: compare the instant
    let parts = want.split(" ")
    let w = if parts[0] == "ok" { "ok ${parts[1]}" } else { "err" }
    // core time is stricter than Go in places (a one digit 15, spec/std.md 67): on a changed text it may refuse what
    // Go accepts, but never accept what Go refuses or read another instant; Go's own output must parse exactly
    let stricter = changed && got == "err" && w != "err"
    if got != w && !stricter {
      bad.push("${layout} ${text}: got ${got} want ${w}")
    }
  }
  if !bad.is_empty() {
    fault("${bad.len()} of ${PARSES.len()} parses differ, first: ${bad[0]}")
  }
}
`
