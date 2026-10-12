package ssa

import (
	"strconv"
	"strings"

	"github.com/xo-lang/xo/internal/ast"
	"github.com/xo-lang/xo/internal/check"
)

// Fault unwinding (core 8, decision 0085). A fault on the native backends
// unwinds the faulting task like the Go backend's panic: the deferred code
// of every block it leaves runs, the scopes it leaves close (canceling
// and waiting for their tasks), held Mutexes are unlocked, and every
// reference the frames own is released. It reuses the unwind paths of
// cancellation (decision 0068): a check after an operation branches to
// the cleanup of that point (its first successor) and on to the
// function's unwind exit (Func.UnwindExit), which hands the fault to the
// caller (`rt.fault_continue`).
//
//   - The runtime raises a fault as a forced unwind of the platform
//     unwinder (std/llrt `fault_raise`, `_Unwind_ForcedUnwind`). Every
//     native frame has unwind tables; the landing pads are the cleanup
//     paths of the SSA checks. The value that can fault carries its
//     landing (Value.Pad: the check's `rt.unwinding` or `rt.landing`
//     call), and the backends emit a landing pad for it (LLVM `invoke`,
//     arm64 call site tables): the normal path pays nothing.
//   - A check that only a fault takes calls `rt.landing`, which the
//     backends do not emit: the branch goes straight to the normal
//     successor. Checks after calls of functions that can cancel stay
//     `rt.unwinding` (a real test) and get the landing too.
//   - Landings are placed (1) by the builder after calls (check), (2) by
//     the builder after operations that can fault inside a block with
//     deferred code or an open scope (flushLanding: the cleanup runs that
//     code), and (3) after the build after every other operation that can
//     fault (addLandings: the cleanup is what reference counting
//     releases there). After InsertRC, the counting code between an
//     operation and its check moves into the check's successors
//     (sinkLandings), and checks whose cleanup does nothing are removed
//     (pruneLandings): a fault there passes through the frame.
//   - On the cleanup path the operation produced nothing: its results are
//     never released or used there (InsertRC skips them, the RC verifier
//     treats them as not held). A cancelled call returns zero values,
//     which own nothing, so this holds for cancellation too.
//   - `fault(...)` and failed contracts raise with `rt.fault_raise`
//     (instead of OpFault, which ends the program) so their cleanup has
//     an edge to hang on.

// faultingRT are the runtime functions (std/llrt) that can raise a fault.
var faultingRT = map[string]bool{
	"rt.fault_raise": true, "rt.fault_rethrow": true, "rt.fault_at": true,
	"rt.int_op": true, "rt.int_conv": true, "rt.rune_of": true,
	"rt.str_slice_at": true, "rt.bytes_slice_at": true, "rt.str_repeat": true,
	"rt.rand_int": true, "rt.shield_end": true,
	"rt.mutex_lock": true, "rt.mutex_rlock": true,
	"rt.task_spawn": true, "rt.task_await": true,
	"rt.chan_new": true, "rt.chan_send": true, "rt.select_wait": true,
	"rt.atomic_add": true,
	// the end of a real-time section that allocated (XO-F007, decision 0133)
	"rt.realtime_exit": true,
}

// RuntimeRaises reports whether the runtime call name (`rt.*`) can raise
// a fault: backends must not declare it as never unwinding.
func RuntimeRaises(name string) bool { return faultingRT[name] || name == "rt.fault_continue" }

// checkedCalls are the inline runtime operations that fault when they
// carry a position.
var checkedCalls = map[string]bool{"list.index": true, "list.set": true, "list.take": true, "bytes.index": true, "Int.abs": true}

// selfFaults reports whether v can raise a fault by itself (not through a
// user function it calls).
func selfFaults(v *Value) bool {
	switch v.Op {
	case OpFault:
		return true
	case OpAdd, OpSub, OpMul, OpNeg, OpDiv, OpRem, OpShl, OpShr, OpFloatToInt:
		return v.Pos != ""
	case OpCallValue:
		return true
	case OpCall:
		name, _ := v.Aux.(string)
		if checkedCalls[name] {
			return v.Pos != ""
		}
		return faultingRT[name]
	}
	return false
}

