package llvm

import (
	"fmt"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/xo-lang/xo/internal/xotool"
)

// TestTimeDiff compares std/time on the native backends (std/native/time.xo
// and std/llrt/xo_time.c, decision 0094) with the Go backend (Go's time
// package) byte for byte: zone equality and Debug texts, civil readings,
// offsets, abbreviations, every
// layout field formatted and parsed back, instants of civil times with
// each Ambiguity, parse errors of mutated texts, Time.parse_rfc3339,
// check_layout of random layouts, and Date arithmetic (negative years
// too). The instants are random (the whole range of Time, 1900 to 2100,
// near now), every kind of edge (leap days, the ends of Time), and
// instants around the daylight saving transitions of zones with odd
// rules (half hour and 45 minute offsets, negative DST, two hour DST,
// a skipped day, Ramadan suspensions), found with the host's zone
// database. One program names its zones as constants (per zone TZif
// data); another reads them from its arguments through `time.zone` as a
// function value (the embedded database).
// The native programs must free every allocation.
func TestTimeDiff(t *testing.T) {
	t.Parallel() // its programs build in t.TempDir()
	if _, err := xotool.Clang(); err != nil {
		t.Skip("clang not found: the LLVM backend tests need it")
	}
	backends := []string{xotool.BackendLLVM}
	if runtime.GOARCH == "arm64" && (runtime.GOOS == "darwin" || runtime.GOOS == "linux") {
		backends = append(backends, xotool.BackendArm64)
	}
	named := []string{"America/New_York", "Europe/London", "Australia/Lord_Howe", "Asia/Kolkata", "Pacific/Chatham",
		"America/St_Johns", "Europe/Dublin", "Africa/Casablanca", "America/Sao_Paulo", "Antarctica/Troll",
		"Pacific/Apia", "America/Santiago", "Europe/Moscow", "Asia/Tehran", "America/Nuuk", "Pacific/Kiritimati",
		"Etc/GMT-14", "Etc/UTC", "UTC"}
	fixed := []struct {
		name string
		secs int
	}{{"UTC+5:30", 19800}, {"X-3", -10800}, {"", 0}, {"Far+14", 50400}, {"Odd-0:45:30", -2730}, {"Day+24", 86400}}
	scale := 1
	if testing.Short() {
		scale = 4
	}
	src := timeDiffBody(t, named, len(fixed), scale)
	var fixedSrc strings.Builder
	for _, f := range fixed {
		fmt.Fprintf(&fixedSrc, "  zones.push(time.fixed(r\"%s\", Duration.from_secs_int(%d)))\n", f.name, f.secs)
	}
	var constSrc strings.Builder
	constSrc.WriteString("fn main(os: Os) {\n  var zones: List[time.Zone] = []\n")
	for _, n := range named {
		fmt.Fprintf(&constSrc, "  zones.push(time.zone(r\"%s\").or_fault(r\"%s\"))\n", n, n)
	}
	constSrc.WriteString(fixedSrc.String() + "  run(os, zones)\n}\n")
	dynSrc := "fn main(os: Os) uses env, stdio {\n  var zones: List[time.Zone] = []\n  let load = time.zone\n  for name in os.env.args() {\n    zones.push(load(name).or_fault(name))\n  }\n" + fixedSrc.String() + "  run(os, zones)\n}\n"
	for _, prog := range []struct {
		name, main, args string
	}{
		{"const", constSrc.String(), ""},
		{"dynamic", dynSrc, strings.Join(named, " ")},
	} {
		prog := prog
		t.Run(prog.name, func(t *testing.T) {
			t.Parallel()
			dir := filepath.Join(t.TempDir(), "timediff")
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, "main.xo"), []byte(src+prog.main), 0o644); err != nil {
				t.Fatal(err)
			}
			if prog.args != "" {
				if err := os.WriteFile(filepath.Join(dir, "args.txt"), []byte(prog.args), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			goRes := buildAndRun(t, dir, xotool.Options{Backend: xotool.BackendGo})
			if goRes.exit != 0 {
				t.Fatalf("the Go backend's run failed:\n%s", goRes)
			}
			if out := os.Getenv("XO_TIMEDIFF_OUT"); out != "" {
				// keep the program and its output for a look by hand
				os.WriteFile(out+"."+prog.name+".xo", []byte(src+prog.main), 0o644)
				os.WriteFile(out+"."+prog.name+".txt", []byte(goRes.stdout), 0o644)
			}
			for _, be := range backends {
				res, b := buildRun(t, dir, xotool.Options{Backend: be})
				if goRes.String() != res.String() {
					t.Errorf("%s backend differs from the Go backend:\n%s", be, firstDiff(goRes.String(), res.String()))
				}
				leakCheck(t, b.Binaries[xotool.Host()], dir)
			}
			t.Logf("%d lines compared", strings.Count(goRes.stdout, "\n"))
		})
	}
}

