package ssa

import (
	"fmt"
	"strings"

	"github.com/xo-lang/xo/internal/ast"
	"github.com/xo-lang/xo/internal/check"
	"github.com/xo-lang/xo/internal/token"
)

// block lowers a block and returns the value of its last expression
// statement (unit when there is none).
func (s *fn) block(bl *ast.Block) *Value {
	var last *Value
	fr := &deferFrame{}
	s.exits = append(s.exits, exitEntry{defers: fr})
	for i, st := range bl.Stmts {
		if es, ok := st.(*ast.ExprStmt); ok && i == len(bl.Stmts)-1 {
			last = s.expr(es.X)
			break
		}
		s.stmt(st)
	}
	s.exits = s.exits[:len(s.exits)-1]
	if !s.isDead() {
		// the block's value is computed before its deferred code runs
		s.runDefers(fr)
	}
	if last == nil {
		return s.unit()
	}
	return last
}

func (s *fn) stmt(st ast.Stmt) {
	b := s.b
	switch x := st.(type) {
	case *ast.ExprStmt:
		s.expr(x.X)
	case *ast.LetStmt:
		if bp, ok := x.Pattern.(*ast.BindPat); ok {
			s.noteBinding(x.Value, b.info.VarOf(bp.Name))
		}
		v := s.value(x.Value)
		fail := s.newBlock()
		s.pattern(v, x.Pattern, fail)
		cont := s.cur
		s.cur = fail
		s.seal(fail)
		s.add(OpUnreachable, nil, nil) // `let` patterns are irrefutable
		s.cur = cont
	case *ast.VarStmt:
		vo := b.info.VarOf(x.Name)
		s.noteBinding(x.Value, vo)
		s.writeVar(vo, s.value(x.Value))
	case *ast.ConstDecl:
		// a local constant is a binding
		if vo := b.info.VarOf(x.Name); vo != nil {
			s.writeVar(vo, s.value(x.Value))
		}
	case *ast.AssignStmt:
		s.assign(x)
	case *ast.ReturnStmt:
		var v *Value
		if x.Value == nil {
			v = s.unit()
		} else {
			v = s.expr(x.Value)
		}
		var at ast.Node = x
		if x.Value != nil {
			// the returned expression: an implicit conversion reads its type
			at = x.Value
		}
		s.ret(v, at)
	case *ast.BreakStmt, *ast.ContinueStmt:
		var label *ast.Ident
		brk := false
		if bs, ok := x.(*ast.BreakStmt); ok {
			label, brk = bs.Label, true
		} else {
			label = x.(*ast.ContinueStmt).Label
		}
		var lc *loopCtx
		for i := len(s.loops) - 1; i >= 0; i-- {
			if label == nil || s.loops[i].label == label.Name {
				lc = s.loops[i]
				break
			}
		}
		if lc == nil {
			b.unsupported(x, "this `break` or `continue`")
			return
		}
		s.exitTo(lc.exits)
		if brk {
			s.jump(lc.brk)
		} else {
			s.jump(lc.cont)
		}
		s.dead()
	case *ast.WhileStmt:
		label := ""
		if x.Label != nil {
			label = x.Label.Name
		}
		if x.Pattern != nil {
			s.whileLet(label, x)
			return
		}
		s.while(label, x.Cond, x.Bound, x.Body)
	case *ast.ForStmt:
		s.forStmt(x)
	case *ast.DeferStmt:
		s.deferStmt(x)
	default:
		b.unsupported(st, "this statement")
	}
}

// loopBound starts the iteration counter of a loop with `bound N` (core
// 3.5, decision 0133); the function it returns, called at the start of
// each iteration, counts and faults with XO-F006 past N. Without a bound
// both do nothing.
func (s *fn) loopBound(bound ast.Expr) func() {
	if bound == nil {
		return func() {}
	}
	b := s.b
	n, ok := b.info.LoopBound(bound)
	if !ok {
		b.unsupported(bound, "this loop bound")
		return func() {}
	}
	intT, boolT := b.mod.Builtin(KInt), b.mod.Builtin(KBool)
	cnt := &check.VarObj{Name: "$bound"}
	s.writeVar(cnt, s.cint(0))
	pos := b.pos(bound)
	msg := fmt.Sprintf("XO-F006: loop ran past its bound of %d iterations", n)
	return func() {
		// at most n + 1 iterations start, so the increment cannot overflow
		v := s.add(OpAdd, intT, nil, s.read(cnt), s.cint(1))
		s.writeVar(cnt, v)
		over, ok := s.branch(s.add(OpGt, boolT, nil, v, s.cint(n)))
		s.cur = over
		s.fault(s.add(OpConst, b.mod.Builtin(KStr), msg), pos)
		s.cur = ok
	}
}

