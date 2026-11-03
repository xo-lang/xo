package rt

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// M_display is the message of robfig/cron's error (std/native/cron.xo
// cronerr_display, which the Go backend compiles for the other functions).
func (e Cron_CronErr) M_display(*Task) string { return e.F_msg }

// M_zone is `s.zone()`.
func (s *Cron_Schedule) M_zone(*Task) Time_Zone { return s.F__loc }

// M_next is `s.next(after)`: robfig/cron v3.0.1's SpecSchedule.Next
// (spec.go, Copyright (C) 2012 Rob Figueiredo, MIT license) on the bit sets
// of the parsed fields, and ConstantDelaySchedule.Next for `@every`.
func (s *Cron_Schedule) M_next(_ *Task, after Time) Option[Time] {
	ns := int64(after)
	if s.F__every > 0 {
		sec := ns / 1e9
		if ns%1e9 < 0 {
			sec--
		}
		return Some(Time((sec + s.F__every) * 1e9))
	}
	loc := s.F__loc.data().loc
	t := time.Unix(0, ns).In(loc)
	t = t.Add(1*time.Second - time.Duration(t.Nanosecond())*time.Nanosecond)
	added := false
	yearLimit := t.Year() + 5
WRAP:
	if t.Year() > yearLimit {
		return Option[Time]{}
	}
	for 1<<uint(t.Month())&s.F__month == 0 {
		if !added {
			added = true
			t = time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, loc)
		}
		t = t.AddDate(0, 1, 0)
		if t.Month() == time.January {
			goto WRAP
		}
	}
	for !s.dayMatches(t) {
		if !added {
			added = true
			t = time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, loc)
		}
		t = t.AddDate(0, 0, 1)
		if t.Hour() != 0 {
			if t.Hour() > 12 {
				t = t.Add(time.Duration(24-t.Hour()) * time.Hour)
			} else {
				t = t.Add(time.Duration(-t.Hour()) * time.Hour)
			}
		}
		if t.Day() == 1 {
			goto WRAP
		}
	}
	for 1<<uint(t.Hour())&s.F__hour == 0 {
		if !added {
			added = true
			t = time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), 0, 0, 0, loc)
		}
		t = t.Add(1 * time.Hour)
		if t.Hour() == 0 {
			goto WRAP
		}
	}
	for 1<<uint(t.Minute())&s.F__minute == 0 {
		if !added {
			added = true
			t = t.Truncate(time.Minute)
		}
		t = t.Add(1 * time.Minute)
		if t.Minute() == 0 {
			goto WRAP
		}
	}
	// the standard parser's second field is always 0
	if t.Second() != 0 {
		added = true
		t = t.Truncate(time.Second).Add(time.Duration(60-t.Second()) * time.Second)
		goto WRAP
	}
	return Some(Time(t.UnixNano()))
}

func (s *Cron_Schedule) dayMatches(t time.Time) bool {
	domMatch := 1<<uint(t.Day())&s.F__dom > 0
	dowMatch := 1<<uint(t.Weekday())&s.F__dow > 0
	if s.F__dom_star || s.F__dow_star {
		return domMatch && dowMatch
	}
	return domMatch || dowMatch
}

// The standard parser of robfig/cron v3.0.1 (parser.go) on the Go side:
// five fields or a descriptor, with an optional TZ= or CRON_TZ= prefix.


type cronBounds struct {
	min, max int64
	names    map[string]int64
}

var (
	cronMinutes = cronBounds{0, 59, nil}
	cronHours   = cronBounds{0, 23, nil}
	cronDom     = cronBounds{1, 31, nil}
	cronMonths  = cronBounds{1, 12, map[string]int64{"jan": 1, "feb": 2, "mar": 3, "apr": 4, "may": 5, "jun": 6, "jul": 7, "aug": 8, "sep": 9, "oct": 10, "nov": 11, "dec": 12}}
	cronDow     = cronBounds{0, 6, map[string]int64{"sun": 0, "mon": 1, "tue": 2, "wed": 3, "thu": 4, "fri": 5, "sat": 6}}
)

func cronFail(format string, a ...any) *Cron_CronErr {
	return &Cron_CronErr{F_msg: fmt.Sprintf(format, a...)}
}

func cronAtoi(expr string) (int64, *Cron_CronErr) {
	num, err := strconv.Atoi(expr)
	if err != nil {
		return 0, cronFail("failed to parse int from %s: %s", expr, err)
	}
	if num < 0 {
		return 0, cronFail("negative number (%d) not allowed: %s", num, expr)
	}
	return int64(num), nil
}

func cronIntOrName(expr string, names map[string]int64) (int64, *Cron_CronErr) {
	if names != nil {
		if v, ok := names[strings.ToLower(expr)]; ok {
			return v, nil
		}
	}
	return cronAtoi(expr)
}

func cronBits(min, max, step int64) int64 {
	var b int64
	for i := min; i <= max; i += step {
		b |= 1 << uint(i)
	}
	return b
}

