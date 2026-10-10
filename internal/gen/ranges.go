package gen

// Range analysis for checked Int arithmetic (core 2.1, 8).
//
// Every Int `+ - * / %` and unary minus faults on overflow (and `/`, `%`
// on division by zero) with the operands and the position. Before the
// body of a function is generated, an abstract interpretation over its Xo
// syntax tree computes an interval [lo, hi] for every Int value and
// binding, and marks each operation it proves cannot fault. The generator
// emits plain Go arithmetic for marked operations and the checked runtime
// helpers for every other one. An operation is marked only when every
// visit of it during the analysis proves it (loops are iterated to a fixed
// point, so the last visit sees the widest intervals), and an operation the
// analysis never visits (dead code, constructs it does not model) is never
// marked.
//
// Facts used:
//   - constants: literals, `const` declarations (global and local);
//   - bindings: `let` keeps the interval of its value, `var` the join of
//     every value assigned to it; a `var` that a closure or `defer` assigns,
//     or that is passed as a `var` argument or receiver, is not tracked
//     (its interval is the whole Int range everywhere);
//   - loops: `for i in a..b` binds i to [a.lo, b.hi - 1] (`..=`: b.hi);
//     `while` and `for` bodies are iterated until the bindings at the loop
//     head stop growing, widening bounds to thresholds (powers of two)
//     after two rounds;
//   - conditions: in the branch where `x < e` (`<=`, `>`, `>=`, `==`,
//     `!=`, `!`, `&&`, `||`) holds, x is narrowed by the interval of e;
//   - results: `%` by b lies within (-|b|, |b|) and has the sign of the
//     dividend; `&` with a non negative operand lies in [0, that operand];
//     `.len()` of a List, Str, Bytes, Map, or Set lies in [0, 2^56] (a
//     collection of elements that are not zero sized cannot have more
//     elements than the address space has bytes; a List whose element type
//     may be zero sized gets [0, max Int]);
//   - widening: a value of a sized integer type (Int32, UInt16, ...)
//     converted to Int keeps the range of its type.
//
// Operands that are a single value (a constant) also select cheaper
// checked forms in binop (rt.AddIP, rt.MulIK, plain `/` and `%` by a
// constant other than 0 and -1).

import (
	"math"
	"math/bits"
	"strconv"

	"github.com/xo-lang/xo/internal/ast"
	"github.com/xo-lang/xo/internal/check"
	"github.com/xo-lang/xo/internal/token"
)

// ival is a closed interval of int64 values, with an optional symbolic
// upper bound relative to the length of a list (sym).
type ival struct {
	lo, hi int64
	sym    *symb
}

func iv(lo, hi int64) ival { return ival{lo: lo, hi: hi} }

// symb is the bound `value <= m * len(L) + off` where L is the List at
// path below the binding root, a binding that cannot change while it is in
// scope (a parameter, or a `let` or `for` binding, whose facts are dropped
// when it is bound again).
type symb struct {
	root *check.VarObj
	path string
	m    int64
	off  int64
}

func (s *symb) same(t *symb) bool {
	return s != nil && t != nil && s.root == t.root && s.path == t.path && s.m == t.m
}

// maxSymOff is the largest offset kept: larger ones prove nothing useful.
const maxSymOff = 1 << 40

func mkSym(base *symb, m, off int64) *symb {
	if base == nil || m < 1 || m > 8 || off > maxSymOff || off < -maxSymOff {
		return nil
	}
	return &symb{root: base.root, path: base.path, m: m, off: off}
}

// symOf returns the symbolic bound of a (or, when a has none, the bound
// implied by its interval for the list of like: v <= hi <= m*len + hi).
func symOf(a ival, like *symb) *symb {
	if a.sym != nil {
		return a.sym
	}
	if like == nil || a.hi > maxSymOff {
		return nil
	}
	return mkSym(like, like.m, a.hi)
}

var fullRange = iv(math.MinInt64, math.MaxInt64)

// maxLen bounds the length of a collection whose elements are not zero
// sized (see the comment at the top of the file).
const maxLen = 1 << 56

func pointIval(k int64) ival { return iv(k, k) }

func (a ival) join(b ival) ival {
	r := iv(min(a.lo, b.lo), max(a.hi, b.hi))
	sa, sb := symOf(a, b.sym), symOf(b, a.sym)
	if sa.same(sb) {
		r.sym = mkSym(sa, sa.m, max(sa.off, sb.off))
	}
	return r
}

// within reports whether every value a allows is allowed by b.
func (a ival) within(b ival) bool {
	if a.lo < b.lo || a.hi > b.hi {
		return false
	}
	if b.sym == nil {
		return true
	}
	if b.sym.same(a.sym) && a.sym.off <= b.sym.off {
		return true
	}
	return a.hi <= b.sym.off
}

func add64(a, b int64) (int64, bool) {
	c := a + b
	return c, (c > a) == (b > 0)
}

func sub64(a, b int64) (int64, bool) {
	c := a - b
	return c, (c < a) == (b > 0)
}

func mul64(a, b int64) (int64, bool) {
	hi, lo := bits.Mul64(uint64(a), uint64(b))
	return int64(lo), int64(hi)-(a>>63)&b-(b>>63)&a == int64(lo)>>63
}

// satLo and satHi saturate a bound computed with overflow: a checked
// operation that returns produced the exact value, which lies in the
// interval clamped to Int.
func satLo(v int64, ok bool) int64 {
	if ok {
		return v
	}
	return math.MinInt64
}

func satHi(v int64, ok bool) int64 {
	if ok {
		return v
	}
	return math.MaxInt64
}

// arith returns the interval of `a op b` and whether the operation cannot
// fault (core 2.1: overflow and division by zero fault), with the symbolic
// bound of the result. A checked operation that returns produced the exact
// result, so the bounds hold whether or not it can fault.
func arith(op string, a, b ival) (ival, bool) {
	r, ok := arith1(op, a, b)
	r.sym = symArith(op, a, b)
	return r, ok
}

