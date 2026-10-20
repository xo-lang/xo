package rt

import (
	"strings"
	"sync"
	"sync/atomic"
)

// std/sql (std.md 10): a Db over a driver. The only driver that ships is the
// in memory one (sqlmem.go); real drivers implement SqlOpener and register
// with RegisterSqlDriver (a Go package bound with `use go`).

// SqlKind classifies a driver error.
type SqlKind int

// The kinds of SqlError; each maps to the SqlErr variant of the same name.
const (
	SqlSyntax SqlKind = iota + 1
	SqlConstraint
	SqlParam
	SqlNoTable
	SqlConn
	SqlDriver
)

// SqlError is the error a driver returns. Any other error is a Driver error.
// Code is the SQLSTATE and Name the constraint of a SqlConstraint error
// (decision 0278).
type SqlError struct {
	Kind SqlKind
	Msg  string
	Code string
	Name string
}

func (e *SqlError) Error() string { return e.Msg }

// SqlOpts are the pool options passed to a driver.
type SqlOpts struct {
	MaxOpen     int
	MaxIdle     int
	MaxLifetime Duration
}

// SqlOpener opens a backend from a DSN.
type SqlOpener interface {
	Open(dsn string, o SqlOpts) (SqlBackend, error)
}

// SqlBackend is one database. Values are nil, int64, string, float64, bool,
// Time, Bytes, SqlJSON, and Sql_SqlValue (an Array, decision 0278). Parameters are the positional arguments of $1, $2, ...
type SqlBackend interface {
	Query(q string, args []any) (cols []string, rows [][]any, err error)
	Exec(q string, args []any) (affected, lastInsertID int64, err error)
	Begin() (SqlTxBackend, error)
	Ping() error
	Close() error
	// SingleWriter reports that at most one transaction may be open at a
	// time (a driver without its own transaction isolation).
	SingleWriter() bool
}

// SqlTxBackend is one open transaction.
type SqlTxBackend interface {
	Query(q string, args []any) (cols []string, rows [][]any, err error)
	Exec(q string, args []any) (affected, lastInsertID int64, err error)
	Commit() error
	Rollback() error
}

var (
	sqlDriversMu sync.Mutex
	sqlDrivers   = map[string]SqlOpener{}
)

// RegisterSqlDriver makes a driver available to `sql.open`.
func RegisterSqlDriver(name string, d SqlOpener) {
	sqlDriversMu.Lock()
	sqlDrivers[name] = d
	sqlDriversMu.Unlock()
}

type sqlDB struct {
	be     SqlBackend
	pool   *lock // max_open permits: operations and open transactions
	writer *lock // one permit when the backend is a single writer
	closed atomic.Bool
}

// Sql_Db is `sql.Db`: copies share the database.
type Sql_Db struct{ d *sqlDB }

// Sql_Tx is `sql.Tx`.
type Sql_Tx struct{ t *sqlTx }

type sqlTx struct {
	db   *sqlDB
	be   SqlTxBackend
	mu   sync.Mutex
	done bool
}

// Sql_Row is `sql.Row`.
type Sql_Row struct {
	cols []string
	vals []any
}

// XoDebug implements Debug.
func (Sql_Db) XoDebug(b *strings.Builder) { b.WriteString("<db>") }

// XoDebug implements Debug.
func (Sql_Tx) XoDebug(b *strings.Builder) { b.WriteString("<tx>") }

// XoDebug implements Debug.
func (r Sql_Row) XoDebug(b *strings.Builder) {
	b.WriteString("Row(")
	for i, c := range r.cols {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(c + ": ")
		switch v := r.vals[i].(type) {
		case nil:
			b.WriteString("null")
		case Time:
			DebugTo(b, v)
		default:
			DebugTo(b, v)
		}
	}
	b.WriteString(")")
}

