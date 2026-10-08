package rt

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"unicode"
)

// The in memory SQL driver of std/sql (std.md 10). It implements a subset of
// SQL meant for tests:
//
//	CREATE TABLE [IF NOT EXISTS] t (col type [PRIMARY KEY] [NOT NULL] [UNIQUE]
//	    [AUTOINCREMENT] [DEFAULT lit], ...)
//	DROP TABLE [IF EXISTS] t
//	INSERT INTO t [(cols)] VALUES (exprs) [, (exprs)]
//	SELECT * | expr [AS name] | COUNT(*) {, ...} FROM t [WHERE expr]
//	    [ORDER BY expr [ASC|DESC] {, ...}] [LIMIT n [OFFSET m]]
//	UPDATE t SET col = expr {, ...} [WHERE expr]
//	DELETE FROM t [WHERE expr]
//
// Expressions: columns, literals, $n, + - * /, = != <> < <= > >=, AND, OR,
// NOT, IS [NOT] NULL, [NOT] IN (...), [NOT] LIKE. Statements are separated by
// ';' (only a schema or an exec without arguments may hold several). Names
// are case insensitive.
//
// Values are nil, int64, string, float64, bool, and Time.
//
// Tables are immutable once published: a statement that writes builds a new
// table and swaps the pointer, so a transaction is a copy of the table map,
// commit publishes the map, and rollback drops it.

type memCol struct {
	name    string
	kind    byte // 'i', 't', 'r', 'b', 'T' (time)
	notNull bool
	unique  bool
	pk      bool
	auto    bool
	def     any
}

type memTable struct {
	name   string
	cols   []memCol
	rows   [][]any
	nextID int64
}

func (t *memTable) clone() *memTable {
	c := *t
	c.rows = append([][]any(nil), t.rows...)
	return &c
}

func (t *memTable) index(name string) int {
	for i, c := range t.cols {
		if c.name == name {
			return i
		}
	}
	return -1
}

type memDB struct {
	mu     sync.Mutex
	tables map[string]*memTable
}

// memBackend is the in memory SqlBackend.
type memBackend struct {
	db *memDB
}

func newMemBackend() *memBackend {
	return &memBackend{db: &memDB{tables: map[string]*memTable{}}}
}

func (m *memBackend) SingleWriter() bool { return true }
func (m *memBackend) Ping() error        { return nil }
func (m *memBackend) Close() error       { return nil }

func copyTables(src map[string]*memTable) map[string]*memTable {
	work := make(map[string]*memTable, len(src))
	for k, v := range src {
		work[k] = v
	}
	return work
}

func (m *memBackend) Query(q string, args []any) ([]string, [][]any, error) {
	m.db.mu.Lock()
	defer m.db.mu.Unlock()
	return memRun(m.db.tables, q, args)
}

func (m *memBackend) Exec(q string, args []any) (int64, int64, error) {
	m.db.mu.Lock()
	defer m.db.mu.Unlock()
	work := copyTables(m.db.tables)
	n, id, err := memExec(work, q, args)
	if err != nil {
		return 0, 0, err
	}
	m.db.tables = work
	return n, id, nil
}

func (m *memBackend) Begin() (SqlTxBackend, error) {
	m.db.mu.Lock()
	defer m.db.mu.Unlock()
	return &memTx{b: m, tables: copyTables(m.db.tables)}, nil
}

type memTx struct {
	b      *memBackend
	tables map[string]*memTable
}

func (t *memTx) Query(q string, args []any) ([]string, [][]any, error) {
	return memRun(t.tables, q, args)
}

func (t *memTx) Exec(q string, args []any) (int64, int64, error) {
	// a failed statement leaves the transaction as it was
	work := copyTables(t.tables)
	n, id, err := memExec(work, q, args)
	if err != nil {
		return 0, 0, err
	}
	t.tables = work
	return n, id, nil
}

func (t *memTx) Commit() error {
	t.b.db.mu.Lock()
	t.b.db.tables = t.tables
	t.b.db.mu.Unlock()
	return nil
}

func (t *memTx) Rollback() error { return nil }

func sqlErr(kind SqlKind, f string, a ...any) error {
	return &SqlError{Kind: kind, Msg: fmt.Sprintf(f, a...)}
}

// ---------------------------------------------------------------------------
// Lexer

const (
	tEOF = iota
	tIdent
	tInt
	tFloat
	tStr
	tParam
	tSym
)

type sqlTok struct {
	k int
	s string // identifiers are lower case; symbols are as written
	i int64
	f float64
}

