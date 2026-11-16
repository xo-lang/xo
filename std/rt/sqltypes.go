package rt

// The typed readers of Postgres types and their parameters (decision
// 0679), as std/native/sql.xo: a column's Postgres text parsed as pgx's
// pgtype parses it (Date, Time, Interval, InetCodec, UUID).

import (
	"fmt"
	"net/netip"
	"strconv"
	"strings"
)

// Sql_uuid is `sql.uuid(u)`.
func Sql_uuid(tk *Task, u Uuid_Uuid) Sql_SqlValue { return Sql_str(tk, u.M_display(tk)) }

// Sql_date is `sql.date(d)`.
func Sql_date(tk *Task, d Time_Date) Sql_SqlValue {
	if d.F_year <= 0 {
		return Sql_str(tk, fmt.Sprintf("%04d-%02d-%02d BC", 1-d.F_year, d.F_month, d.F_day))
	}
	return Sql_str(tk, fmt.Sprintf("%04d-%02d-%02d", d.F_year, d.F_month, d.F_day))
}

// Sql_time_of_day is `sql.time_of_day(t)`.
func Sql_time_of_day(tk *Task, t Time_ClockTime) Sql_SqlValue {
	return Sql_str(tk, fmt.Sprintf("%02d:%02d:%02d.%06d", t.F_hour, t.F_minute, t.F_second, t.F_nanos/1000))
}

// Sql_interval is `sql.interval(i)`.
func Sql_interval(tk *Task, i Sql_Interval) Sql_SqlValue {
	return Sql_str(tk, fmt.Sprintf("%d mon %d day %d microsecond", i.F_months, i.F_days, i.F_micros))
}

// Sql_prefix is `sql.prefix(p)`.
func Sql_prefix(tk *Task, p Netip_Prefix) Sql_SqlValue { return Sql_str(tk, p.M_display(tk)) }

// M_duration is `i.duration()`.
func (i Sql_Interval) M_duration(_ *Task) Option[Duration] {
	if i.F_months != 0 || i.F_days != 0 {
		return None[Duration]()
	}
	return Some(Duration(i.F_micros * 1000))
}

// pgDigits is s when it is all ASCII digits (not empty).
func pgDigits(s string) (int64, bool) {
	if s == "" {
		return 0, false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return 0, false
		}
	}
	n, err := strconv.ParseInt(s, 10, 64)
	return n, err == nil
}

// pgFracNanos is a fraction's digits (at most 9) as nanoseconds.
func pgFracNanos(f string) (int64, bool) {
	if len(f) > 9 {
		return 0, false
	}
	n, ok := pgDigits(f)
	for k := len(f); ok && k < 9; k++ {
		n *= 10
	}
	return n, ok
}

func pgDate(s string) (Time_Date, bool) {
	t, bc := strings.CutSuffix(s, " BC")
	ps := strings.Split(t, "-")
	if len(ps) != 3 || len(ps[1]) != 2 || len(ps[2]) != 2 || len(ps[0]) < 4 {
		return Time_Date{}, false
	}
	y, ok1 := pgDigits(ps[0])
	m, ok2 := pgDigits(ps[1])
	d, ok3 := pgDigits(ps[2])
	if !ok1 || !ok2 || !ok3 || m < 1 || m > 12 || d < 1 || d > 31 {
		return Time_Date{}, false
	}
	if bc {
		y = 1 - y
	}
	return Time_Date{F_year: y, F_month: m, F_day: d}, true
}

func pgTimeOfDay(s string) (Time_ClockTime, bool) {
	if len(s) < 8 || s[2] != ':' || s[5] != ':' {
		return Time_ClockTime{}, false
	}
	h, ok1 := pgDigits(s[0:2])
	m, ok2 := pgDigits(s[3:5])
	sec, ok3 := pgDigits(s[6:8])
	var nanos int64
	ok4 := true
	if len(s) > 8 {
		if s[8] != '.' {
			return Time_ClockTime{}, false
		}
		nanos, ok4 = pgFracNanos(s[9:])
	}
	if !ok1 || !ok2 || !ok3 || !ok4 || h > 24 || m > 59 || sec > 59 || (h == 24 && (m > 0 || sec > 0 || nanos > 0)) {
		return Time_ClockTime{}, false
	}
	return Time_ClockTime{F_hour: h, F_minute: m, F_second: sec, F_nanos: nanos}, true
}