// firstDiff shows the first differing line of two outputs with context.
func firstDiff(a, b string) string {
	al, bl := strings.Split(a, "\n"), strings.Split(b, "\n")
	for i := 0; i < len(al) || i < len(bl); i++ {
		var x, y string
		if i < len(al) {
			x = al[i]
		}
		if i < len(bl) {
			y = bl[i]
		}
		if x != y {
			lo := max(0, i-3)
			return fmt.Sprintf("line %d\n--- go\n%s\n--- native\n%s", i+1, strings.Join(al[lo:min(len(al), i+4)], "\n"), strings.Join(bl[lo:min(len(bl), i+4)], "\n"))
		}
	}
	return "(same lines)"
}

// timeDiffBody is the Xo program without its main: the cases as list
// literals and the code that prints them.
func timeDiffBody(t *testing.T, named []string, nfixed, scale int) string {
	r := rand.New(rand.NewSource(94))
	nz := len(named) + nfixed
	var zi, ns []int64
	add := func(z int, n int64) {
		zi = append(zi, int64(z))
		ns = append(ns, n)
	}
	y1900, y2100 := time.Date(1900, 1, 1, 0, 0, 0, 0, time.UTC).UnixNano(), time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC).UnixNano()
	now := time.Now().UnixNano()
	edges := []int64{0, -1, 1, math.MinInt64, math.MaxInt64, math.MinInt64 + 86400e9, math.MaxInt64 - 86400e9,
		time.Date(2024, 2, 29, 23, 59, 59, 999999999, time.UTC).UnixNano(), time.Date(2000, 2, 29, 12, 0, 0, 0, time.UTC).UnixNano(),
		time.Date(1900, 3, 1, 0, 0, 0, 0, time.UTC).UnixNano(), time.Date(2100, 2, 28, 23, 0, 0, 0, time.UTC).UnixNano(),
		time.Date(1970, 1, 1, 0, 0, 0, 120000000, time.UTC).UnixNano(), time.Date(2037, 12, 31, 23, 59, 59, 0, time.UTC).UnixNano(),
		time.Date(2038, 1, 19, 3, 14, 8, 0, time.UTC).UnixNano(), time.Date(2262, 4, 11, 0, 0, 0, 0, time.UTC).UnixNano(),
		time.Date(1677, 9, 21, 0, 13, 0, 0, time.UTC).UnixNano()}
	for z := 0; z < nz; z++ {
		for _, e := range edges {
			add(z, e)
		}
		for i := 0; i < 60/scale; i++ {
			add(z, int64(r.Uint64()))
			add(z, y1900+r.Int63n(y2100-y1900))
			add(z, now+r.Int63n(4*365*86400e9)-2*365*86400e9)
			add(z, (y1900+r.Int63n(y2100-y1900))/1e9*1e9)
		}
	}
	// instants around transitions, and civil probes inside the gaps and
	// overlaps (the wall clock of the old offset just after the change)
	var probes [][6]int64
	for z, name := range named {
		loc, err := time.LoadLocation(name)
		if err != nil {
			continue
		}
		var trans []int64
		key := func(s int64) string {
			n, off := time.Unix(s, 0).In(loc).Zone()
			return fmt.Sprintf("%s%d", n, off)
		}
		start := time.Date(1800, 1, 1, 0, 0, 0, 0, time.UTC).Unix()
		end := time.Date(2200, 1, 1, 0, 0, 0, 0, time.UTC).Unix()
		prev := key(start)
		for s := start + 5*86400; s < end; s += 5 * 86400 {
			k := key(s)
			if k != prev {
				lo, hi := s-5*86400, s
				for hi-lo > 1 {
					m := lo + (hi-lo)/2
					if key(m) == key(lo) {
						lo = m
					} else {
						hi = m
					}
				}
				trans = append(trans, hi)
				prev = k
			}
		}
		keep := 40 / scale
		for i, tr := range trans {
			if len(trans) > keep && i > 3 && i < len(trans)-4 && r.Intn(len(trans)) >= keep {
				continue
			}
			add(z, (tr-1)*1e9)
			add(z, tr*1e9)
			add(z, tr*1e9+r.Int63n(1e9))
			_, before := time.Unix(tr-1, 0).In(loc).Zone()
			wall := time.Unix(tr+int64(before), 0).UTC().Add(time.Duration(r.Intn(3600)) * time.Second)
			probes = append(probes, [6]int64{int64(z), int64(wall.Year()), int64(wall.Month()), int64(wall.Day()), int64(wall.Hour()), int64(wall.Minute())})
		}
	}
	// texts to parse: Go formatted RFC 3339 and RFC 1123 texts, some
	// mutated
	alphabet := "0123456789:-+TZ .,ABCabcDEGMPST/"
	var ps []string
	var pl, pz []int64
	goLayouts := []string{"2006-01-02T15:04:05.999999999Z07:00", "Mon, 02 Jan 2006 15:04:05 MST", "", "", "2006-01-02", "15:04:05"}
	for i := 0; i < 1500/scale; i++ {
		z := r.Intn(nz)
		l := []int{0, 0, 1, 4, 5}[r.Intn(5)]
		var loc *time.Location = time.UTC
		if z < len(named) {
			if x, err := time.LoadLocation(named[z]); err == nil {
				loc = x
			}
		}
		s := time.Unix(0, y1900+r.Int63n(y2100-y1900)).In(loc).Format(goLayouts[l])
		if r.Intn(3) > 0 && len(s) > 0 {
			b := []byte(s)
			switch r.Intn(3) {
			case 0:
				k := r.Intn(len(b))
				b = append(b[:k:k], b[k+1:]...)
			case 1:
				k := r.Intn(len(b) + 1)
				b = append(b[:k:k], append([]byte{alphabet[r.Intn(len(alphabet))]}, b[k:]...)...)
			default:
				b[r.Intn(len(b))] = alphabet[r.Intn(len(alphabet))]
			}
			s = string(b)
		}
		ps = append(ps, s)
		pl = append(pl, int64(l))
		pz = append(pz, int64(z))
	}
	ps = append(ps, "2026-01-02T3:04:05Z", "2026-01-02T03:04:05,5Z", "2026-01-02T03:04:05.1234567891Z", "2026-01-02T03:04:05+24:60",
		"0000-01-01T00:00:00Z", "9999-12-31T23:59:59Z", "2026-02-29T00:00:00Z", "2024-02-29T00:00:00-00:00", "2026-01-02T03:04:05.Z",
		"2026-01-02T03:04:60Z", "2026-01-02T24:00:00Z", "2026-1-02T03:04:05Z", "2026-01-02t03:04:05z", "2026-01-02T03:04:05+0100")
	for len(pl) < len(ps) {
		pl = append(pl, 0)
		pz = append(pz, 0)
	}
	// layouts from fragments for check_layout
	frags := []string{"{year}", "{yaer}", "{", "}", "{month:", "{frac:0}", "{frac:9}", "{hour12}", "{ampm}", "x", "{lbrace}", "{rbrace}",
		"{offset:compact}", "{zone}", "{month:long}", "{day:1}", "{weekday}", ":", "{}", "{:}", "{year:1}", "{frac:10}", "-"}
	var layouts []string
	for i := 0; i < 200/scale; i++ {
		var b strings.Builder
		for k := 0; k <= r.Intn(3); k++ {
			b.WriteString(frags[r.Intn(len(frags))])
		}
		layouts = append(layouts, b.String())
	}
	// dates, some out of range and with negative years
	var dy, dm, dd, dn []int64
	for i := 0; i < 400/scale; i++ {
		dy = append(dy, []int64{int64(r.Intn(10100)) - 50, int64(r.Intn(400)) + 1800, int64(r.Intn(40)) - 20}[r.Intn(3)])
		dm = append(dm, int64(r.Intn(18))-2)
		dd = append(dd, int64(r.Intn(34))-1)
		dn = append(dn, int64(r.Intn(200001))-100000)
	}
	// the cases are texts the program splits: list literals this long
	// make a function too large for the arm64 backend's branches
	// (gotchas/arm64-tbz-range.md) and slow to compile
	ints := func(xs []int64) string {
		var b strings.Builder
		b.WriteString(`ints(r"`)
		for i, x := range xs {
			if i > 0 {
				b.WriteString(" ")
			}
			fmt.Fprint(&b, x)
		}
		b.WriteString(`")`)
		return b.String()
	}
	strs := func(xs []string) string {
		for _, x := range xs {
			if strings.ContainsAny(x, `"|`) {
				t.Fatalf("a quote or bar in %q", x)
			}
		}
		return `strs(r"` + strings.Join(xs, "|") + `")`
	}
	var pcol [6][]int64
	for _, p := range probes {
		for k := range p {
			pcol[k] = append(pcol[k], p[k])
		}
	}
	var b strings.Builder
	b.WriteString(timeDiffCode)
	b.WriteString("fn run(os: Os, zones: List[time.Zone]) {\n")
	fmt.Fprintf(&b, "  let zi: List[Int] = %s\n  let ns: List[Int] = %s\n", ints(zi), ints(ns))
	fmt.Fprintf(&b, "  let pzi: List[Int] = %s\n  let py: List[Int] = %s\n  let pmo: List[Int] = %s\n  let pd: List[Int] = %s\n  let ph: List[Int] = %s\n  let pmi: List[Int] = %s\n", ints(pcol[0]), ints(pcol[1]), ints(pcol[2]), ints(pcol[3]), ints(pcol[4]), ints(pcol[5]))
	fmt.Fprintf(&b, "  let ps: List[Str] = %s\n  let pl: List[Int] = %s\n  let pz: List[Int] = %s\n", strs(ps), ints(pl), ints(pz))
	fmt.Fprintf(&b, "  let ls: List[Str] = %s\n", strs(layouts))
	fmt.Fprintf(&b, "  let dy: List[Int] = %s\n  let dm: List[Int] = %s\n  let dd: List[Int] = %s\n  let dn: List[Int] = %s\n", ints(dy), ints(dm), ints(dd), ints(dn))
	b.WriteString(timeDiffRun)
	return b.String()
}

