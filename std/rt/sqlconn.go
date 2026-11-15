package rt

import (
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
)

// std/sql batch S1 (decision 0648 item 14), as std/native/sql.xo: Db.conn's
// Conn (one pool connection held across calls), Tx.begin's savepoints,
// LISTEN/NOTIFY through DriverConn.wait_notification, and open_driver's
// on_notice. A Conn of a Xo driver is a session on the held connection
// (sqlDriverTx kind sqlTxSession); on the in memory database, and on a
// backend registered from Go, a Conn runs on the Db.

// Sql_Conn is `sql.Conn`: copies share the connection.
type Sql_Conn struct{ c *sqlConn }

type sqlConn struct {
	db *sqlDB
	// tx is a Xo driver's connection as a session; nil: the Conn runs on
	// the Db
	tx       *sqlTx
	closed   atomic.Bool
	listened atomic.Bool
	mu       sync.Mutex
	// txs are the transactions begun on the Conn: one still open at close
	// is rolled back first
	txs []*sqlTx
}

// XoDebug implements Debug.
func (Sql_Conn) XoDebug(b *strings.Builder) { b.WriteString("<conn>") }

const sqlNoNotify = "LISTEN/NOTIFY: the in memory database has no notifications"

// M_conn is `db.conn()`: a pool permit and a connection, held until close.
func (d Sql_Db) M_conn(tk *Task) Result[Sql_Conn, Sql_SqlErr] {
	db := d.d
	if db == nil || db.closed.Load() {
		return sqlFail[Sql_Conn](sqlTagClosed)
	}
	if tk.Canceled() {
		return sqlFail[Sql_Conn](sqlTagCanceled)
	}
	b, ok := db.be.(*sqlDriverBackend)
	if !ok {
		return Ok[Sql_Conn, Sql_SqlErr](Sql_Conn{&sqlConn{db: db}})
	}
	if !db.pool.acquire(tk) {
		return sqlFail[Sql_Conn](sqlTagCanceled)
	}
	x, err := b.take(tk)
	if err != nil {
		db.pool.release()
		return sqlFailErr[Sql_Conn](err)
	}
	s := &sqlTx{db: db, be: &sqlDriverTx{b: b, x: x, kind: sqlTxSession}, goneTag: sqlTagClosed, sp: new(atomic.Int64)}
	return Ok[Sql_Conn, Sql_SqlErr](Sql_Conn{&sqlConn{db: db, tx: s}})
}

// connRun runs onTx on a driver Conn's session, else onDb on the Db;
// Closed after close.
func connRun[T any](c Sql_Conn, onTx func(Sql_Tx) Result[T, Sql_SqlErr], onDb func(Sql_Db) Result[T, Sql_SqlErr]) Result[T, Sql_SqlErr] {
	if c.c == nil || c.c.closed.Load() {
		return sqlFail[T](sqlTagClosed)
	}
	if c.c.tx != nil {
		return onTx(Sql_Tx{c.c.tx})
	}
	return onDb(Sql_Db{c.c.db})
}

// M_query is `conn.query(q, args)`.
func (c Sql_Conn) M_query(tk *Task, q string, args List[Sql_SqlValue]) Result[List[Sql_Row], Sql_SqlErr] {
	return connRun(c, func(t Sql_Tx) Result[List[Sql_Row], Sql_SqlErr] { return t.M_query(tk, q, args) },
		func(d Sql_Db) Result[List[Sql_Row], Sql_SqlErr] { return d.M_query(tk, q, args) })
}

// M_rows is `conn.rows(q, args)`: read whole, as Db.rows on the Go backend.
func (c Sql_Conn) M_rows(tk *Task, q string, args List[Sql_SqlValue]) Result[Sql_Rows, Sql_SqlErr] {
	return sqlRowsOf(c.M_query(tk, q, args))
}

// M_each is `conn.each(q, args, f)`.
func (c Sql_Conn) M_each(tk *Task, q string, args List[Sql_SqlValue], f func(*Task, Sql_Row) Result[Unit, Sql_SqlErr]) Result[Unit, Sql_SqlErr] {
	return sqlEach(tk, c.M_rows(tk, q, args), f)
}

