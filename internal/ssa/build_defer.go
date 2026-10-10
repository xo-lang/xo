package ssa

import (
	"github.com/xo-lang/xo/internal/ast"
)

// `defer` (core 3.5, decisions 0009 and 0022) is lowered inline: the
// deferred statements of a block are lowered again at every way out of
// it, last registered first, reading the variables as they are there.
//
//   - The end of the block runs its own deferred code after the block's
//     value is computed.
//   - `return` and `?` run the deferred code of every open block of the
//     function, `break` and `continue` that of the blocks inside the loop;
//     the scopes in between close at their place in the nesting (a scope
//     body's deferred code runs before the scope waits for its tasks).
//   - Unwinding (cancellation, decision 0068) runs the deferred code of
//     the blocks inside the innermost scope, then jumps to that scope's
//     unwind block, which closes it and goes on with the blocks outside.
//
// Deferred code is shielded from cancellation (decision 0022): it runs
// between rt.shield_begin and rt.shield_end, so its cancellation points
// do not report cancellation and it never unwinds (no checks are emitted
// in it). On the unwind path the shield also clears the unwinding flag
// while the deferred code runs and sets it again after. shield_end
// faults with XO-F005 when the deferred code ran longer than 5 seconds.
// A program built without unwinding checks (no tasks, clock, I/O, or
// signals) cannot be canceled, and its deferred code runs without the
// shield calls.
//
// A fault inside deferred code (decision 0085, amended 2026-10-10) goes
// on with the rest of the cleanup, as a panic in Go's deferred code does:
// the faulting statement's shield ends (rt.shield_abort), the deferred
// statements registered before it run, then the blocks and scopes
// outside, and the new fault replaces the one being unwound (the
// runtime). The landing of a faulting operation in deferred statement j
// of a block jumps to faultCont(frame, j), a block shared by every path
// that runs statement j: it runs statement j-1 (whose faults land in
// faultCont(frame, j-1)) and jumps on, down to faultCont(frame, 0),
// which unwinds the code around the block. Each deferred statement is
// lowered once more at most, whatever the number of faulting operations.
// Blocks with deferred code inside the faulting statement run theirs
// first (shielded; a fault in that nested deferred code, or in deferred
// code under a scope opened inside deferred code, keeps the function's
// landing: the references are released, the rest is skipped).

// deferFrame is an open block: the `defer` statements met so far.
type deferFrame struct {
	stmts []*ast.DeferStmt
	// outer are the blocks and scopes around it (as snapshot copies), set
	// when its deferred code first runs; cont are its fault continuations
	// by statement index (faultCont)
	outer *pendingAt
	cont  map[int]*Block
}

// deferLanding is the top level deferred statement being lowered: where
// a fault in it continues.
type deferLanding struct {
	fr   *deferFrame
	j    int
	tok  *Value // its shield token (nil without concurrency)
	base int    // len(s.exits) when it began
}

// exitEntry is an open block (defers) or scope (scope) around the
// current code.
type exitEntry struct {
	defers *deferFrame
	scope  *scopeCtx
}

// deferStmt registers deferred code in the innermost open block.
func (s *fn) deferStmt(x *ast.DeferStmt) {
	for i := len(s.exits) - 1; i >= 0; i-- {
		if fr := s.exits[i].defers; fr != nil {
			fr.stmts = append(fr.stmts, x)
			return
		}
	}
	s.b.unsupported(x, "`defer` outside a block")
}

// runDefers lowers the deferred code of fr, last registered first, each
// statement shielded.
func (s *fn) runDefers(fr *deferFrame) {
	top := s.shield == 0 && s.b.faultUnwind
	if top && fr.outer == nil {
		o := s.snapshot()
		fr.outer = &o
	}
	for i := len(fr.stmts) - 1; i >= 0 && !s.isDead(); i-- {
		s.runDefer(fr, i, top)
	}
}