func sqlLex(q string) ([]sqlTok, error) {
	var out []sqlTok
	rs := []rune(q)
	for i := 0; i < len(rs); {
		c := rs[i]
		switch {
		case unicode.IsSpace(c):
			i++
		case c == '-' && i+1 < len(rs) && rs[i+1] == '-':
			for i < len(rs) && rs[i] != '\n' {
				i++
			}
		case unicode.IsLetter(c) || c == '_':
			j := i
			for j < len(rs) && (unicode.IsLetter(rs[j]) || unicode.IsDigit(rs[j]) || rs[j] == '_') {
				j++
			}
			out = append(out, sqlTok{k: tIdent, s: strings.ToLower(string(rs[i:j]))})
			i = j
		case unicode.IsDigit(c):
			j := i
			isF := false
			for j < len(rs) && (unicode.IsDigit(rs[j]) || (rs[j] == '.' && !isF)) {
				if rs[j] == '.' {
					isF = true
				}
				j++
			}
			s := string(rs[i:j])
			if isF {
				f, err := strconv.ParseFloat(s, 64)
				if err != nil {
					return nil, sqlErr(SqlSyntax, "bad number %q", s)
				}
				out = append(out, sqlTok{k: tFloat, f: f})
			} else {
				n, err := strconv.ParseInt(s, 10, 64)
				if err != nil {
					return nil, sqlErr(SqlSyntax, "bad number %q", s)
				}
				out = append(out, sqlTok{k: tInt, i: n})
			}
			i = j
		case c == '\'':
			var b strings.Builder
			j := i + 1
			closed := false
			for j < len(rs) {
				if rs[j] == '\'' {
					if j+1 < len(rs) && rs[j+1] == '\'' {
						b.WriteRune('\'')
						j += 2
						continue
					}
					closed = true
					j++
					break
				}
				b.WriteRune(rs[j])
				j++
			}
			if !closed {
				return nil, sqlErr(SqlSyntax, "unterminated string")
			}
			out = append(out, sqlTok{k: tStr, s: b.String()})
			i = j
		case c == '$':
			j := i + 1
			for j < len(rs) && unicode.IsDigit(rs[j]) {
				j++
			}
			n, err := strconv.ParseInt(string(rs[i+1:j]), 10, 64)
			if err != nil || n < 1 {
				return nil, sqlErr(SqlSyntax, "bad placeholder")
			}
			out = append(out, sqlTok{k: tParam, i: n})
			i = j
		default:
			if i+1 < len(rs) {
				switch two := string(rs[i : i+2]); two {
				case "<=", ">=", "<>", "!=":
					out = append(out, sqlTok{k: tSym, s: two})
					i += 2
					continue
				}
			}
			if strings.ContainsRune("(),*=<>;.+-/", c) {
				out = append(out, sqlTok{k: tSym, s: string(c)})
				i++
				continue
			}
			return nil, sqlErr(SqlSyntax, "unexpected character %q", string(c))
		}
	}
	out = append(out, sqlTok{k: tEOF})
	return out, nil
}

// ---------------------------------------------------------------------------
// Parser

type sqlNode struct {
	op   string
	l, r *sqlNode
	v    any
	col  string
	list []*sqlNode
}

type sqlParser struct {
	toks     []sqlTok
	i        int
	maxParam int64
}

func (p *sqlParser) peek() sqlTok { return p.toks[p.i] }
func (p *sqlParser) next() sqlTok {
	t := p.toks[p.i]
	if t.k != tEOF {
		p.i++
	}
	return t
}

func (p *sqlParser) isKw(kw string) bool {
	t := p.peek()
	return t.k == tIdent && t.s == kw
}

func (p *sqlParser) isSym(s string) bool {
	t := p.peek()
	return t.k == tSym && t.s == s
}

func (p *sqlParser) acceptKw(kw string) bool {
	if p.isKw(kw) {
		p.i++
		return true
	}
	return false
}

func (p *sqlParser) acceptSym(s string) bool {
	if p.isSym(s) {
		p.i++
		return true
	}
	return false
}

func (p *sqlParser) fail(what string) error {
	t := p.peek()
	got := "end of input"
	switch t.k {
	case tIdent, tSym:
		got = strconv.Quote(t.s)
	case tInt, tFloat, tStr, tParam:
		got = "a literal"
	}
	return sqlErr(SqlSyntax, "expected %s, found %s", what, got)
}

func (p *sqlParser) expectKw(kw string) error {
	if !p.acceptKw(kw) {
		return p.fail(strings.ToUpper(kw))
	}
	return nil
}

func (p *sqlParser) expectSym(s string) error {
	if !p.acceptSym(s) {
		return p.fail(strconv.Quote(s))
	}
	return nil
}

func (p *sqlParser) ident(what string) (string, error) {
	t := p.peek()
	if t.k != tIdent {
		return "", p.fail(what)
	}
	p.i++
	return t.s, nil
}

func (p *sqlParser) expr() (*sqlNode, error) { return p.orExpr() }

func (p *sqlParser) orExpr() (*sqlNode, error) {
	l, err := p.andExpr()
	for err == nil && p.acceptKw("or") {
		var r *sqlNode
		if r, err = p.andExpr(); err == nil {
			l = &sqlNode{op: "or", l: l, r: r}
		}
	}
	return l, err
}

func (p *sqlParser) andExpr() (*sqlNode, error) {
	l, err := p.notExpr()
	for err == nil && p.acceptKw("and") {
		var r *sqlNode
		if r, err = p.notExpr(); err == nil {
			l = &sqlNode{op: "and", l: l, r: r}
		}
	}
	return l, err
}

func (p *sqlParser) notExpr() (*sqlNode, error) {
	if p.acceptKw("not") {
		x, err := p.notExpr()
		return &sqlNode{op: "not", l: x}, err
	}
	return p.cmpExpr()
}