// isUserCall reports a call of a function of the module (not a runtime or
// prelude operation).
func isUserCall(m *Module, v *Value) bool {
	if v.Op != OpCall {
		return false
	}
	name, _ := v.Aux.(string)
	return m.Func(name) != nil
}

// mayRaise reports whether v may turn out to fault once built: a call, or
// an operation that faults when it gets a position (set after it is
// added). flushLanding decides.
func mayRaise(v *Value) bool {
	switch v.Op {
	case OpAdd, OpSub, OpMul, OpNeg, OpDiv, OpRem, OpShl, OpShr, OpFloatToInt, OpCallValue, OpCall:
		return true
	}
	return false
}

// raises reports whether v, once built, can fault: by itself, or by
// calling a function of the module that can (faults, from mayFault; nil
// means any).
func raises(m *Module, v *Value, faults map[*Func]bool) bool {
	if v.Op == OpCallValue && v.Args[0].Op == OpClosure {
		if f := m.Func(v.Args[0].Aux.(string)); f != nil && f.Catches {
			return false // it returns on a fault
		}
	}
	if selfFaults(v) {
		return true
	}
	if isUserCall(m, v) {
		name := v.Aux.(string)
		return faults == nil || faults[m.Func(name)]
	}
	return false
}

// mayFault returns the functions that can fault: those with an operation
// that faults, and their callers.
func (m *Module) mayFault() map[*Func]bool {
	may := map[*Func]bool{}
	calls := map[*Func][]*Func{}
	for _, f := range m.Funcs {
		for _, b := range f.Blocks {
			for _, v := range b.Instrs {
				if selfFaults(v) {
					may[f] = true
				}
				if isUserCall(m, v) {
					calls[f] = append(calls[f], m.Func(v.Aux.(string)))
				}
			}
		}
	}
	for changed := true; changed; {
		changed = false
		for _, f := range m.Funcs {
			if may[f] {
				continue
			}
			for _, g := range calls[f] {
				if may[g] {
					may[f] = true
					changed = true
					break
				}
			}
		}
	}
	return may
}

// lexicalCleanup reports whether leaving the current code by a fault runs
// deferred code or closes a scope of this function.
func (s *fn) lexicalCleanup() bool {
	for _, e := range s.exits {
		if e.scope != nil || (e.defers != nil && len(e.defers.stmts) > 0) {
			return true
		}
	}
	return false
}

// pendingAt records the blocks and scopes open around the pending
// operation (the cleanup of its landing leaves them), as they are when it
// is added: a block may end, or register more deferred code, before the
// landing is placed.
type pendingAt struct {
	exits  []exitEntry
	scopes []*scopeCtx
}

func (s *fn) snapshot() pendingAt {
	p := pendingAt{scopes: s.scopes[:len(s.scopes):len(s.scopes)]}
	for _, e := range s.exits {
		if e.defers != nil {
			n := len(e.defers.stmts)
			e = exitEntry{defers: &deferFrame{stmts: e.defers.stmts[:n:n]}}
		}
		p.exits = append(p.exits, e)
	}
	return p
}

