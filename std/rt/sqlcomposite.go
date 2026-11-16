// Postgres composites and records on the Go backend (decision 0679): a
// port of pgx v5.12.0 pgtype/composite.go (CompositeTextScanner,
// CompositeTextBuilder, quoteCompositeFieldIfNeeded), Copyright (c)
// 2013-2021 Jack Christensen, MIT License; the same as std/native/sql.xo's.

package rt

import (
	"strconv"
	"strings"
)

// compSplit is a composite's text split into its fields, or the reason
// it is not one.
func compSplit(s string) ([]Option[string], string) {
	n := len(s)
	if n < 2 {
		return nil, "record incomplete"
	}
	if s[0] != '(' {
		return nil, "composite text format must start with '('"
	}
	if s[n-1] != ')' {
		return nil, "composite text format must end with ')'"
	}
	var out []Option[string]
	for i := 1; i < n; {
		switch s[i] {
		case ',', ')':
			out = append(out, None[string]())
			i++
		case '"':
			i++
			var f strings.Builder
		quoted:
			for {
				if i >= n {
					return nil, "composite text format unterminated quoted field"
				}
				switch d := s[i]; d {
				case '"':
					i++
					if i < n && s[i] == '"' {
						f.WriteByte('"')
						i++
					} else {
						break quoted
					}
				case '\\':
					i++
					if i >= n {
						return nil, "composite text format unterminated quoted field"
					}
					f.WriteByte(s[i])
					i++
				default:
					f.WriteByte(d)
					i++
				}
			}
			out = append(out, Some(f.String()))
			i++
		default:
			start := i
			for s[i] != ',' && s[i] != ')' {
				i++
			}
			out = append(out, Some(s[start:i]))
			i++
		}
	}
	return out, ""
}

// Sql_parse_composite is `sql.parse_composite(s)`.
func Sql_parse_composite(_ *Task, s string) Result[List[Option[string]], Sql_SqlErr] {
	fs, msg := compSplit(s)
	if msg != "" {
		return Err[List[Option[string]]](sqlMsgErr(sqlTagSyntax, msg+": "+strconv.Quote(s)))
	}
	return Ok[List[Option[string]], Sql_SqlErr](ListOf(fs...))
}

func compQuote(f string) string {
	if f == "" || f[0] == ' ' || f[len(f)-1] == ' ' || strings.ContainsAny(f, `(),"\`) {
		return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(f) + `"`
	}
	return f
}

// Sql_composite is `sql.composite(fields)`.
func Sql_composite(tk *Task, fields List[Option[string]]) Sql_SqlValue {
	parts := make([]string, 0, len(List_Slice(fields)))
	for _, f := range List_Slice(fields) {
		if f.Some {
			parts = append(parts, compQuote(f.V))
		} else {
			parts = append(parts, "")
		}
	}
	return Sql_str(tk, "("+strings.Join(parts, ",")+")")
}

func pgComposite(s string) (List[Option[string]], bool) {
	fs, msg := compSplit(s)
	return ListOf(fs...), msg == ""
}

// M_fields is `row.fields(name)`.
func (r Sql_Row) M_fields(_ *Task, name string) Result[List[Option[string]], Sql_SqlErr] {
	return sqlCol(r, name, "a composite", pgTyped(pgComposite))
}

// M_fields_opt is `row.fields_opt(name)`.
func (r Sql_Row) M_fields_opt(_ *Task, name string) Result[Option[List[Option[string]]], Sql_SqlErr] {
	return sqlColOpt(r, name, "a composite", pgTyped(pgComposite))
}
