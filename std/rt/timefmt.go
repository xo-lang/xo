package rt

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"
)

// std/time layouts (std.md 13.3, decision 0094): named, lowercase fields
// with one spelling each. internal/check/timelayout.go checks constant
// layouts at compile time with the same rules (TestTimeLayoutAgrees keeps
// the two equal).

// Time_RFC3339 and the other layout constants of std/time.
const (
	Time_RFC3339 = "{year}-{month}-{day}T{hour}:{minute}:{second}{frac}{offset}"
	Time_RFC1123 = "{weekday:short}, {day} {month:short} {year} {hour}:{minute}:{second} {zone}"
	Time_DATE    = "{year}-{month}-{day}"
	Time_TIME    = "{hour}:{minute}:{second}"
)

// tlItem is one piece of a layout: literal text (field "") or a field.
type tlItem struct {
	lit, field, arg string
}

// tlFieldArgs lists the fields and the arguments each takes ("" is the
// field without an argument).
var tlFieldArgs = map[string][]string{
	"year": {""}, "month": {"", "1", "short", "name"}, "day": {"", "1"},
	"weekday": {"short", "name"}, "hour": {""}, "hour12": {""}, "ampm": {""},
	"minute": {""}, "second": {""}, "frac": {"", "1", "2", "3", "4", "5", "6", "7", "8", "9"},
	"offset": {"", "compact"}, "zone": {""}, "lbrace": {""}, "rbrace": {""},
}

// TimeLayoutParse splits a layout into items, or returns the error
// message and its byte offset in the layout.
func TimeLayoutParse(layout string) ([]tlItem, string, int) {
	var items []tlItem
	var lit strings.Builder
	flush := func() {
		if lit.Len() > 0 {
			items = append(items, tlItem{lit: lit.String()})
			lit.Reset()
		}
	}
	hour12, ampm := -1, false
	for i := 0; i < len(layout); i++ {
		switch layout[i] {
		case '}':
			return nil, "unmatched } (a literal brace is written {rbrace})", i
		case '{':
		default:
			lit.WriteByte(layout[i])
			continue
		}
		j := strings.IndexAny(layout[i+1:], "{}")
		if j < 0 || layout[i+1+j] == '{' {
			return nil, "unclosed { (a literal brace is written {lbrace})", i
		}
		body := layout[i+1 : i+1+j]
		name, arg, hasArg := strings.Cut(body, ":")
		args, ok := tlFieldArgs[name]
		if !ok {
			return nil, "unknown field {" + body + "}", i
		}
		if hasArg && arg == "" || !hasArg && !contains(args, "") || hasArg && !contains(args, arg) {
			return nil, "{" + body + "}: " + tlArgsText(name, args), i
		}
		switch name {
		case "lbrace":
			lit.WriteByte('{')
		case "rbrace":
			lit.WriteByte('}')
		default:
			flush()
			items = append(items, tlItem{field: name, arg: arg})
			if name == "hour12" && hour12 < 0 {
				hour12 = i
			}
			if name == "ampm" {
				ampm = true
			}
		}
		i += 1 + j
	}
	flush()
	if hour12 >= 0 && !ampm {
		return nil, "{hour12} needs {ampm} (or use {hour}, 00 to 23)", hour12
	}
	return items, "", 0
}

func contains(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}

// tlArgsText says which forms a field has.
func tlArgsText(name string, args []string) string {
	var forms []string
	for _, a := range args {
		if a == "" {
			forms = append(forms, "{"+name+"}")
		} else {
			forms = append(forms, "{"+name+":"+a+"}")
		}
	}
	if name == "frac" {
		forms = []string{"{frac}", "{frac:1} to {frac:9}"}
	}
	return "the forms are " + strings.Join(forms, ", ")
}

var tlCache sync.Map // layout -> []tlItem

// timeLayout returns the parsed layout or faults (a bad layout built at
// run time is a bug, like a bad index; check_layout returns a Result).
func timeLayout(fn, layout string) []tlItem {
	if v, ok := tlCache.Load(layout); ok {
		return v.([]tlItem)
	}
	items, msg, _ := TimeLayoutParse(layout)
	if msg != "" {
		FaultHere("time.%s: bad layout %s: %s (check layouts built at run time with time.check_layout)", fn, strconv.Quote(layout), msg)
	}
	tlCache.Store(layout, items)
	return items
}

// M_display of a LayoutErr.
func (e Time_LayoutErr) M_display(*Task) string {
	return "bad layout " + strconv.Quote(e.F_layout) + ": " + e.F_msg
}

