package rt

import (
	"sync"
	"time"
)

// Drivers written in Xo (decision 0278): sql.open_driver over a
// Sql_Driver, as a SqlBackend whose calls take the caller's task
// (sqlTaskRunner), so a driver's waits are cancelled with it. The pool's
// permits are sqlDB's (max_open); this backend keeps the idle
// connections (max_idle, max_lifetime).

// sqlTaskRunner is a backend or transaction whose calls take the task.
type sqlTaskRunner interface {
	QueryTk(tk *Task, q string, args []any) ([]string, [][]any, error)
	ExecTk(tk *Task, q string, args []any) (int64, int64, error)
}

// sqlTypedTaskRunner and sqlTypedRunner report a query's column types
// too (decision 0609): the Xo driver adapter and the memory database. A
// backend registered with RegisterSqlDriver without them gives
// ColumnType{name} per column.
type sqlTypedTaskRunner interface {
	QueryTypesTk(tk *Task, q string, args []any) ([]string, []Sql_ColumnType, [][]any, error)
}

type sqlTypedRunner interface {
	QueryTypes(q string, args []any) ([]string, []Sql_ColumnType, [][]any, error)
}

// sqlErrBox carries a driver's SqlErr through a Go error.
type sqlErrBox struct{ e Sql_SqlErr }

func (b *sqlErrBox) Error() string { return Debug(b.e) }

type sqlIdle struct {
	c     Sql_DriverConn
	born  time.Time
	stmts *sqlConnStmts // the statements prepared on c (sqlstmt.go)
}

type sqlDriverBackend struct {
	drv     Sql_Driver
	net     Net
	dsn     string
	mu      sync.Mutex
	idle    []sqlIdle
	maxIdle int
	maxLife time.Duration
	// onNotice is open_driver's on_notice: each new connection is a
	// sqlNoticeConn then (decision 0648 item 14)
	onNotice Option[func(*Task, Sql_Notice) Result[Unit, Never]]
	// onTrace is open_driver's on_trace (decision 0648 item 16, sqltrace.go)
	onTrace Option[sqlTraceFn]
	maxOpen int
	n       sqlCounts
}

func sqlValues(args []any) List[Sql_SqlValue] {
	out := make([]Sql_SqlValue, len(args))
	for i, a := range args {
		out[i] = sqlValueOf(a)
	}
	return ListFromSlice(out)
}

func (b *sqlDriverBackend) take(tk *Task) (sqlIdle, error) {
	now := time.Now()
	start := now
	for {
		b.mu.Lock()
		if len(b.idle) == 0 {
			b.mu.Unlock()
			break
		}
		x := b.idle[len(b.idle)-1]
		b.idle = b.idle[:len(b.idle)-1]
		b.mu.Unlock()
		if b.maxLife > 0 && now.Sub(x.born) >= b.maxLife {
			x.c.M_close(tk)
			b.n.open.Add(-1)
			b.n.lifeClosed.Add(1)
			continue
		}
		x.stmts.sweep(tk)
		b.acquired(tk, start, nil)
		return x, nil
	}
	b.n.connecting.Add(1)
	cs := time.Now()
	r := b.drv.M_connect(tk, b.net, b.dsn)
	b.n.connecting.Add(-1)
	if !r.Ok {
		e := &sqlErrBox{r.E}
		b.trace(tk, cs, sqlTraceConnect, e)
		b.acquired(tk, start, e)
		return sqlIdle{}, e
	}
	b.n.newConns.Add(1)
	b.n.open.Add(1)
	b.trace(tk, cs, sqlTraceConnect, nil)
	b.acquired(tk, start, nil)
	c := r.V
	if b.onTrace.Some {
		c = sqlTraceConn{Sql_DriverConn: c, h: b.onTrace.V}
	}
	if b.onNotice.Some {
		c = sqlNoticeConn{Sql_DriverConn: c, h: b.onNotice.V}
	}
	return sqlIdle{c: c, born: now, stmts: &sqlConnStmts{}}, nil
}

func (b *sqlDriverBackend) give(tk *Task, x sqlIdle) {
	start := time.Now()
	b.n.inUse.Add(-1)
	defer b.trace(tk, start, sqlTraceRelease, nil)
	young := b.maxLife == 0 || time.Since(x.born) < b.maxLife
	keep := !x.c.M_is_broken(tk) && young
	if keep {
		x.stmts.sweep(tk)
		b.mu.Lock()
		if len(b.idle) < b.maxIdle {
			b.idle = append(b.idle, x)
			b.mu.Unlock()
			return
		}
		b.mu.Unlock()
	}
	x.c.M_close(tk)
	b.n.open.Add(-1)
	if keep {
		b.n.idleClosed.Add(1)
	} else if !young {
		b.n.lifeClosed.Add(1)
	}
}