// XoEq implements Eq.
func (r Sql_Row) XoEq(x any) bool {
	o, ok := x.(Sql_Row)
	if !ok || len(o.cols) != len(r.cols) {
		return false
	}
	for i := range r.cols {
		if r.cols[i] != o.cols[i] || !EqAny(r.vals[i], o.vals[i]) {
			return false
		}
	}
	return true
}

// SqlErr tags, in declaration order (std/sql.xo).
const (
	sqlTagSyntax = iota + 1
	sqlTagConstraint
	sqlTagParam
	sqlTagNoTable
	sqlTagNoRows
	sqlTagNoColumn
	sqlTagMismatch
	sqlTagConn
	sqlTagDriver
	sqlTagServer
	sqlTagTxDone
	sqlTagClosed
	sqlTagCanceled
)

func sqlE(tag int, p any) Sql_SqlErr { return Sql_SqlErr{Tag: tag, P: p} }

func sqlMsgErr(tag int, msg string) Sql_SqlErr {
	switch tag {
	case sqlTagSyntax:
		return sqlE(tag, Sql_SqlErr_Syntax{F_msg: msg})
	case sqlTagParam:
		return sqlE(tag, Sql_SqlErr_Param{F_msg: msg})
	case sqlTagNoTable:
		return sqlE(tag, Sql_SqlErr_NoTable{F_name: msg})
	case sqlTagConn:
		return sqlE(tag, Sql_SqlErr_Conn{F_msg: msg})
	}
	return sqlE(sqlTagDriver, Sql_SqlErr_Driver{F_msg: msg})
}

// toSqlErr converts a backend error.
func toSqlErr(err error) Sql_SqlErr {
	if b, ok := err.(*sqlErrBox); ok {
		return b.e
	}
	if se, ok := err.(*SqlError); ok {
		switch se.Kind {
		case SqlSyntax:
			return sqlMsgErr(sqlTagSyntax, se.Msg)
		case SqlConstraint:
			return sqlE(sqlTagConstraint, Sql_SqlErr_Constraint{F_msg: se.Msg, F_code: se.Code, F_name: se.Name})
		case SqlParam:
			return sqlMsgErr(sqlTagParam, se.Msg)
		case SqlNoTable:
			return sqlMsgErr(sqlTagNoTable, se.Msg)
		case SqlConn:
			return sqlMsgErr(sqlTagConn, se.Msg)
		}
		return sqlMsgErr(sqlTagDriver, se.Msg)
	}
	return sqlMsgErr(sqlTagDriver, err.Error())
}

func sqlFail[T any](tag int) Result[T, Sql_SqlErr] { return Err[T](sqlE(tag, nil)) }

func sqlFailErr[T any](err error) Result[T, Sql_SqlErr] { return Err[T](toSqlErr(err)) }

// Sql_null, Sql_int, ... build SqlValues.
func Sql_null(*Task) Sql_SqlValue { return Sql_SqlValue{Tag: 1} }
func Sql_int(_ *Task, n int64) Sql_SqlValue {
	return Sql_SqlValue{Tag: 2, P: Sql_SqlValue_Integer{F_n: n}}
}
func Sql_str(_ *Task, s string) Sql_SqlValue {
	return Sql_SqlValue{Tag: 3, P: Sql_SqlValue_Text{F_s: s}}
}
func Sql_float(_ *Task, f float64) Sql_SqlValue {
	return Sql_SqlValue{Tag: 4, P: Sql_SqlValue_Real{F_f: f}}
}
func Sql_bool(_ *Task, b bool) Sql_SqlValue {
	return Sql_SqlValue{Tag: 5, P: Sql_SqlValue_Flag{F_b: b}}
}
func Sql_time(_ *Task, t Time) Sql_SqlValue { return Sql_SqlValue{Tag: 6, P: Sql_SqlValue_At{F_t: t}} }
func Sql_bytes(_ *Task, b Bytes) Sql_SqlValue {
	return Sql_SqlValue{Tag: 7, P: Sql_SqlValue_Blob{F_b: b}}
}
func Sql_json[T any](tk *Task, v T) Sql_SqlValue {
	return Sql_SqlValue{Tag: 8, P: Sql_SqlValue_JsonText{F_s: Json_encode_str(tk, v)}}
}
func Sql_array(_ *Task, items List[Sql_SqlValue]) Sql_SqlValue {
	return Sql_SqlValue{Tag: 9, P: Sql_SqlValue_Array{F_items: items}}
}

