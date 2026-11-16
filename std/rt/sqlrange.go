// Postgres ranges and multiranges on the Go backend (decision 0679): a
// port of pgx v5.12.0 pgtype/range.go (parseUntypedTextRange,
// rangeParseValue, rangeParseQuotedValue) and pgtype/multirange.go
// (parseUntypedTextMultirange, parseRange), Copyright (c) 2013-2021 Jack
// Christensen, MIT License; the same as std/native/sql.xo's.

package rt

import (
	"strconv"
	"strings"
)

// rawRange is a range's text split into its edges: kind 0 unbounded, 1
// inclusive, 2 exclusive.
type rawRange struct {
	empty  bool
	lk, uk int
	lo, hi string
}

func rgSpace(s string, i int) int {
	for i < len(s) && (s[i] == ' ' || s[i] >= '\t' && s[i] <= '\r') {
		i++
	}
	return i
}

// rgQuoted is a double quoted bound's text from after its opening quote.
func rgQuoted(s string, i int) (string, int, bool) {
	var out strings.Builder
	for j := i; j < len(s); j++ {
		switch s[j] {
		case '\\':
			if j+1 >= len(s) {
				return "", 0, false
			}
			j++
			out.WriteByte(s[j])
		case '"':
			if j+1 >= len(s) {
				return "", 0, false
			}
			if s[j+1] != '"' {
				return out.String(), j + 1, true
			}
			j++
			out.WriteByte('"')
		default:
			out.WriteByte(s[j])
		}
	}
	return "", 0, false
}

// rgValue is a bound's text at i, up to a `,`, bracket or parenthesis.
func rgValue(s string, i int) (string, int, bool) {
	if i >= len(s) {
		return "", 0, false
	}
	if s[i] == '"' {
		return rgQuoted(s, i+1)
	}
	var out strings.Builder
	for j := i; j < len(s); j++ {
		switch s[j] {
		case '\\':
			if j+1 >= len(s) {
				return "", 0, false
			}
			j++
			out.WriteByte(s[j])
		case ',', '[', ']', '(', ')':
			return out.String(), j, true
		default:
			out.WriteByte(s[j])
		}
	}
	return "", 0, false
}

func rgParse(s string) (rawRange, bool) {
	var r rawRange
	if s == "empty" {
		r.empty = true
		return r, true
	}
	n := len(s)
	i := rgSpace(s, 0)
	if i >= n {
		return r, false
	}
	switch s[i] {
	case '(':
		r.lk = 2
	case '[':
		r.lk = 1
	default:
		return r, false
	}
	i++
	if i >= n {
		return r, false
	}
	if s[i] == ',' {
		r.lk = 0
	} else {
		v, j, ok := rgValue(s, i)
		if !ok {
			return r, false
		}
		r.lo, i = v, j
	}
	if i >= n || s[i] != ',' {
		return r, false
	}
	i++
	if i >= n {
		return r, false
	}
	if s[i] == ')' || s[i] == ']' {
		i++
	} else {
		v, j, ok := rgValue(s, i)
		if !ok {
			return r, false
		}
		r.hi, i = v, j
		if i >= n {
			return r, false
		}
		switch s[i] {
		case ')':
			r.uk = 2
		case ']':
			r.uk = 1
		default:
			return r, false
		}
		i++
	}
	if rgSpace(s, i) < n {
		return r, false
	}
	return r, true
}

func rgEdge[T any](k int, v string, f func(string) (T, bool)) (Sql_Edge[T], bool) {
	if k == 0 {
		return Sql_Edge[T]{Tag: 3}, true
	}
	x, ok := f(v)
	if !ok {
		return Sql_Edge[T]{}, false
	}
	if k == 1 {
		return Sql_Edge[T]{Tag: 1, P: Sql_Edge_Inclusive[T]{F_v: x}}, true
	}
	return Sql_Edge[T]{Tag: 2, P: Sql_Edge_Exclusive[T]{F_v: x}}, true
}

func rgTyped[T any](f func(string) (T, bool)) func(string) (Sql_Range[T], bool) {
	return func(s string) (Sql_Range[T], bool) {
		r, ok := rgParse(s)
		if !ok {
			return Sql_Range[T]{}, false
		}
		if r.empty {
			return Sql_Range[T]{Tag: 1}, true
		}
		lo, ok1 := rgEdge(r.lk, r.lo, f)
		hi, ok2 := rgEdge(r.uk, r.hi, f)
		if !ok1 || !ok2 {
			return Sql_Range[T]{}, false
		}
		return Sql_Range[T]{Tag: 2, P: Sql_Range_Span[T]{F_lower: lo, F_upper: hi}}, true
	}
}