// runDefer lowers deferred statement i of fr, shielded; top: a fault in it
// continues with the rest of the cleanup (faultCont).
func (s *fn) runDefer(fr *deferFrame, i int, top bool) {
	d := fr.stmts[i]
	var tok *Value
	if s.b.conc {
		tok = s.rt("shield_begin", s.b.mod.Builtin(KInt))
	}
	// deferred code is its own frame for control flow: the loops of
	// the code around it are not its own
	loops := s.loops
	s.loops = nil
	land := s.deferLand
	s.deferLand = nil
	if top {
		s.deferLand = &deferLanding{fr: fr, j: i, tok: tok, base: len(s.exits)}
	}
	s.shield++
	s.stmt(d.Body)
	s.shield--
	s.deferLand = land
	s.loops = loops
	if tok != nil && !s.isDead() {
		s.rt("shield_end", nil, tok, s.add(OpConst, s.b.mod.Builtin(KStr), s.b.pos(d)))
	}
}

// deferLandingHere reports whether an operation added now is in top level
// deferred code that faultCont handles: not nested in other deferred
// code, and no scope opened inside it.
func (s *fn) deferLandingHere() bool {
	dl := s.deferLand
	if dl == nil || s.shield != 1 || len(s.exits) < dl.base {
		return false
	}
	for _, e := range s.exits[dl.base:] {
		if e.scope != nil {
			return false
		}
	}
	return true
}

// faultCont returns the block where a fault in deferred statement j of fr
// continues: statement j-1 runs, then faultCont(fr, j-1); at 0 the code
// around the block unwinds. Made once per frame and index; sealed when
// the function ends (sealConts).
func (s *fn) faultCont(fr *deferFrame, j int) *Block {
	if s.pending != nil {
		// the pending operation lands in the current block, before this
		// builds code elsewhere (a jump to the result comes after)
		s.flushLanding()
	}
	if b := fr.cont[j]; b != nil {
		return b
	}
	b := s.newBlock()
	if fr.cont == nil {
		fr.cont = map[int]*Block{}
	}
	fr.cont[j] = b
	s.conts = append(s.conts, b)
	cur, exits, scopes, land, shield, loops := s.cur, s.exits, s.scopes, s.deferLand, s.shield, s.loops
	s.cur, s.deferLand, s.shield, s.loops = b, nil, 0, nil
	s.exits = fr.outer.exits
	// the scopes open around the block: those of its exits (a scope
	// closed early by a return is not among them any more)
	s.scopes = nil
	for _, e := range s.exits {
		if e.scope != nil {
			s.scopes = append(s.scopes, e.scope)
		}
	}
	if j > 0 {
		s.runDefer(fr, j-1, true)
		if !s.isDead() {
			s.jump(s.faultCont(fr, j-1))
		}
	} else {
		s.unwind()
	}
	s.cur, s.exits, s.scopes, s.deferLand, s.shield, s.loops = cur, exits, scopes, land, shield, loops
	return b
}

// sealConts seals the fault continuations once every landing that jumps
// to them is built (the end of the function).
func (s *fn) sealConts() {
	for _, b := range s.conts {
		s.seal(b)
	}
	s.conts = nil
}

// exitTo leaves the open blocks and scopes down to depth (an index into
// s.exits) for an early exit: deferred code runs and scopes close (mode
// 1), innermost first.
func (s *fn) exitTo(depth int) {
	saved := s.exits
	for i := len(saved) - 1; i >= depth && !s.isDead(); i-- {
		s.exits = saved[:i:i] // appends must not overwrite saved
		if e := saved[i]; e.defers != nil {
			s.runDefers(e.defers)
		} else {
			s.closeEarly(e.scope)
		}
	}
	s.exits = saved
}

// unwind ends the current block on the unwind path: the deferred code of
// the blocks inside the innermost scope runs, then the code jumps to that
// scope's unwind block (which closes it and goes on), or the function's.
func (s *fn) unwind() {
	saved := s.exits
	for i := len(saved) - 1; i >= 0 && saved[i].scope == nil && !s.isDead(); i-- {
		s.exits = saved[:i:i] // appends must not overwrite saved
		s.runDefers(saved[i].defers)
	}
	s.exits = saved
	if s.isDead() {
		return
	}
	s.jump(s.unwindTarget())
}
