package rt

import (
	"sync/atomic"
	"time"
)

// std/sql pool statistics and tracing (decision 0648 item 16), as
// std/native/sql.xo: Db.stats's counts of a Xo driver's pool, and
// open_driver's on_trace, given a TraceRecord as each traced operation
// ends (pgx's tracers, pgxpool's AcquireTracer and ReleaseTracer). The in
// memory database and a backend registered from Go count nothing.

// sqlCounts are a Xo driver pool's counts.
type sqlCounts struct {
	inUse, open, connecting atomic.Int64
	acquires, acquireNs     atomic.Int64
	waits, waitNs, canceled atomic.Int64
	newConns                atomic.Int64
	idleClosed, lifeClosed  atomic.Int64
}

const (
	sqlTraceConnect = iota + 1
	sqlTraceAcquire
	sqlTraceRelease
	sqlTraceQuery
	sqlTraceExec
	sqlTracePrepare
	sqlTraceBatchQuery
	sqlTraceBatch
	sqlTraceCopyFrom
)

type sqlTraceFn = func(*Task, Sql_TraceRecord) Result[Unit, Never]

// sqlEmit gives h the record of an operation that started at start.
func sqlEmit(tk *Task, h sqlTraceFn, start time.Time, op int, q string, args List[Sql_SqlValue], n int64, err *Sql_SqlErr) {
	rec := Sql_TraceRecord{F_op: Sql_TraceOp{Tag: op}, F_query: q, F_args: args, F_rows_affected: n, F_start_ns: start.UnixNano(), F_elapsed: Duration(time.Since(start))}
	if err != nil {
		rec.F_err = Some(*err)
	}
	h(tk, rec)
}

func sqlErrPtr[T any](r Result[T, Sql_SqlErr]) *Sql_SqlErr {
	if r.Ok {
		return nil
	}
	e := r.E
	return &e
}

// trace reports a pool operation (Connect, Acquire, Release).
func (b *sqlDriverBackend) trace(tk *Task, start time.Time, op int, err error) {
	if !b.onTrace.Some {
		return
	}
	var e *Sql_SqlErr
	if err != nil {
		x := toSqlErr(err)
		e = &x
	}
	sqlEmit(tk, b.onTrace.V, start, op, "", ListOf[Sql_SqlValue](), 0, e)
}

// acquirePermit takes one of d's max_open permits, counting a wait and a
// cancelled one for a Xo driver's pool.
func (d *sqlDB) acquirePermit(tk *Task) bool {
	b, ok := d.be.(*sqlDriverBackend)
	if !ok {
		return d.pool.acquire(tk)
	}
	start := time.Now()
	d.pool.mu.Lock()
	free := d.pool.free(false, false)
	d.pool.mu.Unlock()
	if !free {
		// counted as the wait starts, as Go's DB.Stats WaitCount
		b.n.waits.Add(1)
	}
	ok = d.pool.acquire(tk)
	if !free {
		b.n.waitNs.Add(int64(time.Since(start)))
	}
	if !ok {
		b.n.canceled.Add(1)
		b.trace(tk, start, sqlTraceAcquire, &sqlErrBox{sqlE(sqlTagCanceled, nil)})
	}
	return ok
}

// acquired counts an acquire that began at start and reports it.
func (b *sqlDriverBackend) acquired(tk *Task, start time.Time, err error) {
	b.n.acquires.Add(1)
	b.n.acquireNs.Add(int64(time.Since(start)))
	if err == nil {
		b.n.inUse.Add(1)
	}
	b.trace(tk, start, sqlTraceAcquire, err)
}