// mrSplit is a multirange's text split into its ranges' texts.
func mrSplit(s string) ([]string, bool) {
	n := len(s)
	i := rgSpace(s, 0)
	if i >= n || s[i] != '{' {
		return nil, false
	}
	i++
	out := []string{}
	for {
		if i >= n {
			return nil, false
		}
		c := s[i]
		if c == ',' {
			i++
			continue
		}
		if c == '}' {
			i++
			break
		}
		j := i
		inRange, inQ, esc := false, false, false
	scan:
		for {
			if j >= n {
				return nil, false
			}
			d := s[j]
			switch {
			case esc:
				esc = false
			case d == '\\':
				esc = true
			case d == '"':
				inQ = !inQ
			case !inQ:
				switch d {
				case '[', '(':
					inRange = true
				case ']', ')':
					inRange = false
				case ',', '}':
					if !inRange {
						break scan
					}
				}
			}
			j++
		}
		out = append(out, s[i:j])
		i = j
	}
	if rgSpace(s, i) < n {
		return nil, false
	}
	return out, true
}

func mrTyped[T any](f func(string) (T, bool)) func(string) (List[Sql_Range[T]], bool) {
	rf := rgTyped(f)
	return func(s string) (List[Sql_Range[T]], bool) {
		parts, ok := mrSplit(s)
		if !ok {
			return List[Sql_Range[T]]{}, false
		}
		out := make([]Sql_Range[T], 0, len(parts))
		for _, p := range parts {
			r, ok := rf(p)
			if !ok {
				return List[Sql_Range[T]]{}, false
			}
			out = append(out, r)
		}
		return ListOf(out...), true
	}
}

func pgIntText(s string) (int64, bool) {
	n, err := strconv.ParseInt(s, 10, 64)
	return n, err == nil
}

// pgTimestamp is a timestamp or timestamptz's ISO text: no offset after
// the time reads as UTC (pg.xo's iso_to_rfc3339).
func pgTimestamp(s string) (Time, bool) {
	t := strings.ReplaceAll(s, " ", "T")
	i := len(t) - 1
	for i > 10 && t[i] != '+' && t[i] != '-' {
		i--
	}
	if i <= 10 {
		t += "Z"
	} else if len(t)-i == 3 {
		t += ":00"
	}
	o := Time_parse_rfc3339(nil, t)
	return o.V, o.Some
}

func rgText[T any](r Sql_Range[T], f func(T) string) string {
	if r.Tag == 1 {
		return "empty"
	}
	sp := r.P.(Sql_Range_Span[T])
	var b strings.Builder
	switch sp.F_lower.Tag {
	case 1:
		b.WriteString("[" + f(sp.F_lower.P.(Sql_Edge_Inclusive[T]).F_v))
	case 2:
		b.WriteString("(" + f(sp.F_lower.P.(Sql_Edge_Exclusive[T]).F_v))
	default:
		b.WriteString("(")
	}
	b.WriteString(",")
	switch sp.F_upper.Tag {
	case 1:
		b.WriteString(f(sp.F_upper.P.(Sql_Edge_Inclusive[T]).F_v) + "]")
	case 2:
		b.WriteString(f(sp.F_upper.P.(Sql_Edge_Exclusive[T]).F_v) + ")")
	default:
		b.WriteString(")")
	}
	return b.String()
}

func mrText[T any](rs List[Sql_Range[T]], f func(T) string) string {
	parts := make([]string, 0, len(List_Slice(rs)))
	for _, r := range List_Slice(rs) {
		parts = append(parts, rgText(r, f))
	}
	return "{" + strings.Join(parts, ",") + "}"
}

func intText(n int64) string { return strconv.FormatInt(n, 10) }

func dateText(d Time_Date) string {
	v := Sql_date(nil, d)
	return v.P.(Sql_SqlValue_Text).F_s
}

func timeText(t Time) string { return `"` + Time_format_rfc3339(nil, t) + `"` }

// Sql_range_int is `sql.range_int(r)`.
func Sql_range_int(tk *Task, r Sql_Range[int64]) Sql_SqlValue {
	return Sql_str(tk, rgText(r, intText))
}

// Sql_range_date is `sql.range_date(r)`.
func Sql_range_date(tk *Task, r Sql_Range[Time_Date]) Sql_SqlValue {
	return Sql_str(tk, rgText(r, dateText))
}