// flushLanding gives the pending operation its landing: a check only a
// fault takes, whose cleanup leaves the blocks and scopes open where it
// was added.
func (s *fn) flushLanding() {
	w := s.pending
	s.pending = nil
	at, defs, dl := s.pendingAt, s.pendingDefs, s.pendingDefer
	s.pendingDefs, s.pendingDefer = nil, nil
	if w.Block != s.cur || s.cur.Term() != nil || s.isDead() || w.Pad != nil {
		return
	}
	if !selfFaults(w) && !isUserCall(s.b.mod, w) {
		return
	}
	if name, _ := w.Aux.(string); w.Op == OpCall && name == "rt.shield_end" {
		// deferred code ends there: a landing would lower the outer
		// deferred code again for every level
		return
	}
	u := s.cur.Add(OpCall, s.b.mod.Builtin(KBool), "rt.landing")
	w.Pad = u
	callee := ""
	if w.Op == OpCall && !selfFaults(w) {
		callee = w.Aux.(string)
	}
	yes, no := s.branch(u)
	s.f.markRest(no)
	s.cur = yes
	savedExits, savedScopes := s.exits, s.scopes
	if dl == nil {
		s.exits, s.scopes = at.exits, at.scopes
	}
	// the cleanup reads the variables as they were before w: put back
	// the definitions made in its block since
	now := map[*check.VarObj]*Value{}
	for vo, old := range defs {
		now[vo] = s.defs[vo][w.Block]
		if old == nil {
			delete(s.defs[vo], w.Block)
		} else {
			s.defs[vo][w.Block] = old
		}
	}
	if dl != nil {
		// a fault in deferred code: its shield ends, the rest of the
		// cleanup runs (build_defer.go faultCont)
		land := s.deferLand
		s.deferLand = nil
		// the blocks opened inside the statement run their deferred code
		// first, innermost first
		for i := len(at.exits) - 1; i >= dl.base && !s.isDead(); i-- {
			s.exits = at.exits[:i:i]
			s.runDefers(at.exits[i].defers)
		}
		s.exits = savedExits
		if !s.isDead() {
			if dl.tok != nil {
				s.rt("shield_abort", nil, dl.tok)
			}
			s.jump(s.faultCont(dl.fr, dl.j))
		}
		s.deferLand = land
	} else {
		s.unwind()
	}
	for vo, v := range now {
		s.defs[vo][w.Block] = v
	}
	s.exits, s.scopes = savedExits, savedScopes
	s.cur = no
	s.b.checks = append(s.b.checks, unwindCheck{s.f, u, callee, w})
}

// markRest records that b continues the block before it after a check
// (merged back when the check goes, mergeJumps).
func (f *Func) markRest(b *Block) {
	if f.landRest == nil {
		f.landRest = map[*Block]bool{}
	}
	f.landRest[b] = true
}

// fault raises a fault with message msg (a Str) at pos: rt.fault_raise
// when faults unwind, else OpFault. The current block ends.
func (s *fn) fault(msg *Value, pos string) {
	if !s.b.faultUnwind {
		f := s.add(OpFault, nil, nil, msg)
		f.Pos = pos
		return
	}
	s.rt("fault_raise", nil, msg, s.add(OpConst, s.b.mod.Builtin(KStr), pos))
	s.add(OpUnreachable, nil, nil)
}

// ---------------------------------------------------------------------------
// After the build

// addLandings gives every operation that can fault and has no landing yet
// a check whose cleanup goes to the function's unwind exit (what InsertRC
// releases there); pruneLandings removes those that release nothing.
func (b *builder) addLandings() {
	m := b.mod
	faults := m.mayFault()
	for _, f := range m.Funcs {
		for bi := 0; bi < len(f.Blocks); bi++ {
			bl := f.Blocks[bi]
			for i := 0; i < len(bl.Instrs); i++ {
				v := bl.Instrs[i]
				if v.Pad != nil || v.Op.IsTerminator() || !raises(m, v, faults) {
					continue
				}
				b.landAt(f, bl, i)
				break // the rest of the block moved to a new block
			}
		}
	}
}

// landAt splits bl after its instruction i and gives it a check only a
// fault takes, whose cleanup goes to f's unwind exit.
func (b *builder) landAt(f *Func, bl *Block, i int) {
	m := b.mod
	w := bl.Instrs[i]
	rest := f.NewBlock()
	rest.Instrs = bl.Instrs[i+1:]
	for _, v := range rest.Instrs {
		v.Block = rest
		for _, e := range v.Succs {
			e.From = rest
		}
	}
	bl.Instrs = bl.Instrs[: i+1 : i+1]
	f.markRest(rest)
	u := bl.Add(OpCall, m.Builtin(KBool), "rt.landing")
	w.Pad = u
	pad := f.NewBlock()
	bl.Branch(u, pad, rest)
	pad.Jump(b.unwindExit(f))
}

