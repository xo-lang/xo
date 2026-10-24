package rt

import (
	"math"
	"strconv"
	"strings"
	"time"
)

// Duration is the Xo Duration: nanoseconds, may be negative (core 2.1).
type Duration int64

// Time is the Xo Time: a wall clock instant in nanoseconds since the Unix
// epoch, UTC.
type Time int64

// String formats a duration (std.md 2.6): `1.5s`, `250ms`, `1m30s`, `0s`.
func (d Duration) String() string { return time.Duration(d).String() }

// String formats a time as RFC 3339 in UTC (std.md 2.6).
func (t Time) String() string {
	return time.Unix(0, int64(t)).UTC().Format(time.RFC3339Nano)
}

// DurMulInt is checked `d * n`.
func DurMulInt(d Duration, n int64, at string) Duration { return MulS(d, Duration(n), at) }

// DurDivInt is checked `d / n`.
func DurDivInt(d Duration, n int64, at string) Duration { return DivS(d, Duration(n), at) }

// DurDivDur is `d1 / d2`, truncated.
func DurDivDur(a, b Duration, at string) int64 { return int64(DivS(a, b, at)) }

// TimeAdd is checked `t + d`.
func TimeAdd(t Time, d Duration, at string) Time { return Time(AddS(int64(t), int64(d), at)) }

// TimeSub is checked `t - d`.
func TimeSub(t Time, d Duration, at string) Time { return Time(SubS(int64(t), int64(d), at)) }

// TimeDiff is checked `t1 - t2`.
func TimeDiff(a, b Time, at string) Duration { return Duration(SubS(int64(a), int64(b), at)) }

// Duration_from_secs is `Duration.from_secs(s)`.
func Duration_from_secs(_ *Task, s float64) Duration {
	return Duration(FloatToInt[int64](math.Round(s*1e9), ""))
}

// Duration_from_secs_int is `Duration.from_secs_int(s)`: faults on
// overflow like `1s * s`.
func Duration_from_secs_int(_ *Task, s int64) Duration { return Duration(MulS(s, 1e9, "")) }

// Duration_from_millis is `Duration.from_millis(ms)`.
func Duration_from_millis(_ *Task, ms int64) Duration { return Duration(MulS(ms, 1e6, "")) }

// Duration_from_nanos is `Duration.from_nanos(ns)`.
func Duration_from_nanos(_ *Task, ns int64) Duration { return Duration(ns) }

// Duration_parse is `Duration.parse(s)` (std.md 2.6): time.ParseDuration
// of the text with surrounding white space removed, the form JSON
// decoding of a Duration accepts.
func Duration_parse(_ *Task, s string) Result[Duration, DurationErr] {
	if d, ok := parseDuration(s); ok {
		return Ok[Duration, DurationErr](d)
	}
	return Err[Duration](DurationErr{F_input: s})
}

// M_display is the text of a DurationErr (the same on every backend).
func (e DurationErr) M_display(*Task) string {
	return "invalid duration " + strconv.Quote(e.F_input) + `: expected numbers with units ns, us, ms, s, m, h, such as "1.5s" or "1h30m"`
}

// Duration_seconds is `d.seconds()`.
func Duration_seconds(_ *Task, d Duration) float64 { return float64(d) / 1e9 }

// Duration_millis is `d.millis()`, truncated.
func Duration_millis(_ *Task, d Duration) int64 { return int64(d) / 1e6 }

// Duration_nanos is `d.nanos()`.
func Duration_nanos(_ *Task, d Duration) int64 { return int64(d) }

// Duration_scale is `d.scale(f)`, rounded to the nearest nanosecond.
func Duration_scale(_ *Task, d Duration, f float64) Duration {
	return Duration(FloatToInt[int64](math.Round(float64(d)*f), ""))
}

// Duration_ceil_seconds is `d.ceil_seconds()`.
func Duration_ceil_seconds(_ *Task, d Duration) int64 {
	s := int64(d) / 1e9
	if int64(d)%1e9 > 0 {
		s++
	}
	return s
}