// while lowers `while cond { body }` (cond nil: forever).
func (s *fn) while(label string, cond, bound ast.Expr, body *ast.Block) {
	tick := s.loopBound(bound)
	head, exit := s.newBlock(), s.newBlock()
	s.jump(head)
	s.cur = head
	if cond != nil {
		c := s.value(cond)
		in := s.newBlock()
		s.cur.Branch(c, in, exit)
		s.seal(in)
		s.cur = in
	}
	tick()
	s.loops = append(s.loops, &loopCtx{label: label, brk: exit, cont: head, scopes: len(s.scopes), exits: len(s.exits)})
	s.block(body)
	s.loops = s.loops[:len(s.loops)-1]
	s.jump(head)
	s.seal(head)
	s.seal(exit)
	s.cur = exit
}

// forStmt lowers `for p in a..b`, `a..=b`, and a List.
func (s *fn) forStmt(x *ast.ForStmt) {
	b := s.b
	intT := b.mod.Builtin(KInt)
	label := ""
	if x.Label != nil {
		label = x.Label.Name
	}
	idx := &check.VarObj{Name: "$i"}
	var hi, list, bytes *Value
	var pairT *Type // `xs.indexed()`: the (Int, T) element type
	inclusive := false
	if recv := s.indexedRecv(x.Iter); recv != nil {
		list = s.value(recv)
		if list.Type.Kind != KList {
			b.unsupported(x.Iter, "this `for` iteration")
			return
		}
		pairT = b.mod.Tuple(intT, list.Type.Elems[0])
		hi = s.call("list.len", intT, "", list)
		s.writeVar(idx, s.cint(0))
	} else if r, ok := x.Iter.(*ast.RangeExpr); ok && r.Lo != nil && r.Hi != nil {
		s.writeVar(idx, s.valueAs(r.Lo, intT))
		hi = s.valueAs(r.Hi, intT)
		inclusive = r.Inclusive
	} else if lt := s.natOf(x.Iter); lt.Kind == KHandle && strings.HasPrefix(lt.Name, "Chan[") {
		s.forChan(x, label)
		return
	} else if lt.Coll {
		s.forMap(x, label, s.value(x.Iter))
		return
	} else if lt.Kind == KStr && isPreludeNamed(s.natCheck(x.Iter), "Bytes") {
		// `for b in bytes` yields each byte as a UInt8 (decision 0138)
		bytes = s.value(x.Iter)
		hi = s.call("Str.len", intT, "", bytes)
		s.writeVar(idx, s.cint(0))
	} else if lt.Kind == KList {
		list = s.value(x.Iter)
		hi = s.call("list.len", intT, "", list)
		s.writeVar(idx, s.cint(0))
	} else {
		b.unsupported(x.Iter, "this `for` iteration")
		return
	}
	tick := s.loopBound(x.Bound)
	head, body, latch, exit := s.newBlock(), s.newBlock(), s.newBlock(), s.newBlock()
	s.jump(head)
	s.cur = head
	i := s.read(idx)
	var c *Value
	if inclusive {
		c = s.add(OpLe, b.mod.Builtin(KBool), nil, i, hi)
	} else {
		c = s.add(OpLt, b.mod.Builtin(KBool), nil, i, hi)
	}
	s.cur.Branch(c, body, exit)
	s.seal(body)
	s.cur = body
	tick()
	var elem *Value
	if list != nil {
		i := s.read(idx)
		elem = s.call("list.index", list.Type.Elems[0], "", list, i)
		if pairT != nil {
			elem = s.add(OpTuple, pairT, nil, i, elem)
		}
	} else if bytes != nil {
		// 0 <= i < len: no bounds check
		elem = s.call("bytes.index", b.mod.IntType("UInt8"), "", bytes, s.read(idx))
	} else {
		elem = s.read(idx)
	}
	fail := s.newBlock()
	s.pattern(elem, x.Pattern, fail)
	cont := s.cur
	s.cur = fail
	s.seal(fail)
	s.add(OpUnreachable, nil, nil)
	s.cur = cont
	s.loops = append(s.loops, &loopCtx{label: label, brk: exit, cont: latch, scopes: len(s.scopes), exits: len(s.exits)})
	s.block(x.Body)
	s.loops = s.loops[:len(s.loops)-1]
	s.jump(latch)
	s.seal(latch)
	s.cur = latch
	i = s.read(idx)
	if inclusive {
		// i == hi ends the loop before i + 1 could overflow
		done := s.add(OpEq, b.mod.Builtin(KBool), nil, i, hi)
		next := s.newBlock()
		s.cur.Branch(done, exit, next)
		s.seal(next)
		s.cur = next
	}
	// i < hi (or i != hi): the increment cannot overflow, so no Pos
	s.writeVar(idx, s.add(OpAdd, intT, nil, i, s.cint(1)))
	s.jump(head)
	s.seal(head)
	s.seal(exit)
	s.cur = exit
}