// unwindExit returns f's unwind exit, made when f has none: it hands a
// fault on and returns zero values.
func (b *builder) unwindExit(f *Func) *Block {
	if f.UnwindExit != nil {
		return f.UnwindExit
	}
	s := b.newFn(f, nil, nil)
	if f.Catches {
		s.packOrig = f.Params()[1]
	}
	s.unwindB = f.NewBlock()
	s.finishFn()
	return f.UnwindExit
}

// ---------------------------------------------------------------------------
// After InsertRC

// landingSite returns the value whose landing check ends block b (the
// branch on u), or nil.
func landingSite(b *Block) (w, u *Value) {
	t := b.Term()
	if t == nil || t.Op != OpBranch {
		return nil, nil
	}
	u = t.Args[0]
	if u.Op != OpCall || u.Block != b {
		return nil, nil
	}
	if name := u.Aux.(string); name != "rt.landing" && name != "rt.unwinding" {
		return nil, nil
	}
	for _, v := range b.Instrs {
		if v.Pad == u {
			return v, u
		}
	}
	return nil, u
}

// keepsOperands reports an inline operation that faults before it
// consumes its operands (`list.set` and `list.take` check the index
// first): a landing releases them. Calls hand theirs to the callee (or the runtime), which
// releases what it holds when it faults.
func keepsOperands(w *Value) bool {
	name, _ := w.Aux.(string)
	return w.Op == OpCall && (name == "list.set" || name == "list.take") && w.Pos != ""
}

// consumedLeaves lists the leaves w consumes that the function owns.
func consumedLeaves(r *rcInfo, w *Value) []leaf {
	var out []leaf
	for _, u := range r.uses(w) {
		if u.m == mConsume && r.tracked(u.l) && r.kind(u.l.root) == rkOwned {
			out = append(out, u.l)
		}
	}
	return out
}

// isRCOp reports the counting instructions InsertRC places after an
// operation, which move into the check's successors.
func isRCOp(v *Value) bool {
	switch v.Op {
	case OpRetain, OpRelease, OpShare: // (share probes, decision 0058)
		return true
	}
	return false
}

// sinkLandings moves the counting code between each landing site and its
// check into the check's successors: on the cleanup path it runs from the
// landing pad, except what concerns the site's own results (they do not
// exist there). A site followed by anything else loses its landing.
func (f *Func) sinkLandings(r *rcInfo) {
	for _, b := range append([]*Block(nil), f.Blocks...) {
		w, u := landingSite(b)
		if w == nil || w.Block != b {
			continue
		}
		if keepsOperands(w) {
			// the operation faults before it takes its operands: on the
			// cleanup path the function still owns them
			t := b.Term()
			var code []*Value
			for _, l := range consumedLeaves(r, w) {
				v := l.root
				if l.path != "" {
					for _, s := range strings.Split(l.path[1:], ".") {
						i, _ := strconv.Atoi(s)
						v = f.newValue(OpField, fieldType(v.Type, i), []*Value{v}, i)
						code = append(code, v)
					}
				}
				code = append(code, f.newValue(OpRelease, nil, []*Value{v}, nil))
			}
			f.edgePrepend(t.Succs[0], code)
		}
		wi, ui := -1, -1
		for i, v := range b.Instrs {
			if v == w {
				wi = i
			}
			if v == u {
				ui = i
			}
		}
		if wi < 0 || ui < wi {
			w.Pad = nil
			continue
		}
		between := append([]*Value(nil), b.Instrs[wi+1:ui]...)
		ok := true
		fields := false
		inBetween := map[*Value]bool{}
		for _, v := range between {
			inBetween[v] = true
			fields = fields || v.Op == OpField
			if !isRCOp(v) && v.Op != OpField {
				ok = false
			}
		}
		// the field reads must serve only the counting code moved with them
		for _, bb := range f.Blocks {
			if !fields || !ok {
				break
			}
			for _, v := range bb.Instrs {
				if inBetween[v] {
					continue
				}
				for _, a := range v.Args {
					if inBetween[a] {
						ok = false
					}
				}
				for _, e := range v.Succs {
					for _, a := range e.Args {
						if inBetween[a] {
							ok = false
						}
					}
				}
			}
		}
		if !ok {
			w.Pad = nil
			continue
		}
		if len(between) == 0 {
			continue
		}
		t := b.Term()
		pad, normal := t.Succs[0], t.Succs[1]
		var padCode []*Value
		copies := map[*Value]*Value{}
		for _, v := range between {
			root, _ := base(v)
			if isRCOp(v) {
				root, _ = base(v.Args[0])
			}
			if root == w {
				continue // the site's own results: they do not exist there
			}
			args := make([]*Value, len(v.Args))
			for i, a := range v.Args {
				args[i] = a
				if c, ok := copies[a]; ok {
					args[i] = c
				}
			}
			c := f.newValue(v.Op, v.Type, args, v.Aux)
			copies[v] = c
			padCode = append(padCode, c)
		}
		b.Instrs = append(append(b.Instrs[:wi+1:wi+1], u), t)
		if nt := normal.To.Term(); nt == nil || nt.Op != OpUnreachable || len(normal.To.Instrs) != 1 {
			// (after a raise the normal successor is never reached)
			f.edgePrepend(normal, between)
		}
		f.edgePrepend(pad, padCode)
	}
}