func (p *sqlParser) cmpExpr() (*sqlNode, error) {
	l, err := p.addExpr()
	if err != nil {
		return nil, err
	}
	t := p.peek()
	if t.k == tSym {
		switch t.s {
		case "=", "!=", "<>", "<", "<=", ">", ">=":
			p.i++
			r, err := p.addExpr()
			if err != nil {
				return nil, err
			}
			op := t.s
			if op == "<>" {
				op = "!="
			}
			return &sqlNode{op: op, l: l, r: r}, nil
		}
	}
	if p.acceptKw("is") {
		neg := p.acceptKw("not")
		if err := p.expectKw("null"); err != nil {
			return nil, err
		}
		if neg {
			return &sqlNode{op: "notnull", l: l}, nil
		}
		return &sqlNode{op: "isnull", l: l}, nil
	}
	neg := false
	if p.isKw("not") && p.i+1 < len(p.toks) && p.toks[p.i+1].k == tIdent && (p.toks[p.i+1].s == "in" || p.toks[p.i+1].s == "like") {
		p.i++
		neg = true
	}
	var n *sqlNode
	switch {
	case p.acceptKw("in"):
		if err := p.expectSym("("); err != nil {
			return nil, err
		}
		n = &sqlNode{op: "in", l: l}
		for {
			x, err := p.addExpr()
			if err != nil {
				return nil, err
			}
			n.list = append(n.list, x)
			if !p.acceptSym(",") {
				break
			}
		}
		if err := p.expectSym(")"); err != nil {
			return nil, err
		}
	case p.acceptKw("like"):
		r, err := p.addExpr()
		if err != nil {
			return nil, err
		}
		n = &sqlNode{op: "like", l: l, r: r}
	default:
		return l, nil
	}
	if neg {
		return &sqlNode{op: "not", l: n}, nil
	}
	return n, nil
}

func (p *sqlParser) addExpr() (*sqlNode, error) {
	l, err := p.mulExpr()
	for err == nil && (p.isSym("+") || p.isSym("-")) {
		op := p.next().s
		var r *sqlNode
		if r, err = p.mulExpr(); err == nil {
			l = &sqlNode{op: op, l: l, r: r}
		}
	}
	return l, err
}

func (p *sqlParser) mulExpr() (*sqlNode, error) {
	l, err := p.primary()
	for err == nil && (p.isSym("*") || p.isSym("/")) {
		op := p.next().s
		var r *sqlNode
		if r, err = p.primary(); err == nil {
			l = &sqlNode{op: op, l: l, r: r}
		}
	}
	return l, err
}

func (p *sqlParser) primary() (*sqlNode, error) {
	t := p.next()
	switch t.k {
	case tInt:
		return &sqlNode{op: "lit", v: t.i}, nil
	case tFloat:
		return &sqlNode{op: "lit", v: t.f}, nil
	case tStr:
		return &sqlNode{op: "lit", v: t.s}, nil
	case tParam:
		if t.i > p.maxParam {
			p.maxParam = t.i
		}
		return &sqlNode{op: "param", v: t.i}, nil
	case tSym:
		switch t.s {
		case "(":
			x, err := p.expr()
			if err != nil {
				return nil, err
			}
			return x, p.expectSym(")")
		case "-":
			x, err := p.primary()
			return &sqlNode{op: "-", l: &sqlNode{op: "lit", v: int64(0)}, r: x}, err
		}
	case tIdent:
		switch t.s {
		case "null":
			return &sqlNode{op: "lit"}, nil
		case "true":
			return &sqlNode{op: "lit", v: true}, nil
		case "false":
			return &sqlNode{op: "lit", v: false}, nil
		}
		if p.isSym("(") && t.s == "count" {
			p.i++
			if err := p.expectSym("*"); err != nil {
				return nil, err
			}
			return &sqlNode{op: "count"}, p.expectSym(")")
		}
		if p.acceptSym(".") { // table.column: the table is ignored
			c, err := p.ident("a column name")
			return &sqlNode{op: "col", col: c}, err
		}
		return &sqlNode{op: "col", col: t.s}, nil
	}
	p.i--
	return nil, p.fail("an expression")
}

// ---------------------------------------------------------------------------
// Evaluation

type sqlEnv struct {
	t    *memTable
	row  []any
	args []any
}

func sqlNum(v any) (float64, bool, bool) { // value, isInt, ok
	switch x := v.(type) {
	case int64:
		return float64(x), true, true
	case float64:
		return x, false, true
	}
	return 0, false, false
}

// sqlCmp orders two non NULL values.
func sqlCmp(a, b any) (int, error) {
	if x, ai, ok := sqlNum(a); ok {
		if y, bi, ok := sqlNum(b); ok {
			if ai && bi {
				return sqlCmpInt(a.(int64), b.(int64)), nil
			}
			switch {
			case x < y:
				return -1, nil
			case x > y:
				return 1, nil
			}
			return 0, nil
		}
	}
	switch x := a.(type) {
	case string:
		if y, ok := b.(string); ok {
			return strings.Compare(x, y), nil
		}
	case bool:
		if y, ok := b.(bool); ok {
			if x == y {
				return 0, nil
			}
			if !x {
				return -1, nil
			}
			return 1, nil
		}
	case Time:
		if y, ok := b.(Time); ok {
			return sqlCmpInt(int64(x), int64(y)), nil
		}
	}
	return 0, sqlErr(SqlDriver, "cannot compare %T with %T", a, b)
}

