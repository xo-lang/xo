package rt

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

// std/time civil time (std.md 13.2, decision 0094): dates in the
// proleptic Gregorian calendar, wall clock readings, and the conversion
// of a civil time in a zone to an instant with explicit daylight saving
// gap and overlap handling.

// daysFromCivil is the number of days from 1970-01-01 to y-m-d (H.
// Hinnant's algorithm; months and days outside their ranges are
// normalized by the callers first).
func daysFromCivil(y, m, d int64) int64 {
	if m <= 2 {
		y--
	}
	era := floorDiv(y, 400)
	yoe := y - era*400
	mp := (m + 9) % 12
	doy := (153*mp+2)/5 + d - 1
	doe := yoe*365 + yoe/4 - yoe/100 + doy
	return era*146097 + doe - 719468
}

// civilFromDays is the inverse of daysFromCivil.
func civilFromDays(z int64) (y, m, d int64) {
	z += 719468
	era := floorDiv(z, 146097)
	doe := z - era*146097
	yoe := (doe - doe/1460 + doe/36524 - doe/146096) / 365
	y = yoe + era*400
	doy := doe - (365*yoe + yoe/4 - yoe/100)
	mp := (5*doy + 2) / 153
	d = doy - (153*mp+2)/5 + 1
	if mp < 10 {
		m = mp + 3
	} else {
		m = mp - 9
	}
	if m <= 2 {
		y++
	}
	return
}

func isLeap(y int64) bool { return y%4 == 0 && (y%100 != 0 || y%400 == 0) }

func daysIn(y, m int64) int64 {
	switch m {
	case 2:
		if isLeap(y) {
			return 29
		}
		return 28
	case 4, 6, 9, 11:
		return 30
	}
	return 31
}

// dateDays is the day number of d with the month normalized and the day
// counted from the first of the month, so a Date built with a struct
// literal and out of range fields still has a meaning.
func dateDays(d Time_Date) int64 {
	q, r := splitDays(d)
	return q*eraDays + r
}

// eraDays is the days of 400 Gregorian years, a whole number of weeks.
const eraDays = 146097

// splitDays is the native backend's split_days (std/native/time.xo): d's
// day number as q*eraDays + r with r near 1970, without multiplying a
// field, so any fields (a struct literal, a decoded JSON body) give the
// native answers; Go's int64 arithmetic would wrap to wrong ones.
func splitDays(d Time_Date) (int64, int64) {
	k, mon := monthCarry(d.F_month)
	yr := d.F_year%400 + k%400
	r := daysFromCivil(yr, mon, 1) + d.F_day%eraDays - 1
	return d.F_year/400 + k/400 + d.F_day/eraDays, r
}

// monthCarry is a month field as (years, month 1 to 12).
func monthCarry(m int64) (int64, int64) {
	q, r := m/12, m%12
	if r <= 0 {
		return q - 1, r + 12
	}
	return q, r
}

// splitFloor is (n floor k, n mod k) for k > 0.
func splitFloor(n, k int64) (int64, int64) {
	q, r := n/k, n%k
	if r < 0 {
		return q - 1, r + k
	}
	return q, r
}

func normSplit(q, r int64) (int64, int64) {
	c, rr := splitFloor(r, eraDays)
	return q + c, rr
}

// eraYear is q*400 + y for y in 1970 to 2369, in the native order.
func eraYear(q, y int64) int64 {
	if q < 0 {
		return (q+6)*400 + (y - 2400)
	}
	return q*400 + y
}

func dateOfSplit(q, r int64) Time_Date {
	q2, r2 := normSplit(q, r)
	d := dateOf(r2)
	d.F_year = eraYear(q2, d.F_year)
	return d
}

// civilSplit is the native civil_split: c's wall clock as q*eraDays + r
// days and s seconds.
func civilSplit(c Time_Civil) (int64, int64, int64) {
	q, r := splitDays(c.F_date)
	hd, hr := splitFloor(c.F_time.F_hour, 24)
	md, mr := splitFloor(c.F_time.F_minute, 1440)
	sd, sr := splitFloor(c.F_time.F_second, 86400)
	secs := hr*3600 + mr*60 + sr
	extra := floorDiv(secs, 86400)
	q2 := q + hd/eraDays + md/eraDays + sd/eraDays
	r2 := r + hd%eraDays + md%eraDays + sd%eraDays + extra
	q3, r3 := normSplit(q2, r2)
	return q3, r3, secs - extra*86400
}