const timeDiffCode = `use std/time

const FULL: Str = "{weekday:name} {weekday:short} {month:name} {month:short} {month:1} {month} {day:1} {day} {year} {hour12}{ampm} {hour} {minute} {second}{frac} {frac:3} {offset:compact} {offset} {zone} {lbrace}{rbrace}"
const NOOFF: Str = "{year}-{month}-{day} {hour}:{minute}:{second}{frac:9}"

fn ints(s: Str) -> List[Int] {
  var out: List[Int] = []
  if s == "" {
    return out
  }
  for f in s.split(" ") {
    out.push(Int.parse(f) ?? 0)
  }
  out
}

fn strs(s: Str) -> List[Str] {
  if s == "" {
    return []
  }
  s.split("|")
}

fn layout(i: Int) -> Str {
  match i {
    0 => time.RFC3339
    1 => time.RFC1123
    2 => FULL
    3 => NOOFF
    4 => time.DATE
    _ => time.TIME
  }
}

fn show(os: Os, z: time.Zone, t: Time) {
  let c = time.civil(t, z)
  os.stdio.println("${z} ${t} | ${c.date} ${c.time.hour}:${c.time.minute}:${c.time.second}.${c.time.nanos} ${c.offset} ${c.zone} | ${z.offset_at(t)} ${z.abbreviation_at(t)} ${c.date.weekday()}")
  for i in 0..4 {
    let s = time.format(t, z, layout(i))
    match time.parse(s, layout(i), z) {
      Ok(x) => os.stdio.println("  ${s} -> ${x} ${x == t}")
      Err(e) => os.stdio.println("  ${s} -> ${e}")
    }
  }
  for a in [time.Ambiguity.Reject, time.Ambiguity.Earlier, time.Ambiguity.Later] {
    match time.instant(c.date, c.time, z, a) {
      Ok(x) => os.stdio.println("  instant ${x}")
      Err(e) => os.stdio.println("  instant ${e}")
    }
  }
}

fn probe(os: Os, z: time.Zone, y: Int, mo: Int, d: Int, h: Int, mi: Int) {
  let date = time.Date{year: y, month: mo, day: d}
  let clock = time.ClockTime{hour: h, minute: mi, second: 0, nanos: 0}
  var line = "probe ${z} ${date} ${h}:${mi}"
  for a in [time.Ambiguity.Reject, time.Ambiguity.Earlier, time.Ambiguity.Later] {
    match time.instant(date, clock, z, a) {
      Ok(x) => {
        line = line + " | ${x} ${time.format(x, z, FULL)}"
      }
      Err(e) => {
        line = line + " | ${e}"
      }
    }
  }
  os.stdio.println(line)
  let text = "${date} ${h}:${mi}"
  match time.parse("${date} ${clock.hour / 10}${clock.hour % 10}:${clock.minute / 10}${clock.minute % 10}:00.000000000", NOOFF, z) {
    Ok(x) => os.stdio.println("  ${text} -> ${x}")
    Err(e) => os.stdio.println("  ${text} -> ${e}")
  }
}

`

