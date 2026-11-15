package rt

import (
	"strings"
	"sync"
	"sync/atomic"
)

// Prepared statements and multi results (decision 0648 items 6 and 7),
// as std/native/sql.xo: a Db's Stmt prepares on each connection of a Xo
// driver it runs on (sqlDriverBackend's per connection cache); the in
// memory database and a backend registered from Go run the text each time.

// Sql_Stmt is `sql.Stmt`: copies share the statement.
type Sql_Stmt struct{ s *sqlStmt }

type sqlStmt struct {
	db     *sqlDB
	tx     *sqlTx // nil for a Db's statement
	q      string
	id     int64
	closed atomic.Bool
}

// XoDebug implements Debug.
func (Sql_Stmt) XoDebug(b *strings.Builder) { b.WriteString("<stmt>") }

// Sql_ResultSet is `sql.ResultSet`.
type Sql_ResultSet struct {
	cols  []string
	types []Sql_ColumnType
	rows  List[Sql_Row]
	// affected is a batch query's count (decision 0648 item 15); 0 from
	// query_all
	affected int64
}

// XoDebug implements Debug.
func (s Sql_ResultSet) XoDebug(b *strings.Builder) {
	b.WriteString("ResultSet{columns: ")
	DebugTo(b, ListOf(s.cols...))
	b.WriteString(", rows: ")
	DebugTo(b, s.rows)
	b.WriteString("}")
}

// sqlSet is one result set of a backend.
type sqlSet struct {
	cols     []string
	types    []Sql_ColumnType
	rows     [][]any
	affected int64
}

// sqlAllRunner is a backend or transaction with query_all (the memory
// database and the Xo driver adapter); any other runs Query once.
type sqlAllRunner interface {
	QueryAllTk(tk *Task, q string, args []any) ([]sqlSet, error)
}

// sqlStmtRunner is a backend or transaction that prepares statements (the
// Xo driver adapter).
type sqlStmtRunner interface {
	PrepareTk(tk *Task, st *sqlStmt) error
	StmtQueryTk(tk *Task, st *sqlStmt, args []any) ([]string, []Sql_ColumnType, [][]any, error)
	StmtExecTk(tk *Task, st *sqlStmt, args []any) (int64, int64, error)
}

var sqlStmtIDs atomic.Int64

func sqlSets(sets []sqlSet) List[Sql_ResultSet] {
	out := make([]Sql_ResultSet, len(sets))
	for i, s := range sets {
		out[i] = Sql_ResultSet{cols: s.cols, types: List_Slice(sqlColumnTypes(s.cols, s.types)), rows: sqlRows(s.cols, s.types, s.rows), affected: s.affected}
	}
	return ListFromSlice(out)
}

func sqlQueryAll(tk *Task, r sqlRunner, q string, args List[Sql_SqlValue]) Result[List[Sql_ResultSet], Sql_SqlErr] {
	if a, ok := r.(sqlAllRunner); ok {
		sets, err := a.QueryAllTk(tk, q, sqlArgs(args))
		if err != nil {
			return sqlFailErr[List[Sql_ResultSet]](err)
		}
		return Ok[List[Sql_ResultSet], Sql_SqlErr](sqlSets(sets))
	}
	cols, types, rows, err := runQuery(tk, r, q, sqlArgs(args))
	if err != nil {
		return sqlFailErr[List[Sql_ResultSet]](err)
	}
	var sets []sqlSet
	if len(cols) > 0 {
		sets = append(sets, sqlSet{cols: cols, types: types, rows: rows})
	}
	return Ok[List[Sql_ResultSet], Sql_SqlErr](sqlSets(sets))
}

// M_query_all is `db.query_all(q, args)`.
func (d Sql_Db) M_query_all(tk *Task, q string, args List[Sql_SqlValue]) Result[List[Sql_ResultSet], Sql_SqlErr] {
	return withConn(tk, d.d, func() Result[List[Sql_ResultSet], Sql_SqlErr] { return sqlQueryAll(tk, d.d.be, q, args) })
}

// M_query_all is `tx.query_all(q, args)`.
func (t Sql_Tx) M_query_all(tk *Task, q string, args List[Sql_SqlValue]) Result[List[Sql_ResultSet], Sql_SqlErr] {
	return inTx(tk, t, func(be SqlTxBackend) Result[List[Sql_ResultSet], Sql_SqlErr] { return sqlQueryAll(tk, be, q, args) })
}