func dateOf(days int64) Time_Date {
	y, m, d := civilFromDays(days)
	return Time_Date{F_year: y, F_month: m, F_day: d}
}

// validDate reports what is wrong with y-m-d, or "".
func validDate(y, m, d int64) string {
	switch {
	case y < 1 || y > 9999:
		return fmt.Sprintf("year %d out of range 1 to 9999", y)
	case m < 1 || m > 12:
		return fmt.Sprintf("month %d out of range 1 to 12", m)
	case d < 1 || d > daysIn(y, m):
		return fmt.Sprintf("day %d out of range 1 to %d for %04d-%02d", d, daysIn(y, m), y, m)
	}
	return ""
}

func validClock(c Time_ClockTime) string {
	switch {
	case c.F_hour < 0 || c.F_hour > 23:
		return fmt.Sprintf("hour %d out of range 0 to 23", c.F_hour)
	case c.F_minute < 0 || c.F_minute > 59:
		return fmt.Sprintf("minute %d out of range 0 to 59", c.F_minute)
	case c.F_second < 0 || c.F_second > 59:
		return fmt.Sprintf("second %d out of range 0 to 59", c.F_second)
	case c.F_nanos < 0 || c.F_nanos > 999999999:
		return fmt.Sprintf("nanos %d out of range 0 to 999999999", c.F_nanos)
	}
	return ""
}

func civilErr(tag int, s string) Time_CivilErr {
	switch tag {
	case 1:
		return Time_CivilErr{Tag: 1, P: Time_CivilErr_Invalid{F_msg: s}}
	case 2:
		return Time_CivilErr{Tag: 2, P: Time_CivilErr_Skipped{F_at: s}}
	}
	return Time_CivilErr{Tag: 3, P: Time_CivilErr_Repeated{F_at: s}}
}

// Time_date is `time.date(year, month, day)`.
func Time_date(_ *Task, y, m, d int64) Result[Time_Date, Time_CivilErr] {
	if msg := validDate(y, m, d); msg != "" {
		return Err[Time_Date](civilErr(1, msg))
	}
	return Ok[Time_Date, Time_CivilErr](Time_Date{F_year: y, F_month: m, F_day: d})
}

// Time_civil is `time.civil(t, z)`.
func Time_civil(_ *Task, t Time, z Time_Zone) Time_Civil {
	gt := goTime(t, z)
	_, off := gt.Zone()
	return Time_Civil{
		F_date:   Time_Date{F_year: int64(gt.Year()), F_month: int64(gt.Month()), F_day: int64(gt.Day())},
		F_time:   Time_ClockTime{F_hour: int64(gt.Hour()), F_minute: int64(gt.Minute()), F_second: int64(gt.Second()), F_nanos: int64(gt.Nanosecond())},
		F_offset: Duration(int64(off) * 1e9),
		F_zone:   z.data().name,
	}
}

// Time ranges over int64 nanoseconds: about 1677-09-21 to 2262-04-11.
const (
	minTimeSecs int64 = math.MinInt64/1_000_000_000 + 1
	maxTimeSecs int64 = math.MaxInt64/1_000_000_000 - 1
)

// civilInstant converts local seconds (the civil time read as if in UTC)
// in zone z to an instant. kind is 0 for a unique instant, 2 for a gap
// (no instant), 3 for an overlap (two instants); early and late are the
// candidates, in seconds since the epoch.
func civilInstant(local int64, z Time_Zone) (early, late int64, kind int) {
	return civilInstantW(local, z, false)
}

// civilInstantW is civilInstant; wide looks the zone up at any second
// (decision 0614), else within Time's range.
func civilInstantW(local int64, z Time_Zone, wide bool) (early, late int64, kind int) {
	const span = 2 * 86400
	at := func(s int64) int64 {
		if wide {
			return z.offsetAt(s)
		}
		s = max(min(s, maxTimeSecs), minTimeSecs)
		return z.offsetSecs(s * 1e9)
	}
	before := at(local - span)
	after := at(local + span)
	tb, ta := local-before, local-after
	okB := at(tb) == before
	okA := at(ta) == after
	switch {
	case before == after:
		if okB {
			return tb, tb, 0
		}
		// a transition and its reversal within the window: use the
		// offset at the naive instant
		off := at(local)
		return local - off, local - off, 0
	case okB && okA:
		return min(tb, ta), max(tb, ta), 3
	case okB:
		return tb, tb, 0
	case okA:
		return ta, ta, 0
	}
	// a gap: Earlier is the earlier instant (the offset after the change
	// applied), Later the later one (std.md 13.2)
	return min(tb, ta), max(tb, ta), 2
}