// pgInt is strconv.ParseInt's syntax: an optional sign, then digits.
func pgInt(s string) (int64, bool) {
	if r, ok := strings.CutPrefix(s, "+"); ok {
		return pgDigits(r)
	}
	if r, ok := strings.CutPrefix(s, "-"); ok {
		n, ok := pgDigits(r)
		return -n, ok
	}
	return pgDigits(s)
}

func pgInterval(s string) (Sql_Interval, bool) {
	ps := strings.Split(s, " ")
	var iv Sql_Interval
	i := 0
	for ; i+1 < len(ps); i += 2 {
		n, ok := pgInt(ps[i])
		if !ok {
			return Sql_Interval{}, false
		}
		switch ps[i+1] {
		case "year", "years":
			iv.F_months += n * 12
		case "mon", "mons":
			iv.F_months += n
		case "day", "days":
			iv.F_days += n
		default:
			return Sql_Interval{}, false
		}
	}
	if len(ps)%2 == 1 {
		tp := strings.Split(ps[len(ps)-1], ":")
		if len(tp) != 3 {
			return Sql_Interval{}, false
		}
		hs := tp[0]
		neg := strings.HasPrefix(hs, "-")
		if neg || strings.HasPrefix(hs, "+") {
			hs = hs[1:]
		}
		h, ok1 := pgDigits(hs)
		m, ok2 := pgDigits(tp[1])
		secs := tp[2]
		var frac int64
		ok3 := true
		if dot := strings.IndexByte(secs, '.'); dot >= 0 {
			var f int64
			f, ok3 = pgFracNanos(secs[dot+1:])
			frac = f / 1000
			secs = secs[:dot]
		}
		sec, ok4 := pgDigits(secs)
		if !ok1 || !ok2 || !ok3 || !ok4 {
			return Sql_Interval{}, false
		}
		us := h*3600000000 + m*60000000 + sec*1000000 + frac
		if neg {
			us = -us
		}
		iv.F_micros = us
	}
	return iv, true
}

func pgPrefix(s string) (Netip_Prefix, bool) {
	if strings.Contains(s, "/") {
		p, err := netip.ParsePrefix(s)
		if err != nil {
			return Netip_Prefix{}, false
		}
		return Netip_Prefix{F_addr: netipOf(p.Addr()), F_len: int64(p.Bits())}, true
	}
	a, err := netip.ParseAddr(s)
	if err != nil {
		return Netip_Prefix{}, false
	}
	return Netip_Prefix{F_addr: netipOf(a), F_len: int64(a.BitLen())}, true
}

// pgTyped is a typed reader's conversion: the column's text through parse.
func pgTyped[T any](parse func(string) (T, bool)) func(any) (T, bool) {
	return func(v any) (T, bool) {
		if s, ok := v.(string); ok {
			return parse(s)
		}
		var zero T
		return zero, false
	}
}

func pgUUID(tk *Task) func(string) (Uuid_Uuid, bool) {
	return func(s string) (Uuid_Uuid, bool) {
		r := Uuid_parse(tk, s)
		return r.V, r.Ok
	}
}

// M_uuid is `row.uuid(name)` (decision 0679).
func (r Sql_Row) M_uuid(tk *Task, name string) Result[Uuid_Uuid, Sql_SqlErr] {
	return sqlCol(r, name, "a uuid", pgTyped(pgUUID(tk)))
}

// M_uuid_opt is `row.uuid_opt(name)`.
func (r Sql_Row) M_uuid_opt(tk *Task, name string) Result[Option[Uuid_Uuid], Sql_SqlErr] {
	return sqlColOpt(r, name, "a uuid", pgTyped(pgUUID(tk)))
}

// M_date is `row.date(name)`.
func (r Sql_Row) M_date(_ *Task, name string) Result[Time_Date, Sql_SqlErr] {
	return sqlCol(r, name, "a date", pgTyped(pgDate))
}

// M_date_opt is `row.date_opt(name)`.
func (r Sql_Row) M_date_opt(_ *Task, name string) Result[Option[Time_Date], Sql_SqlErr] {
	return sqlColOpt(r, name, "a date", pgTyped(pgDate))
}