// SqlJSON is the value of a json column (decision 0278): its text.
type SqlJSON string

// sqlValueOf is the SqlValue of a backend value (sqlArgs' inverse).
func sqlValueOf(v any) Sql_SqlValue {
	switch x := v.(type) {
	case int64:
		return Sql_SqlValue{Tag: 2, P: Sql_SqlValue_Integer{F_n: x}}
	case string:
		return Sql_SqlValue{Tag: 3, P: Sql_SqlValue_Text{F_s: x}}
	case float64:
		return Sql_SqlValue{Tag: 4, P: Sql_SqlValue_Real{F_f: x}}
	case bool:
		return Sql_SqlValue{Tag: 5, P: Sql_SqlValue_Flag{F_b: x}}
	case Time:
		return Sql_SqlValue{Tag: 6, P: Sql_SqlValue_At{F_t: x}}
	case Bytes:
		return Sql_SqlValue{Tag: 7, P: Sql_SqlValue_Blob{F_b: x}}
	case SqlJSON:
		return Sql_SqlValue{Tag: 8, P: Sql_SqlValue_JsonText{F_s: string(x)}}
	case Sql_SqlValue:
		return x
	}
	return Sql_SqlValue{Tag: 1}
}

func sqlArgs(args List[Sql_SqlValue]) []any {
	xs := List_Slice(args)
	out := make([]any, len(xs))
	for i, x := range xs {
		switch x.Tag {
		case 2:
			out[i] = x.P.(Sql_SqlValue_Integer).F_n
		case 3:
			out[i] = x.P.(Sql_SqlValue_Text).F_s
		case 4:
			out[i] = x.P.(Sql_SqlValue_Real).F_f
		case 5:
			out[i] = x.P.(Sql_SqlValue_Flag).F_b
		case 6:
			out[i] = x.P.(Sql_SqlValue_At).F_t
		case 7:
			out[i] = x.P.(Sql_SqlValue_Blob).F_b
		case 8:
			out[i] = SqlJSON(x.P.(Sql_SqlValue_JsonText).F_s)
		case 9:
			out[i] = x
		}
	}
	return out
}

func newSqlDB(be SqlBackend, maxOpen int) Sql_Db {
	if maxOpen < 1 {
		maxOpen = 1
	}
	d := &sqlDB{be: be, pool: &lock{permits: maxOpen}}
	if be.SingleWriter() {
		d.writer = &lock{permits: 1}
	}
	return Sql_Db{d: d}
}

// Sql_memory is `sql.memory(schema)`.
func Sql_memory(_ *Task, schema string) Result[Sql_Db, Sql_SqlErr] {
	be, err := memSchema(schema)
	if err != nil {
		return sqlFailErr[Sql_Db](err)
	}
	return Ok[Sql_Db, Sql_SqlErr](newSqlDB(be, 10))
}

// Testing_fake_db is `testing.fake_db(schema)`.
func Testing_fake_db(tk *Task, schema string) Result[Sql_Db, Sql_SqlErr] {
	return Sql_memory(tk, schema)
}