// M_display of a ParseErr: `at byte 5 of "2026-1x-09": expected 2 digit month`.
func (e Time_ParseErr) M_display(*Task) string {
	return fmt.Sprintf("at byte %d of %s: %s", e.F_at, strconv.Quote(e.F_input), e.F_msg)
}

// Time_check_layout is `time.check_layout(layout)`.
func Time_check_layout(_ *Task, layout string) Result[Unit, Time_LayoutErr] {
	if _, msg, _ := TimeLayoutParse(layout); msg != "" {
		return Err[Unit](Time_LayoutErr{F_layout: layout, F_msg: msg})
	}
	return Ok[Unit, Time_LayoutErr](Unit{})
}

var (
	monthShort = []string{"Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"}
	monthName  = []string{"January", "February", "March", "April", "May", "June", "July", "August", "September", "October", "November", "December"}
	// Monday first, as Weekday
	weekdayShort = []string{"Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"}
	weekdayName  = []string{"Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday", "Sunday"}
)

// Time_format is `time.format(t, z, layout)`.
func Time_format(_ *Task, t Time, z Time_Zone, layout string) string {
	return formatItems(timeLayout("format", layout), t, z)
}

func formatItems(items []tlItem, t Time, z Time_Zone) string {
	gt := goTime(t, z)
	abbr, off := gt.Zone()
	var b strings.Builder
	two := func(n int) {
		b.WriteByte(byte('0' + n/10%10))
		b.WriteByte(byte('0' + n%10))
	}
	for _, it := range items {
		switch it.field {
		case "":
			b.WriteString(it.lit)
		case "year":
			fmt.Fprintf(&b, "%04d", gt.Year())
		case "month":
			switch it.arg {
			case "":
				two(int(gt.Month()))
			case "1":
				b.WriteString(strconv.Itoa(int(gt.Month())))
			case "short":
				b.WriteString(monthShort[gt.Month()-1])
			case "name":
				b.WriteString(monthName[gt.Month()-1])
			}
		case "day":
			if it.arg == "1" {
				b.WriteString(strconv.Itoa(gt.Day()))
			} else {
				two(gt.Day())
			}
		case "weekday":
			w := goWeekdayTag(gt.Weekday()) - 1
			if it.arg == "short" {
				b.WriteString(weekdayShort[w])
			} else {
				b.WriteString(weekdayName[w])
			}
		case "hour":
			two(gt.Hour())
		case "hour12":
			h := gt.Hour() % 12
			if h == 0 {
				h = 12
			}
			two(h)
		case "ampm":
			if gt.Hour() < 12 {
				b.WriteString("AM")
			} else {
				b.WriteString("PM")
			}
		case "minute":
			two(gt.Minute())
		case "second":
			two(gt.Second())
		case "frac":
			digits := fmt.Sprintf("%09d", gt.Nanosecond())
			if it.arg == "" {
				digits = strings.TrimRight(digits, "0")
			} else {
				digits = digits[:it.arg[0]-'0']
			}
			if digits != "" {
				b.WriteByte('.')
				b.WriteString(digits)
			}
		case "offset":
			if off == 0 && it.arg == "" {
				b.WriteByte('Z')
				continue
			}
			// Go's sign is the offset's in whole minutes: -59s is +00:00
			sign := byte('+')
			if off/60 < 0 {
				sign = '-'
			}
			neg := off < 0
			if neg {
				off = -off
			}
			b.WriteByte(sign)
			two(off / 3600)
			if it.arg == "" {
				b.WriteByte(':')
			}
			two(off / 60 % 60)
			if neg {
				off = -off
			}
		case "zone":
			b.WriteString(abbr)
		}
	}
	return b.String()
}

// timeParser holds the fields read by parse.
type timeParser struct {
	s, layout string
	pos       int
}

func (p *timeParser) fail(at int, format string, args ...any) Result[Time, Time_ParseErr] {
	return Err[Time](Time_ParseErr{F_input: p.s, F_layout: p.layout, F_at: int64(at), F_msg: fmt.Sprintf(format, args...)})
}

// digits reads between lo and hi ASCII digits.
func (p *timeParser) digits(lo, hi int) (int64, bool) {
	n, v := 0, int64(0)
	for n < hi && p.pos+n < len(p.s) && p.s[p.pos+n] >= '0' && p.s[p.pos+n] <= '9' {
		v = v*10 + int64(p.s[p.pos+n]-'0')
		n++
	}
	if n < lo {
		return 0, false
	}
	p.pos += n
	return v, true
}

// word reads one of the names, longest first.
func (p *timeParser) word(names []string) (int, bool) {
	best := -1
	for i, n := range names {
		if strings.HasPrefix(p.s[p.pos:], n) && (best < 0 || len(n) > len(names[best])) {
			best = i
		}
	}
	if best < 0 {
		return 0, false
	}
	p.pos += len(names[best])
	return best, true
}