// symArith derives `result <= m*len(L) + off` from the operands' bounds.
func symArith(op string, a, b ival) *symb {
	switch op {
	case "+":
		switch {
		case a.sym != nil && b.sym != nil:
			if a.sym.root == b.sym.root && a.sym.path == b.sym.path {
				return mkSym(a.sym, a.sym.m+b.sym.m, a.sym.off+b.sym.off)
			}
		case a.sym != nil && b.hi <= maxSymOff:
			return mkSym(a.sym, a.sym.m, a.sym.off+b.hi)
		case b.sym != nil && a.hi <= maxSymOff:
			return mkSym(b.sym, b.sym.m, b.sym.off+a.hi)
		}
	case "-":
		if a.sym != nil && b.lo >= -maxSymOff {
			return mkSym(a.sym, a.sym.m, a.sym.off-b.lo)
		}
	case "/":
		// a >= 0 and k >= 1: a / k <= a, and when k divides m,
		// a / k <= (m/k)*len + floor(off / k)
		if a.sym != nil && a.lo >= 0 && b.lo == b.hi && b.lo >= 1 {
			k := b.lo
			if a.sym.m%k == 0 {
				off := a.sym.off / k
				if a.sym.off%k != 0 && a.sym.off < 0 {
					off-- // floor
				}
				return mkSym(a.sym, a.sym.m/k, off)
			}
			return a.sym
		}
	}
	return nil
}

func arith1(op string, a, b ival) (ival, bool) {
	switch op {
	case "+":
		lo, ok1 := add64(a.lo, b.lo)
		hi, ok2 := add64(a.hi, b.hi)
		return iv(satLo(lo, ok1), satHi(hi, ok2)), ok1 && ok2
	case "-":
		lo, ok1 := sub64(a.lo, b.hi)
		hi, ok2 := sub64(a.hi, b.lo)
		return iv(satLo(lo, ok1), satHi(hi, ok2)), ok1 && ok2
	case "*":
		r := iv(math.MaxInt64, math.MinInt64)
		for _, x := range []int64{a.lo, a.hi} {
			for _, y := range []int64{b.lo, b.hi} {
				p, ok := mul64(x, y)
				if !ok {
					return fullRange, false
				}
				r.lo, r.hi = min(r.lo, p), max(r.hi, p)
			}
		}
		return r, true
	case "/":
		if b.lo <= 0 && b.hi >= 0 {
			return fullRange, false
		}
		if a.lo == math.MinInt64 && b.lo <= -1 && b.hi >= -1 {
			return fullRange, false
		}
		// b has one sign: the quotient is monotone in each operand
		r := iv(math.MaxInt64, math.MinInt64)
		for _, x := range []int64{a.lo, a.hi} {
			for _, y := range []int64{b.lo, b.hi} {
				q := x / y
				r.lo, r.hi = min(r.lo, q), max(r.hi, q)
			}
		}
		return r, true
	case "%":
		m := max(absSat(b.lo), absSat(b.hi)) - 1 // |a % b| <= |b| - 1
		if m < 0 {
			m = 0
		}
		r := iv(max(a.lo, -m), min(a.hi, m))
		if a.lo >= 0 {
			r.lo = 0
		}
		if a.hi <= 0 {
			r.hi = 0
		}
		return r, b.lo > 0 || b.hi < 0
	case "&":
		switch {
		case a.lo >= 0 && b.lo >= 0:
			return iv(0, min(a.hi, b.hi)), true
		case a.lo >= 0:
			return iv(0, a.hi), true
		case b.lo >= 0:
			return iv(0, b.hi), true
		}
		return fullRange, true
	case "|", "^":
		if a.lo >= 0 && b.lo >= 0 {
			n := bits.Len64(uint64(max(a.hi, b.hi)))
			return iv(0, int64(uint64(1)<<n-1)), true
		}
		return fullRange, true
	case ">>":
		// rt.Shr faults only for a shift amount out of [0, 63]
		ok := b.lo >= 0 && b.hi <= 63
		if ok && a.lo >= 0 {
			return iv(a.lo>>uint(b.hi), a.hi>>uint(b.lo)), true
		}
		return fullRange, ok
	}
	return fullRange, false
}

func absSat(x int64) int64 {
	if x == math.MinInt64 {
		return math.MaxInt64
	}
	if x < 0 {
		return -x
	}
	return x
}

// thresholds are the bounds widening jumps to.
var thresholds = func() []int64 {
	pos := []int64{0, 1, 1 << 8, 1 << 16, 1 << 31, 1 << 32, 1 << 48, maxLen, 1 << 57, 1 << 62, math.MaxInt64}
	var out []int64
	for i := len(pos) - 1; i > 0; i-- {
		if pos[i] == math.MaxInt64 {
			out = append(out, math.MinInt64)
			continue
		}
		out = append(out, -pos[i])
	}
	return append(out, pos...)
}()

func widenUp(x int64) int64 {
	for _, t := range thresholds {
		if t >= x {
			return t
		}
	}
	return math.MaxInt64
}

func widenDown(x int64) int64 {
	for i := len(thresholds) - 1; i >= 0; i-- {
		if thresholds[i] <= x {
			return thresholds[i]
		}
	}
	return math.MinInt64
}

// renv maps tracked Int bindings to their intervals at a program point. A
// binding that is not in m has the full range of its type; dead marks an
// unreachable point.
type renv struct {
	m    map[*check.VarObj]ival
	dead bool
}

func deadEnv() renv { return renv{dead: true} }

func (e renv) clone() renv {
	if e.dead {
		return e
	}
	m := make(map[*check.VarObj]ival, len(e.m))
	for k, v := range e.m {
		m[k] = v
	}
	return renv{m: m}
}

func joinEnv(a, b renv) renv {
	if a.dead {
		return b.clone()
	}
	if b.dead {
		return a.clone()
	}
	m := map[*check.VarObj]ival{}
	for k, x := range a.m {
		if y, ok := b.m[k]; ok {
			m[k] = x.join(y)
		}
	}
	return renv{m: m}
}