// Duration_abs is `d.abs()`.
func Duration_abs(_ *Task, d Duration) Duration {
	if d < 0 {
		return NegS(d, "")
	}
	return d
}

// Time_UNIX_EPOCH is `Time.UNIX_EPOCH`.
func Time_UNIX_EPOCH(*Task) Time { return 0 }

// Time_from_unix is `Time.from_unix(secs)`.
func Time_from_unix(_ *Task, secs int64) Time { return Time(MulS(secs, 1e9, "")) }

// Time_parse_rfc3339 is `Time.parse_rfc3339(s)`.
func Time_parse_rfc3339(_ *Task, s string) Option[Time] {
	t, ok := parseRFC3339(s)
	if !ok || !inTimeRange(t) {
		return Option[Time]{}
	}
	return Some(Time(t.UnixNano()))
}

// inTimeRange reports whether t is a Time: 1677-09-21T00:12:43.145224192Z
// to 2262-04-11T23:47:16.854775807Z, the nanoseconds of an int64 (decision
// 0364; UnixNano wraps outside it).
func inTimeRange(t time.Time) bool {
	return !t.Before(time.Unix(0, math.MinInt64)) && !t.After(time.Unix(0, math.MaxInt64))
}

// parseRFC3339 reads an RFC 3339 time as the native backends do (text.xo
// parse_time): the fixed two and four digit fields, an upper case T and Z, a
// '.' fraction of one or more digits, a Z or +-HH:MM offset. Go's time.Parse
// alone also takes a one digit hour, minute, second, month or day (its
// lenient fallback after the strict parser refuses), a lower case t or z and
// a comma fraction, which the spec (RFC 3339) does not.
func parseRFC3339(s string) (time.Time, bool) {
	if !rfc3339Shape(s) {
		return time.Time{}, false
	}
	t, err := time.Parse(time.RFC3339Nano, s)
	return t, err == nil
}

func rfc3339Shape(s string) bool {
	d := func(i int) bool { return s[i] >= '0' && s[i] <= '9' }
	if len(s) < 20 {
		return false
	}
	for _, i := range []int{0, 1, 2, 3, 5, 6, 8, 9, 11, 12, 14, 15, 17, 18} {
		if !d(i) {
			return false
		}
	}
	if s[4] != '-' || s[7] != '-' || s[10] != 'T' || s[13] != ':' || s[16] != ':' {
		return false
	}
	i := 19
	if len(s)-i >= 2 && s[i] == '.' && d(i+1) {
		i++
		for i < len(s) && d(i) {
			i++
		}
	}
	if len(s)-i == 1 {
		return s[i] == 'Z'
	}
	if !(len(s)-i == 6 && (s[i] == '+' || s[i] == '-') && d(i+1) && d(i+2) && s[i+3] == ':' && d(i+4) && d(i+5)) {
		return false
	}
	// an offset of at most 23:59 (Go's parser takes +24:60)
	return (s[i+1]-'0')*10+(s[i+2]-'0') <= 23 && (s[i+4]-'0')*10+(s[i+5]-'0') <= 59
}

// Time_unix is `t.unix()`.
func Time_unix(_ *Task, t Time) int64 { return floorDiv(int64(t), 1e9) }

// Time_unix_millis is `t.unix_millis()`.
func Time_unix_millis(_ *Task, t Time) int64 { return floorDiv(int64(t), 1e6) }

// Time_format_rfc3339 is `t.format_rfc3339()`.
func Time_format_rfc3339(_ *Task, t Time) string { return t.String() }

func floorDiv(a, b int64) int64 {
	q := a / b
	if (a%b != 0) && ((a < 0) != (b < 0)) {
		q--
	}
	return q
}

// parseDuration parses the text form of a duration (`"1.5s"`).
func parseDuration(s string) (Duration, bool) {
	d, err := time.ParseDuration(strings.TrimSpace(s))
	if err != nil {
		return 0, false
	}
	return Duration(d), true
}

// IntStr formats an integer for interpolation.
func IntStr(n int64) string { return strconv.FormatInt(n, 10) }
