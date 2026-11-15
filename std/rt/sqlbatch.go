package rt

// std/sql batches and pipelines (decision 0648 item 15, pgx's SendBatch
// and pgconn's Pipeline), as std/native/sql.xo: a Xo driver's
// DriverConn.pipeline, and each batch on a copy of the tables on the in
// memory database. A backend registered from Go has no batches (Driver).

// sqlBatchOut is one batch's sets, or its first error.
type sqlBatchOut struct {
	sets []sqlSet
	err  error
}

// sqlPipeRunner is a backend or transaction with batches (the memory
// database and the Xo driver adapter). inTx tells a driver that the
// batches run inside the caller's transaction.
type sqlPipeRunner interface {
	PipelineTk(tk *Task, batches List[List[Sql_BatchQuery]], inTx bool) ([]sqlBatchOut, error)
}

type sqlOutcomes = List[Result[List[Sql_ResultSet], Sql_SqlErr]]

const sqlNoBatches = "batches: the driver has no DriverConn.pipeline"

func sqlPipeline(tk *Task, r any, batches List[List[Sql_BatchQuery]], inTx bool) Result[sqlOutcomes, Sql_SqlErr] {
	if len(List_Slice(batches)) == 0 {
		return Ok[sqlOutcomes, Sql_SqlErr](ListOf[Result[List[Sql_ResultSet], Sql_SqlErr]]())
	}
	p, ok := r.(sqlPipeRunner)
	if !ok {
		return Err[sqlOutcomes](sqlMsgErr(sqlTagDriver, sqlNoBatches))
	}
	outs, err := p.PipelineTk(tk, batches, inTx)
	if err != nil {
		return sqlFailErr[sqlOutcomes](err)
	}
	res := make([]Result[List[Sql_ResultSet], Sql_SqlErr], len(outs))
	for i, o := range outs {
		if o.err != nil {
			res[i] = Err[List[Sql_ResultSet]](toSqlErr(o.err))
		} else {
			res[i] = Ok[List[Sql_ResultSet], Sql_SqlErr](sqlSets(o.sets))
		}
	}
	return Ok[sqlOutcomes, Sql_SqlErr](ListFromSlice(res))
}

// sqlOnlyBatch is the one batch of a pipeline of one: its sets, or its
// error.
func sqlOnlyBatch(r Result[sqlOutcomes, Sql_SqlErr]) Result[List[Sql_ResultSet], Sql_SqlErr] {
	if !r.Ok {
		return Err[List[Sql_ResultSet]](r.E)
	}
	xs := List_Slice(r.V)
	if len(xs) == 0 {
		return Ok[List[Sql_ResultSet], Sql_SqlErr](ListOf[Sql_ResultSet]())
	}
	return xs[0]
}

// M_pipeline is `db.pipeline(batches)`.
func (d Sql_Db) M_pipeline(tk *Task, batches List[List[Sql_BatchQuery]]) Result[sqlOutcomes, Sql_SqlErr] {
	return withConn(tk, d.d, func() Result[sqlOutcomes, Sql_SqlErr] { return sqlPipeline(tk, d.d.be, batches, false) })
}

// M_batch is `db.batch(queries)`: an empty batch sends nothing.
func (d Sql_Db) M_batch(tk *Task, queries List[Sql_BatchQuery]) Result[List[Sql_ResultSet], Sql_SqlErr] {
	if len(List_Slice(queries)) == 0 {
		return sqlOnlyBatch(d.M_pipeline(tk, ListOf[List[Sql_BatchQuery]]()))
	}
	return sqlOnlyBatch(d.M_pipeline(tk, ListOf(queries)))
}

// txPipeline is the batches on t's connection (a Conn's session runs
// each in a transaction of its own) or on its tables.
func txPipeline(tk *Task, t Sql_Tx, batches List[List[Sql_BatchQuery]]) Result[sqlOutcomes, Sql_SqlErr] {
	return inTx(tk, t, func(be SqlTxBackend) Result[sqlOutcomes, Sql_SqlErr] { return sqlPipeline(tk, be, batches, true) })
}

// M_batch is `tx.batch(queries)`.
func (t Sql_Tx) M_batch(tk *Task, queries List[Sql_BatchQuery]) Result[List[Sql_ResultSet], Sql_SqlErr] {
	if len(List_Slice(queries)) == 0 {
		return sqlOnlyBatch(txPipeline(tk, t, ListOf[List[Sql_BatchQuery]]()))
	}
	return sqlOnlyBatch(txPipeline(tk, t, ListOf(queries)))
}

// M_pipeline is `conn.pipeline(batches)`.
func (c Sql_Conn) M_pipeline(tk *Task, batches List[List[Sql_BatchQuery]]) Result[sqlOutcomes, Sql_SqlErr] {
	return connRun(c, func(t Sql_Tx) Result[sqlOutcomes, Sql_SqlErr] { return txPipeline(tk, t, batches) },
		func(d Sql_Db) Result[sqlOutcomes, Sql_SqlErr] { return d.M_pipeline(tk, batches) })
}

// M_batch is `conn.batch(queries)`.
func (c Sql_Conn) M_batch(tk *Task, queries List[Sql_BatchQuery]) Result[List[Sql_ResultSet], Sql_SqlErr] {
	return connRun(c, func(t Sql_Tx) Result[List[Sql_ResultSet], Sql_SqlErr] { return t.M_batch(tk, queries) },
		func(d Sql_Db) Result[List[Sql_ResultSet], Sql_SqlErr] { return d.M_batch(tk, queries) })
}