// M_time_of_day is `row.time_of_day(name)`.
func (r Sql_Row) M_time_of_day(_ *Task, name string) Result[Time_ClockTime, Sql_SqlErr] {
	return sqlCol(r, name, "a time of day", pgTyped(pgTimeOfDay))
}

// M_time_of_day_opt is `row.time_of_day_opt(name)`.
func (r Sql_Row) M_time_of_day_opt(_ *Task, name string) Result[Option[Time_ClockTime], Sql_SqlErr] {
	return sqlColOpt(r, name, "a time of day", pgTyped(pgTimeOfDay))
}

// M_interval is `row.interval(name)`.
func (r Sql_Row) M_interval(_ *Task, name string) Result[Sql_Interval, Sql_SqlErr] {
	return sqlCol(r, name, "an interval", pgTyped(pgInterval))
}

// M_interval_opt is `row.interval_opt(name)`.
func (r Sql_Row) M_interval_opt(_ *Task, name string) Result[Option[Sql_Interval], Sql_SqlErr] {
	return sqlColOpt(r, name, "an interval", pgTyped(pgInterval))
}

// M_prefix is `row.prefix(name)`.
func (r Sql_Row) M_prefix(_ *Task, name string) Result[Netip_Prefix, Sql_SqlErr] {
	return sqlCol(r, name, "an inet", pgTyped(pgPrefix))
}

// M_prefix_opt is `row.prefix_opt(name)`.
func (r Sql_Row) M_prefix_opt(_ *Task, name string) Result[Option[Netip_Prefix], Sql_SqlErr] {
	return sqlColOpt(r, name, "an inet", pgTyped(pgPrefix))
}

// Sql_mac is `sql.mac(b)`.
func Sql_mac(tk *Task, b Bytes) Sql_SqlValue {
	parts := make([]string, 0, len(b))
	for i := 0; i < len(b); i++ {
		parts = append(parts, fmt.Sprintf("%02x", b[i]))
	}
	return Sql_str(tk, strings.Join(parts, ":"))
}