// M_prepare is `db.prepare(q)`: on one connection now (a Xo driver's).
func (d Sql_Db) M_prepare(tk *Task, q string) Result[Sql_Stmt, Sql_SqlErr] {
	st := &sqlStmt{db: d.d, q: q, id: sqlStmtIDs.Add(1)}
	return withConn(tk, d.d, func() Result[Sql_Stmt, Sql_SqlErr] {
		if p, ok := d.d.be.(sqlStmtRunner); ok {
			if err := p.PrepareTk(tk, st); err != nil {
				return sqlFailErr[Sql_Stmt](err)
			}
		}
		return Ok[Sql_Stmt, Sql_SqlErr](Sql_Stmt{st})
	})
}

// M_prepare is `tx.prepare(q)`.
func (t Sql_Tx) M_prepare(tk *Task, q string) Result[Sql_Stmt, Sql_SqlErr] {
	return inTx(tk, t, func(be SqlTxBackend) Result[Sql_Stmt, Sql_SqlErr] {
		st := &sqlStmt{db: t.t.db, tx: t.t, q: q, id: sqlStmtIDs.Add(1)}
		if p, ok := be.(sqlStmtRunner); ok {
			if err := p.PrepareTk(tk, st); err != nil {
				return sqlFailErr[Sql_Stmt](err)
			}
		}
		return Ok[Sql_Stmt, Sql_SqlErr](Sql_Stmt{st})
	})
}

// M_text is `stmt.text()`.
func (s Sql_Stmt) M_text(*Task) string { return s.s.q }

// run runs f on the statement's runner: its transaction's, or the Db's
// under a pool permit; TxDone after the transaction, Closed after close.
func stmtRun[T any](tk *Task, s Sql_Stmt, f func(r sqlRunner) Result[T, Sql_SqlErr]) Result[T, Sql_SqlErr] {
	st := s.s
	if st.tx != nil {
		return inTx(tk, Sql_Tx{st.tx}, func(be SqlTxBackend) Result[T, Sql_SqlErr] {
			if st.closed.Load() {
				return sqlFail[T](sqlTagClosed)
			}
			return f(be)
		})
	}
	if st.closed.Load() {
		return sqlFail[T](sqlTagClosed)
	}
	return withConn(tk, st.db, func() Result[T, Sql_SqlErr] { return f(st.db.be) })
}

func stmtQueryRows(tk *Task, st *sqlStmt, r sqlRunner, args List[Sql_SqlValue]) ([]string, []Sql_ColumnType, [][]any, error) {
	if p, ok := r.(sqlStmtRunner); ok {
		return p.StmtQueryTk(tk, st, sqlArgs(args))
	}
	return runQuery(tk, r, st.q, sqlArgs(args))
}

// M_query is `stmt.query(args)`.
func (s Sql_Stmt) M_query(tk *Task, args List[Sql_SqlValue]) Result[List[Sql_Row], Sql_SqlErr] {
	return stmtRun(tk, s, func(r sqlRunner) Result[List[Sql_Row], Sql_SqlErr] {
		cols, types, rows, err := stmtQueryRows(tk, s.s, r, args)
		if err != nil {
			return sqlFailErr[List[Sql_Row]](err)
		}
		return Ok[List[Sql_Row], Sql_SqlErr](sqlRows(cols, types, rows))
	})
}

// M_query_one is `stmt.query_one(args)`.
func (s Sql_Stmt) M_query_one(tk *Task, args List[Sql_SqlValue]) Result[Sql_Row, Sql_SqlErr] {
	return stmtRun(tk, s, func(r sqlRunner) Result[Sql_Row, Sql_SqlErr] {
		cols, types, rows, err := stmtQueryRows(tk, s.s, r, args)
		if err != nil {
			return sqlFailErr[Sql_Row](err)
		}
		if len(rows) == 0 {
			return sqlFail[Sql_Row](sqlTagNoRows)
		}
		return Ok[Sql_Row, Sql_SqlErr](Sql_Row{cols: cols, vals: rows[0], types: types})
	})
}

// M_exec is `stmt.exec(args)`.
func (s Sql_Stmt) M_exec(tk *Task, args List[Sql_SqlValue]) Result[Sql_ExecResult, Sql_SqlErr] {
	return stmtRun(tk, s, func(r sqlRunner) Result[Sql_ExecResult, Sql_SqlErr] {
		if p, ok := r.(sqlStmtRunner); ok {
			n, id, err := p.StmtExecTk(tk, s.s, sqlArgs(args))
			if err != nil {
				return sqlFailErr[Sql_ExecResult](err)
			}
			return Ok[Sql_ExecResult, Sql_SqlErr](Sql_ExecResult{F_rows_affected: n, F_last_insert_id: id})
		}
		return sqlExec(tk, r, s.s.q, args)
	})
}

