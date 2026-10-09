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
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return Option[Time]{}
	}
	return Some(Time(t.UnixNano()))
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
