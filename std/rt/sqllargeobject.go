// Postgres large objects on the Go backend (decision 0679): a port of pgx
// v5.12.0 large_objects.go (LargeObjects.Create, Open, Unlink;
// LargeObject.Write, Read, Seek, Tell, Truncate, Close), Copyright (c)
// 2013-2021 Jack Christensen, MIT License; the same as
// std/native/sql.xo's.

package rt

import "strings"

// loChunk is pgx's maxLargeObjectMessageLength.
const loChunk = 1024*1024*1024 - 1024

// loInt is the one column of a `select lo_...(...)` row as an int64.
func loInt(tk *Task, t Sql_Tx, q string, args ...Sql_SqlValue) Result[int64, Sql_SqlErr] {
	r := t.M_query_one(tk, q, ListOf(args...))
	if !r.Ok {
		return Err[int64](r.E)
	}
	return r.V.M_int(tk, List_Slice(r.V.M_columns(tk))[0])
}

func loErr[T any](msg string) Result[T, Sql_SqlErr] {
	return Err[T](sqlE(sqlTagDriver, Sql_SqlErr_Driver{F_msg: msg}))
}

// M_lo_create is `tx.lo_create(oid)`.
func (t Sql_Tx) M_lo_create(tk *Task, oid int64) Result[int64, Sql_SqlErr] {
	return loInt(tk, t, "select lo_create($1)", Sql_int(tk, oid))
}

// M_lo_open is `tx.lo_open(oid, mode)`.
func (t Sql_Tx) M_lo_open(tk *Task, oid int64, mode Sql_LoMode) Result[Sql_LargeObject, Sql_SqlErr] {
	m := int64(0x60000)
	switch mode.Tag {
	case 1:
		m = 0x40000
	case 2:
		m = 0x20000
	}
	r := loInt(tk, t, "select lo_open($1, $2)", Sql_int(tk, oid), Sql_int(tk, m))
	if !r.Ok {
		return Err[Sql_LargeObject](r.E)
	}
	return Ok[Sql_LargeObject, Sql_SqlErr](Sql_LargeObject{F_fd: r.V})
}

// M_lo_unlink is `tx.lo_unlink(oid)`.
func (t Sql_Tx) M_lo_unlink(tk *Task, oid int64) Result[Unit, Sql_SqlErr] {
	r := loInt(tk, t, "select lo_unlink($1)", Sql_int(tk, oid))
	if !r.Ok {
		return Err[Unit](r.E)
	}
	if r.V != 1 {
		return loErr[Unit]("failed to remove large object")
	}
	return Ok[Unit, Sql_SqlErr](Unit{})
}

// M_lo_write is `tx.lo_write(o, b)` (pgx's Write).
func (t Sql_Tx) M_lo_write(tk *Task, o Sql_LargeObject, b Bytes) Result[int64, Sql_SqlErr] {
	total := 0
	for total < len(b) {
		want := min(len(b)-total, loChunk)
		r := loInt(tk, t, "select lowrite($1, $2)", Sql_int(tk, o.F_fd), Sql_bytes(tk, b[total:total+want]))
		if !r.Ok {
			return r
		}
		n := int(r.V)
		if n < 0 {
			return loErr[int64]("failed to write to large object")
		}
		total += n
		if n < want {
			return loErr[int64]("short write to large object")
		}
		if n > want {
			return loErr[int64]("invalid write to large object")
		}
	}
	return Ok[int64, Sql_SqlErr](int64(total))
}

// M_lo_read is `tx.lo_read(o, n)` (pgx's Read; a short read is the end).
func (t Sql_Tx) M_lo_read(tk *Task, o Sql_LargeObject, n int64) Result[Bytes, Sql_SqlErr] {
	var out strings.Builder
	total := int64(0)
	for total < n {
		want := min(n-total, loChunk)
		r := t.M_query_one(tk, "select loread($1, $2)", ListOf(Sql_int(tk, o.F_fd), Sql_int(tk, want)))
		if !r.Ok {
			return Err[Bytes](r.E)
		}
		got := r.V.M_bytes(tk, List_Slice(r.V.M_columns(tk))[0])
		if !got.Ok {
			return got
		}
		out.WriteString(string(got.V))
		total += int64(len(got.V))
		if int64(len(got.V)) < want {
			break
		}
		if int64(len(got.V)) > want {
			return loErr[Bytes]("invalid read of large object")
		}
	}
	return Ok[Bytes, Sql_SqlErr](Bytes(out.String()))
}

// M_lo_seek is `tx.lo_seek(o, offset, whence)`.
func (t Sql_Tx) M_lo_seek(tk *Task, o Sql_LargeObject, offset, whence int64) Result[int64, Sql_SqlErr] {
	return loInt(tk, t, "select lo_lseek64($1, $2, $3)", Sql_int(tk, o.F_fd), Sql_int(tk, offset), Sql_int(tk, whence))
}

// M_lo_tell is `tx.lo_tell(o)`.
func (t Sql_Tx) M_lo_tell(tk *Task, o Sql_LargeObject) Result[int64, Sql_SqlErr] {
	return loInt(tk, t, "select lo_tell64($1)", Sql_int(tk, o.F_fd))
}

func loExec(tk *Task, t Sql_Tx, q string, args ...Sql_SqlValue) Result[Unit, Sql_SqlErr] {
	r := t.M_exec(tk, q, ListOf(args...))
	if !r.Ok {
		return Err[Unit](r.E)
	}
	return Ok[Unit, Sql_SqlErr](Unit{})
}

// M_lo_truncate is `tx.lo_truncate(o, size)`.
func (t Sql_Tx) M_lo_truncate(tk *Task, o Sql_LargeObject, size int64) Result[Unit, Sql_SqlErr] {
	return loExec(tk, t, "select lo_truncate64($1, $2)", Sql_int(tk, o.F_fd), Sql_int(tk, size))
}

// M_lo_close is `tx.lo_close(o)`.
func (t Sql_Tx) M_lo_close(tk *Task, o Sql_LargeObject) Result[Unit, Sql_SqlErr] {
	return loExec(tk, t, "select lo_close($1)", Sql_int(tk, o.F_fd))
}