// indexedRecv returns xs of a `for` iterator `xs.indexed()` on a List,
// or nil.
func (s *fn) indexedRecv(e ast.Expr) ast.Expr {
	call, ok := e.(*ast.CallExpr)
	if !ok {
		return nil
	}
	ci := s.b.info.CallOf(call)
	if ci == nil || ci.Fn == nil || ci.Recv == nil || !ci.Fn.Module.IsPrelude || ci.Fn.Name != "indexed" || ci.Fn.Recv == nil || ci.Fn.Recv.Name != "List" {
		return nil
	}
	return ci.Recv
}

// place is an assignable location: its current value and how to store a
// new one. Storing into a field or element rebuilds the enclosing values
// (setfield, list.set) up to the local.
//
// Reading a list element on the way to a store moves it out of the list
// (`list.take`, decision 0169): the list is made unique and its slot
// holds a placeholder until the store puts the element back, so the
// element's own collections are unique too and update in place
// (`xs[i].f.push(x)`, `xs[i][j] = v`). Value semantics hold on every
// path: a list still used afterwards (shared, or read on a path where
// the store does not happen) is copied by `reuse` before the take.
// keep turns this off for a place whose store may not follow (Map.update
// stores only on success; a compound assignment whose operand can
// leave the statement): the element stays a projection (`list.index`).
type place struct {
	t    *Type
	get  func() *Value
	set  func(*Value)
	keep *bool
}