func sqlCmpInt(a, b int64) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

func sqlLike(s, pat string) bool {
	sr, pr := []rune(s), []rune(pat)
	var m func(i, j int) bool
	m = func(i, j int) bool {
		for j < len(pr) {
			switch pr[j] {
			case '%':
				for k := i; k <= len(sr); k++ {
					if m(k, j+1) {
						return true
					}
				}
				return false
			case '_':
				if i >= len(sr) {
					return false
				}
			default:
				if i >= len(sr) || unicode.ToLower(sr[i]) != unicode.ToLower(pr[j]) {
					return false
				}
			}
			i++
			j++
		}
		return i == len(sr)
	}
	return m(0, 0)
}

func (e *sqlEnv) eval(n *sqlNode) (any, error) {
	switch n.op {
	case "lit":
		return n.v, nil
	case "param":
		return e.args[n.v.(int64)-1], nil
	case "col":
		if e.t == nil {
			return nil, sqlErr(SqlSyntax, "no such column %s", n.col)
		}
		i := e.t.index(n.col)
		if i < 0 {
			return nil, sqlErr(SqlSyntax, "no such column %s", n.col)
		}
		return e.row[i], nil
	case "count":
		return nil, sqlErr(SqlSyntax, "COUNT(*) is only allowed in the select list")
	case "and", "or":
		l, err := e.eval(n.l)
		if err != nil {
			return nil, err
		}
		r, err := e.eval(n.r)
		if err != nil {
			return nil, err
		}
		lb, lok := l.(bool)
		rb, rok := r.(bool)
		if n.op == "and" {
			if (lok && !lb) || (rok && !rb) {
				return false, nil
			}
			if lok && rok {
				return true, nil
			}
			return nil, nil
		}
		if (lok && lb) || (rok && rb) {
			return true, nil
		}
		if lok && rok {
			return false, nil
		}
		return nil, nil
	case "not":
		v, err := e.eval(n.l)
		if b, ok := v.(bool); ok {
			return !b, err
		}
		return nil, err
	case "isnull", "notnull":
		v, err := e.eval(n.l)
		return (v == nil) == (n.op == "isnull"), err
	case "in":
		v, err := e.eval(n.l)
		if err != nil || v == nil {
			return nil, err
		}
		for _, x := range n.list {
			w, err := e.eval(x)
			if err != nil {
				return nil, err
			}
			if w == nil {
				continue
			}
			c, err := sqlCmp(v, w)
			if err != nil {
				return nil, err
			}
			if c == 0 {
				return true, nil
			}
		}
		return false, nil
	case "like":
		v, err := e.eval(n.l)
		if err != nil {
			return nil, err
		}
		pv, err := e.eval(n.r)
		if err != nil {
			return nil, err
		}
		if v == nil || pv == nil {
			return nil, nil
		}
		s, ok1 := v.(string)
		pat, ok2 := pv.(string)
		if !ok1 || !ok2 {
			return nil, sqlErr(SqlDriver, "LIKE needs text")
		}
		return sqlLike(s, pat), nil
	}
	l, err := e.eval(n.l)
	if err != nil {
		return nil, err
	}
	r, err := e.eval(n.r)
	if err != nil {
		return nil, err
	}
	if l == nil || r == nil {
		return nil, nil
	}
	switch n.op {
	case "=", "!=", "<", "<=", ">", ">=":
		c, err := sqlCmp(l, r)
		if err != nil {
			return nil, err
		}
		switch n.op {
		case "=":
			return c == 0, nil
		case "!=":
			return c != 0, nil
		case "<":
			return c < 0, nil
		case "<=":
			return c <= 0, nil
		case ">":
			return c > 0, nil
		}
		return c >= 0, nil
	}
	// arithmetic
	x, xi, ok1 := sqlNum(l)
	y, yi, ok2 := sqlNum(r)
	if !ok1 || !ok2 {
		return nil, sqlErr(SqlDriver, "arithmetic needs numbers")
	}
	if xi && yi {
		a, b := l.(int64), r.(int64)
		switch n.op {
		case "+":
			return a + b, nil
		case "-":
			return a - b, nil
		case "*":
			return a * b, nil
		}
		if b == 0 {
			return nil, sqlErr(SqlDriver, "division by zero")
		}
		return a / b, nil
	}
	switch n.op {
	case "+":
		return x + y, nil
	case "-":
		return x - y, nil
	case "*":
		return x * y, nil
	}
	if y == 0 {
		return nil, sqlErr(SqlDriver, "division by zero")
	}
	return x / y, nil
}

func (e *sqlEnv) where(n *sqlNode) (bool, error) {
	if n == nil {
		return true, nil
	}
	v, err := e.eval(n)
	if err != nil {
		return false, err
	}
	b, _ := v.(bool)
	return b, nil
}

// ---------------------------------------------------------------------------
// Statements