// Sql_open is `sql.open(net, driver, dsn, ...)`.
func Sql_open(tk *Task, n Net, driver, dsn string, maxOpen, maxIdle int64, maxLife Duration) Result[Sql_Db, Sql_SqlErr] {
	if tk.Canceled() {
		return sqlFail[Sql_Db](sqlTagCanceled)
	}
	if _, ok := n.(*onlyNet); ok {
		// the driver dials by itself, so an allowlist cannot be
		// enforced: fail closed (decision 0156)
		return Err[Sql_Db](sqlMsgErr(sqlTagConn, "an attenuated Net (Net.only) cannot open a database: the driver dials by itself; pass an unattenuated Net"))
	}
	if maxOpen < 1 {
		return Err[Sql_Db](sqlMsgErr(sqlTagParam, "max_open must be at least 1"))
	}
	sqlDriversMu.Lock()
	op, ok := sqlDrivers[driver]
	sqlDriversMu.Unlock()
	if !ok {
		return Err[Sql_Db](sqlMsgErr(sqlTagDriver, "unknown driver "+driver))
	}
	be, err := op.Open(dsn, SqlOpts{MaxOpen: int(maxOpen), MaxIdle: int(maxIdle), MaxLifetime: maxLife})
	if err != nil {
		return sqlFailErr[Sql_Db](err)
	}
	return Ok[Sql_Db, Sql_SqlErr](newSqlDB(be, int(maxOpen)))
}

// withConn runs f holding a pool permit.
func withConn[T any](tk *Task, d *sqlDB, f func() Result[T, Sql_SqlErr]) Result[T, Sql_SqlErr] {
	if d == nil || d.closed.Load() {
		return sqlFail[T](sqlTagClosed)
	}
	if tk.Canceled() {
		return sqlFail[T](sqlTagCanceled)
	}
	if !d.pool.acquire(tk) {
		return sqlFail[T](sqlTagCanceled)
	}
	defer d.pool.release()
	return f()
}

func sqlRows(cols []string, rows [][]any) List[Sql_Row] {
	out := make([]Sql_Row, len(rows))
	for i, r := range rows {
		out[i] = Sql_Row{cols: cols, vals: r}
	}
	return ListFromSlice(out)
}

type sqlRunner interface {
	Query(q string, args []any) ([]string, [][]any, error)
	Exec(q string, args []any) (int64, int64, error)
}

func runQuery(tk *Task, r sqlRunner, q string, args []any) ([]string, [][]any, error) {
	if t, ok := r.(sqlTaskRunner); ok {
		return t.QueryTk(tk, q, args)
	}
	return r.Query(q, args)
}

func sqlQuery(tk *Task, r sqlRunner, q string, args List[Sql_SqlValue]) Result[List[Sql_Row], Sql_SqlErr] {
	cols, rows, err := runQuery(tk, r, q, sqlArgs(args))
	if err != nil {
		return sqlFailErr[List[Sql_Row]](err)
	}
	return Ok[List[Sql_Row], Sql_SqlErr](sqlRows(cols, rows))
}

func sqlQueryOne(tk *Task, r sqlRunner, q string, args List[Sql_SqlValue]) Result[Sql_Row, Sql_SqlErr] {
	cols, rows, err := runQuery(tk, r, q, sqlArgs(args))
	if err != nil {
		return sqlFailErr[Sql_Row](err)
	}
	if len(rows) == 0 {
		return sqlFail[Sql_Row](sqlTagNoRows)
	}
	return Ok[Sql_Row, Sql_SqlErr](Sql_Row{cols: cols, vals: rows[0]})
}

func sqlExec(tk *Task, r sqlRunner, q string, args List[Sql_SqlValue]) Result[Sql_ExecResult, Sql_SqlErr] {
	var n, id int64
	var err error
	if t, ok := r.(sqlTaskRunner); ok {
		n, id, err = t.ExecTk(tk, q, sqlArgs(args))
	} else {
		n, id, err = r.Exec(q, sqlArgs(args))
	}
	if err != nil {
		return sqlFailErr[Sql_ExecResult](err)
	}
	return Ok[Sql_ExecResult, Sql_SqlErr](Sql_ExecResult{F_rows_affected: n, F_last_insert_id: id})
}

// M_query is `db.query(q, args)`.
func (d Sql_Db) M_query(tk *Task, q string, args List[Sql_SqlValue]) Result[List[Sql_Row], Sql_SqlErr] {
	return withConn(tk, d.d, func() Result[List[Sql_Row], Sql_SqlErr] { return sqlQuery(tk, d.d.be, q, args) })
}