// driverRowsTypes is the result of a driver's query: its columns, their
// types (decision 0609; nil when the driver gave none) and every row.
func driverRowsTypes(tk *Task, r Result[Sql_DriverRows, Sql_SqlErr]) ([]string, []Sql_ColumnType, [][]any, error) {
	cols, rows, err := driverRows(tk, r)
	if err != nil {
		return nil, nil, nil, err
	}
	return cols, List_Slice(r.V.F_column_types), rows, nil
}

func driverRows(tk *Task, r Result[Sql_DriverRows, Sql_SqlErr]) ([]string, [][]any, error) {
	if !r.Ok {
		return nil, nil, &sqlErrBox{r.E}
	}
	cols := List_Slice(r.V.F_columns)
	var rows [][]any
	for _, row := range List_Slice(r.V.F_rows) {
		rows = append(rows, sqlArgs(row))
	}
	// a driver's cursor (decision 0479): the Go backend reads the result whole
	if r.V.F_cursor.Some {
		rest := r.V.F_cursor.V.M_rest(tk)
		if !rest.Ok {
			return nil, nil, &sqlErrBox{rest.E}
		}
		for _, row := range List_Slice(rest.V) {
			rows = append(rows, sqlArgs(row))
		}
	}
	return cols, rows, nil
}

func driverExec(r Result[Sql_ExecResult, Sql_SqlErr]) (int64, int64, error) {
	if !r.Ok {
		return 0, 0, &sqlErrBox{r.E}
	}
	return r.V.F_rows_affected, r.V.F_last_insert_id, nil
}

func driverUnit(r Result[Unit, Sql_SqlErr]) error {
	if !r.Ok {
		return &sqlErrBox{r.E}
	}
	return nil
}

func (b *sqlDriverBackend) QueryTk(tk *Task, q string, args []any) ([]string, [][]any, error) {
	cols, _, rows, err := b.QueryTypesTk(tk, q, args)
	return cols, rows, err
}

// QueryTypesTk is QueryTk with the driver's column types (decision 0609).
func (b *sqlDriverBackend) QueryTypesTk(tk *Task, q string, args []any) ([]string, []Sql_ColumnType, [][]any, error) {
	x, err := b.take(tk)
	if err != nil {
		return nil, nil, nil, err
	}
	defer b.give(tk, x)
	return driverRowsTypes(tk, x.c.M_query(tk, q, sqlValues(args)))
}

func (b *sqlDriverBackend) ExecTk(tk *Task, q string, args []any) (int64, int64, error) {
	x, err := b.take(tk)
	if err != nil {
		return 0, 0, err
	}
	defer b.give(tk, x)
	return driverExec(x.c.M_exec(tk, q, sqlValues(args)))
}

func (b *sqlDriverBackend) PingTk(tk *Task) error {
	x, err := b.take(tk)
	if err != nil {
		return err
	}
	defer b.give(tk, x)
	return driverUnit(x.c.M_ping(tk))
}

func (b *sqlDriverBackend) BeginTk(tk *Task, mode string) (SqlTxBackend, error) {
	x, err := b.take(tk)
	if err != nil {
		return nil, err
	}
	if err := beginOn(tk, x, mode); err != nil {
		b.give(tk, x)
		return nil, err
	}
	return &sqlDriverTx{b: b, x: x}, nil
}

// beginOn is BEGIN on x, then mode's SET TRANSACTION; when that fails,
// the transaction is rolled back and its error returned (native begin_on).
func beginOn(tk *Task, x sqlIdle, mode string) error {
	if err := driverUnit(x.c.M_begin(tk)); err != nil {
		return err
	}
	if mode != "" {
		if r := x.c.M_exec(tk, mode, sqlValues(nil)); !r.Ok {
			x.c.M_rollback(tk)
			return &sqlErrBox{r.E}
		}
	}
	return nil
}

func (b *sqlDriverBackend) CloseTk(tk *Task) error {
	b.mu.Lock()
	idle := b.idle
	b.idle = nil
	b.mu.Unlock()
	for _, x := range idle {
		x.c.M_close(tk)
		b.n.open.Add(-1)
	}
	return nil
}