const timeDiffRun = `  for (i, z) in zones.indexed() {
    var eqs = ""
    for o in zones {
      if z == o {
        eqs = eqs + "1"
      } else {
        eqs = eqs + "0"
      }
    }
    os.stdio.println("zone ${i} ${z.debug()} ${z.name()} ${eqs} ${z == time.utc()}")
  }
  for (i, k) in zi.indexed() {
    show(os, zones[k], Time.UNIX_EPOCH + Duration.from_nanos(ns[i]))
  }
  for (i, k) in pzi.indexed() {
    probe(os, zones[k], py[i], pmo[i], pd[i], ph[i], pmi[i])
  }
  for (i, s) in ps.indexed() {
    var line = "parse ${s}"
    match time.parse(s, layout(pl[i]), zones[pz[i]]) {
      Ok(x) => {
        line = line + " -> ${x}"
      }
      Err(e) => {
        line = line + " -> ${e}"
      }
    }
    match Time.parse_rfc3339(s) {
      Some(x) => {
        line = line + " | ${x}"
      }
      None => {
        line = line + " | none"
      }
    }
    os.stdio.println(line)
  }
  for l in ls {
    match time.check_layout(l) {
      Ok(_) => os.stdio.println("layout ${l} ok")
      Err(e) => os.stdio.println("layout ${e}")
    }
  }
  for i in 0..dy.len() {
    match time.date(dy[i], dm[i], dd[i]) {
      Ok(d) => os.stdio.println("date ${d} ${d.weekday()}")
      Err(e) => os.stdio.println("date ${e}")
    }
    let d = time.Date{year: dy[i], month: dm[i], day: dd[i]}
    let j = (i + 1) % dy.len()
    let o = time.Date{year: dy[j], month: dm[j], day: dd[j]}
    let c1 = time.ClockTime{hour: dm[i], minute: dd[i], second: 0, nanos: dn[i]}
    let c2 = time.ClockTime{hour: dm[j], minute: dd[j], second: 0, nanos: dn[j]}
    os.stdio.println("  ${d} ${d.weekday()} ${d.add_days(dn[i])} ${d.add_months(dn[i] % 40)} ${d.days_until(o)} ${d < o} ${d == o} ${d.weekday() < o.weekday()} ${c1 < c2} ${c1 == c2}")
  }
}
`

// TestZoneDataPerUseNative read the zone data file (xo_tzdata.c) the Go
// build package wrote into its build directory; it went with decision
// 0215 (xo's zone data selection is selfhost/xo tz.xo; the runs of the
// time goldens check what it embeds).