// M_rows is `stmt.rows(args)`: read whole, as Db.rows on the Go backend.
func (s Sql_Stmt) M_rows(tk *Task, args List[Sql_SqlValue]) Result[Sql_Rows, Sql_SqlErr] {
	return sqlRowsOf(s.M_query(tk, args))
}

// M_close is `stmt.close()`: a Xo driver's connections close it at their
// next take or give (the idle ones now).
func (s Sql_Stmt) M_close(tk *Task) {
	if !s.s.closed.CompareAndSwap(false, true) || s.s.tx != nil {
		return
	}
	if b, ok := s.s.db.be.(*sqlDriverBackend); ok {
		b.sweepIdle(tk)
	}
}

// Sql_Stmt_query_as is `stmt.query_as[T](args)`.
func Sql_Stmt_query_as[T any](tk *Task, s Sql_Stmt, args List[Sql_SqlValue]) Result[List[T], Sql_SqlErr] {
	return sqlDecodeAll[T](tk, s.M_query(tk, args))
}

// Sql_Stmt_query_one_as is `stmt.query_one_as[T](args)`.
func Sql_Stmt_query_one_as[T any](tk *Task, s Sql_Stmt, args List[Sql_SqlValue]) Result[T, Sql_SqlErr] {
	return sqlDecodeOne[T](tk, s.M_query_one(tk, args))
}

// M_columns is `set.columns()`.
func (s Sql_ResultSet) M_columns(*Task) List[string] { return ListOf(s.cols...) }

// M_column_types is `set.column_types()`.
func (s Sql_ResultSet) M_column_types(*Task) List[Sql_ColumnType] { return ListOf(s.types...) }

// M_rows is `set.rows()`.
func (s Sql_ResultSet) M_rows(*Task) List[Sql_Row] { return s.rows }

// sqlCached is a statement as prepared on one Xo driver connection.
type sqlCached struct {
	ds     Option[Sql_DriverStmt] // none: the driver has no server statements
	st     *sqlStmt
	closed *atomic.Bool
}

// sqlConnStmts is one connection's prepared statements, in the order
// they were prepared (native Taken.stmts).
type sqlConnStmts struct {
	mu sync.Mutex
	l  []sqlCached
}

// sweep closes and drops the statements closed since, in order (native
// sweep).
func (cs *sqlConnStmts) sweep(tk *Task) {
	if cs == nil {
		return
	}
	cs.mu.Lock()
	var gone, keep []sqlCached
	for _, c := range cs.l {
		if c.closed.Load() {
			gone = append(gone, c)
		} else {
			keep = append(keep, c)
		}
	}
	cs.l = keep
	cs.mu.Unlock()
	for _, c := range gone {
		if c.ds.Some {
			c.ds.V.M_close(tk)
		}
	}
}

// on is st as prepared on x, preparing it there on its first use.
func (x sqlIdle) on(tk *Task, st *sqlStmt) (Option[Sql_DriverStmt], error) {
	x.stmts.mu.Lock()
	for _, c := range x.stmts.l {
		if c.st.id == st.id {
			x.stmts.mu.Unlock()
			return c.ds, nil
		}
	}
	x.stmts.mu.Unlock()
	r := x.c.M_prepare(tk, st.q)
	if !r.Ok {
		return Option[Sql_DriverStmt]{}, &sqlErrBox{r.E}
	}
	x.stmts.mu.Lock()
	x.stmts.l = append(x.stmts.l, sqlCached{ds: r.V, st: st, closed: &st.closed})
	x.stmts.mu.Unlock()
	return r.V, nil
}

func (x sqlIdle) stmtQuery(tk *Task, st *sqlStmt, args []any) ([]string, []Sql_ColumnType, [][]any, error) {
	ds, err := x.on(tk, st)
	if err != nil {
		return nil, nil, nil, err
	}
	if ds.Some {
		return driverRowsTypes(tk, ds.V.M_query(tk, sqlValues(args)))
	}
	return driverRowsTypes(tk, x.c.M_query(tk, st.q, sqlValues(args)))
}

func (x sqlIdle) stmtExec(tk *Task, st *sqlStmt, args []any) (int64, int64, error) {
	ds, err := x.on(tk, st)
	if err != nil {
		return 0, 0, err
	}
	if ds.Some {
		return driverExec(ds.V.M_exec(tk, sqlValues(args)))
	}
	return driverExec(x.c.M_exec(tk, st.q, sqlValues(args)))
}