func checkParams(p *sqlParser, args []any) error {
	if int(p.maxParam) != len(args) {
		return sqlErr(SqlParam, "statement has %d placeholders but %d arguments were given", p.maxParam, len(args))
	}
	return nil
}

// splitStatements splits the tokens at ';'.
func splitStatements(toks []sqlTok) [][]sqlTok {
	var out [][]sqlTok
	var cur []sqlTok
	for _, t := range toks {
		if t.k == tEOF {
			break
		}
		if t.k == tSym && t.s == ";" {
			if len(cur) > 0 {
				out = append(out, append(cur, sqlTok{k: tEOF}))
			}
			cur = nil
			continue
		}
		cur = append(cur, t)
	}
	if len(cur) > 0 {
		out = append(out, append(cur, sqlTok{k: tEOF}))
	}
	return out
}

// memRun runs a read statement.
func memRun(tables map[string]*memTable, q string, args []any) ([]string, [][]any, error) {
	toks, err := sqlLex(q)
	if err != nil {
		return nil, nil, err
	}
	stmts := splitStatements(toks)
	if len(stmts) != 1 {
		return nil, nil, sqlErr(SqlSyntax, "expected one statement")
	}
	p := &sqlParser{toks: stmts[0]}
	if !p.isKw("select") {
		return nil, nil, sqlErr(SqlSyntax, "query needs a SELECT statement (use exec for writes)")
	}
	return memSelect(tables, p, args)
}

// memExec runs write statements on tables (a private copy of the map).
func memExec(tables map[string]*memTable, q string, args []any) (int64, int64, error) {
	toks, err := sqlLex(q)
	if err != nil {
		return 0, 0, err
	}
	stmts := splitStatements(toks)
	if len(stmts) == 0 {
		return 0, 0, sqlErr(SqlSyntax, "empty statement")
	}
	if len(stmts) > 1 && len(args) > 0 {
		return 0, 0, sqlErr(SqlParam, "several statements cannot take arguments")
	}
	var total, lastID int64
	for _, st := range stmts {
		p := &sqlParser{toks: st}
		var n, id int64
		switch {
		case p.isKw("create"):
			err = memCreate(tables, p)
		case p.isKw("drop"):
			err = memDrop(tables, p)
		case p.isKw("insert"):
			n, id, err = memInsert(tables, p, args)
		case p.isKw("update"):
			n, err = memUpdate(tables, p, args)
		case p.isKw("delete"):
			n, err = memDelete(tables, p, args)
		case p.isKw("select"):
			err = sqlErr(SqlSyntax, "exec cannot run a SELECT (use query)")
		default:
			err = p.fail("a statement")
		}
		if err == nil && p.peek().k != tEOF {
			err = p.fail("end of statement")
		}
		if err != nil {
			return 0, 0, err
		}
		total += n
		if id != 0 {
			lastID = id
		}
	}
	return total, lastID, nil
}

func memTableFor(tables map[string]*memTable, name string) (*memTable, error) {
	t, ok := tables[name]
	if !ok {
		return nil, sqlErr(SqlNoTable, "%s", name)
	}
	return t, nil
}

func memCreate(tables map[string]*memTable, p *sqlParser) error {
	p.i++
	if err := p.expectKw("table"); err != nil {
		return err
	}
	ifNot := false
	if p.acceptKw("if") {
		if err := p.expectKw("not"); err != nil {
			return err
		}
		if err := p.expectKw("exists"); err != nil {
			return err
		}
		ifNot = true
	}
	name, err := p.ident("a table name")
	if err != nil {
		return err
	}
	if _, ok := tables[name]; ok {
		if !ifNot {
			return sqlErr(SqlSyntax, "table %s already exists", name)
		}
		// consume the definition
		depth := 0
		for p.peek().k != tEOF {
			if p.isSym("(") {
				depth++
			} else if p.isSym(")") {
				depth--
			}
			p.i++
			if depth == 0 {
				break
			}
		}
		return nil
	}
	if err := p.expectSym("("); err != nil {
		return err
	}
	t := &memTable{name: name, nextID: 1}
	var pkCols []string
	for {
		if p.acceptKw("primary") {
			if err := p.expectKw("key"); err != nil {
				return err
			}
			if err := p.expectSym("("); err != nil {
				return err
			}
			for {
				c, err := p.ident("a column name")
				if err != nil {
					return err
				}
				pkCols = append(pkCols, c)
				if !p.acceptSym(",") {
					break
				}
			}
			if err := p.expectSym(")"); err != nil {
				return err
			}
		} else {
			c, err := memColDef(p)
			if err != nil {
				return err
			}
			if t.index(c.name) >= 0 {
				return sqlErr(SqlSyntax, "duplicate column %s", c.name)
			}
			t.cols = append(t.cols, c)
		}
		if !p.acceptSym(",") {
			break
		}
	}
	if err := p.expectSym(")"); err != nil {
		return err
	}
	for _, c := range pkCols {
		i := t.index(c)
		if i < 0 {
			return sqlErr(SqlSyntax, "no such column %s", c)
		}
		t.cols[i].pk = true
		t.cols[i].notNull = true
		if len(pkCols) == 1 {
			t.cols[i].unique = true
			t.cols[i].auto = t.cols[i].kind == 'i'
		}
	}
	if len(t.cols) == 0 {
		return sqlErr(SqlSyntax, "a table needs a column")
	}
	tables[name] = t
	return nil
}