func cronRange(expr string, r cronBounds) (int64, bool, *Cron_CronErr) {
	var start, end, step int64
	rangeAndStep := strings.Split(expr, "/")
	lowAndHigh := strings.Split(rangeAndStep[0], "-")
	single := len(lowAndHigh) == 1
	var err *Cron_CronErr
	star := false
	if lowAndHigh[0] == "*" || lowAndHigh[0] == "?" {
		start, end, star = r.min, r.max, true
	} else {
		if start, err = cronIntOrName(lowAndHigh[0], r.names); err != nil {
			return 0, false, err
		}
		switch len(lowAndHigh) {
		case 1:
			end = start
		case 2:
			if end, err = cronIntOrName(lowAndHigh[1], r.names); err != nil {
				return 0, false, err
			}
		default:
			return 0, false, cronFail("too many hyphens: %s", expr)
		}
	}
	switch len(rangeAndStep) {
	case 1:
		step = 1
	case 2:
		if step, err = cronAtoi(rangeAndStep[1]); err != nil {
			return 0, false, err
		}
		if single {
			end = r.max
		}
		if step > 1 {
			star = false
		}
	default:
		return 0, false, cronFail("too many slashes: %s", expr)
	}
	if start < r.min {
		return 0, false, cronFail("beginning of range (%d) below minimum (%d): %s", start, r.min, expr)
	}
	if end > r.max {
		return 0, false, cronFail("end of range (%d) above maximum (%d): %s", end, r.max, expr)
	}
	if start > end {
		return 0, false, cronFail("beginning of range (%d) beyond end of range (%d): %s", start, end, expr)
	}
	if step == 0 {
		return 0, false, cronFail("step of range should be a positive number: %s", expr)
	}
	return cronBits(start, end, step), star, nil
}

func cronField(field string, r cronBounds) (int64, bool, *Cron_CronErr) {
	var b int64
	star := false
	for _, expr := range strings.FieldsFunc(field, func(r rune) bool { return r == ',' }) {
		bit, s, err := cronRange(expr, r)
		if err != nil {
			return 0, false, err
		}
		b |= bit
		star = star || s
	}
	return b, star, nil
}

func cronSched(minute, hour, dom, month, dow int64, domStar, dowStar bool, z Time_Zone) Cron_Schedule {
	return Cron_Schedule{F__minute: minute, F__hour: hour, F__dom: dom, F__month: month, F__dow: dow, F__dom_star: domStar, F__dow_star: dowStar, F__loc: z}
}

func cronDescriptor(d string, z Time_Zone) Result[Cron_Schedule, Cron_CronErr] {
	allHours, allDom, allMonths, allDow := cronBits(0, 23, 1), cronBits(1, 31, 1), cronBits(1, 12, 1), cronBits(0, 6, 1)
	switch d {
	case "@yearly", "@annually":
		return Ok[Cron_Schedule, Cron_CronErr](cronSched(1, 1, 1<<1, 1<<1, allDow, false, true, z))
	case "@monthly":
		return Ok[Cron_Schedule, Cron_CronErr](cronSched(1, 1, 1<<1, allMonths, allDow, false, true, z))
	case "@weekly":
		return Ok[Cron_Schedule, Cron_CronErr](cronSched(1, 1, allDom, allMonths, 1, true, false, z))
	case "@daily", "@midnight":
		return Ok[Cron_Schedule, Cron_CronErr](cronSched(1, 1, allDom, allMonths, allDow, true, true, z))
	case "@hourly":
		return Ok[Cron_Schedule, Cron_CronErr](cronSched(1, allHours, allDom, allMonths, allDow, true, true, z))
	}
	if strings.HasPrefix(d, "@every ") {
		dur, err := time.ParseDuration(d[len("@every "):])
		if err != nil {
			return Err[Cron_Schedule](Cron_CronErr{F_msg: fmt.Sprintf("failed to parse duration %s: %s", d, err)})
		}
		if dur < time.Second {
			dur = time.Second
		}
		return Ok[Cron_Schedule, Cron_CronErr](Cron_Schedule{F__every: int64(dur / time.Second), F__loc: z})
	}
	return Err[Cron_Schedule](Cron_CronErr{F_msg: "unrecognized descriptor: " + d})
}

// Cron_parse is `cron.parse(spec, zone)`.
func Cron_parse(tk *Task, spec string, zone Time_Zone) Result[Cron_Schedule, Cron_CronErr] {
	if len(spec) == 0 {
		return Err[Cron_Schedule](Cron_CronErr{F_msg: "empty spec string"})
	}
	z := zone
	if strings.HasPrefix(spec, "TZ=") || strings.HasPrefix(spec, "CRON_TZ=") {
		i := strings.Index(spec, " ")
		eq := strings.Index(spec, "=")
		if i < 0 {
			return Err[Cron_Schedule](Cron_CronErr{F_msg: "provided bad location " + spec[eq+1:] + ": missing space after the time zone"})
		}
		name := spec[eq+1 : i]
		r := Time_zone(tk, name)
		if !r.Ok {
			return Err[Cron_Schedule](Cron_CronErr{F_msg: "provided bad location " + name + ": " + r.E.M_display(tk)})
		}
		z = r.V
		spec = strings.TrimSpace(spec[i:])
	}
	if strings.HasPrefix(spec, "@") {
		return cronDescriptor(spec, z)
	}
	fields := strings.Fields(spec)
	if len(fields) != 5 {
		return Err[Cron_Schedule](*cronFail("expected exactly 5 fields, found %d: %s", len(fields), fields))
	}
	minute, _, err := cronField(fields[0], cronMinutes)
	if err != nil {
		return Err[Cron_Schedule](*err)
	}
	hour, _, err := cronField(fields[1], cronHours)
	if err != nil {
		return Err[Cron_Schedule](*err)
	}
	dom, domStar, err := cronField(fields[2], cronDom)
	if err != nil {
		return Err[Cron_Schedule](*err)
	}
	month, _, err := cronField(fields[3], cronMonths)
	if err != nil {
		return Err[Cron_Schedule](*err)
	}
	dow, dowStar, err := cronField(fields[4], cronDow)
	if err != nil {
		return Err[Cron_Schedule](*err)
	}
	return Ok[Cron_Schedule, Cron_CronErr](cronSched(minute, hour, dom, month, dow, domStar, dowStar, z))
}