// edgePrepend places code at the start of edge e's target (which then
// must have e as its only predecessor and no parameters), or in a block
// that splits the edge.
func (f *Func) edgePrepend(e *Edge, code []*Value) {
	if len(code) == 0 {
		return
	}
	to := e.To
	if len(to.Preds) == 1 && len(e.Args) == 0 && len(to.Params) == 0 {
		for _, v := range code {
			v.Block = to
		}
		to.Instrs = append(append([]*Value(nil), code...), to.Instrs...)
		return
	}
	n := f.NewBlock()
	for _, v := range code {
		v.Block = n
	}
	n.Instrs = code
	j := n.Add(OpJump, nil, nil)
	ne := &Edge{From: n, To: to, Args: e.Args}
	j.Succs = []*Edge{ne}
	for i, p := range to.Preds {
		if p == e {
			to.Preds[i] = ne
		}
	}
	e.To = n
	e.Args = nil
	n.Preds = []*Edge{e}
}

// pruneLandings removes the checks only a fault takes whose cleanup does
// nothing but reach the unwind exit: a fault passes through the frame.
func (f *Func) pruneLandings() {
	defer f.mergeJumps()
	exit := f.UnwindExit
	if exit == nil {
		return
	}
	trivial := func(b *Block) bool {
		for i := 0; i < 16; i++ {
			if b == exit {
				return !f.Catches
			}
			if len(b.Instrs) != 1 || len(b.Params) != 0 {
				return false
			}
			t := b.Term()
			if t == nil || t.Op != OpJump || len(t.Succs[0].Args) != 0 {
				return false
			}
			b = t.Succs[0].To
		}
		return false
	}
	sites := map[*Value][]*Value{}
	for _, b := range f.Blocks {
		for _, v := range b.Instrs {
			if v.Pad != nil {
				sites[v.Pad] = append(sites[v.Pad], v)
			}
		}
	}
	changed := false
	for _, b := range f.Blocks {
		t := b.Term()
		if t == nil || t.Op != OpBranch {
			continue
		}
		u := t.Args[0]
		if u.Op != OpCall || u.Aux.(string) != "rt.landing" || u.Block != b {
			continue
		}
		pad, normal := t.Succs[0], t.Succs[1]
		if !trivial(pad.To) {
			continue
		}
		for _, v := range sites[u] {
			v.Pad = nil
		}
		var preds []*Edge
		for _, e := range pad.To.Preds {
			if e != pad {
				preds = append(preds, e)
			}
		}
		pad.To.Preds = preds
		t.Op, t.Args, t.Succs = OpJump, nil, []*Edge{normal}
		var ins []*Value
		for _, v := range b.Instrs {
			if v != u {
				ins = append(ins, v)
			}
		}
		b.Instrs = ins
		changed = true
	}
	if changed {
		f.prune()
	}
}