// M_copy_from is `conn.copy_from(table, columns, rows)`.
func (c Sql_Conn) M_copy_from(tk *Task, table string, cols List[string], rows func(*Task) Result[Option[List[Sql_SqlValue]], Sql_SqlErr]) Result[int64, Sql_SqlErr] {
	return connRun(c, func(t Sql_Tx) Result[int64, Sql_SqlErr] { return t.M_copy_from(tk, table, cols, rows) },
		func(d Sql_Db) Result[int64, Sql_SqlErr] { return d.M_copy_from(tk, table, cols, rows) })
}

// M_query_one is `conn.query_one(q, args)`.
func (c Sql_Conn) M_query_one(tk *Task, q string, args List[Sql_SqlValue]) Result[Sql_Row, Sql_SqlErr] {
	return connRun(c, func(t Sql_Tx) Result[Sql_Row, Sql_SqlErr] { return t.M_query_one(tk, q, args) },
		func(d Sql_Db) Result[Sql_Row, Sql_SqlErr] { return d.M_query_one(tk, q, args) })
}

// M_exec is `conn.exec(q, args)`.
func (c Sql_Conn) M_exec(tk *Task, q string, args List[Sql_SqlValue]) Result[Sql_ExecResult, Sql_SqlErr] {
	return connRun(c, func(t Sql_Tx) Result[Sql_ExecResult, Sql_SqlErr] { return t.M_exec(tk, q, args) },
		func(d Sql_Db) Result[Sql_ExecResult, Sql_SqlErr] { return d.M_exec(tk, q, args) })
}

// Sql_Conn_query_as is `conn.query_as[T](q, args)`.
func Sql_Conn_query_as[T any](tk *Task, c Sql_Conn, q string, args List[Sql_SqlValue]) Result[List[T], Sql_SqlErr] {
	return sqlDecodeAll[T](tk, c.M_query(tk, q, args))
}

// Sql_Conn_query_one_as is `conn.query_one_as[T](q, args)`.
func Sql_Conn_query_one_as[T any](tk *Task, c Sql_Conn, q string, args List[Sql_SqlValue]) Result[T, Sql_SqlErr] {
	return sqlDecodeOne[T](tk, c.M_query_one(tk, q, args))
}

// M_begin is `conn.begin(isolation, read_only, deferrable)`: a transaction
// on the held connection, which stays the Conn's when it ends.
func (c Sql_Conn) M_begin(tk *Task, isolation Sql_Isolation, readOnly, deferrable bool) Result[Sql_Tx, Sql_SqlErr] {
	return connRun(c, func(s Sql_Tx) Result[Sql_Tx, Sql_SqlErr] {
		r := inTx(tk, s, func(be SqlTxBackend) Result[Sql_Tx, Sql_SqlErr] {
			dt := be.(*sqlDriverTx)
			if err := beginOn(tk, dt.x, sqlTxMode(isolation, readOnly, deferrable)); err != nil {
				return sqlFailErr[Sql_Tx](err)
			}
			t := &sqlTx{db: s.t.db, be: &sqlDriverTx{b: dt.b, x: dt.x, kind: sqlTxOnConn}, outer: []*sqlTx{s.t}, noRelease: true, sp: new(atomic.Int64)}
			return Ok[Sql_Tx, Sql_SqlErr](Sql_Tx{t})
		})
		if r.Ok {
			c.c.mu.Lock()
			c.c.txs = append(c.c.txs, r.V.t)
			c.c.mu.Unlock()
		}
		return r
	}, func(d Sql_Db) Result[Sql_Tx, Sql_SqlErr] { return d.M_begin(tk, isolation, readOnly, deferrable) })
}

// M_prepare is `conn.prepare(q)`: on the held connection, closed with the
// Conn.
func (c Sql_Conn) M_prepare(tk *Task, q string) Result[Sql_Stmt, Sql_SqlErr] {
	return connRun(c, func(t Sql_Tx) Result[Sql_Stmt, Sql_SqlErr] { return t.M_prepare(tk, q) },
		func(d Sql_Db) Result[Sql_Stmt, Sql_SqlErr] { return d.M_prepare(tk, q) })
}

// M_query_all is `conn.query_all(q, args)`.
func (c Sql_Conn) M_query_all(tk *Task, q string, args List[Sql_SqlValue]) Result[List[Sql_ResultSet], Sql_SqlErr] {
	return connRun(c, func(t Sql_Tx) Result[List[Sql_ResultSet], Sql_SqlErr] { return t.M_query_all(tk, q, args) },
		func(d Sql_Db) Result[List[Sql_ResultSet], Sql_SqlErr] { return d.M_query_all(tk, q, args) })
}