func digitText(lo, hi int) string {
	if lo == hi {
		return fmt.Sprintf("%d digit", lo)
	}
	if hi == lo+1 {
		return fmt.Sprintf("%d or %d digit", lo, hi)
	}
	return fmt.Sprintf("%d to %d digit", lo, hi)
}

// Time_parse is `time.parse(s, layout, z)`: strict, exactly the layout.
func Time_parse(_ *Task, s, layout string, z Time_Zone) Result[Time, Time_ParseErr] {
	return parseItems(timeLayout("parse", layout), s, layout, z)
}

func parseItems(items []tlItem, s, layout string, z Time_Zone) Result[Time, Time_ParseErr] {
	p := &timeParser{s: s, layout: layout}
	year, month, day := int64(1970), int64(1), int64(1)
	var hour, minute, second, nanos, hour12 int64
	var off int64
	hasOff, has12, pm := false, false, false
	weekday, weekdayAt := -1, 0
	abbr, abbrAt := "", -1
	dayAt := 0
	for _, it := range items {
		at := p.pos
		if it.field == "" {
			if !strings.HasPrefix(s[p.pos:], it.lit) {
				return p.fail(at, "expected %s", strconv.Quote(it.lit))
			}
			p.pos += len(it.lit)
			continue
		}
		num := func(what string, lo, hi int, min, max int64) (int64, bool, Result[Time, Time_ParseErr]) {
			v, ok := p.digits(lo, hi)
			if !ok {
				return 0, false, p.fail(at, "expected %s %s", digitText(lo, hi), what)
			}
			if v < min || v > max {
				return 0, false, p.fail(at, "%s %d out of range %d to %d", what, v, min, max)
			}
			return v, true, Result[Time, Time_ParseErr]{}
		}
		var ok bool
		var bad Result[Time, Time_ParseErr]
		switch it.field {
		case "year":
			year, ok, bad = num("year", 4, 4, 1, 9999)
		case "month":
			switch it.arg {
			case "", "1":
				lo := 2
				if it.arg == "1" {
					lo = 1
				}
				month, ok, bad = num("month", lo, 2, 1, 12)
			case "short", "name":
				names := monthShort
				if it.arg == "name" {
					names = monthName
				}
				var i int
				if i, ok = p.word(names); !ok {
					return p.fail(at, "expected a month name such as %s", strconv.Quote(names[0]))
				}
				month = int64(i + 1)
			}
		case "day":
			dayAt = at
			lo := 2
			if it.arg == "1" {
				lo = 1
			}
			day, ok, bad = num("day", lo, 2, 1, 31)
		case "weekday":
			names := weekdayShort
			if it.arg == "name" {
				names = weekdayName
			}
			if weekday, ok = p.word(names); !ok {
				return p.fail(at, "expected a weekday name such as %s", strconv.Quote(names[0]))
			}
			weekdayAt = at
		case "hour":
			hour, ok, bad = num("hour", 2, 2, 0, 23)
		case "hour12":
			hour12, ok, bad = num("hour", 2, 2, 1, 12)
			has12 = true
		case "ampm":
			switch {
			case strings.HasPrefix(s[p.pos:], "AM"):
			case strings.HasPrefix(s[p.pos:], "PM"):
				pm = true
			default:
				return p.fail(at, "expected AM or PM")
			}
			p.pos += 2
			ok = true
		case "minute":
			minute, ok, bad = num("minute", 2, 2, 0, 59)
		case "second":
			second, ok, bad = num("second", 2, 2, 0, 59)
		case "frac":
			n := 0
			if it.arg != "" {
				n = int(it.arg[0] - '0')
			}
			if p.pos >= len(s) || s[p.pos] != '.' {
				if n > 0 {
					return p.fail(at, "expected \".\" and %d digit fraction", n)
				}
				ok = true
				break
			}
			p.pos++
			start := p.pos
			lo, hi := 1, 9
			if n > 0 {
				lo, hi = n, n
			}
			var v int64
			if v, ok = p.digits(lo, hi); !ok {
				return p.fail(start, "expected %s fraction", digitText(lo, hi))
			}
			for k := p.pos - start; k < 9; k++ {
				v *= 10
			}
			nanos = v
		case "offset":
			if it.arg == "" && strings.HasPrefix(s[p.pos:], "Z") {
				p.pos++
				hasOff, ok, off = true, true, 0
				break
			}
			if p.pos >= len(s) || (s[p.pos] != '+' && s[p.pos] != '-') {
				if it.arg == "" {
					return p.fail(at, "expected an offset: Z or +hh:mm")
				}
				return p.fail(at, "expected an offset: +hhmm")
			}
			neg := s[p.pos] == '-'
			p.pos++
			var h, m int64
			if h, ok = p.digits(2, 2); ok {
				if it.arg == "" {
					ok = strings.HasPrefix(s[p.pos:], ":")
					p.pos++
				}
				if ok {
					m, ok = p.digits(2, 2)
				}
			}
			if !ok || h > 23 || m > 59 {
				if it.arg == "" {
					return p.fail(at, "expected an offset: Z or +hh:mm")
				}
				return p.fail(at, "expected an offset: +hhmm")
			}
			off = h*3600 + m*60
			if neg {
				off = -off
			}
			hasOff = true
		case "zone":
			n := 0
			for p.pos+n < len(s) {
				c := s[p.pos+n]
				if c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '+' || c == '-' {
					n++
					continue
				}
				break
			}
			if n == 0 {
				return p.fail(at, "expected a zone abbreviation such as \"UTC\"")
			}
			abbr, abbrAt = s[p.pos:p.pos+n], at
			p.pos += n
			ok = true
		}
		if !ok {
			return bad
		}
	}
	if p.pos < len(s) {
		return p.fail(p.pos, "extra text after the layout: %s", strconv.Quote(s[p.pos:]))
	}
	if day > daysIn(year, month) {
		return p.fail(dayAt, "day %d out of range 1 to %d for %04d-%02d", day, daysIn(year, month), year, month)
	}
	if has12 {
		hour = hour12 % 12
		if pm {
			hour += 12
		}
	}
	days := daysFromCivil(year, month, day)
	if weekday >= 0 {
		w := days + 3
		if want := int(w - floorDiv(w, 7)*7); want != weekday {
			return p.fail(weekdayAt, "weekday %s does not match %04d-%02d-%02d, a %s", weekdayName[weekday], year, month, day, weekdayName[want])
		}
	}
	local := days*86400 + hour*3600 + minute*60 + second
	at := func() string {
		return fmt.Sprintf("%04d-%02d-%02dT%02d:%02d:%02d", year, month, day, hour, minute, second)
	}
	var secs int64
	switch {
	case hasOff:
		secs = local - off
	case abbr != "":
		early, late, _ := civilInstant(local, z)
		found := false
		for _, c := range []int64{early, late} {
			if c > minTimeSecs && c < maxTimeSecs {
				if a, _ := time.Unix(c, 0).In(z.data().loc).Zone(); a == abbr {
					secs, found = c, true
					break
				}
			}
		}
		if !found {
			if abbr != "UTC" && abbr != "GMT" {
				return p.fail(abbrAt, "zone abbreviation %s is not used by %s at %s", strconv.Quote(abbr), z.data().name, at())
			}
			secs = local
		}
	default:
		early, _, kind := civilInstant(local, z)
		switch kind {
		case 2:
			return p.fail(0, "%s does not exist in %s (skipped by a daylight saving change); the layout needs {offset}", at(), z.data().name)
		case 3:
			return p.fail(0, "%s happens twice in %s (repeated by a daylight saving change); the layout needs {offset} or {zone}", at(), z.data().name)
		}
		secs = early
	}
	if secs <= minTimeSecs || secs >= maxTimeSecs {
		return p.fail(0, "%s is outside the range of Time (1678 to 2262)", at())
	}
	return Ok[Time, Time_ParseErr](Time(secs*1e9 + nanos))
}

// Time_Layout is time.Layout: a layout parsed once (decision 0332).
type Time_Layout struct {
	text  string
	items []tlItem
}

// Time_layout is `time.layout(text)`; a bad layout faults.
func Time_layout(_ *Task, text string) Time_Layout {
	return Time_Layout{text: text, items: timeLayout("layout", text)}
}

func (l Time_Layout) XoEq(x any) bool {
	o, ok := x.(Time_Layout)
	return ok && o.text == l.text
}

func (l Time_Layout) XoHash() uint64 { return Hash(l.text) }

func (l Time_Layout) XoDebug(b *strings.Builder) {
	b.WriteString("Layout(" + strconv.Quote(l.text) + ")")
}

func (l Time_Layout) M_debug(*Task) string { return Debug(l) }

func (l Time_Layout) M_display(*Task) string { return l.text }

func (l Time_Layout) M_format(_ *Task, t Time, z Time_Zone) string { return formatItems(l.items, t, z) }

func (l Time_Layout) M_parse(_ *Task, s string, z Time_Zone) Result[Time, Time_ParseErr] {
	return parseItems(l.items, s, l.text, z)
}