// Sql_range_time is `sql.range_time(r)`.
func Sql_range_time(tk *Task, r Sql_Range[Time]) Sql_SqlValue {
	return Sql_str(tk, rgText(r, timeText))
}

// Sql_multirange_int is `sql.multirange_int(rs)`.
func Sql_multirange_int(tk *Task, rs List[Sql_Range[int64]]) Sql_SqlValue {
	return Sql_str(tk, mrText(rs, intText))
}

// Sql_multirange_date is `sql.multirange_date(rs)`.
func Sql_multirange_date(tk *Task, rs List[Sql_Range[Time_Date]]) Sql_SqlValue {
	return Sql_str(tk, mrText(rs, dateText))
}

// Sql_multirange_time is `sql.multirange_time(rs)`.
func Sql_multirange_time(tk *Task, rs List[Sql_Range[Time]]) Sql_SqlValue {
	return Sql_str(tk, mrText(rs, timeText))
}

// M_range_int is `row.range_int(name)`.
func (r Sql_Row) M_range_int(_ *Task, name string) Result[Sql_Range[int64], Sql_SqlErr] {
	return sqlCol(r, name, "an int range", pgTyped(rgTyped(pgIntText)))
}

// M_range_int_opt is `row.range_int_opt(name)`.
func (r Sql_Row) M_range_int_opt(_ *Task, name string) Result[Option[Sql_Range[int64]], Sql_SqlErr] {
	return sqlColOpt(r, name, "an int range", pgTyped(rgTyped(pgIntText)))
}

// M_range_date is `row.range_date(name)`.
func (r Sql_Row) M_range_date(_ *Task, name string) Result[Sql_Range[Time_Date], Sql_SqlErr] {
	return sqlCol(r, name, "a date range", pgTyped(rgTyped(pgDate)))
}

// M_range_date_opt is `row.range_date_opt(name)`.
func (r Sql_Row) M_range_date_opt(_ *Task, name string) Result[Option[Sql_Range[Time_Date]], Sql_SqlErr] {
	return sqlColOpt(r, name, "a date range", pgTyped(rgTyped(pgDate)))
}

// M_range_time is `row.range_time(name)`.
func (r Sql_Row) M_range_time(_ *Task, name string) Result[Sql_Range[Time], Sql_SqlErr] {
	return sqlCol(r, name, "a time range", pgTyped(rgTyped(pgTimestamp)))
}

// M_range_time_opt is `row.range_time_opt(name)`.
func (r Sql_Row) M_range_time_opt(_ *Task, name string) Result[Option[Sql_Range[Time]], Sql_SqlErr] {
	return sqlColOpt(r, name, "a time range", pgTyped(rgTyped(pgTimestamp)))
}

// M_multirange_int is `row.multirange_int(name)`.
func (r Sql_Row) M_multirange_int(_ *Task, name string) Result[List[Sql_Range[int64]], Sql_SqlErr] {
	return sqlCol(r, name, "an int multirange", pgTyped(mrTyped(pgIntText)))
}

// M_multirange_int_opt is `row.multirange_int_opt(name)`.
func (r Sql_Row) M_multirange_int_opt(_ *Task, name string) Result[Option[List[Sql_Range[int64]]], Sql_SqlErr] {
	return sqlColOpt(r, name, "an int multirange", pgTyped(mrTyped(pgIntText)))
}

// M_multirange_date is `row.multirange_date(name)`.
func (r Sql_Row) M_multirange_date(_ *Task, name string) Result[List[Sql_Range[Time_Date]], Sql_SqlErr] {
	return sqlCol(r, name, "a date multirange", pgTyped(mrTyped(pgDate)))
}

// M_multirange_date_opt is `row.multirange_date_opt(name)`.
func (r Sql_Row) M_multirange_date_opt(_ *Task, name string) Result[Option[List[Sql_Range[Time_Date]]], Sql_SqlErr] {
	return sqlColOpt(r, name, "a date multirange", pgTyped(mrTyped(pgDate)))
}

// M_multirange_time is `row.multirange_time(name)`.
func (r Sql_Row) M_multirange_time(_ *Task, name string) Result[List[Sql_Range[Time]], Sql_SqlErr] {
	return sqlCol(r, name, "a time multirange", pgTyped(mrTyped(pgTimestamp)))
}

// M_multirange_time_opt is `row.multirange_time_opt(name)`.
func (r Sql_Row) M_multirange_time_opt(_ *Task, name string) Result[Option[List[Sql_Range[Time]]], Sql_SqlErr] {
	return sqlColOpt(r, name, "a time multirange", pgTyped(mrTyped(pgTimestamp)))
}