// M_ping is `conn.ping()`.
func (c Sql_Conn) M_ping(tk *Task) Result[Unit, Sql_SqlErr] {
	return connRun(c, func(s Sql_Tx) Result[Unit, Sql_SqlErr] {
		return inTx(tk, s, func(be SqlTxBackend) Result[Unit, Sql_SqlErr] {
			return be.(*sqlDriverTx).x.c.M_ping(tk)
		})
	}, func(d Sql_Db) Result[Unit, Sql_SqlErr] { return d.M_ping(tk) })
}

// M_listen is `conn.listen(channel)`: LISTEN with the name quoted.
func (c Sql_Conn) M_listen(tk *Task, channel string) Result[Unit, Sql_SqlErr] {
	return c.notifyExec(tk, "LISTEN "+Sql_quote_ident(tk, channel), true)
}

// M_unlisten is `conn.unlisten(channel)`.
func (c Sql_Conn) M_unlisten(tk *Task, channel string) Result[Unit, Sql_SqlErr] {
	return c.notifyExec(tk, "UNLISTEN "+Sql_quote_ident(tk, channel), false)
}

func (c Sql_Conn) notifyExec(tk *Task, q string, listen bool) Result[Unit, Sql_SqlErr] {
	return connRun(c, func(s Sql_Tx) Result[Unit, Sql_SqlErr] {
		if listen {
			c.c.listened.Store(true)
		}
		r := s.M_exec(tk, q, sqlValues(nil))
		if !r.Ok {
			return Err[Unit](r.E)
		}
		return Ok[Unit, Sql_SqlErr](Unit{})
	}, func(Sql_Db) Result[Unit, Sql_SqlErr] { return Err[Unit](sqlMsgErr(sqlTagDriver, sqlNoNotify)) })
}

// M_wait_notification is `conn.wait_notification()`: no cancellation
// check first, so a notification already received is returned to a
// cancelled task too, as pgx's.
func (c Sql_Conn) M_wait_notification(tk *Task) Result[Sql_Notification, Sql_SqlErr] {
	return connRun(c, func(s Sql_Tx) Result[Sql_Notification, Sql_SqlErr] {
		s.t.mu.Lock()
		defer s.t.mu.Unlock()
		if s.t.done {
			return sqlFail[Sql_Notification](s.t.gone())
		}
		return s.t.be.(*sqlDriverTx).x.c.M_wait_notification(tk)
	}, func(Sql_Db) Result[Sql_Notification, Sql_SqlErr] {
		return Err[Sql_Notification](sqlMsgErr(sqlTagDriver, sqlNoNotify))
	})
}