func (s *fn) place(e ast.Expr) (place, bool) {
	b := s.b
	switch x := e.(type) {
	case *ast.ParenExpr:
		return s.place(x.X)
	case *ast.Ident:
		vo := b.info.VarOf(x)
		if vo == nil {
			break
		}
		return place{s.typeOf(x), func() *Value { return s.read(vo) }, func(v *Value) { s.writeVar(vo, v) }, new(bool)}, true
	case *ast.SelectorExpr:
		base, ok := s.place(x.X)
		if !ok {
			break
		}
		idx, ft := s.fieldIndex(base.t, x.Sel.Name)
		if idx < 0 {
			break
		}
		var bv *Value
		cur := func() *Value {
			if bv == nil {
				bv = base.get()
			}
			return bv
		}
		pt := ft
		if ft.Kind == KBox {
			// a field of a recursive type: read out of and store into a box
			pt = ft.Elems[0]
		}
		return place{pt,
			func() *Value { return s.unbox(s.add(OpField, ft, idx, cur())) },
			func(v *Value) { base.set(s.add(OpSetField, base.t, idx, cur(), s.coerce(v, ft, x))) }, base.keep}, true
	case *ast.IndexExpr:
		base, ok := s.place(x.X)
		if ok && base.t.Coll {
			// `m[k] = v` is `m.put(k, v)` (core 3.6); reading m[k] is a
			// checker error
			t := base.t
			k := s.valueAs(x.Index, mapKey(t))
			return place{mapVal(t),
				func() *Value {
					b.unsupported(x, "reading `m[k]`")
					return s.undef(mapVal(t))
				},
				func(v *Value) { base.set(s.call(b.mapPut(t, x), t, "", base.get(), k, s.coerce(v, mapVal(t), x))) }, base.keep}, true
		}
		if !ok || base.t.Kind != KList {
			break
		}
		i := s.valueAs(x.Index, b.mod.Builtin(KInt))
		et := base.t.Elems[0]
		pos := b.pos(x)
		// the list is read when first used, as a field's base is: in
		// `xs[i] = f()` after f, which may change xs (as the Go backend
		// does; the value stored into xs[i] must not drop f's changes)
		var l, taken *Value
		cur := func() *Value {
			if l == nil {
				l = base.get()
			}
			return l
		}
		keep := base.keep
		get := func() *Value {
			if taken != nil {
				// read again before the store: the element moved out
				return taken
			}
			if *keep || !et.Heap() {
				return s.call("list.index", et, pos, cur(), i)
			}
			// moved out until the store puts it back (decision 0169)
			tk := s.call("list.take", b.mod.Tuple(base.t, et), pos, cur(), i)
			l = s.add(OpField, base.t, 0, tk)
			taken = s.add(OpField, et, 1, tk)
			return taken
		}
		set := func(v *Value) {
			p := pos
			if taken != nil {
				p = "" // the take checked the index of the same list
			}
			base.set(s.call("list.set", base.t, p, cur(), i, v))
		}
		return place{et, get, set, keep}, true
	}
	b.unsupported(e, "assigning to this expression")
	return place{}, false
}

// fieldIndex is the index and type of a struct field or tuple element.
func (s *fn) fieldIndex(t *Type, name string) (int, *Type) {
	switch t.Kind {
	case KStruct:
		if i := t.FieldIndex(name); i >= 0 {
			return i, t.Fields[i].Type
		}
	case KTuple:
		for i, e := range t.Elems {
			if name == itoa(i) {
				return i, e
			}
		}
	}
	return -1, nil
}

var compound = map[token.Kind]token.Kind{
	token.PlusAssign: token.Plus, token.MinusAssign: token.Minus, token.StarAssign: token.Star,
	token.SlashAssign: token.Slash, token.PercentAssign: token.Percent, token.AmpAssign: token.Amp,
	token.PipeAssign: token.Pipe, token.CaretAssign: token.Caret, token.ShlAssign: token.Shl,
	token.ShrAssign: token.Shr,
}

func (s *fn) assign(x *ast.AssignStmt) {
	p, ok := s.place(x.Target)
	if !ok {
		return
	}
	if x.Op == token.Assign {
		p.set(s.valueAs(x.Value, p.t))
		return
	}
	op, ok := compound[x.Op]
	if !ok {
		s.b.unsupported(x, "this assignment operator")
		return
	}
	// the operand runs between the read and the store: an element moves
	// out only when the operand cannot leave the statement or read a
	// list element (decision 0169)
	*p.keep = !plainOperand(x.Value)
	old := p.get()
	var r *Value
	if op == token.Shl || op == token.Shr {
		r = s.shiftCount(x.Value, p.t)
	} else {
		r = s.valueAs(x.Value, p.t)
	}
	p.set(s.arith(op, old, r, x))
}

// plainOperand reports an expression that cannot leave the statement (no
// call, `?`, block, or match) and reads no list element: literals, names,
// fields, and operators over them.
func plainOperand(e ast.Expr) bool {
	switch x := e.(type) {
	case *ast.IntLit, *ast.FloatLit, *ast.DurationLit, *ast.RuneLit, *ast.BytesLit, *ast.Ident:
		return true
	case *ast.StringLit:
		for _, p := range x.Parts {
			if p.Expr != nil && !plainOperand(p.Expr) {
				return false
			}
		}
		return true
	case *ast.ParenExpr:
		return plainOperand(x.X)
	case *ast.UnaryExpr:
		return plainOperand(x.X)
	case *ast.BinaryExpr:
		return plainOperand(x.X) && plainOperand(x.Y)
	case *ast.SelectorExpr:
		return plainOperand(x.X)
	}
	return false
}
