package rt

import (
	"strconv"
	"strings"
)

// Decoding std/sql rows into structs (`derive SqlRow`, std.md 10.4).

// sqlRowDecoder is implemented by the pointer of a struct with
// `derive SqlRow`; the method is generated (internal/gen/sqlrow.go).
type sqlRowDecoder interface {
	XoSqlRowDec(tk *Task, r Sql_Row) *Sql_SqlErr
}

// SqlRowField decodes column name of r into *dst, for the generated
// XoSqlRowDec. The field types are the ones the checker accepts: Int,
// Float, Str, Bool, Time, and an Option of one. The rules are those of the
// row accessors: a missing column is NoColumn, a value of another type (or
// NULL for a field that is not an Option) is Mismatch.
func SqlRowField[T any](r Sql_Row, name string, dst *T) *Sql_SqlErr {
	var res any
	switch any(dst).(type) {
	case *int64:
		res = sqlCol(r, name, "an integer", asInt)
	case *float64:
		res = sqlCol(r, name, "a float", asFloat)
	case *string:
		res = sqlCol(r, name, "text", asStr)
	case *bool:
		res = sqlCol(r, name, "a boolean", asBool)
	case *Time:
		res = sqlCol(r, name, "a time", asTime)
	case *Option[int64]:
		res = sqlColOpt(r, name, "an integer", asInt)
	case *Option[float64]:
		res = sqlColOpt(r, name, "a float", asFloat)
	case *Option[string]:
		res = sqlColOpt(r, name, "text", asStr)
	case *Option[bool]:
		res = sqlColOpt(r, name, "a boolean", asBool)
	case *Option[Time]:
		res = sqlColOpt(r, name, "a time", asTime)
	case *Bytes:
		res = sqlCol(r, name, "bytes", asBytes)
	case *Option[Bytes]:
		res = sqlColOpt(r, name, "bytes", asBytes)
	case *List[int64]:
		res = r.M_ints(nil, name)
	case *List[string]:
		res = r.M_strs(nil, name)
	default:
		e := sqlE(sqlTagMismatch, Sql_SqlErr_Mismatch{F_column: name, F_msg: "the field type cannot be read from a column"})
		return &e
	}
	x := res.(Result[T, Sql_SqlErr])
	if !x.Ok {
		return &x.E
	}
	*dst = x.V
	return nil
}

func sqlDecode[T any](tk *Task, r Sql_Row) Result[T, Sql_SqlErr] {
	var v T
	d, ok := any(&v).(sqlRowDecoder)
	if !ok {
		panic("std/sql: decode of a type without derive SqlRow")
	}
	if e := d.XoSqlRowDec(tk, r); e != nil {
		return Err[T](*e)
	}
	return Ok[T, Sql_SqlErr](v)
}

func sqlDecodeAll[T any](tk *Task, rows Result[List[Sql_Row], Sql_SqlErr]) Result[List[T], Sql_SqlErr] {
	if !rows.Ok {
		return Err[List[T]](rows.E)
	}
	xs := List_Slice(rows.V)
	out := make([]T, len(xs))
	for i, r := range xs {
		v := sqlDecode[T](tk, r)
		if !v.Ok {
			return Err[List[T]](v.E)
		}
		out[i] = v.V
	}
	return Ok[List[T], Sql_SqlErr](ListFromSlice(out))
}

func sqlDecodeOne[T any](tk *Task, row Result[Sql_Row, Sql_SqlErr]) Result[T, Sql_SqlErr] {
	if !row.Ok {
		return Err[T](row.E)
	}
	return sqlDecode[T](tk, row.V)
}

// Sql_Row_decode is `row.decode[T]()`.
func Sql_Row_decode[T any](tk *Task, r Sql_Row) Result[T, Sql_SqlErr] { return sqlDecode[T](tk, r) }

// Sql_Db_query_as is `db.query_as[T](q, args)`.
func Sql_Db_query_as[T any](tk *Task, d Sql_Db, q string, args List[Sql_SqlValue]) Result[List[T], Sql_SqlErr] {
	return sqlDecodeAll[T](tk, d.M_query(tk, q, args))
}