// M_close is `conn.close()`: a transaction begun on the Conn and still
// open is rolled back, UNLISTEN * runs when the Conn listened, and the
// connection and its permit go back to the pool. Idempotent.
func (c Sql_Conn) M_close(tk *Task) {
	if c.c == nil || !c.c.closed.CompareAndSwap(false, true) || c.c.tx == nil {
		return
	}
	s := c.c.tx
	dt := s.be.(*sqlDriverTx)
	c.c.mu.Lock()
	txs := c.c.txs
	c.c.txs = nil
	c.c.mu.Unlock()
	for _, t := range txs {
		t.mu.Lock()
		if !t.done {
			t.done = true
			t.ended.Store(true)
			dt.x.c.M_rollback(tk)
		}
		t.mu.Unlock()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.done {
		return
	}
	if c.c.listened.Load() {
		dt.x.c.M_exec(tk, "UNLISTEN *", sqlValues(nil))
	}
	Sql_Tx{s}.finish(tk, false)
}

// M_begin is `tx.begin()`: a savepoint (pgx's Tx.Begin), SAVEPOINT sp_N
// on a driver's connection, a copy of the tables on the in memory
// database.
func (t Sql_Tx) M_begin(tk *Task) Result[Sql_Tx, Sql_SqlErr] {
	return inTx(tk, t, func(be SqlTxBackend) Result[Sql_Tx, Sql_SqlErr] {
		name := "sp_" + strconv.FormatInt(t.t.sp.Add(1), 10)
		outer := append(append([]*sqlTx(nil), t.t.outer...), t.t)
		child := &sqlTx{db: t.t.db, outer: outer, noRelease: true, sp: t.t.sp}
		switch b := be.(type) {
		case *sqlDriverTx:
			if r := b.x.c.M_exec(tk, "savepoint "+name, sqlValues(nil)); !r.Ok {
				return Err[Sql_Tx](r.E)
			}
			child.be = &sqlDriverTx{b: b.b, x: b.x, kind: sqlTxSavepoint, name: name}
		case *memTx:
			child.be = &memTx{b: b.b, tables: copyTables(b.tables), readOnly: b.readOnly, parent: b}
		default:
			if _, _, err := be.Exec("savepoint "+name, nil); err != nil {
				return sqlFailErr[Sql_Tx](err)
			}
			child.be = &sqlSavepointTx{SqlTxBackend: be, name: name}
		}
		return Ok[Sql_Tx, Sql_SqlErr](Sql_Tx{child})
	})
}

// sqlSavepointTx is a savepoint in a transaction of a backend registered
// from Go.
type sqlSavepointTx struct {
	SqlTxBackend
	name string
}

func (t *sqlSavepointTx) Commit() error {
	_, _, err := t.SqlTxBackend.Exec("release savepoint "+t.name, nil)
	return err
}

func (t *sqlSavepointTx) Rollback() error {
	_, _, err := t.SqlTxBackend.Exec("rollback to savepoint "+t.name, nil)
	return err
}

// sqlNoticeConn is a Xo driver's connection whose notices go to
// open_driver's on_notice after each call, in order (pgconn calls
// OnNotice as it reads each NoticeResponse).
type sqlNoticeConn struct {
	Sql_DriverConn
	h func(*Task, Sql_Notice) Result[Unit, Never]
}

func (n sqlNoticeConn) drain(tk *Task) {
	for _, x := range List_Slice(n.Sql_DriverConn.M_notices(tk)) {
		n.h(tk, x)
	}
}

func (n sqlNoticeConn) M_query(tk *Task, q string, args List[Sql_SqlValue]) Result[Sql_DriverRows, Sql_SqlErr] {
	r := n.Sql_DriverConn.M_query(tk, q, args)
	n.drain(tk)
	return r
}

func (n sqlNoticeConn) M_exec(tk *Task, q string, args List[Sql_SqlValue]) Result[Sql_ExecResult, Sql_SqlErr] {
	r := n.Sql_DriverConn.M_exec(tk, q, args)
	n.drain(tk)
	return r
}

func (n sqlNoticeConn) M_begin(tk *Task) Result[Unit, Sql_SqlErr] {
	r := n.Sql_DriverConn.M_begin(tk)
	n.drain(tk)
	return r
}

func (n sqlNoticeConn) M_commit(tk *Task) Result[Unit, Sql_SqlErr] {
	r := n.Sql_DriverConn.M_commit(tk)
	n.drain(tk)
	return r
}

func (n sqlNoticeConn) M_rollback(tk *Task) Result[Unit, Sql_SqlErr] {
	r := n.Sql_DriverConn.M_rollback(tk)
	n.drain(tk)
	return r
}

func (n sqlNoticeConn) M_ping(tk *Task) Result[Unit, Sql_SqlErr] {
	r := n.Sql_DriverConn.M_ping(tk)
	n.drain(tk)
	return r
}

func (n sqlNoticeConn) M_copy_from(tk *Task, table string, cols List[string], rows func(*Task) Result[Option[List[Sql_SqlValue]], Sql_SqlErr]) Result[int64, Sql_SqlErr] {
	r := n.Sql_DriverConn.M_copy_from(tk, table, cols, rows)
	n.drain(tk)
	return r
}

func (n sqlNoticeConn) M_prepare(tk *Task, q string) Result[Option[Sql_DriverStmt], Sql_SqlErr] {
	r := n.Sql_DriverConn.M_prepare(tk, q)
	n.drain(tk)
	return r
}

func (n sqlNoticeConn) M_wait_notification(tk *Task) Result[Sql_Notification, Sql_SqlErr] {
	r := n.Sql_DriverConn.M_wait_notification(tk)
	n.drain(tk)
	return r
}
