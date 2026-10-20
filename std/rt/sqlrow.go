package rt

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