// M_query_one is `db.query_one(q, args)`.
func (d Sql_Db) M_query_one(tk *Task, q string, args List[Sql_SqlValue]) Result[Sql_Row, Sql_SqlErr] {
	return withConn(tk, d.d, func() Result[Sql_Row, Sql_SqlErr] { return sqlQueryOne(tk, d.d.be, q, args) })
}

// M_exec is `db.exec(q, args)`.
func (d Sql_Db) M_exec(tk *Task, q string, args List[Sql_SqlValue]) Result[Sql_ExecResult, Sql_SqlErr] {
	return withConn(tk, d.d, func() Result[Sql_ExecResult, Sql_SqlErr] { return sqlExec(tk, d.d.be, q, args) })
}

// M_ping is `db.ping()`.
func (d Sql_Db) M_ping(tk *Task) Result[Unit, Sql_SqlErr] {
	return withConn(tk, d.d, func() Result[Unit, Sql_SqlErr] {
		if b, ok := d.d.be.(*sqlDriverBackend); ok {
			if err := b.PingTk(tk); err != nil {
				return sqlFailErr[Unit](err)
			}
			return Ok[Unit, Sql_SqlErr](Unit{})
		}
		if err := d.d.be.Ping(); err != nil {
			return sqlFailErr[Unit](err)
		}
		return Ok[Unit, Sql_SqlErr](Unit{})
	})
}

// M_close is `db.close()`: later operations fail with Closed. Closing twice
// is fine. Open transactions keep working until they finish.
func (d Sql_Db) M_close(tk *Task) Result[Unit, Sql_SqlErr] {
	if d.d != nil && d.d.closed.CompareAndSwap(false, true) {
		if b, ok := d.d.be.(*sqlDriverBackend); ok {
			b.CloseTk(tk)
			return Ok[Unit, Sql_SqlErr](Unit{})
		}
		if err := d.d.be.Close(); err != nil {
			return sqlFailErr[Unit](err)
		}
	}
	return Ok[Unit, Sql_SqlErr](Unit{})
}

// M_begin is `db.begin()`: the transaction holds a pool permit (and the
// writer permit of a single writer backend) until commit or rollback.
func (d Sql_Db) M_begin(tk *Task) Result[Sql_Tx, Sql_SqlErr] {
	db := d.d
	if db == nil || db.closed.Load() {
		return sqlFail[Sql_Tx](sqlTagClosed)
	}
	if tk.Canceled() {
		return sqlFail[Sql_Tx](sqlTagCanceled)
	}
	if !db.pool.acquire(tk) {
		return sqlFail[Sql_Tx](sqlTagCanceled)
	}
	if db.writer != nil && !db.writer.acquire(tk) {
		db.pool.release()
		return sqlFail[Sql_Tx](sqlTagCanceled)
	}
	var be SqlTxBackend
	var err error
	if b, ok := db.be.(*sqlDriverBackend); ok {
		be, err = b.BeginTk(tk)
	} else {
		be, err = db.be.Begin()
	}
	if err != nil {
		db.release()
		return sqlFailErr[Sql_Tx](err)
	}
	return Ok[Sql_Tx, Sql_SqlErr](Sql_Tx{t: &sqlTx{db: db, be: be}})
}

func (d *sqlDB) release() {
	if d.writer != nil {
		d.writer.release()
	}
	d.pool.release()
}

// inTx runs f if the transaction is open.
func inTx[T any](tk *Task, t Sql_Tx, f func(be SqlTxBackend) Result[T, Sql_SqlErr]) Result[T, Sql_SqlErr] {
	if t.t == nil {
		return sqlFail[T](sqlTagTxDone)
	}
	if tk.Canceled() {
		return sqlFail[T](sqlTagCanceled)
	}
	t.t.mu.Lock()
	defer t.t.mu.Unlock()
	if t.t.done {
		return sqlFail[T](sqlTagTxDone)
	}
	return f(t.t.be)
}