// The SqlBackend methods without a task: every call goes through the Tk
// forms (sql.go checks for them first).
func (b *sqlDriverBackend) Query(q string, args []any) ([]string, [][]any, error) {
	panic("std/sql: a driver backend is called with its task")
}
func (b *sqlDriverBackend) Exec(q string, args []any) (int64, int64, error) {
	panic("std/sql: a driver backend is called with its task")
}
func (b *sqlDriverBackend) Begin() (SqlTxBackend, error) {
	panic("std/sql: a driver backend is called with its task")
}
func (b *sqlDriverBackend) Ping() error        { panic("std/sql: a driver backend is called with its task") }
func (b *sqlDriverBackend) Close() error       { panic("std/sql: a driver backend is called with its task") }
func (b *sqlDriverBackend) SingleWriter() bool { return false }

// sqlDriverTx is a transaction on one held connection.
type sqlDriverTx struct {
	b *sqlDriverBackend
	x sqlIdle
	// kind is sqlTxOwn (Db.begin's), sqlTxOnConn (Conn.begin's),
	// sqlTxSavepoint (Tx.begin's, named name) or sqlTxSession (a Conn's
	// own; decision 0648 item 14)
	kind int
	name string
}

const (
	sqlTxOwn = iota
	sqlTxOnConn
	sqlTxSavepoint
	sqlTxSession
)

func (t *sqlDriverTx) QueryTk(tk *Task, q string, args []any) ([]string, [][]any, error) {
	cols, _, rows, err := t.QueryTypesTk(tk, q, args)
	return cols, rows, err
}

// QueryTypesTk is QueryTk with the driver's column types (decision 0609).
func (t *sqlDriverTx) QueryTypesTk(tk *Task, q string, args []any) ([]string, []Sql_ColumnType, [][]any, error) {
	return driverRowsTypes(tk, t.x.c.M_query(tk, q, sqlValues(args)))
}

func (t *sqlDriverTx) ExecTk(tk *Task, q string, args []any) (int64, int64, error) {
	return driverExec(t.x.c.M_exec(tk, q, sqlValues(args)))
}

func (t *sqlDriverTx) FinishTk(tk *Task, commit bool) error {
	switch t.kind {
	case sqlTxSavepoint:
		q := "rollback to savepoint "
		if commit {
			q = "release savepoint "
		}
		_, _, err := driverExec(t.x.c.M_exec(tk, q+t.name, sqlValues(nil)))
		return err
	case sqlTxOnConn:
		if commit {
			return driverUnit(t.x.c.M_commit(tk))
		}
		return driverUnit(t.x.c.M_rollback(tk))
	}
	// a Conn's session (its close) runs no statement
	var err error
	if t.kind != sqlTxSession {
		if commit {
			err = driverUnit(t.x.c.M_commit(tk))
		} else {
			err = driverUnit(t.x.c.M_rollback(tk))
		}
	}
	// the transaction's statements end with it; give closes them
	t.x.stmts.mu.Lock()
	for _, c := range t.x.stmts.l {
		if c.st.tx != nil {
			c.closed.Store(true)
		}
	}
	t.x.stmts.mu.Unlock()
	t.b.give(tk, t.x)
	return err
}

func (t *sqlDriverTx) Query(q string, args []any) ([]string, [][]any, error) {
	panic("std/sql: a driver transaction is called with its task")
}
func (t *sqlDriverTx) Exec(q string, args []any) (int64, int64, error) {
	panic("std/sql: a driver transaction is called with its task")
}
func (t *sqlDriverTx) Commit() error { panic("std/sql: a driver transaction is called with its task") }
func (t *sqlDriverTx) Rollback() error {
	panic("std/sql: a driver transaction is called with its task")
}

// Sql_open_driver is `sql.open_driver(net, d, dsn, ...)`.
func Sql_open_driver(tk *Task, net Net, d Sql_Driver, dsn string, maxOpen, maxIdle int64, maxLifetime Duration, onNotice Option[func(*Task, Sql_Notice) Result[Unit, Never]], onTrace Option[sqlTraceFn]) Result[Sql_Db, Sql_SqlErr] {
	if tk.Canceled() {
		return sqlFail[Sql_Db](sqlTagCanceled)
	}
	if maxOpen < 1 {
		return Err[Sql_Db](sqlMsgErr(sqlTagParam, "max_open must be at least 1"))
	}
	if maxIdle < 0 {
		maxIdle = 0
	}
	life := time.Duration(maxLifetime)
	if life < 0 {
		life = 0
	}
	be := &sqlDriverBackend{drv: d, net: net, dsn: dsn, maxIdle: int(maxIdle), maxLife: life, onNotice: onNotice, onTrace: onTrace, maxOpen: int(maxOpen)}
	db := newSqlDB(be, int(maxOpen))
	// connect once now, so a wrong password or host fails at open
	if r := db.M_ping(tk); !r.Ok {
		return Err[Sql_Db](r.E)
	}
	return Ok[Sql_Db, Sql_SqlErr](db)
}