// leqEnv reports whether every state of a is a state of b.
func leqEnv(a, b renv) bool {
	if a.dead {
		return true
	}
	if b.dead {
		return false
	}
	for k, y := range b.m {
		x, ok := a.m[k]
		if !ok || !x.within(y) {
			return false
		}
	}
	return true
}

// widenEnv widens old toward cur, which contains old.
func widenEnv(old, cur renv) renv {
	if old.dead || cur.dead {
		return cur
	}
	m := map[*check.VarObj]ival{}
	for k, y := range cur.m {
		x, ok := old.m[k]
		if !ok {
			continue
		}
		if y.lo < x.lo {
			y.lo = widenDown(y.lo)
		}
		if y.hi > x.hi {
			y.hi = widenUp(y.hi)
		}
		if y.sym != nil && !(x.sym.same(y.sym) && y.sym.off <= x.sym.off) {
			y.sym = nil
		}
		m[k] = y
	}
	return renv{m: m}
}

// rloop collects the states at `break` and `continue` of one loop.
type rloop struct {
	label        string
	breaks, cont []renv
}

// pointRec records the value of an operand that was the same single value
// at every visit.
type pointRec struct {
	v  int64
	ok bool
}

// rangeAn is the analysis of one function body.
type rangeAn struct {
	f        *fnGen
	env      renv
	volatile map[*check.VarObj]bool
	// stable are the bindings a symbolic list bound may be rooted at
	stable map[*check.VarObj]bool
	safe   map[ast.Node]bool
	points map[ast.Expr]pointRec
	loops  []*rloop
	busy   map[*check.ConstObj]bool
}

// maxLoopRounds bounds the iterations of one loop before its bindings are
// dropped (full range), which always reaches a fixed point.
const maxLoopRounds = 24

// analyzeRanges runs the analysis on body (a function, method, or test
// body) and keeps the results in f.rng. params are the bindings of the
// parameters and receiver that are not `var`; varParams the `var` ones,
// which may alias each other (or a caller's binding) and are not tracked.
func (f *fnGen) analyzeRanges(body *ast.Block, params, varParams []*check.VarObj) {
	if body == nil || f.g.opts.NoRanges {
		return
	}
	a := &rangeAn{f: f, env: renv{m: map[*check.VarObj]ival{}}, volatile: map[*check.VarObj]bool{},
		stable: map[*check.VarObj]bool{}, safe: map[ast.Node]bool{}, points: map[ast.Expr]pointRec{},
		busy: map[*check.ConstObj]bool{}}
	for _, v := range params {
		if v != nil && !v.Mutable {
			a.stable[v] = true
		}
	}
	for _, v := range varParams {
		if v != nil {
			a.volatile[v] = true
		}
	}
	inspectNode(body, func(n ast.Node) bool {
		var p ast.Pattern
		switch x := n.(type) {
		case *ast.LetStmt:
			p = x.Pattern
		case *ast.ForStmt:
			p = x.Pattern
		}
		if bp, ok := p.(*ast.BindPat); ok {
			if v := f.info.VarOf(bp.Name); v != nil && !v.Mutable {
				a.stable[v] = true
			}
		}
		return true
	})
	a.findVolatile(body)
	a.block(body)
	f.rng = a
}

// safeOp reports whether the analysis proved that the operation at n
// cannot fault.
func (f *fnGen) safeOp(n ast.Node) bool {
	return f.rng != nil && f.rng.safe[n]
}

// constOperand returns the value of operand e when it was the same single
// value at every visit.
func (f *fnGen) constOperand(e ast.Expr) (int64, bool) {
	if f.rng == nil || e == nil {
		return 0, false
	}
	p, ok := f.rng.points[e]
	return p.v, ok && p.ok
}

func (a *rangeAn) mark(n ast.Node, ok bool) {
	if a.env.dead {
		return
	}
	if prev, seen := a.safe[n]; seen {
		ok = ok && prev
	}
	a.safe[n] = ok
}

func (a *rangeAn) notePoint(e ast.Expr, r ival) {
	if a.env.dead {
		return
	}
	single := r.lo == r.hi
	if prev, seen := a.points[e]; seen {
		a.points[e] = pointRec{v: prev.v, ok: prev.ok && single && prev.v == r.lo}
		return
	}
	a.points[e] = pointRec{v: r.lo, ok: single}
}

// ---------------------------------------------------------------------------
// Types

// isInt reports whether values of t are Go int64 integers (Int, Int64):
// the type the typed helpers and plain arithmetic cover.
func (a *rangeAn) isInt(t check.Type) bool {
	if t == nil {
		return false
	}
	t = a.f.pg.monoType(check.Deep(t))
	if _, ok := check.Resolve(t).(*check.ParamType); ok {
		return false
	}
	return a.f.goType(t) == "int64" && !a.f.g.isFloat(t)
}

// typeRange is the range of every value of type t (as an Int).
func (a *rangeAn) typeRange(t check.Type) ival {
	if t == nil {
		return fullRange
	}
	t = a.f.pg.monoType(check.Deep(t))
	if _, ok := check.Resolve(t).(*check.ParamType); ok {
		return fullRange
	}
	switch a.f.goType(t) {
	case "int8":
		return iv(math.MinInt8, math.MaxInt8)
	case "int16":
		return iv(math.MinInt16, math.MaxInt16)
	case "int32":
		return iv(math.MinInt32, math.MaxInt32)
	case "uint8":
		return iv(0, math.MaxUint8)
	case "uint16":
		return iv(0, math.MaxUint16)
	case "uint32":
		return iv(0, math.MaxUint32)
	case "uint64":
		// only used as the source of a conversion to Int, which faults
		// above max Int
		return iv(0, math.MaxInt64)
	}
	return fullRange
}

