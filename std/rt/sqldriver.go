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

// sqlErrBox carries a driver's SqlErr through a Go error.
type sqlErrBox struct{ e Sql_SqlErr }

func (b *sqlErrBox) Error() string { return Debug(b.e) }

type sqlIdle struct {
	c    Sql_DriverConn
	born time.Time
}

type sqlDriverBackend struct {
	drv     Sql_Driver
	net     Net
	dsn     string
	mu      sync.Mutex
	idle    []sqlIdle
	maxIdle int
	maxLife time.Duration
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
			continue
		}
		return x, nil
	}
	r := b.drv.M_connect(tk, b.net, b.dsn)
	if !r.Ok {
		return sqlIdle{}, &sqlErrBox{r.E}
	}
	return sqlIdle{c: r.V, born: now}, nil
}

func (b *sqlDriverBackend) give(tk *Task, x sqlIdle) {
	keep := !x.c.M_is_broken(tk) && (b.maxLife == 0 || time.Since(x.born) < b.maxLife)
	if keep {
		b.mu.Lock()
		if len(b.idle) < b.maxIdle {
			b.idle = append(b.idle, x)
			b.mu.Unlock()
			return
		}
		b.mu.Unlock()
	}
	x.c.M_close(tk)
}

func driverRows(r Result[Sql_DriverRows, Sql_SqlErr]) ([]string, [][]any, error) {
	if !r.Ok {
		return nil, nil, &sqlErrBox{r.E}
	}
	cols := List_Slice(r.V.F_columns)
	var rows [][]any
	for _, row := range List_Slice(r.V.F_rows) {
		rows = append(rows, sqlArgs(row))
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
	x, err := b.take(tk)
	if err != nil {
		return nil, nil, err
	}
	defer b.give(tk, x)
	return driverRows(x.c.M_query(tk, q, sqlValues(args)))
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

func (b *sqlDriverBackend) BeginTk(tk *Task) (SqlTxBackend, error) {
	x, err := b.take(tk)
	if err != nil {
		return nil, err
	}
	if err := driverUnit(x.c.M_begin(tk)); err != nil {
		b.give(tk, x)
		return nil, err
	}
	return &sqlDriverTx{b: b, x: x}, nil
}

func (b *sqlDriverBackend) CloseTk(tk *Task) error {
	b.mu.Lock()
	idle := b.idle
	b.idle = nil
	b.mu.Unlock()
	for _, x := range idle {
		x.c.M_close(tk)
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
}

func (t *sqlDriverTx) QueryTk(tk *Task, q string, args []any) ([]string, [][]any, error) {
	return driverRows(t.x.c.M_query(tk, q, sqlValues(args)))
}

func (t *sqlDriverTx) ExecTk(tk *Task, q string, args []any) (int64, int64, error) {
	return driverExec(t.x.c.M_exec(tk, q, sqlValues(args)))
}

func (t *sqlDriverTx) FinishTk(tk *Task, commit bool) error {
	var err error
	if commit {
		err = driverUnit(t.x.c.M_commit(tk))
	} else {
		err = driverUnit(t.x.c.M_rollback(tk))
	}
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
func Sql_open_driver(tk *Task, net Net, d Sql_Driver, dsn string, maxOpen, maxIdle int64, maxLifetime Duration) Result[Sql_Db, Sql_SqlErr] {
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
	be := &sqlDriverBackend{drv: d, net: net, dsn: dsn, maxIdle: int(maxIdle), maxLife: life}
	db := newSqlDB(be, int(maxOpen))
	// connect once now, so a wrong password or host fails at open
	if r := db.M_ping(tk); !r.Ok {
		return Err[Sql_Db](r.E)
	}
	return Ok[Sql_Db, Sql_SqlErr](db)
}