func memColDef(p *sqlParser) (memCol, error) {
	var c memCol
	name, err := p.ident("a column name")
	if err != nil {
		return c, err
	}
	c.name = name
	ty, err := p.ident("a column type")
	if err != nil {
		return c, err
	}
	switch ty {
	case "int", "integer", "bigint", "smallint", "int64":
		c.kind = 'i'
	case "serial", "bigserial":
		c.kind, c.auto, c.pk, c.unique, c.notNull = 'i', true, true, true, true
	case "text", "varchar", "char", "string", "uuid":
		c.kind = 't'
	case "real", "float", "double", "numeric", "decimal":
		c.kind = 'r'
		p.acceptKw("precision")
	case "bool", "boolean":
		c.kind = 'b'
	case "timestamp", "timestamptz", "datetime", "time", "date":
		c.kind = 'T'
	default:
		return c, sqlErr(SqlSyntax, "unknown column type %s", ty)
	}
	if p.acceptSym("(") {
		for !p.isSym(")") && p.peek().k != tEOF {
			p.i++
		}
		if err := p.expectSym(")"); err != nil {
			return c, err
		}
	}
	for {
		switch {
		case p.acceptKw("primary"):
			if err := p.expectKw("key"); err != nil {
				return c, err
			}
			c.pk, c.unique, c.notNull = true, true, true
			c.auto = c.kind == 'i'
		case p.acceptKw("not"):
			if err := p.expectKw("null"); err != nil {
				return c, err
			}
			c.notNull = true
		case p.acceptKw("null"):
		case p.acceptKw("unique"):
			c.unique = true
		case p.acceptKw("autoincrement"):
			c.auto = true
		case p.acceptKw("default"):
			n, err := p.primary()
			if err != nil {
				return c, err
			}
			if n.op != "lit" {
				return c, sqlErr(SqlSyntax, "DEFAULT must be a literal")
			}
			c.def = n.v
		default:
			return c, nil
		}
	}
}

func memDrop(tables map[string]*memTable, p *sqlParser) error {
	p.i++
	if err := p.expectKw("table"); err != nil {
		return err
	}
	ifEx := false
	if p.acceptKw("if") {
		if err := p.expectKw("exists"); err != nil {
			return err
		}
		ifEx = true
	}
	name, err := p.ident("a table name")
	if err != nil {
		return err
	}
	if _, ok := tables[name]; !ok {
		if ifEx {
			return nil
		}
		return sqlErr(SqlNoTable, "%s", name)
	}
	delete(tables, name)
	return nil
}

// coerce converts v to the column kind.
func (c *memCol) coerce(v any) (any, error) {
	if v == nil {
		return nil, nil
	}
	switch c.kind {
	case 'i':
		if x, ok := v.(int64); ok {
			return x, nil
		}
	case 't':
		if s, ok := v.(string); ok {
			return s, nil
		}
	case 'r':
		switch x := v.(type) {
		case float64:
			return x, nil
		case int64:
			return float64(x), nil
		}
	case 'b':
		if b, ok := v.(bool); ok {
			return b, nil
		}
	case 'T':
		if t, ok := v.(Time); ok {
			return t, nil
		}
	}
	want := map[byte]string{'i': "an integer", 't': "text", 'r': "a float", 'b': "a boolean", 'T': "a time"}[c.kind]
	return nil, sqlErr(SqlConstraint, "column %s expects %s, got %T", c.name, want, v)
}

func (t *memTable) checkRow(row []any, skip int) error {
	for i, c := range t.cols {
		if row[i] == nil && c.notNull {
			return sqlErr(SqlConstraint, "NOT NULL constraint failed: %s.%s", t.name, c.name)
		}
		if c.unique && row[i] != nil {
			for j, o := range t.rows {
				if j == skip || o[i] == nil {
					continue
				}
				if cmp, err := sqlCmp(o[i], row[i]); err == nil && cmp == 0 {
					return sqlErr(SqlConstraint, "UNIQUE constraint failed: %s.%s", t.name, c.name)
				}
			}
		}
	}
	return nil
}