// driverSets is every result set of a driver's query (decision 0648 item
// 7): the first, when it has columns, and DriverRows.more.
func driverSets(tk *Task, r Result[Sql_DriverRows, Sql_SqlErr]) ([]sqlSet, error) {
	cols, types, rows, err := driverRowsTypes(tk, r)
	if err != nil {
		return nil, err
	}
	var sets []sqlSet
	if len(cols) > 0 {
		sets = append(sets, sqlSet{cols: cols, types: types, rows: rows})
	}
	for _, m := range List_Slice(r.V.F_more) {
		var rs [][]any
		for _, row := range List_Slice(m.F_rows) {
			rs = append(rs, sqlArgs(row))
		}
		sets = append(sets, sqlSet{cols: List_Slice(m.F_columns), types: List_Slice(m.F_column_types), rows: rs})
	}
	return sets, nil
}

// PrepareTk prepares st on one connection now.
func (b *sqlDriverBackend) PrepareTk(tk *Task, st *sqlStmt) error {
	x, err := b.take(tk)
	if err != nil {
		return err
	}
	defer b.give(tk, x)
	_, err = x.on(tk, st)
	return err
}

func (b *sqlDriverBackend) StmtQueryTk(tk *Task, st *sqlStmt, args []any) ([]string, []Sql_ColumnType, [][]any, error) {
	x, err := b.take(tk)
	if err != nil {
		return nil, nil, nil, err
	}
	defer b.give(tk, x)
	return x.stmtQuery(tk, st, args)
}

func (b *sqlDriverBackend) StmtExecTk(tk *Task, st *sqlStmt, args []any) (int64, int64, error) {
	x, err := b.take(tk)
	if err != nil {
		return 0, 0, err
	}
	defer b.give(tk, x)
	return x.stmtExec(tk, st, args)
}

func (b *sqlDriverBackend) QueryAllTk(tk *Task, q string, args []any) ([]sqlSet, error) {
	x, err := b.take(tk)
	if err != nil {
		return nil, err
	}
	defer b.give(tk, x)
	return driverSets(tk, x.c.M_query(tk, q, sqlValues(args)))
}

// sweepIdle closes the closed statements of the idle connections, taken
// out of the idle list meanwhile so no task uses one (native Stmt.close).
func (b *sqlDriverBackend) sweepIdle(tk *Task) {
	b.mu.Lock()
	idle := b.idle
	b.idle = nil
	b.mu.Unlock()
	for _, x := range idle {
		x.stmts.sweep(tk)
	}
	var extra []sqlIdle
	b.mu.Lock()
	for _, x := range idle {
		if len(b.idle) < b.maxIdle {
			b.idle = append(b.idle, x)
		} else {
			extra = append(extra, x)
		}
	}
	b.mu.Unlock()
	for _, x := range extra {
		x.c.M_close(tk)
	}
}

func (t *sqlDriverTx) PrepareTk(tk *Task, st *sqlStmt) error {
	_, err := t.x.on(tk, st)
	return err
}

func (t *sqlDriverTx) StmtQueryTk(tk *Task, st *sqlStmt, args []any) ([]string, []Sql_ColumnType, [][]any, error) {
	return t.x.stmtQuery(tk, st, args)
}

func (t *sqlDriverTx) StmtExecTk(tk *Task, st *sqlStmt, args []any) (int64, int64, error) {
	return t.x.stmtExec(tk, st, args)
}

func (t *sqlDriverTx) QueryAllTk(tk *Task, q string, args []any) ([]sqlSet, error) {
	return driverSets(tk, t.x.c.M_query(tk, q, sqlValues(args)))
}

// QueryAllTk is query_all on the in memory database (memRunAll).
func (m *memBackend) QueryAllTk(_ *Task, q string, args []any) ([]sqlSet, error) {
	m.db.mu.Lock()
	defer m.db.mu.Unlock()
	work := copyTables(m.db.tables)
	sets, err := memRunAll(work, q, args)
	if err != nil {
		return nil, err
	}
	m.db.tables = work
	return sets, nil
}

func (t *memTx) QueryAllTk(_ *Task, q string, args []any) ([]sqlSet, error) {
	if t.readOnly {
		if err := memReadOnly(q); err != nil {
			return nil, err
		}
	}
	work := copyTables(t.tables)
	sets, err := memRunAll(work, q, args)
	if err != nil {
		return nil, err
	}
	t.tables = work
	return sets, nil
}
