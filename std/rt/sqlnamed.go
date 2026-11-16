// sql.named on the Go backend: a port of pgx v5.12.0 named_args.go
// (NamedArgs and StrictNamedArgs RewriteQuery and its lexer), Copyright
// (c) 2013-2021 Jack Christensen, MIT License. Byte based, as
// std/native/sql.xo's: every delimiter is ASCII and a byte from 0x80 up
// is an identifier byte.

package rt

import (
	"strconv"
	"strings"
)

func naLetter(c byte) bool { return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' }

func naIdent(c byte) bool { return naLetter(c) || c == '_' || c >= 0x80 }

func naDigit(c byte) bool { return c >= '0' && c <= '9' }

// naQuoted is the index after a quoted string or identifier opened
// before i, with q doubled to escape it; esc also lets a backslash escape
// the next byte (an E'...' string).
func naQuoted(s string, i int, q byte, esc bool) int {
	for j := i; j < len(s); {
		c := s[j]
		j++
		if esc && c == '\\' {
			j++
		} else if c == q {
			if j < len(s) && s[j] == q {
				j++
			} else {
				return j
			}
		}
	}
	return len(s)
}

// naTag is the length of a dollar quote's tag at s[i:] up to its closing
// `$` (pgx's scanDollarQuoteTag).
func naTag(s string, i int) (int, bool) {
	for j := i; j < len(s); j++ {
		c := s[j]
		if c == '$' {
			return j - i, true
		}
		if !(naIdent(c) || j > i && naDigit(c)) {
			return 0, false
		}
	}
	return 0, false
}

// Sql_named is `sql.named(q, args, strict)` (decision 0679).
func Sql_named(tk *Task, q string, args Map[string, Sql_SqlValue], strict bool) Result[Tuple2[string, List[Sql_SqlValue]], Sql_SqlErr] {
	type res = Result[Tuple2[string, List[Sql_SqlValue]], Sql_SqlErr]
	n := len(q)
	var out strings.Builder
	var names []string
	ord := map[string]int{}
	start, i := 0, 0
	for i < n {
		c := q[i]
		i++
		if (c == 'e' || c == 'E') && i < n && q[i] == '\'' {
			i = naQuoted(q, i+1, '\'', true)
			continue
		}
		switch {
		case c == '\'' || c == '"':
			i = naQuoted(q, i, c, false)
		case c == '$':
			if t, ok := naTag(q, i); ok {
				tag := q[i-1 : i+t+1]
				i += t + 1
				if k := strings.Index(q[i:], tag); k >= 0 {
					i += k + len(tag)
				} else {
					i = n
				}
			}
		case c == '@' && i < n && (naLetter(q[i]) || q[i] == '_'):
			out.WriteString(q[start : i-1])
			j := i
			for j < n && (naLetter(q[j]) || naDigit(q[j]) || q[j] == '_') {
				j++
			}
			name := q[i:j]
			k, ok := ord[name]
			if !ok {
				names = append(names, name)
				k = len(names)
				ord[name] = k
			}
			out.WriteString("$" + strconv.Itoa(k))
			start, i = j, j
			continue
		case c == '-' && i < n && q[i] == '-':
			for i < n && q[i] != '\n' && q[i] != '\r' {
				i++
			}
			i++
		case c == '/' && i < n && q[i] == '*':
			i++
			depth := 0
			for i < n {
				d := q[i]
				i++
				if d == '/' && i < n && q[i] == '*' {
					i++
					depth++
				} else if d == '*' && i < n && q[i] == '/' {
					i++
					if depth == 0 {
						break
					}
					depth--
				}
			}
		}
		if naIdent(c) {
			for i < n && (naIdent(q[i]) || naDigit(q[i]) || q[i] == '$') {
				i++
			}
		}
	}
	if start < n {
		out.WriteString(q[start:])
	}
	vals := make([]Sql_SqlValue, 0, len(names))
	for _, name := range names {
		v := Map_get(tk, args, name)
		if !v.Some {
			if strict {
				return Err[Tuple2[string, List[Sql_SqlValue]]](sqlMsgErr(sqlTagParam, "named argument @"+name+" has no value"))
			}
			vals = append(vals, Sql_null(tk))
			continue
		}
		vals = append(vals, v.V)
	}
	if strict {
		for name := range Map_All(args) {
			if _, ok := ord[name]; !ok {
				return Err[Tuple2[string, List[Sql_SqlValue]]](sqlMsgErr(sqlTagParam, "named argument "+name+" is not used by the query"))
			}
		}
	}
	return res(Ok[Tuple2[string, List[Sql_SqlValue]], Sql_SqlErr](Tuple2[string, List[Sql_SqlValue]]{F0: out.String(), F1: ListOf(vals...)}))
}