func memInsert(tables map[string]*memTable, p *sqlParser, args []any) (int64, int64, error) {
	p.i++
	if err := p.expectKw("into"); err != nil {
		return 0, 0, err
	}
	name, err := p.ident("a table name")
	if err != nil {
		return 0, 0, err
	}
	old, err := memTableFor(tables, name)
	if err != nil {
		return 0, 0, err
	}
	var cols []int
	if p.acceptSym("(") {
		for {
			c, err := p.ident("a column name")
			if err != nil {
				return 0, 0, err
			}
			i := old.index(c)
			if i < 0 {
				return 0, 0, sqlErr(SqlSyntax, "no such column %s", c)
			}
			cols = append(cols, i)
			if !p.acceptSym(",") {
				break
			}
		}
		if err := p.expectSym(")"); err != nil {
			return 0, 0, err
		}
	} else {
		for i := range old.cols {
			cols = append(cols, i)
		}
	}
	if err := p.expectKw("values"); err != nil {
		return 0, 0, err
	}
	var tuples [][]*sqlNode
	for {
		if err := p.expectSym("("); err != nil {
			return 0, 0, err
		}
		var tup []*sqlNode
		for {
			x, err := p.expr()
			if err != nil {
				return 0, 0, err
			}
			tup = append(tup, x)
			if !p.acceptSym(",") {
				break
			}
		}
		if err := p.expectSym(")"); err != nil {
			return 0, 0, err
		}
		if len(tup) != len(cols) {
			return 0, 0, sqlErr(SqlSyntax, "%d columns but %d values", len(cols), len(tup))
		}
		tuples = append(tuples, tup)
		if !p.acceptSym(",") {
			break
		}
	}
	if err := checkParams(p, args); err != nil {
		return 0, 0, err
	}
	t := old.clone()
	env := &sqlEnv{args: args}
	var lastID int64
	for _, tup := range tuples {
		row := make([]any, len(t.cols))
		for i, c := range t.cols {
			row[i] = c.def
		}
		for k, ci := range cols {
			v, err := env.eval(tup[k])
			if err != nil {
				return 0, 0, err
			}
			if row[ci], err = t.cols[ci].coerce(v); err != nil {
				return 0, 0, err
			}
		}
		for i, c := range t.cols {
			if !c.auto {
				continue
			}
			if row[i] == nil {
				row[i] = t.nextID
			}
			id := row[i].(int64)
			if id >= t.nextID {
				t.nextID = id + 1
			}
			lastID = id
		}
		if err := t.checkRow(row, -1); err != nil {
			return 0, 0, err
		}
		t.rows = append(t.rows, row)
	}
	tables[name] = t
	return int64(len(tuples)), lastID, nil
}

func memUpdate(tables map[string]*memTable, p *sqlParser, args []any) (int64, error) {
	p.i++
	name, err := p.ident("a table name")
	if err != nil {
		return 0, err
	}
	old, err := memTableFor(tables, name)
	if err != nil {
		return 0, err
	}
	if err := p.expectKw("set"); err != nil {
		return 0, err
	}
	type asg struct {
		col int
		e   *sqlNode
	}
	var asgs []asg
	for {
		c, err := p.ident("a column name")
		if err != nil {
			return 0, err
		}
		i := old.index(c)
		if i < 0 {
			return 0, sqlErr(SqlSyntax, "no such column %s", c)
		}
		if err := p.expectSym("="); err != nil {
			return 0, err
		}
		e, err := p.expr()
		if err != nil {
			return 0, err
		}
		asgs = append(asgs, asg{i, e})
		if !p.acceptSym(",") {
			break
		}
	}
	var w *sqlNode
	if p.acceptKw("where") {
		if w, err = p.expr(); err != nil {
			return 0, err
		}
	}
	if err := checkParams(p, args); err != nil {
		return 0, err
	}
	t := old.clone()
	var n int64
	for ri, row := range t.rows {
		env := &sqlEnv{t: t, row: row, args: args}
		ok, err := env.where(w)
		if err != nil {
			return 0, err
		}
		if !ok {
			continue
		}
		nr := append([]any(nil), row...)
		for _, a := range asgs {
			v, err := env.eval(a.e)
			if err != nil {
				return 0, err
			}
			if nr[a.col], err = t.cols[a.col].coerce(v); err != nil {
				return 0, err
			}
		}
		if err := t.checkRow(nr, ri); err != nil {
			return 0, err
		}
		t.rows[ri] = nr
		n++
	}
	tables[name] = t
	return n, nil
}

func memDelete(tables map[string]*memTable, p *sqlParser, args []any) (int64, error) {
	p.i++
	if err := p.expectKw("from"); err != nil {
		return 0, err
	}
	name, err := p.ident("a table name")
	if err != nil {
		return 0, err
	}
	old, err := memTableFor(tables, name)
	if err != nil {
		return 0, err
	}
	var w *sqlNode
	if p.acceptKw("where") {
		if w, err = p.expr(); err != nil {
			return 0, err
		}
	}
	if err := checkParams(p, args); err != nil {
		return 0, err
	}
	t := old.clone()
	t.rows = nil
	var n int64
	for _, row := range old.rows {
		env := &sqlEnv{t: old, row: row, args: args}
		ok, err := env.where(w)
		if err != nil {
			return 0, err
		}
		if ok {
			n++
		} else {
			t.rows = append(t.rows, row)
		}
	}
	tables[name] = t
	return n, nil
}