// nonZeroSize reports whether values of t certainly occupy memory, so a
// collection of them is bounded by the address space.
func (a *rangeAn) nonZeroSize(t check.Type, depth int) bool {
	if t == nil || depth > 8 {
		return false
	}
	t = a.f.pg.monoType(check.Deep(t))
	switch x := check.Resolve(t).(type) {
	case *check.Func:
		return true
	case *check.Tuple:
		for _, e := range x.Elems {
			if a.nonZeroSize(e, depth+1) {
				return true
			}
		}
		return false
	case *check.Named:
		o := x.Obj
		if o.Module.IsPrelude {
			switch o.Name {
			case "Never":
				return false
			}
			// scalars, Str, collections, Option, Result, handles
			return true
		}
		switch o.Kind {
		case check.KStruct:
			for _, fd := range o.Fields {
				ft := fd.Type
				if len(x.Args) > 0 {
					ft = check.NewSubst(o.TParams, x.Args).Apply(ft)
				}
				if a.nonZeroSize(ft, depth+1) {
					return true
				}
			}
			return false
		case check.KNewtype:
			return a.nonZeroSize(o.Target, depth+1)
		case check.KEnum, check.KInterface:
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Bindings

func (a *rangeAn) lookup(v *check.VarObj) ival {
	if a.volatile[v] || a.env.dead {
		return a.typeRange(v.Type)
	}
	if r, ok := a.env.m[v]; ok {
		return r
	}
	return a.typeRange(v.Type)
}

// rebind drops the symbolic bounds on lists under v, which is about to be
// bound to a new value (a `let` or `for` binding run again in a loop).
func (a *rangeAn) rebind(v *check.VarObj) {
	if v == nil || a.env.dead {
		return
	}
	for k, r := range a.env.m {
		if r.sym != nil && r.sym.root == v {
			r.sym = nil
			a.env.m[k] = r
		}
	}
}

// stableKey returns the binding and field path of a list expression that
// cannot change while the binding is in scope: a field chain under a
// parameter or a `let` or `for` binding (value semantics: fields of an
// immutable binding are immutable).
func (a *rangeAn) stableKey(e ast.Expr) (*check.VarObj, string, bool) {
	path := ""
	for {
		switch x := unparen(e).(type) {
		case *ast.Ident:
			v := a.f.info.VarOf(x)
			if v == nil || !a.stable[v] || v.Mutable || a.volatile[v] {
				return nil, "", false
			}
			return v, path, true
		case *ast.SelectorExpr:
			if a.f.info.Ref(x) != nil || a.f.info.Ctor(x) != nil || a.f.info.VarOf(x.Sel) != nil {
				return nil, "", false
			}
			path = "." + x.Sel.Name + path
			e = x.X
		default:
			return nil, "", false
		}
	}
}

func (a *rangeAn) bind(v *check.VarObj, r ival) {
	if v == nil || a.env.dead || a.volatile[v] || !a.isInt(v.Type) {
		return
	}
	a.env.m[v] = r
}

// tracked returns the binding an identifier expression names when the
// analysis tracks it.
func (a *rangeAn) tracked(e ast.Expr) *check.VarObj {
	e = unparen(e)
	id, ok := e.(*ast.Ident)
	if !ok {
		return nil
	}
	v := a.f.info.VarOf(id)
	if v == nil || a.volatile[v] || !a.isInt(v.Type) {
		return nil
	}
	return v
}

func unparen(e ast.Expr) ast.Expr {
	for {
		p, ok := e.(*ast.ParenExpr)
		if !ok {
			return e
		}
		e = p.X
	}
}

// rootVar is the binding at the root of a place (`x`, `x.f`, `x[i]`).
func (a *rangeAn) rootVar(e ast.Expr) *check.VarObj {
	for {
		switch x := e.(type) {
		case *ast.ParenExpr:
			e = x.X
		case *ast.SelectorExpr:
			e = x.X
		case *ast.IndexExpr:
			e = x.X
		case *ast.Ident:
			return a.f.info.VarOf(x)
		default:
			return nil
		}
	}
}

// findVolatile marks the bindings the analysis cannot follow: assigned in
// a closure or `defer` body declared outside it (the body runs at an
// unknown time), or passed as a `var` argument or receiver.
func (a *rangeAn) findVolatile(body *ast.Block) {
	outside := func(n ast.Node, v *check.VarObj) bool {
		sp, vs := n.Span(), v.Span
		return vs.File != sp.File || vs.Start.Offset < sp.Start.Offset || vs.Start.Offset >= sp.End.Offset
	}
	deferred := func(n ast.Node) {
		inspectNode(n, func(c ast.Node) bool {
			if as, ok := c.(*ast.AssignStmt); ok {
				if v := a.rootVar(as.Target); v != nil && outside(n, v) {
					a.volatile[v] = true
				}
			}
			return true
		})
	}
	inspectNode(body, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.FnLit:
			deferred(x)
			// `var` parameters of a closure may alias
			vs := a.f.info.ClosureParams(x)
			for i, p := range x.Params {
				if p.Var && i < len(vs) && vs[i] != nil {
					a.volatile[vs[i]] = true
				}
			}
		case *ast.DeferStmt:
			deferred(x)
		case *ast.CallExpr:
			a.varArgs(x)
		}
		return true
	})
}

func (a *rangeAn) varArgs(x *ast.CallExpr) {
	vol := func(e ast.Expr) {
		if v := a.rootVar(e); v != nil {
			a.volatile[v] = true
		}
	}
	if ci := a.f.info.CallOf(x); ci != nil && ci.Fn != nil {
		for i, e := range argExprs(ci.Fn.Params, x) {
			if e != nil && ci.Fn.Params[i].Var {
				vol(e)
			}
		}
		if ci.Fn.RecvVar && ci.Recv != nil {
			vol(ci.Recv)
		}
		return
	}
	if ft, ok := check.Resolve(a.f.typeOf(x.Fun)).(*check.Func); ok {
		for i, p := range ft.Params {
			if p.Var && i < len(x.Args) {
				vol(x.Args[i].Value)
			}
		}
		return
	}
	// a call the analysis cannot resolve: any binding passed may change
	for _, arg := range x.Args {
		vol(arg.Value)
	}
}

// havoc steps over a construct the analysis does not model: bindings it
// assigns lose their intervals, and a `break` or `continue` inside may
// leave any enclosing loop with the current state.
func (a *rangeAn) havoc(n ast.Node) {
	if a.env.dead {
		return
	}
	jumps := false
	inspectNode(n, func(c ast.Node) bool {
		switch x := c.(type) {
		case *ast.AssignStmt:
			if v := a.rootVar(x.Target); v != nil {
				delete(a.env.m, v)
			}
		case *ast.BreakStmt, *ast.ContinueStmt:
			jumps = true
		}
		return true
	})
	if jumps {
		for _, l := range a.loops {
			l.breaks = append(l.breaks, a.env.clone())
			l.cont = append(l.cont, a.env.clone())
		}
	}
}

// ---------------------------------------------------------------------------
// Expressions

// expr evaluates e: it records the operations e performs and returns the
// interval of its value (meaningful for integer values).
func (a *rangeAn) expr(e ast.Expr) ival {
	if e == nil {
		return fullRange
	}
	r := a.expr1(e)
	if !a.isInt(a.f.nat(e)) {
		return a.typeRange(a.f.nat(e))
	}
	return r
}

func (a *rangeAn) expr1(e ast.Expr) ival {
	f := a.f
	switch x := e.(type) {
	case *ast.IntLit:
		if v, err := strconv.ParseInt(x.Value, 10, 64); err == nil {
			return pointIval(v)
		}
		return fullRange
	case *ast.ParenExpr:
		return a.expr(x.X)
	case *ast.Ident:
		if v := f.info.VarOf(x); v != nil {
			return a.lookup(v)
		}
		if co, ok := f.info.Ref(x).(*check.ConstObj); ok {
			return a.constRange(co)
		}
		return fullRange
	case *ast.UnaryExpr:
		r := a.expr(x.X)
		if x.Op == token.Minus && a.isInt(f.nat(x)) {
			res, ok := arith("-", pointIval(0), r)
			if _, lit := x.X.(*ast.IntLit); !lit {
				a.mark(x, ok)
			}
			return res
		}
		return fullRange
	case *ast.BinaryExpr:
		return a.binary(x)
	case *ast.CallExpr:
		return a.call(x)
	case *ast.IfExpr:
		return a.ifExpr(x)
	case *ast.Block:
		return a.block(x)
	case *ast.MatchExpr:
		return a.match(x)
	case *ast.WhileExpr:
		a.loop("", nil, x.Body, false, nil, fullRange)
		return fullRange
	case *ast.ScopeExpr:
		return a.block(x.Body)
	case *ast.FnLit:
		a.closure(x.Body)
		return fullRange
	case *ast.SelectorExpr:
		if co, ok := f.info.Ref(x).(*check.ConstObj); ok {
			return a.constRange(co)
		}
		if f.info.VarOf(rootIdent(x)) != nil || !isPath(x) {
			a.expr(x.X)
		}
		return fullRange
	case *ast.TryExpr:
		a.expr(x.X)
	case *ast.IndexExpr:
		a.expr(x.X)
		idx := a.expr(x.Index)
		if n := f.g.named(f.typeOf(x.X)); n != nil && n.Obj.Module.IsPrelude && n.Obj.Name == "List" {
			if _, isRange := x.Index.(*ast.RangeExpr); !isRange {
				root, path, ok := a.stableKey(x.X)
				s := idx.sym
				a.mark(x, ok && idx.lo >= 0 && s != nil && s.root == root && s.path == path && s.m == 1 && s.off <= -1)
			}
		}
	case *ast.RangeExpr:
		a.expr(x.Lo)
		a.expr(x.Hi)
	case *ast.TupleExpr:
		for _, el := range x.Elems {
			a.expr(el)
		}
	case *ast.ListLit:
		for _, el := range x.Elems {
			a.expr(el)
		}
	case *ast.SetLit:
		for _, el := range x.Elems {
			a.expr(el)
		}
	case *ast.MapLit:
		for _, en := range x.Entries {
			a.expr(en.Key)
			a.expr(en.Value)
		}
	case *ast.StructLit:
		for _, fi := range x.Fields {
			a.expr(fi.Value)
		}
	case *ast.WithExpr:
		a.expr(x.X)
		for _, fi := range x.Fields {
			a.expr(fi.Value)
		}
	case *ast.StringLit:
		for _, p := range x.Parts {
			if p.Expr != nil {
				a.expr(p.Expr)
			}
		}
	case *ast.IsExpr:
		a.expr(x.X)
	case *ast.TypeArgsExpr:
	case *ast.FloatLit, *ast.DurationLit, *ast.RuneLit, *ast.BytesLit, *ast.UnitExpr, *ast.HoleExpr:
	default:
		a.havoc(e)
	}
	return fullRange
}

// rootIdent is the identifier at the left end of a selector chain.
func rootIdent(e ast.Expr) *ast.Ident {
	for {
		switch x := e.(type) {
		case *ast.SelectorExpr:
			e = x.X
		case *ast.Ident:
			return x
		default:
			return nil
		}
	}
}

// isPath reports whether a selector chain is made of identifiers only (a
// module or type path, or a field chain of a binding).
func isPath(e ast.Expr) bool {
	return rootIdent(e) != nil
}

func (a *rangeAn) constRange(co *check.ConstObj) ival {
	if co.Decl == nil || co.Decl.Value == nil || a.busy[co] || !a.isInt(co.Type) {
		return a.typeRange(co.Type)
	}
	a.busy[co] = true
	defer delete(a.busy, co)
	// a constant initializer is a pure constant expression: evaluate it
	// without recording operations of this function
	saved, savedSafe, savedPoints := a.env, a.safe, a.points
	a.env = renv{m: map[*check.VarObj]ival{}}
	a.safe, a.points = map[ast.Node]bool{}, map[ast.Expr]pointRec{}
	r := a.expr(co.Decl.Value)
	a.env, a.safe, a.points = saved, savedSafe, savedPoints
	return r
}

var arithOps = map[token.Kind]string{
	token.Plus: "+", token.Minus: "-", token.Star: "*", token.Slash: "/", token.Percent: "%",
	token.Amp: "&", token.Pipe: "|", token.Caret: "^", token.Shr: ">>", token.Shl: "<<",
}

func (a *rangeAn) binary(x *ast.BinaryExpr) ival {
	switch x.Op {
	case token.AndAnd, token.OrOr:
		a.expr(x.X)
		t, fl := a.refine(a.env, x.X)
		if x.Op == token.AndAnd {
			a.env = t
			a.expr(x.Y)
			a.env = joinEnv(a.env, fl)
		} else {
			a.env = fl
			a.expr(x.Y)
			a.env = joinEnv(a.env, t)
		}
		return fullRange
	case token.Coalesce:
		a.expr(x.X)
		before := a.env.clone()
		r := a.expr(x.Y)
		a.env = joinEnv(a.env, before)
		return r.join(a.typeRange(a.f.nat(x)))
	}
	ra := a.expr(x.X)
	rb := a.expr(x.Y)
	op, ok := arithOps[x.Op]
	if !ok {
		return fullRange
	}
	if !a.isInt(a.f.nat(x)) || !a.isInt(a.f.nat(x.X)) || !a.isInt(a.f.nat(x.Y)) {
		return fullRange
	}
	res, safe := arith(op, ra, rb)
	a.mark(x, safe)
	a.notePoint(x.X, ra)
	a.notePoint(x.Y, rb)
	return res
}

func (a *rangeAn) call(x *ast.CallExpr) ival {
	f := a.f
	ci := f.info.CallOf(x)
	if obj := f.info.TypeCall(x); obj != nil {
		var r ival = fullRange
		for i, arg := range x.Args {
			v := a.expr(arg.Value)
			if i == 0 && len(x.Args) == 1 {
				r = v
			}
		}
		if len(x.Args) == 1 && a.isInt(f.nat(x)) && isIntegerType(f, f.nat(x.Args[0].Value)) {
			// a conversion between integer types keeps the value or faults
			return r
		}
		return a.typeRange(f.nat(x))
	}
	if ci != nil && ci.Recv != nil {
		a.expr(ci.Recv)
	} else if sel, ok := x.Fun.(*ast.SelectorExpr); ok {
		if !isPath(sel) {
			a.expr(sel.X)
		}
	} else if _, ok := x.Fun.(*ast.Ident); !ok {
		a.expr(x.Fun)
	}
	for _, arg := range x.Args {
		a.expr(arg.Value)
	}
	if ci != nil && ci.Fn != nil && ci.Recv != nil && ci.Fn.Name == "len" && len(x.Args) == 0 {
		fo := ci.Fn
		if fo.Module.IsStd && fo.Recv != nil && fo.Recv.Module.IsPrelude {
			switch fo.Recv.Name {
			case "Str", "Bytes", "Map", "Set":
				return iv(0, maxLen)
			case "List":
				r := iv(0, math.MaxInt64)
				if n := f.g.named(f.typeOf(ci.Recv)); n != nil && len(n.Args) > 0 && a.nonZeroSize(n.Args[0], 0) {
					r = iv(0, maxLen)
				}
				if root, path, ok := a.stableKey(ci.Recv); ok {
					r.sym = &symb{root: root, path: path, m: 1}
				}
				return r
			}
		}
	}
	return a.typeRange(f.nat(x))
}

// isIntegerType reports whether t is an integer type (sized or not).
func isIntegerType(f *fnGen, t check.Type) bool {
	if t == nil {
		return false
	}
	switch f.goType(f.pg.monoType(check.Deep(t))) {
	case "int64", "int8", "int16", "int32", "uint8", "uint16", "uint32", "uint64":
		return !f.g.isFloat(t)
	}
	return false
}

func (a *rangeAn) ifExpr(x *ast.IfExpr) ival {
	var t, fl renv
	if x.Cond != nil {
		a.expr(x.Cond)
		t, fl = a.refine(a.env, x.Cond)
	} else {
		a.expr(x.Value)
		t, fl = a.env.clone(), a.env.clone()
	}
	a.env = t
	r1 := a.block(x.Then)
	e1 := a.env
	a.env = fl
	r2 := fullRange
	if x.Else != nil {
		r2 = a.expr(x.Else)
	}
	switch {
	case e1.dead:
	case a.env.dead:
		r2 = r1
	default:
		r2 = r2.join(r1)
	}
	a.env = joinEnv(e1, a.env)
	return r2
}

func (a *rangeAn) match(x *ast.MatchExpr) ival {
	a.expr(x.Subject)
	for _, arm := range x.Arms {
		if arm.Guard != nil && !simpleExpr(arm.Guard) {
			a.havoc(x)
			return fullRange
		}
	}
	base := a.env.clone()
	acc := deadEnv()
	var r ival
	have := false
	for _, arm := range x.Arms {
		a.env = base.clone()
		if arm.Guard != nil {
			a.expr(arm.Guard)
			a.env, _ = a.refine(a.env, arm.Guard)
		}
		v, _ := a.stmt(arm.Body)
		if !a.env.dead {
			if have {
				r = r.join(v)
			} else {
				r, have = v, true
			}
		}
		acc = joinEnv(acc, a.env)
	}
	a.env = acc
	if !have {
		return fullRange
	}
	return r
}

// simpleExpr reports whether evaluating e cannot change a binding: no
// blocks, branches, loops, closures, or assignments inside.
func simpleExpr(e ast.Expr) bool {
	ok := true
	inspectNode(e, func(n ast.Node) bool {
		switch n.(type) {
		case *ast.Block, *ast.IfExpr, *ast.MatchExpr, *ast.WhileExpr, *ast.FnLit, *ast.ScopeExpr,
			*ast.SelectExpr, *ast.AssignStmt:
			ok = false
		}
		return ok
	})
	return ok
}

// refine returns the states of e where the condition c holds and where it
// does not. c has been evaluated in e.
func (a *rangeAn) refine(e renv, c ast.Expr) (renv, renv) {
	if e.dead || !simpleExpr(c) {
		return e.clone(), e.clone()
	}
	switch x := unparen(c).(type) {
	case *ast.UnaryExpr:
		if x.Op == token.Bang {
			t, fl := a.refine(e, x.X)
			return fl, t
		}
	case *ast.BinaryExpr:
		switch x.Op {
		case token.AndAnd:
			t1, f1 := a.refine(e, x.X)
			t2, f2 := a.refine(t1, x.Y)
			return t2, joinEnv(f1, f2)
		case token.OrOr:
			t1, f1 := a.refine(e, x.X)
			t2, f2 := a.refine(f1, x.Y)
			return joinEnv(t1, t2), f2
		case token.Lt, token.LtEq, token.Gt, token.GtEq, token.Eq, token.NotEq:
			if a.isInt(a.f.nat(x.X)) && a.isInt(a.f.nat(x.Y)) {
				return a.refineCmp(e, x)
			}
		}
	}
	return e.clone(), e.clone()
}

// refineCmp narrows the tracked bindings compared by x.
func (a *rangeAn) refineCmp(e renv, x *ast.BinaryExpr) (renv, renv) {
	l, r := x.X, x.Y
	op := x.Op
	// normalize to l < r, l <= r, l == r, l != r
	switch op {
	case token.Gt:
		l, r, op = r, l, token.Lt
	case token.GtEq:
		l, r, op = r, l, token.LtEq
	}
	lr, rr := a.peek(e, l), a.peek(e, r)
	lv, rv := a.tracked(l), a.tracked(r)
	t, fl := e.clone(), e.clone()
	// set narrows v (whose current value is base) to [lo, hi] and, when
	// up is not nil, to the symbolic bound up (preferred over the one it
	// had, unless both bound the same list and the old one is tighter)
	set := func(env *renv, v *check.VarObj, base ival, lo, hi int64, up *symb) {
		if v == nil || env.dead {
			return
		}
		if lo > hi {
			*env = deadEnv()
			return
		}
		r := base
		r.lo, r.hi = lo, hi
		if up != nil && !(r.sym.same(up) && r.sym.off <= up.off) {
			r.sym = up
		}
		env.m[v] = r
	}
	shift := func(s *symb, d int64) *symb {
		if s == nil {
			return nil
		}
		return mkSym(s, s.m, s.off+d)
	}
	switch op {
	case token.Lt:
		// true: l <= r - 1, r >= l + 1; false: l >= r, r <= l
		if rr.hi == math.MinInt64 || lr.lo == math.MaxInt64 {
			t = deadEnv()
		} else {
			set(&t, lv, lr, lr.lo, min(lr.hi, rr.hi-1), shift(rr.sym, -1))
			set(&t, rv, rr, max(rr.lo, lr.lo+1), rr.hi, nil)
		}
		set(&fl, lv, lr, max(lr.lo, rr.lo), lr.hi, nil)
		set(&fl, rv, rr, rr.lo, min(rr.hi, lr.hi), lr.sym)
	case token.LtEq:
		set(&t, lv, lr, lr.lo, min(lr.hi, rr.hi), rr.sym)
		set(&t, rv, rr, max(rr.lo, lr.lo), rr.hi, nil)
		if rr.lo == math.MaxInt64 || lr.hi == math.MinInt64 {
			fl = deadEnv()
		} else {
			set(&fl, lv, lr, max(lr.lo, rr.lo+1), lr.hi, nil)
			set(&fl, rv, rr, rr.lo, min(rr.hi, lr.hi-1), shift(lr.sym, -1))
		}
	case token.Eq:
		lo, hi := max(lr.lo, rr.lo), min(lr.hi, rr.hi)
		set(&t, lv, lr, lo, hi, rr.sym)
		set(&t, rv, rr, lo, hi, lr.sym)
	case token.NotEq:
		lo, hi := max(lr.lo, rr.lo), min(lr.hi, rr.hi)
		set(&fl, lv, lr, lo, hi, rr.sym)
		set(&fl, rv, rr, lo, hi, lr.sym)
	}
	return t, fl
}

// peek is the interval of a condition operand in state e, without
// recording anything (conditions passed to refine have no effects).
func (a *rangeAn) peek(e renv, x ast.Expr) ival {
	saved, savedSafe, savedPoints, savedLoops := a.env, a.safe, a.points, a.loops
	a.env = e.clone()
	a.safe, a.points, a.loops = map[ast.Node]bool{}, map[ast.Expr]pointRec{}, nil
	r := a.expr(x)
	a.env, a.safe, a.points, a.loops = saved, savedSafe, savedPoints, savedLoops
	return r
}

// closure analyzes the body of a closure or a `defer`: it runs at a time
// the analysis does not know, so it sees `let` bindings (immutable) with
// their current intervals and every `var` with its full range.
func (a *rangeAn) closure(body ast.Node) {
	if a.env.dead {
		return
	}
	saved, savedLoops := a.env, a.loops
	m := map[*check.VarObj]ival{}
	for v, r := range a.env.m {
		if !v.Mutable {
			m[v] = r
		}
	}
	a.env, a.loops = renv{m: m}, nil
	switch b := body.(type) {
	case *ast.Block:
		a.block(b)
	case ast.Stmt:
		a.stmt(b)
	}
	a.env, a.loops = saved, savedLoops
}

// ---------------------------------------------------------------------------
// Statements

func (a *rangeAn) block(b *ast.Block) ival {
	if b == nil {
		return fullRange
	}
	r := fullRange
	for i, s := range b.Stmts {
		v, isVal := a.stmt(s)
		if i == len(b.Stmts)-1 && isVal {
			r = v
		}
	}
	return r
}

func (a *rangeAn) stmt(s ast.Stmt) (ival, bool) {
	f := a.f
	switch x := s.(type) {
	case *ast.ExprStmt:
		return a.expr(x.X), true
	case *ast.LetStmt:
		r := a.expr(x.Value)
		if bp, ok := x.Pattern.(*ast.BindPat); ok {
			v := f.info.VarOf(bp.Name)
			a.rebind(v)
			a.bind(v, r)
		}
	case *ast.VarStmt:
		r := a.expr(x.Value)
		if x.Value != nil {
			a.bind(f.info.VarOf(x.Name), r)
		}
	case *ast.ConstDecl:
		r := a.expr(x.Value)
		a.bind(f.info.VarOf(x.Name), r)
	case *ast.AssignStmt:
		a.assign(x)
	case *ast.ReturnStmt:
		a.expr(x.Value)
		a.env = deadEnv()
	case *ast.BreakStmt:
		if l := a.loopFor(x.Label); l != nil && !a.env.dead {
			l.breaks = append(l.breaks, a.env.clone())
		}
		a.env = deadEnv()
	case *ast.ContinueStmt:
		if l := a.loopFor(x.Label); l != nil && !a.env.dead {
			l.cont = append(l.cont, a.env.clone())
		}
		a.env = deadEnv()
	case *ast.WhileStmt:
		if x.Pattern != nil {
			// `while let` (decision 0134): not analyzed; its arithmetic
			// keeps its checks and what it assigns is forgotten
			a.havoc(x)
			return fullRange, false
		}
		a.loop(labelName(x.Label), x.Cond, x.Body, false, nil, fullRange)
	case *ast.ForStmt:
		a.forStmt(x)
	case *ast.DeferStmt:
		a.closure(x.Body)
	default:
		a.havoc(s)
	}
	return fullRange, false
}

func labelName(id *ast.Ident) string {
	if id == nil {
		return ""
	}
	return id.Name
}

func (a *rangeAn) loopFor(label *ast.Ident) *rloop {
	for i := len(a.loops) - 1; i >= 0; i-- {
		if label == nil || a.loops[i].label == label.Name {
			return a.loops[i]
		}
	}
	return nil
}

func (a *rangeAn) assign(x *ast.AssignStmt) {
	f := a.f
	rb := a.expr(x.Value)
	v := a.tracked(x.Target)
	if x.Op == assignOp {
		if v != nil {
			a.bind(v, rb)
			return
		}
		// an untracked binding needs no update
		a.placeExpr(x.Target)
		return
	}
	if v == nil {
		if _, ok := unparen(x.Target).(*ast.Ident); !ok {
			a.placeExpr(x.Target)
		}
		return
	}
	op := compoundOp(x.Op)
	cur := a.lookup(v)
	if !a.isInt(f.nat(x.Value)) {
		return
	}
	res, safe := arith(op, cur, rb)
	a.mark(x, safe)
	a.notePoint(x.Value, rb)
	a.bind(v, res)
}

// placeExpr evaluates the operands of an assignment target (index
// expressions and receivers).
func (a *rangeAn) placeExpr(e ast.Expr) {
	switch x := e.(type) {
	case *ast.ParenExpr:
		a.placeExpr(x.X)
	case *ast.SelectorExpr:
		a.placeExpr(x.X)
	case *ast.IndexExpr:
		a.placeExpr(x.X)
		a.expr(x.Index)
	case *ast.Ident:
	default:
		a.expr(e)
	}
}

func (a *rangeAn) forStmt(x *ast.ForStmt) {
	var bv *check.VarObj
	br := fullRange
	if bp, ok := x.Pattern.(*ast.BindPat); ok {
		bv = a.f.info.VarOf(bp.Name)
	}
	if rng, ok := unparen(x.Iter).(*ast.RangeExpr); ok && rng.Lo != nil && rng.Hi != nil &&
		a.isInt(a.f.nat(rng.Lo)) && a.isInt(a.f.nat(rng.Hi)) {
		lo := a.expr(rng.Lo)
		hi := a.expr(rng.Hi)
		br = iv(lo.lo, hi.hi)
		br.sym = hi.sym
		if !rng.Inclusive && hi.sym != nil {
			br.sym = mkSym(hi.sym, hi.sym.m, hi.sym.off-1)
		}
		if !rng.Inclusive {
			if hi.hi == math.MinInt64 {
				br.hi = math.MinInt64 // empty: never runs
				br.lo = math.MinInt64
			} else {
				br.hi = hi.hi - 1
			}
		}
		if br.lo > br.hi {
			// the loop body never runs
			return
		}
	} else {
		a.expr(x.Iter)
		if bv != nil {
			br = a.typeRange(bv.Type)
		}
	}
	a.loop(labelName(x.Label), nil, x.Body, true, bv, br)
}

// loop iterates a loop body to a fixed point. For a `for` loop (isFor)
// the body may run zero times and bv, when not nil, is bound to br in each
// iteration; a `while` loop exits where cond is false, and one without a
// condition only by `break`.
func (a *rangeAn) loop(label string, cond ast.Expr, body *ast.Block, isFor bool, bv *check.VarObj, br ival) {
	if a.env.dead {
		return
	}
	head := a.env.clone()
	for round := 0; ; round++ {
		a.env = head.clone()
		exit := deadEnv()
		switch {
		case cond != nil:
			a.expr(cond)
			var t renv
			t, exit = a.refine(a.env, cond)
			a.env = t
		case isFor:
			exit = a.env.clone()
			a.rebind(bv)
			a.bind(bv, br)
		}
		l := &rloop{label: label}
		a.loops = append(a.loops, l)
		a.block(body)
		a.loops = a.loops[:len(a.loops)-1]
		back := a.env
		for _, c := range l.cont {
			back = joinEnv(back, c)
		}
		next := joinEnv(head, back)
		if round >= 2 {
			next = widenEnv(head, next)
		}
		if round >= maxLoopRounds {
			next = renv{m: map[*check.VarObj]ival{}}
		}
		if leqEnv(next, head) {
			out := exit
			for _, b := range l.breaks {
				out = joinEnv(out, b)
			}
			a.env = out
			return
		}
		head = next
	}
}