// Time_instant is `time.instant(date, time, z, ambiguity)`.
func Time_instant(_ *Task, d Time_Date, c Time_ClockTime, z Time_Zone, amb Time_Ambiguity) Result[Time, Time_CivilErr] {
	if msg := validDate(d.F_year, d.F_month, d.F_day); msg != "" {
		return Err[Time](civilErr(1, msg))
	}
	if msg := validClock(c); msg != "" {
		return Err[Time](civilErr(1, msg))
	}
	local := daysFromCivil(d.F_year, d.F_month, d.F_day)*86400 + c.F_hour*3600 + c.F_minute*60 + c.F_second
	early, late, kind := civilInstant(local, z)
	at := func() string {
		return fmt.Sprintf("%04d-%02d-%02dT%02d:%02d:%02d %s", d.F_year, d.F_month, d.F_day, c.F_hour, c.F_minute, c.F_second, z.data().name)
	}
	secs := early
	switch {
	case kind != 0 && amb.Tag == 3:
		return Err[Time](civilErr(kind, at()))
	case kind != 0 && amb.Tag == 2:
		secs = late
	}
	if secs < minTimeSecs || secs > maxTimeSecs {
		return Err[Time](civilErr(1, fmt.Sprintf("%s is outside the range of Time (1678 to 2262)", at())))
	}
	return Ok[Time, Time_CivilErr](Time(secs*1e9 + c.F_nanos))
}

// M_weekday is `d.weekday()`.
func (d Time_Date) M_weekday(*Task) Time_Weekday {
	// 1970-01-01 was a Thursday (tag 4; Monday is 1)
	_, r := splitDays(d)
	w := r + 3
	return Time_Weekday{Tag: int(w-floorDiv(w, 7)*7) + 1}
}

// M_add_days is `d.add_days(n)`.
func (d Time_Date) M_add_days(_ *Task, n int64) Time_Date {
	q, r := splitDays(d)
	return dateOfSplit(q+n/eraDays, r+n%eraDays)
}

// M_add_months is `d.add_months(n)`: the day is clamped to the
// length of the resulting month.
func (d Time_Date) M_add_months(_ *Task, n int64) Time_Date {
	mq, mon := monthCarry(d.F_month)
	idx := mon - 1 + n%12
	c := floorDiv(idx, 12)
	y := d.F_year + mq + n/12 + c
	m := idx - c*12 + 1
	day := min(max(d.F_day, 1), daysIn(y, m))
	return Time_Date{F_year: y, F_month: m, F_day: day}
}

// M_days_until is `d.days_until(o)`.
func (d Time_Date) M_days_until(_ *Task, o Time_Date) int64 {
	q1, r1 := splitDays(d)
	q2, r2 := splitDays(o)
	return (q2-q1)*eraDays + (r2 - r1)
}

// M_display is "2026-10-09".
func (d Time_Date) M_display(*Task) string {
	if d.F_year < 0 {
		// strconv's digits of the year, so Int.min (whose negation wraps) prints too
		return fmt.Sprintf("-%04s-%02d-%02d", fmt.Sprint(d.F_year)[1:], d.F_month, d.F_day)
	}
	return fmt.Sprintf("%04d-%02d-%02d", d.F_year, d.F_month, d.F_day)
}

// goWeekdayTag maps Go's weekday (Sunday 0) to the Weekday tag (Monday 1).
func goWeekdayTag(w time.Weekday) int {
	if w == time.Sunday {
		return 7
	}
	return int(w)
}

// M_display of a CivilErr (std.md 13.2): the problem, or the civil time
// and why it does not name one instant.
func (v Time_CivilErr) M_display(*Task) string {
	switch p := v.P.(type) {
	case Time_CivilErr_Invalid:
		return "invalid civil time: " + p.F_msg
	case Time_CivilErr_Skipped:
		return p.F_at + " does not exist (skipped by a daylight saving change)"
	case Time_CivilErr_Repeated:
		return p.F_at + " happens twice (repeated by a daylight saving change)"
	}
	return Debug(v)
}

// ---- decision 0414: Weekday numbers, truncate and round, Ticker ----

// M_number is `w.number()`: ISO 8601, Monday 1 to Sunday 7 (the tags).
func (w Time_Weekday) M_number(*Task) int64 { return int64(w.Tag) }