func memSelect(tables map[string]*memTable, p *sqlParser, args []any) ([]string, [][]any, error) {
	p.i++
	type item struct {
		star  bool
		e     *sqlNode
		alias string
	}
	var items []item
	for {
		if p.acceptSym("*") {
			items = append(items, item{star: true})
		} else {
			e, err := p.expr()
			if err != nil {
				return nil, nil, err
			}
			it := item{e: e}
			if p.acceptKw("as") {
				if it.alias, err = p.ident("an alias"); err != nil {
					return nil, nil, err
				}
			}
			items = append(items, it)
		}
		if !p.acceptSym(",") {
			break
		}
	}
	if err := p.expectKw("from"); err != nil {
		return nil, nil, err
	}
	name, err := p.ident("a table name")
	if err != nil {
		return nil, nil, err
	}
	t, err := memTableFor(tables, name)
	if err != nil {
		return nil, nil, err
	}
	var w *sqlNode
	if p.acceptKw("where") {
		if w, err = p.expr(); err != nil {
			return nil, nil, err
		}
	}
	type ord struct {
		e    *sqlNode
		desc bool
	}
	var orders []ord
	if p.acceptKw("order") {
		if err := p.expectKw("by"); err != nil {
			return nil, nil, err
		}
		for {
			e, err := p.expr()
			if err != nil {
				return nil, nil, err
			}
			o := ord{e: e}
			if p.acceptKw("desc") {
				o.desc = true
			} else {
				p.acceptKw("asc")
			}
			orders = append(orders, o)
			if !p.acceptSym(",") {
				break
			}
		}
	}
	var limit, offset *sqlNode
	if p.acceptKw("limit") {
		if limit, err = p.primary(); err != nil {
			return nil, nil, err
		}
		if p.acceptKw("offset") {
			if offset, err = p.primary(); err != nil {
				return nil, nil, err
			}
		}
	}
	if p.peek().k != tEOF {
		return nil, nil, p.fail("end of statement")
	}
	if err := checkParams(p, args); err != nil {
		return nil, nil, err
	}
	// filter
	var rows [][]any
	for _, row := range t.rows {
		env := &sqlEnv{t: t, row: row, args: args}
		ok, err := env.where(w)
		if err != nil {
			return nil, nil, err
		}
		if ok {
			rows = append(rows, row)
		}
	}
	// order
	if len(orders) > 0 {
		keys := make([][]any, len(rows))
		for i, row := range rows {
			env := &sqlEnv{t: t, row: row, args: args}
			for _, o := range orders {
				v, err := env.eval(o.e)
				if err != nil {
					return nil, nil, err
				}
				keys[i] = append(keys[i], v)
			}
		}
		idx := make([]int, len(rows))
		for i := range idx {
			idx[i] = i
		}
		var sortErr error
		sort.SliceStable(idx, func(a, b int) bool {
			for k, o := range orders {
				x, y := keys[idx[a]][k], keys[idx[b]][k]
				c := 0
				switch {
				case x == nil && y == nil:
				case x == nil:
					c = -1
				case y == nil:
					c = 1
				default:
					var err error
					if c, err = sqlCmp(x, y); err != nil {
						sortErr = err
					}
				}
				if c != 0 {
					if o.desc {
						return c > 0
					}
					return c < 0
				}
			}
			return false
		})
		if sortErr != nil {
			return nil, nil, sortErr
		}
		sorted := make([][]any, len(rows))
		for i, j := range idx {
			sorted[i] = rows[j]
		}
		rows = sorted
	}
	// offset, limit
	intArg := func(n *sqlNode) (int, error) {
		v, err := (&sqlEnv{args: args}).eval(n)
		x, ok := v.(int64)
		if err != nil || !ok || x < 0 {
			return 0, sqlErr(SqlSyntax, "LIMIT and OFFSET need a non negative integer")
		}
		return int(x), nil
	}
	if offset != nil {
		o, err := intArg(offset)
		if err != nil {
			return nil, nil, err
		}
		if o > len(rows) {
			o = len(rows)
		}
		rows = rows[o:]
	}
	if limit != nil {
		l, err := intArg(limit)
		if err != nil {
			return nil, nil, err
		}
		if l < len(rows) {
			rows = rows[:l]
		}
	}
	// project
	var cols []string
	counts := 0
	for _, it := range items {
		switch {
		case it.star:
			for _, c := range t.cols {
				cols = append(cols, c.name)
			}
		case it.alias != "":
			cols = append(cols, it.alias)
			if it.e.op == "count" {
				counts++
			}
		case it.e.op == "col":
			cols = append(cols, it.e.col)
		case it.e.op == "count":
			cols = append(cols, "count")
			counts++
		default:
			cols = append(cols, "?column?")
		}
	}
	if counts > 0 {
		if counts != len(items) {
			return nil, nil, sqlErr(SqlSyntax, "COUNT(*) cannot be mixed with other columns")
		}
		out := make([]any, len(items))
		for i := range out {
			out[i] = int64(len(rows))
		}
		return cols, [][]any{out}, nil
	}
	out := make([][]any, 0, len(rows))
	for _, row := range rows {
		env := &sqlEnv{t: t, row: row, args: args}
		var r []any
		for _, it := range items {
			if it.star {
				r = append(r, row...)
				continue
			}
			v, err := env.eval(it.e)
			if err != nil {
				return nil, nil, err
			}
			r = append(r, v)
		}
		out = append(out, r)
	}
	return cols, out, nil
}

// memSchema builds a database from the statements of a schema.
func memSchema(schema string) (*memBackend, error) {
	b := newMemBackend()
	if strings.TrimSpace(schema) == "" {
		return b, nil
	}
	if _, _, err := memExec(b.db.tables, schema, nil); err != nil {
		return nil, err
	}
	return b, nil
}