// Sql_Db_query_one_as is `db.query_one_as[T](q, args)`.
func Sql_Db_query_one_as[T any](tk *Task, d Sql_Db, q string, args List[Sql_SqlValue]) Result[T, Sql_SqlErr] {
	return sqlDecodeOne[T](tk, d.M_query_one(tk, q, args))
}

// Sql_Tx_query_as is `tx.query_as[T](q, args)`.
func Sql_Tx_query_as[T any](tk *Task, t Sql_Tx, q string, args List[Sql_SqlValue]) Result[List[T], Sql_SqlErr] {
	return sqlDecodeAll[T](tk, t.M_query(tk, q, args))
}

// Sql_Tx_query_one_as is `tx.query_one_as[T](q, args)`.
func Sql_Tx_query_one_as[T any](tk *Task, t Sql_Tx, q string, args List[Sql_SqlValue]) Result[T, Sql_SqlErr] {
	return sqlDecodeOne[T](tk, t.M_query_one(tk, q, args))
}

// Sql_Rows is `sql.Rows` (decision 0479). The Go backend reads the result
// whole, then hands it out row by row.
type Sql_Rows struct{ s *sqlRowsState }

type sqlRowsState struct {
	cols []string
	rows []Sql_Row
	pos  int
}

// XoDebug implements Debug.
func (Sql_Rows) XoDebug(b *strings.Builder) { b.WriteString("<rows>") }

func sqlRowsOf(r Result[List[Sql_Row], Sql_SqlErr]) Result[Sql_Rows, Sql_SqlErr] {
	if !r.Ok {
		return Err[Sql_Rows](r.E)
	}
	rows := List_Slice(r.V)
	var cols []string
	if len(rows) > 0 {
		cols = rows[0].cols
	}
	return Ok[Sql_Rows, Sql_SqlErr](Sql_Rows{&sqlRowsState{cols: cols, rows: rows}})
}

// M_rows is `db.rows(q, args)`.
func (d Sql_Db) M_rows(tk *Task, q string, args List[Sql_SqlValue]) Result[Sql_Rows, Sql_SqlErr] {
	return sqlRowsOf(d.M_query(tk, q, args))
}

// M_rows is `tx.rows(q, args)`.
func (t Sql_Tx) M_rows(tk *Task, q string, args List[Sql_SqlValue]) Result[Sql_Rows, Sql_SqlErr] {
	return sqlRowsOf(t.M_query(tk, q, args))
}

// M_columns is `rows.columns()`.
func (r Sql_Rows) M_columns(*Task) List[string] { return ListOf(r.s.cols...) }

// M_next is `rows.next()`.
func (r Sql_Rows) M_next(*Task) Result[Option[Sql_Row], Sql_SqlErr] {
	if r.s.pos >= len(r.s.rows) {
		return Ok[Option[Sql_Row], Sql_SqlErr](Option[Sql_Row]{})
	}
	row := r.s.rows[r.s.pos]
	r.s.pos++
	return Ok[Option[Sql_Row], Sql_SqlErr](Some(row))
}

// M_close is `rows.close()`.
func (r Sql_Rows) M_close(*Task) { r.s.pos = len(r.s.rows) }

// Sql_Rows_next_as is `rows.next_as[T]()`.
func Sql_Rows_next_as[T any](tk *Task, r Sql_Rows) Result[Option[T], Sql_SqlErr] {
	n := r.M_next(tk)
	if !n.Ok {
		return Err[Option[T]](n.E)
	}
	if !n.V.Some {
		return Ok[Option[T], Sql_SqlErr](Option[T]{})
	}
	d := sqlDecode[T](tk, n.V.V)
	if !d.Ok {
		return Err[Option[T]](d.E)
	}
	return Ok[Option[T], Sql_SqlErr](Some(d.V))
}