// M_stats is `db.stats()`.
func (d Sql_Db) M_stats(*Task) Sql_PoolStats {
	if d.d == nil {
		return Sql_PoolStats{}
	}
	b, ok := d.d.be.(*sqlDriverBackend)
	if !ok {
		return Sql_PoolStats{}
	}
	b.mu.Lock()
	idle := int64(len(b.idle))
	b.mu.Unlock()
	n := &b.n
	return Sql_PoolStats{
		F_max_open: int64(b.maxOpen), F_open: n.open.Load() + n.connecting.Load(), F_in_use: n.inUse.Load(), F_idle: idle,
		F_connecting: n.connecting.Load(), F_acquire_count: n.acquires.Load(), F_acquire_time: Duration(n.acquireNs.Load()),
		F_wait_count: n.waits.Load(), F_wait_time: Duration(n.waitNs.Load()), F_canceled_acquire_count: n.canceled.Load(),
		F_new_conns: n.newConns.Load(), F_max_idle_closed: n.idleClosed.Load(), F_max_lifetime_closed: n.lifeClosed.Load(),
	}
}

// sqlTraceConn is a Xo driver's connection whose calls are reported to
// on_trace as they end.
type sqlTraceConn struct {
	Sql_DriverConn
	h sqlTraceFn
}

func (t sqlTraceConn) M_query(tk *Task, q string, args List[Sql_SqlValue]) Result[Sql_DriverRows, Sql_SqlErr] {
	start := time.Now()
	r := t.Sql_DriverConn.M_query(tk, q, args)
	return sqlTraceRows(tk, t.h, start, q, args, r)
}

// sqlTraceRows reports a query's result, or wraps its cursor to report
// at its end (pgx's TraceQueryEnd at Rows.Close).
func sqlTraceRows(tk *Task, h sqlTraceFn, start time.Time, q string, args List[Sql_SqlValue], r Result[Sql_DriverRows, Sql_SqlErr]) Result[Sql_DriverRows, Sql_SqlErr] {
	if !r.Ok {
		sqlEmit(tk, h, start, sqlTraceQuery, q, args, 0, &r.E)
		return r
	}
	n := int64(len(List_Slice(r.V.F_rows)))
	if r.V.F_cursor.Some {
		c := &sqlTraceCursor{c: r.V.F_cursor.V, h: h, start: start, q: q, args: args}
		c.n.Store(n)
		r.V.F_cursor = Some[Sql_DriverCursor](c)
		return r
	}
	sqlEmit(tk, h, start, sqlTraceQuery, q, args, n, nil)
	return r
}

func (t sqlTraceConn) M_exec(tk *Task, q string, args List[Sql_SqlValue]) Result[Sql_ExecResult, Sql_SqlErr] {
	start := time.Now()
	r := t.Sql_DriverConn.M_exec(tk, q, args)
	sqlEmit(tk, t.h, start, sqlTraceExec, q, args, r.V.F_rows_affected, sqlErrPtr(r))
	return r
}

func (t sqlTraceConn) txOp(tk *Task, what string, f func(*Task) Result[Unit, Sql_SqlErr]) Result[Unit, Sql_SqlErr] {
	start := time.Now()
	r := f(tk)
	sqlEmit(tk, t.h, start, sqlTraceExec, what, ListOf[Sql_SqlValue](), 0, sqlErrPtr(r))
	return r
}

func (t sqlTraceConn) M_begin(tk *Task) Result[Unit, Sql_SqlErr] {
	return t.txOp(tk, "BEGIN", t.Sql_DriverConn.M_begin)
}

func (t sqlTraceConn) M_commit(tk *Task) Result[Unit, Sql_SqlErr] {
	return t.txOp(tk, "COMMIT", t.Sql_DriverConn.M_commit)
}

func (t sqlTraceConn) M_rollback(tk *Task) Result[Unit, Sql_SqlErr] {
	return t.txOp(tk, "ROLLBACK", t.Sql_DriverConn.M_rollback)
}

func (t sqlTraceConn) M_copy_from(tk *Task, table string, cols List[string], rows func(*Task) Result[Option[List[Sql_SqlValue]], Sql_SqlErr]) Result[int64, Sql_SqlErr] {
	start := time.Now()
	r := t.Sql_DriverConn.M_copy_from(tk, table, cols, rows)
	sqlEmit(tk, t.h, start, sqlTraceCopyFrom, table, ListOf[Sql_SqlValue](), r.V, sqlErrPtr(r))
	return r
}

