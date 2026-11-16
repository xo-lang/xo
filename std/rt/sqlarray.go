// Arrays' elements on the Go backend (decision 0679): pgx's array text
// format (pgtype/array.go parseUntypedTextArray) for one dimension, the
// same as std/native/sql.xo's arr_split.

package rt

import "strings"

// arrSplit is a one dimensional array literal's elements, nil for an
// unquoted NULL; ok false for another shape.
func arrSplit(s string) ([]*string, bool) {
	if len(s) < 2 || s[0] != '{' || s[len(s)-1] != '}' {
		return nil, false
	}
	out := []*string{}
	end := len(s) - 1
	for i := 1; i < end; {
		if s[i] == '{' {
			return nil, false
		}
		var cur strings.Builder
		quoted := false
		if s[i] == '"' {
			quoted = true
			i++
			for i < end && s[i] != '"' {
				if s[i] == '\\' && i+1 < len(s) {
					i++
				}
				cur.WriteByte(s[i])
				i++
			}
			i++
		} else {
			for i < end && s[i] != ',' {
				cur.WriteByte(s[i])
				i++
			}
		}
		t := cur.String()
		if !quoted && t == "NULL" {
			out = append(out, nil)
		} else {
			out = append(out, &t)
		}
		if i < end && s[i] == ',' {
			i++
		}
	}
	return out, true
}

// M_elements_opt is `row.elements_opt(name)`.
func (r Sql_Row) M_elements_opt(_ *Task, name string) Result[Option[List[Sql_Row]], Sql_SqlErr] {
	v, e := r.get(name)
	if e != nil {
		return Err[Option[List[Sql_Row]]](*e)
	}
	if v == nil {
		return Ok[Option[List[Sql_Row]], Sql_SqlErr](Option[List[Sql_Row]]{})
	}
	var vals []any
	if items, ok := asItems(v); ok {
		vals = sqlArgs(items)
	} else if s, ok := v.(string); ok {
		ts, ok := arrSplit(s)
		if !ok {
			return Err[Option[List[Sql_Row]]](sqlMismatch(name, "a one dimensional array", v))
		}
		for _, t := range ts {
			if t == nil {
				vals = append(vals, nil)
			} else {
				vals = append(vals, *t)
			}
		}
	} else {
		return Err[Option[List[Sql_Row]]](sqlMismatch(name, "an array", v))
	}
	rows := make([]Sql_Row, len(vals))
	for i, x := range vals {
		rows[i] = Sql_Row{cols: []string{name}, vals: []any{x}}
	}
	return Ok[Option[List[Sql_Row]], Sql_SqlErr](Some(ListOf(rows...)))
}

// M_elements is `row.elements(name)`.
func (r Sql_Row) M_elements(tk *Task, name string) Result[List[Sql_Row], Sql_SqlErr] {
	o := r.M_elements_opt(tk, name)
	if !o.Ok {
		return Err[List[Sql_Row]](o.E)
	}
	if !o.V.Some {
		return Err[List[Sql_Row]](sqlMismatch(name, "an array", nil))
	}
	return Ok[List[Sql_Row], Sql_SqlErr](o.V.V)
}