// M_query is `tx.query(q, args)`.
func (t Sql_Tx) M_query(tk *Task, q string, args List[Sql_SqlValue]) Result[List[Sql_Row], Sql_SqlErr] {
	return inTx(tk, t, func(be SqlTxBackend) Result[List[Sql_Row], Sql_SqlErr] { return sqlQuery(tk, be, q, args) })
}

// M_query_one is `tx.query_one(q, args)`.
func (t Sql_Tx) M_query_one(tk *Task, q string, args List[Sql_SqlValue]) Result[Sql_Row, Sql_SqlErr] {
	return inTx(tk, t, func(be SqlTxBackend) Result[Sql_Row, Sql_SqlErr] { return sqlQueryOne(tk, be, q, args) })
}

// M_exec is `tx.exec(q, args)`.
func (t Sql_Tx) M_exec(tk *Task, q string, args List[Sql_SqlValue]) Result[Sql_ExecResult, Sql_SqlErr] {
	return inTx(tk, t, func(be SqlTxBackend) Result[Sql_ExecResult, Sql_SqlErr] { return sqlExec(tk, be, q, args) })
}

// finish ends the transaction and releases its permits.
func (t Sql_Tx) finish(tk *Task, commit bool) error {
	t.t.done = true
	var err error
	if dt, ok := t.t.be.(*sqlDriverTx); ok {
		err = dt.FinishTk(tk, commit)
	} else if commit {
		err = t.t.be.Commit()
	} else {
		err = t.t.be.Rollback()
	}
	t.t.db.release()
	return err
}

// M_commit is `tx.commit()`. A transaction whose commit fails is rolled back.
func (t Sql_Tx) M_commit(tk *Task) Result[Unit, Sql_SqlErr] {
	if t.t == nil {
		return sqlFail[Unit](sqlTagTxDone)
	}
	t.t.mu.Lock()
	defer t.t.mu.Unlock()
	if t.t.done {
		return sqlFail[Unit](sqlTagTxDone)
	}
	if err := t.finish(tk, true); err != nil {
		return sqlFailErr[Unit](err)
	}
	return Ok[Unit, Sql_SqlErr](Unit{})
}

// M_rollback is `tx.rollback()`. It works while cancelled (it is meant for a
// shielded defer) and does nothing when the transaction is already finished.
func (t Sql_Tx) M_rollback(tk *Task) Result[Unit, Sql_SqlErr] {
	if t.t == nil {
		return Ok[Unit, Sql_SqlErr](Unit{})
	}
	t.t.mu.Lock()
	defer t.t.mu.Unlock()
	if t.t.done {
		return Ok[Unit, Sql_SqlErr](Unit{})
	}
	if err := t.finish(tk, false); err != nil {
		return sqlFailErr[Unit](err)
	}
	return Ok[Unit, Sql_SqlErr](Unit{})
}

// ---------------------------------------------------------------------------
// Row

// M_columns is `row.columns()`.
func (r Sql_Row) M_columns(*Task) List[string] {
	return ListFromSlice(append([]string(nil), r.cols...))
}

func (r Sql_Row) get(name string) (any, *Sql_SqlErr) {
	for i, c := range r.cols {
		if c == name {
			return r.vals[i], nil
		}
	}
	e := sqlE(sqlTagNoColumn, Sql_SqlErr_NoColumn{F_name: name})
	return nil, &e
}

func sqlMismatch(name, want string, v any) Sql_SqlErr {
	got := sqlTypeName(v)
	if v == nil {
		got = "NULL"
	}
	return sqlE(sqlTagMismatch, Sql_SqlErr_Mismatch{F_column: name, F_msg: "expected " + want + ", found " + got})
}