func (t sqlTraceConn) M_prepare(tk *Task, q string) Result[Option[Sql_DriverStmt], Sql_SqlErr] {
	start := time.Now()
	r := t.Sql_DriverConn.M_prepare(tk, q)
	sqlEmit(tk, t.h, start, sqlTracePrepare, q, ListOf[Sql_SqlValue](), 0, sqlErrPtr(r))
	if r.Ok && r.V.Some {
		r.V = Some[Sql_DriverStmt](sqlTraceStmt{s: r.V.V, h: t.h, q: q})
	}
	return r
}

// M_pipeline reports each query of each batch that succeeded (with the
// call's start and length: the queries are pipelined), then the Batch.
func (t sqlTraceConn) M_pipeline(tk *Task, batches List[List[Sql_BatchQuery]], inTx bool) Result[List[Result[List[Sql_DriverResult], Sql_SqlErr]], Sql_SqlErr] {
	start := time.Now()
	r := t.Sql_DriverConn.M_pipeline(tk, batches, inTx)
	first := sqlErrPtr(r)
	var total int64
	if r.Ok {
		bs := List_Slice(batches)
		for i, o := range List_Slice(r.V) {
			if !o.Ok {
				if first == nil {
					e := o.E
					first = &e
				}
				continue
			}
			for j, x := range List_Slice(o.V) {
				q := Sql_BatchQuery{F_args: ListOf[Sql_SqlValue]()}
				if i < len(bs) && j < len(List_Slice(bs[i])) {
					q = List_Slice(bs[i])[j]
				}
				sqlEmit(tk, t.h, start, sqlTraceBatchQuery, q.F_query, q.F_args, x.F_rows_affected, nil)
				total += x.F_rows_affected
			}
		}
	}
	sqlEmit(tk, t.h, start, sqlTraceBatch, "", ListOf[Sql_SqlValue](), total, first)
	return r
}

// sqlTraceStmt is a prepared statement whose runs are Query and Exec
// records of its text.
type sqlTraceStmt struct {
	s Sql_DriverStmt
	h sqlTraceFn
	q string
}

func (t sqlTraceStmt) M_query(tk *Task, args List[Sql_SqlValue]) Result[Sql_DriverRows, Sql_SqlErr] {
	start := time.Now()
	return sqlTraceRows(tk, t.h, start, t.q, args, t.s.M_query(tk, args))
}

func (t sqlTraceStmt) M_exec(tk *Task, args List[Sql_SqlValue]) Result[Sql_ExecResult, Sql_SqlErr] {
	start := time.Now()
	r := t.s.M_exec(tk, args)
	sqlEmit(tk, t.h, start, sqlTraceExec, t.q, args, r.V.F_rows_affected, sqlErrPtr(r))
	return r
}

func (t sqlTraceStmt) M_close(tk *Task) { t.s.M_close(tk) }

// sqlTraceCursor reports its query when the last row has been read, on an
// error, or at close, with the rows read.
type sqlTraceCursor struct {
	c     Sql_DriverCursor
	h     sqlTraceFn
	start time.Time
	q     string
	args  List[Sql_SqlValue]
	n     atomic.Int64
	done  atomic.Bool
}

func (t *sqlTraceCursor) end(tk *Task, err *Sql_SqlErr) {
	if !t.done.Swap(true) {
		sqlEmit(tk, t.h, t.start, sqlTraceQuery, t.q, t.args, t.n.Load(), err)
	}
}

func (t *sqlTraceCursor) M_next(tk *Task) Result[Option[List[Sql_SqlValue]], Sql_SqlErr] {
	r := t.c.M_next(tk)
	switch {
	case !r.Ok:
		t.end(tk, &r.E)
	case r.V.Some:
		t.n.Add(1)
	default:
		t.end(tk, nil)
	}
	return r
}

func (t *sqlTraceCursor) M_rest(tk *Task) Result[List[List[Sql_SqlValue]], Sql_SqlErr] {
	r := t.c.M_rest(tk)
	if r.Ok {
		t.n.Add(int64(len(List_Slice(r.V))))
	}
	t.end(tk, sqlErrPtr(r))
	return r
}

func (t *sqlTraceCursor) M_close(tk *Task) {
	t.c.M_close(tk)
	t.end(tk, nil)
}