func hsQuote(t string) string {
	return `"` + strings.ReplaceAll(strings.ReplaceAll(t, `\`, `\\`), `"`, `\"`) + `"`
}

// Sql_hstore is `sql.hstore(m)`.
func Sql_hstore(tk *Task, m Map[string, Option[string]]) Sql_SqlValue {
	var parts []string
	for k, v := range Map_All(m) {
		val := "NULL"
		if v.Some {
			val = hsQuote(v.V)
		}
		parts = append(parts, hsQuote(k)+"=>"+val)
	}
	return Sql_str(tk, strings.Join(parts, ", "))
}

// Sql_date_bound is `sql.date_bound(b)`.
func Sql_date_bound(tk *Task, b Sql_Bound[Time_Date]) Sql_SqlValue {
	switch b.Tag {
	case 2:
		return Sql_str(tk, "infinity")
	case 3:
		return Sql_str(tk, "-infinity")
	}
	return Sql_date(tk, b.P.(Sql_Bound_Finite[Time_Date]).F_v)
}

// Sql_time_bound is `sql.time_bound(b)`.
func Sql_time_bound(tk *Task, b Sql_Bound[Time]) Sql_SqlValue {
	switch b.Tag {
	case 2:
		return Sql_str(tk, "infinity")
	case 3:
		return Sql_str(tk, "-infinity")
	}
	return Sql_time(tk, b.P.(Sql_Bound_Finite[Time]).F_v)
}

func boundFinite[T any](v T) Sql_Bound[T] {
	return Sql_Bound[T]{Tag: 1, P: Sql_Bound_Finite[T]{F_v: v}}
}

func pgDateBound(s string) (Sql_Bound[Time_Date], bool) {
	switch s {
	case "infinity":
		return Sql_Bound[Time_Date]{Tag: 2}, true
	case "-infinity":
		return Sql_Bound[Time_Date]{Tag: 3}, true
	}
	d, ok := pgDate(s)
	return boundFinite(d), ok
}

func asTimeBound(v any) (Sql_Bound[Time], bool) {
	switch x := v.(type) {
	case Time:
		return boundFinite(x), true
	case string:
		switch x {
		case "infinity":
			return Sql_Bound[Time]{Tag: 2}, true
		case "-infinity":
			return Sql_Bound[Time]{Tag: 3}, true
		}
	}
	return Sql_Bound[Time]{}, false
}

func pgMac(s string) (Bytes, bool) {
	ps := strings.Split(s, ":")
	if len(ps) != 6 && len(ps) != 8 {
		return "", false
	}
	b := make([]byte, 0, len(ps))
	for _, p := range ps {
		if len(p) != 2 {
			return "", false
		}
		n, err := strconv.ParseUint(p, 16, 8)
		if err != nil {
			return "", false
		}
		b = append(b, byte(n))
	}
	return Bytes(b), true
}

// hsQuoted is a quoted hstore key or value at s[i:] and the index after
// its closing quote.
func hsQuoted(s string, i int) (string, int, bool) {
	if i >= len(s) || s[i] != '"' {
		return "", 0, false
	}
	var out strings.Builder
	for j := i + 1; j < len(s); j++ {
		switch s[j] {
		case '\\':
			if j+1 >= len(s) {
				return "", 0, false
			}
			j++
			out.WriteByte(s[j])
		case '"':
			return out.String(), j + 1, true
		default:
			out.WriteByte(s[j])
		}
	}
	return "", 0, false
}

func hsSpace(s string, i int) int {
	for i < len(s) && s[i] == ' ' {
		i++
	}
	return i
}

func pgHstore(s string) (Map[string, Option[string]], bool) {
	var m Map[string, Option[string]]
	i := hsSpace(s, 0)
	for i < len(s) {
		k, j, ok := hsQuoted(s, i)
		if !ok {
			return m, false
		}
		i = hsSpace(s, j)
		if i+1 >= len(s) || s[i] != '=' || s[i+1] != '>' {
			return m, false
		}
		i = hsSpace(s, i+2)
		if strings.HasPrefix(s[i:], "NULL") {
			Map_put(nil, &m, k, None[string]())
			i += 4
		} else {
			v, j2, ok := hsQuoted(s, i)
			if !ok {
				return m, false
			}
			Map_put(nil, &m, k, Some(v))
			i = j2
		}
		i = hsSpace(s, i)
		if i < len(s) {
			if s[i] != ',' {
				return m, false
			}
			i = hsSpace(s, i+1)
			if i >= len(s) {
				return m, false
			}
		}
	}
	return m, true
}

// M_date_bound is `row.date_bound(name)` (decision 0679).
func (r Sql_Row) M_date_bound(_ *Task, name string) Result[Sql_Bound[Time_Date], Sql_SqlErr] {
	return sqlCol(r, name, "a date", pgTyped(pgDateBound))
}

// M_date_bound_opt is `row.date_bound_opt(name)`.
func (r Sql_Row) M_date_bound_opt(_ *Task, name string) Result[Option[Sql_Bound[Time_Date]], Sql_SqlErr] {
	return sqlColOpt(r, name, "a date", pgTyped(pgDateBound))
}

// M_time_bound is `row.time_bound(name)`.
func (r Sql_Row) M_time_bound(_ *Task, name string) Result[Sql_Bound[Time], Sql_SqlErr] {
	return sqlCol(r, name, "a time", asTimeBound)
}

// M_time_bound_opt is `row.time_bound_opt(name)`.
func (r Sql_Row) M_time_bound_opt(_ *Task, name string) Result[Option[Sql_Bound[Time]], Sql_SqlErr] {
	return sqlColOpt(r, name, "a time", asTimeBound)
}

// M_mac is `row.mac(name)`.
func (r Sql_Row) M_mac(_ *Task, name string) Result[Bytes, Sql_SqlErr] {
	return sqlCol(r, name, "a macaddr", pgTyped(pgMac))
}

// M_mac_opt is `row.mac_opt(name)`.
func (r Sql_Row) M_mac_opt(_ *Task, name string) Result[Option[Bytes], Sql_SqlErr] {
	return sqlColOpt(r, name, "a macaddr", pgTyped(pgMac))
}

// M_hstore is `row.hstore(name)`.
func (r Sql_Row) M_hstore(_ *Task, name string) Result[Map[string, Option[string]], Sql_SqlErr] {
	return sqlCol(r, name, "an hstore", pgTyped(pgHstore))
}

// M_hstore_opt is `row.hstore_opt(name)`.
func (r Sql_Row) M_hstore_opt(_ *Task, name string) Result[Option[Map[string, Option[string]]], Sql_SqlErr] {
	return sqlColOpt(r, name, "an hstore", pgTyped(pgHstore))
}