// Time_weekday_from is `time.weekday_from(n)`.
func Time_weekday_from(_ *Task, n int64) Option[Time_Weekday] {
	if n < 1 || n > 7 {
		return None[Time_Weekday]()
	}
	return Some(Time_Weekday{Tag: int(n)})
}

// goTimeOf is t as a Go time (UTC).
func goTimeOf(t Time) time.Time { return time.Unix(0, int64(t)).UTC() }

// Time_truncate is `time.truncate(t, d)`: Go's Time.Truncate.
func Time_truncate(_ *Task, t Time, d Duration) Time {
	return Time(goTimeOf(t).Truncate(time.Duration(d)).UnixNano())
}

// Time_round is `time.round(t, d)`: Go's Time.Round.
func Time_round(_ *Task, t Time, d Duration) Time {
	return Time(goTimeOf(t).Round(time.Duration(d)).UnixNano())
}

// Time_ticker is `time.ticker(clock, period)`.
func Time_ticker(tk *Task, clock Clock, period Duration) Time_Ticker {
	if period <= 0 {
		FaultHere("time: non-positive interval for ticker")
	}
	return Time_Ticker{F_clock: clock, F_period: period, F_next_ns: Atomic_new(tk, int64(clock.M_now(tk))+int64(period))}
}

// M_wait is `t.wait()`: the next tick's time, missed ticks dropped; the
// tick is claimed before the sleep so two waiters never share one.
func (t Time_Ticker) M_wait(tk *Task) Time {
	p := int64(t.F_period)
	next := t.F_next_ns.M_get(tk)
	now := int64(t.F_clock.M_now(tk))
	for {
		n := next + p
		if n <= now {
			n += ((now-n)/p + 1) * p
		}
		if t.F_next_ns.M_compare_and_swap(tk, next, n) {
			break
		}
		next = t.F_next_ns.M_get(tk)
		now = int64(t.F_clock.M_now(tk))
	}
	if next > now {
		t.F_clock.M_sleep(tk, Duration(next-now))
	}
	return Time(next)
}

// M_reset is `t.reset(period)`.
func (t Time_Ticker) M_reset(tk *Task, period Duration) Time_Ticker {
	return Time_ticker(tk, t.F_clock, period)
}

// Time_date_json is Date's JSON value (2026-11-16): a string of its text,
// Go's "2006-01-02" as pgx's pgtype.Date writes it.
func Time_date_json(tk *Task, d Time_Date) Json_Value {
	return Json_Value{Tag: 5, P: Json_Value_Str{F_s: d.M_display(tk)}}
}

// Time_date_of_json is a Date from its JSON value: the text, or the
// object Date wrote before.
func Time_date_of_json(tk *Task, v Json_Value) Result[Time_Date, Time_CivilErr] {
	switch v.Tag {
	case 5:
		return timeDateText(tk, v.P.(Json_Value_Str).F_s)
	case 7:
		var y, m, d *int64
		for _, f := range List_Slice(v.P.(Json_Value_Object).F_fields) {
			if f.F1.Tag != 3 {
				continue
			}
			n := f.F1.P.(Json_Value_Int).F_n
			switch f.F0 {
			case "year":
				y = &n
			case "month":
				m = &n
			case "day":
				d = &n
			}
		}
		if y != nil && m != nil && d != nil {
			return Time_date(tk, *y, *m, *d)
		}
	}
	return Err[Time_Date](civilErr(1, `a date is a "YYYY-MM-DD" string`))
}

func timeDateText(tk *Task, s string) Result[Time_Date, Time_CivilErr] {
	bad := Err[Time_Date](civilErr(1, "cannot parse "+strconv.Quote(s)+" as a date (YYYY-MM-DD)"))
	body, neg := strings.CutPrefix(s, "-")
	parts := strings.Split(body, "-")
	if len(parts) != 3 || len(parts[0]) < 4 || len(parts[1]) != 2 || len(parts[2]) != 2 {
		return bad
	}
	var n [3]int64
	for i, p := range parts {
		for _, c := range []byte(p) {
			if c < '0' || c > '9' {
				return bad
			}
		}
		x, err := strconv.ParseInt(p, 10, 64)
		if err != nil {
			return bad
		}
		n[i] = x
	}
	if neg {
		n[0] = -n[0]
	}
	return Time_date(tk, n[0], n[1], n[2])
}