func sqlTypeName(v any) string {
	switch v.(type) {
	case int64:
		return "an integer"
	case string:
		return "text"
	case float64:
		return "a float"
	case bool:
		return "a boolean"
	case Time:
		return "a time"
	case Bytes:
		return "bytes"
	case SqlJSON:
		return "json"
	case Sql_SqlValue:
		return "an array"
	}
	return "a value"
}

// M_is_null is `row.is_null(name)`.
func (r Sql_Row) M_is_null(_ *Task, name string) Result[bool, Sql_SqlErr] {
	v, e := r.get(name)
	if e != nil {
		return Err[bool](*e)
	}
	return Ok[bool, Sql_SqlErr](v == nil)
}

func sqlCol[T any](r Sql_Row, name, want string, conv func(any) (T, bool)) Result[T, Sql_SqlErr] {
	var zero T
	v, e := r.get(name)
	if e != nil {
		return Err[T](*e)
	}
	if x, ok := conv(v); ok && v != nil {
		return Ok[T, Sql_SqlErr](x)
	}
	_ = zero
	return Err[T](sqlMismatch(name, want, v))
}

func sqlColOpt[T any](r Sql_Row, name, want string, conv func(any) (T, bool)) Result[Option[T], Sql_SqlErr] {
	v, e := r.get(name)
	if e != nil {
		return Err[Option[T]](*e)
	}
	if v == nil {
		return Ok[Option[T], Sql_SqlErr](None[T]())
	}
	if x, ok := conv(v); ok {
		return Ok[Option[T], Sql_SqlErr](Some(x))
	}
	return Err[Option[T]](sqlMismatch(name, want, v))
}

func asInt(v any) (int64, bool)  { x, ok := v.(int64); return x, ok }
func asStr(v any) (string, bool) { x, ok := v.(string); return x, ok }
func asBool(v any) (bool, bool)  { x, ok := v.(bool); return x, ok }
func asTime(v any) (Time, bool)  { x, ok := v.(Time); return x, ok }
func asFloat(v any) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case int64:
		return float64(x), true
	}
	return 0, false
}

// M_int is `row.int(name)`.
func (r Sql_Row) M_int(_ *Task, name string) Result[int64, Sql_SqlErr] {
	return sqlCol(r, name, "an integer", asInt)
}

// M_str is `row.str(name)`.
func (r Sql_Row) M_str(_ *Task, name string) Result[string, Sql_SqlErr] {
	return sqlCol(r, name, "text", asStr)
}

// M_float is `row.float(name)`.
func (r Sql_Row) M_float(_ *Task, name string) Result[float64, Sql_SqlErr] {
	return sqlCol(r, name, "a float", asFloat)
}

// M_bool is `row.bool(name)`.
func (r Sql_Row) M_bool(_ *Task, name string) Result[bool, Sql_SqlErr] {
	return sqlCol(r, name, "a boolean", asBool)
}

// M_time is `row.time(name)`.
func (r Sql_Row) M_time(_ *Task, name string) Result[Time, Sql_SqlErr] {
	return sqlCol(r, name, "a time", asTime)
}

// M_int_opt is `row.int_opt(name)`.
func (r Sql_Row) M_int_opt(_ *Task, name string) Result[Option[int64], Sql_SqlErr] {
	return sqlColOpt(r, name, "an integer", asInt)
}

// M_str_opt is `row.str_opt(name)`.
func (r Sql_Row) M_str_opt(_ *Task, name string) Result[Option[string], Sql_SqlErr] {
	return sqlColOpt(r, name, "text", asStr)
}

// M_float_opt is `row.float_opt(name)`.
func (r Sql_Row) M_float_opt(_ *Task, name string) Result[Option[float64], Sql_SqlErr] {
	return sqlColOpt(r, name, "a float", asFloat)
}

// M_bool_opt is `row.bool_opt(name)`.
func (r Sql_Row) M_bool_opt(_ *Task, name string) Result[Option[bool], Sql_SqlErr] {
	return sqlColOpt(r, name, "a boolean", asBool)
}