// mergeJumps merges the blocks addLandings split off (landAt) back into
// their predecessor once its check is gone.
func (f *Func) mergeJumps() {
	for changed := true; changed; {
		changed = false
		for _, b := range f.Blocks {
			t := b.Term()
			if t == nil || t.Op != OpJump {
				continue
			}
			to := t.Succs[0].To
			if !f.landRest[to] || to == b || to == f.Blocks[0] || len(to.Preds) != 1 || len(to.Params) != 0 || to == f.UnwindExit {
				continue
			}
			delete(f.landRest, to)
			b.Instrs = b.Instrs[:len(b.Instrs)-1]
			for _, v := range to.Instrs {
				v.Block = b
				for _, e := range v.Succs {
					e.From = b
				}
			}
			b.Instrs = append(b.Instrs, to.Instrs...)
			to.Instrs = nil
			to.Preds = nil
			var keep []*Block
			for _, x := range f.Blocks {
				if x != to {
					keep = append(keep, x)
				}
			}
			f.Blocks = keep
			changed = true
			break
		}
	}
}

// ---------------------------------------------------------------------------
// task.isolate

// isolate lowers `task.isolate(f)` (core 8, std.md 7): f runs; a fault in
// it becomes Err(Isolated.Faulted(info)) (the fault is caught at the
// landing of the call), its error Err(Isolated.Failed(e)); cancellation
// goes on unwinding.
func (s *fn) isolate(x *ast.CallExpr) *Value {
	b := s.b
	m := b.mod
	nt := s.natOf(x)
	arg := x.Args[0].Value
	ct, ok := check.Resolve(s.natCheck(arg)).(*check.Func)
	errI := nt.VariantIndex("Err")
	if !ok || nt.Kind != KEnum || errI < 0 {
		b.unsupported(x, "this `task.isolate`")
		return s.undef(nt)
	}
	errT := failErr(s.apply(ct.Err))
	rt := b.typ(b.resultOf(s.apply(ct.Result), errT), x)
	f := s.valueAs(arg, m.FuncType(nil, rt))
	if s.pending != nil {
		s.flushLanding()
	}
	boolT, strT := m.Builtin(KBool), m.Builtin(KStr)
	r := s.cur.Add(OpCallValue, rt, nil, f)
	u := s.cur.Add(OpCall, boolT, "rt.unwinding")
	r.Pad = u
	pad, normal := s.branch(u)
	join := s.newBlock()
	res := join.AddParam(nt)
	iso := nt.Variants[errI].Fields[0].Type
	okI := nt.VariantIndex("Ok")
	// the landing: a fault is caught here, cancellation unwinds on
	s.cur = pad
	caught := s.rt("fault_take", boolT)
	yes, no := s.branch(caught)
	s.cur = no
	s.unwind()
	s.cur = yes
	faultedI := iso.VariantIndex("Faulted")
	ft := iso.Variants[faultedI].Fields[0].Type
	info := s.add(OpStruct, ft, nil, s.rt("fault_msg", strT), s.rt("fault_loc", strT), s.call("list.new", ft.Fields[2].Type, ""))
	s.jump(join, s.add(OpVariant, nt, errI, s.add(OpVariant, iso, faultedI, info)))
	// it returned
	s.cur = normal
	okT := nt.Variants[okI].Fields[0].Type
	if errT == nil {
		s.jump(join, s.add(OpVariant, nt, okI, s.coerce(r, okT, x)))
	} else {
		rOk, rErr := rt.VariantIndex("Ok"), rt.VariantIndex("Err")
		isOk := s.add(OpEq, boolT, nil, s.add(OpTag, m.Builtin(KInt), nil, r), s.cint(int64(rOk)))
		y, n := s.branch(isOk)
		s.cur = y
		s.jump(join, s.add(OpVariant, nt, okI, s.add(OpPayload, rt.Variants[rOk].Fields[0].Type, PayloadRef{rOk, 0}, r)))
		s.cur = n
		failedI := iso.VariantIndex("Failed")
		e := s.add(OpPayload, rt.Variants[rErr].Fields[0].Type, PayloadRef{rErr, 0}, r)
		s.jump(join, s.add(OpVariant, nt, errI, s.add(OpVariant, iso, failedI, e)))
	}
	s.seal(join)
	s.cur = join
	return res
}