func sqlEach(tk *Task, rs Result[Sql_Rows, Sql_SqlErr], f func(*Task, Sql_Row) Result[Unit, Sql_SqlErr]) Result[Unit, Sql_SqlErr] {
	if !rs.Ok {
		return Err[Unit](rs.E)
	}
	for {
		n := rs.V.M_next(tk)
		if !n.Ok {
			return Err[Unit](n.E)
		}
		if !n.V.Some {
			return Ok[Unit, Sql_SqlErr](Unit{})
		}
		if r := f(tk, n.V.V); !r.Ok {
			rs.V.M_close(tk)
			return r
		}
	}
}

// M_each is `db.each(q, args, f)`.
func (d Sql_Db) M_each(tk *Task, q string, args List[Sql_SqlValue], f func(*Task, Sql_Row) Result[Unit, Sql_SqlErr]) Result[Unit, Sql_SqlErr] {
	return sqlEach(tk, d.M_rows(tk, q, args), f)
}

// M_each is `tx.each(q, args, f)`.
func (t Sql_Tx) M_each(tk *Task, q string, args List[Sql_SqlValue], f func(*Task, Sql_Row) Result[Unit, Sql_SqlErr]) Result[Unit, Sql_SqlErr] {
	return sqlEach(tk, t.M_rows(tk, q, args), f)
}

// Sql_quote_ident is `sql.quote_ident(name)`: name in double quotes, a
// double quote doubled.
func Sql_quote_ident(_ *Task, name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

func sqlCopyInsert(table string, cols []string) string {
	if len(cols) == 0 {
		return "INSERT INTO " + table + " VALUES"
	}
	ph := make([]string, len(cols))
	for i := range cols {
		ph[i] = "$" + strconv.Itoa(i+1)
	}
	return "INSERT INTO " + table + " (" + strings.Join(cols, ", ") + ") VALUES (" + strings.Join(ph, ", ") + ")"
}

// sqlCopy is COPY FROM on the Go backend (decision 0479): an INSERT per
// row through exec.
func sqlCopy(tk *Task, table string, cols List[string], rows func(*Task) Result[Option[List[Sql_SqlValue]], Sql_SqlErr], exec func(string, List[Sql_SqlValue]) Result[Sql_ExecResult, Sql_SqlErr]) Result[int64, Sql_SqlErr] {
	q := sqlCopyInsert(table, List_Slice(cols))
	var n int64
	for {
		r := rows(tk)
		if !r.Ok {
			return Err[int64](r.E)
		}
		if !r.V.Some {
			return Ok[int64, Sql_SqlErr](n)
		}
		if e := exec(q, r.V.V); !e.Ok {
			return Err[int64](e.E)
		}
		n++
	}
}

// M_copy_from is `db.copy_from(table, columns, rows)`: a driver's own
// COPY, else an INSERT per row.
func (d Sql_Db) M_copy_from(tk *Task, table string, cols List[string], rows func(*Task) Result[Option[List[Sql_SqlValue]], Sql_SqlErr]) Result[int64, Sql_SqlErr] {
	return withConn(tk, d.d, func() Result[int64, Sql_SqlErr] {
		if b, ok := d.d.be.(*sqlDriverBackend); ok {
			x, err := b.take(tk)
			if err != nil {
				return sqlFailErr[int64](err)
			}
			defer b.give(tk, x)
			return x.c.M_copy_from(tk, table, cols, rows)
		}
		return sqlCopy(tk, table, cols, rows, func(q string, a List[Sql_SqlValue]) Result[Sql_ExecResult, Sql_SqlErr] { return d.M_exec(tk, q, a) })
	})
}

// M_copy_from is `tx.copy_from(table, columns, rows)`.
func (t Sql_Tx) M_copy_from(tk *Task, table string, cols List[string], rows func(*Task) Result[Option[List[Sql_SqlValue]], Sql_SqlErr]) Result[int64, Sql_SqlErr] {
	return inTx(tk, t, func(be SqlTxBackend) Result[int64, Sql_SqlErr] {
		if dt, ok := be.(*sqlDriverTx); ok {
			return dt.x.c.M_copy_from(tk, table, cols, rows)
		}
		return sqlCopy(tk, table, cols, rows, func(q string, a List[Sql_SqlValue]) Result[Sql_ExecResult, Sql_SqlErr] {
			return sqlExec(tk, be, q, a)
		})
	})
}