// M_rows_affected is `set.rows_affected()`.
func (s Sql_ResultSet) M_rows_affected(*Task) int64 { return s.affected }

// driverOutcomes is a Xo driver's DriverConn.pipeline result as batches
// of sets.
func driverOutcomes(r Result[List[Result[List[Sql_DriverResult], Sql_SqlErr]], Sql_SqlErr]) ([]sqlBatchOut, error) {
	if !r.Ok {
		return nil, &sqlErrBox{r.E}
	}
	var outs []sqlBatchOut
	for _, b := range List_Slice(r.V) {
		if !b.Ok {
			outs = append(outs, sqlBatchOut{err: &sqlErrBox{b.E}})
			continue
		}
		var sets []sqlSet
		for _, d := range List_Slice(b.V) {
			var rows [][]any
			for _, row := range List_Slice(d.F_rows) {
				rows = append(rows, sqlArgs(row))
			}
			sets = append(sets, sqlSet{cols: List_Slice(d.F_columns), types: List_Slice(d.F_column_types), rows: rows, affected: d.F_rows_affected})
		}
		outs = append(outs, sqlBatchOut{sets: sets})
	}
	return outs, nil
}

// PipelineTk runs the batches on one pool connection, each a transaction
// of its own.
func (b *sqlDriverBackend) PipelineTk(tk *Task, batches List[List[Sql_BatchQuery]], _ bool) ([]sqlBatchOut, error) {
	x, err := b.take(tk)
	if err != nil {
		return nil, err
	}
	defer b.give(tk, x)
	return driverOutcomes(x.c.M_pipeline(tk, batches, false))
}

// PipelineTk runs the batches on the transaction's connection: inside it,
// except on a Conn's own session.
func (t *sqlDriverTx) PipelineTk(tk *Task, batches List[List[Sql_BatchQuery]], _ bool) ([]sqlBatchOut, error) {
	return driverOutcomes(t.x.c.M_pipeline(tk, batches, t.kind != sqlTxSession))
}

// memBatchOne is one query of a batch on the in memory database, as
// std/native/sql.xo mem_batch_one: one statement, its rows (a SELECT, or a
// write with RETURNING) and its count.
func memBatchOne(tables map[string]*memTable, q string, args []any) (sqlSet, error) {
	toks, err := sqlLex(q)
	if err != nil {
		return sqlSet{}, err
	}
	stmts := splitStatements(toks)
	if len(stmts) != 1 {
		return sqlSet{}, sqlErr(SqlSyntax, "expected one statement")
	}
	p := &sqlParser{toks: stmts[0]}
	var w memWrite
	switch {
	case p.isKw("select"):
		cols, rows, err := memSelect(tables, p, args)
		if err != nil {
			return sqlSet{}, err
		}
		return sqlSet{cols: cols, types: p.types, rows: rows, affected: int64(len(rows))}, nil
	case p.isKw("create"):
		err = memCreate(tables, p)
	case p.isKw("drop"):
		err = memDrop(tables, p)
	case p.isKw("insert"):
		w, err = memInsert(tables, p, args)
	case p.isKw("update"):
		w, err = memUpdate(tables, p, args)
	case p.isKw("delete"):
		w, err = memDelete(tables, p, args)
	default:
		err = p.fail("a statement")
	}
	if err == nil && p.peek().k != tEOF {
		err = p.fail("end of statement")
	}
	if err != nil {
		return sqlSet{}, err
	}
	return sqlSet{cols: w.cols, types: w.types, rows: w.rows, affected: w.n}, nil
}

// memBatch runs qs on a copy of tables: the copy and the sets when every
// query succeeds.
func memBatch(tables map[string]*memTable, qs List[Sql_BatchQuery], readOnly bool) (map[string]*memTable, []sqlSet, error) {
	xs := List_Slice(qs)
	if len(xs) == 0 {
		return tables, nil, nil
	}
	work := copyTables(tables)
	sets := make([]sqlSet, 0, len(xs))
	for _, x := range xs {
		if readOnly {
			if err := memReadOnly(x.F_query); err != nil {
				return nil, nil, err
			}
		}
		s, err := memBatchOne(work, x.F_query, sqlArgs(x.F_args))
		if err != nil {
			return nil, nil, err
		}
		sets = append(sets, s)
	}
	return work, sets, nil
}

func memPipeline(tables *map[string]*memTable, batches List[List[Sql_BatchQuery]], readOnly bool) []sqlBatchOut {
	var outs []sqlBatchOut
	for _, b := range List_Slice(batches) {
		work, sets, err := memBatch(*tables, b, readOnly)
		if err != nil {
			outs = append(outs, sqlBatchOut{err: err})
			continue
		}
		*tables = work
		outs = append(outs, sqlBatchOut{sets: sets})
	}
	return outs
}

// PipelineTk is the batches on the in memory database.
func (m *memBackend) PipelineTk(_ *Task, batches List[List[Sql_BatchQuery]], _ bool) ([]sqlBatchOut, error) {
	m.db.mu.Lock()
	defer m.db.mu.Unlock()
	return memPipeline(&m.db.tables, batches, false), nil
}

func (t *memTx) PipelineTk(_ *Task, batches List[List[Sql_BatchQuery]], _ bool) ([]sqlBatchOut, error) {
	return memPipeline(&t.tables, batches, t.readOnly), nil
}