// M_time_opt is `row.time_opt(name)`.
func (r Sql_Row) M_time_opt(_ *Task, name string) Result[Option[Time], Sql_SqlErr] {
	return sqlColOpt(r, name, "a time", asTime)
}

func asBytes(v any) (Bytes, bool) { x, ok := v.(Bytes); return x, ok }

func asItems(v any) (List[Sql_SqlValue], bool) {
	if x, ok := v.(Sql_SqlValue); ok && x.Tag == 9 {
		return x.P.(Sql_SqlValue_Array).F_items, true
	}
	return List[Sql_SqlValue]{}, false
}

// M_bytes is `row.bytes(name)` (decision 0278).
func (r Sql_Row) M_bytes(_ *Task, name string) Result[Bytes, Sql_SqlErr] {
	return sqlCol(r, name, "bytes", asBytes)
}

// M_bytes_opt is `row.bytes_opt(name)`.
func (r Sql_Row) M_bytes_opt(_ *Task, name string) Result[Option[Bytes], Sql_SqlErr] {
	return sqlColOpt(r, name, "bytes", asBytes)
}

// Sql_Row_json is `row.json[T](name)`: a json column, or text, decoded.
func Sql_Row_json[T any](tk *Task, r Sql_Row, name string) Result[T, Sql_SqlErr] {
	v, e := r.get(name)
	if e != nil {
		return Err[T](*e)
	}
	var text string
	switch x := v.(type) {
	case SqlJSON:
		text = string(x)
	case string:
		text = x
	default:
		return Err[T](sqlMismatch(name, "json", v))
	}
	d := Json_decode_str[T](tk, text)
	if !d.Ok {
		return Err[T](sqlE(sqlTagMismatch, Sql_SqlErr_Mismatch{F_column: name, F_msg: d.E.M_display(tk)}))
	}
	return Ok[T, Sql_SqlErr](d.V)
}

// M_items is `row.items(name)`.
func (r Sql_Row) M_items(_ *Task, name string) Result[List[Sql_SqlValue], Sql_SqlErr] {
	return sqlCol(r, name, "an array", asItems)
}

func sqlElems[T any](r Sql_Row, name, want string, tag int, get func(Sql_SqlValue) T) Result[List[T], Sql_SqlErr] {
	items := sqlCol(r, name, "an array", asItems)
	if !items.Ok {
		return Err[List[T]](items.E)
	}
	var out []T
	for _, x := range List_Slice(items.V) {
		if x.Tag != tag {
			var got any
			switch x.Tag {
			case 2:
				got = x.P.(Sql_SqlValue_Integer).F_n
			case 3:
				got = x.P.(Sql_SqlValue_Text).F_s
			case 4:
				got = x.P.(Sql_SqlValue_Real).F_f
			case 5:
				got = x.P.(Sql_SqlValue_Flag).F_b
			case 6:
				got = x.P.(Sql_SqlValue_At).F_t
			case 7:
				got = x.P.(Sql_SqlValue_Blob).F_b
			case 8:
				got = SqlJSON(x.P.(Sql_SqlValue_JsonText).F_s)
			case 9:
				got = x
			}
			return Err[List[T]](sqlMismatch(name, want, got))
		}
		out = append(out, get(x))
	}
	return Ok[List[T], Sql_SqlErr](ListFromSlice(out))
}

// M_ints is `row.ints(name)`: an array of integers.
func (r Sql_Row) M_ints(_ *Task, name string) Result[List[int64], Sql_SqlErr] {
	return sqlElems(r, name, "an array of integers", 2, func(x Sql_SqlValue) int64 { return x.P.(Sql_SqlValue_Integer).F_n })
}

// M_strs is `row.strs(name)`: an array of text.
func (r Sql_Row) M_strs(_ *Task, name string) Result[List[string], Sql_SqlErr] {
	return sqlElems(r, name, "an array of text", 3, func(x Sql_SqlValue) string { return x.P.(Sql_SqlValue_Text).F_s })
}
