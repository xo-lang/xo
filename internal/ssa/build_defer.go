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
// A fault does not run deferred code on the native backends: the
// process exits at once (the Go backend runs it while it panics).

// deferFrame is an open block: the `defer` statements met so far.
type deferFrame struct {
	stmts []*ast.DeferStmt
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
	for i := len(fr.stmts) - 1; i >= 0 && !s.isDead(); i-- {
		d := fr.stmts[i]
		var tok *Value
		if s.b.conc {
			tok = s.rt("shield_begin", s.b.mod.Builtin(KInt))
		}
		// deferred code is its own frame for control flow: the loops of
		// the code around it are not its own
		loops := s.loops
		s.loops = nil
		s.shield++
		s.stmt(d.Body)
		s.shield--
		s.loops = loops
		if tok != nil && !s.isDead() {
			s.rt("shield_end", nil, tok, s.add(OpConst, s.b.mod.Builtin(KStr), s.b.pos(d)))
		}
	}
}

// exitTo leaves the open blocks and scopes down to depth (an index into
// s.exits) for an early exit: deferred code runs and scopes close (mode
// 1), innermost first.
func (s *fn) exitTo(depth int) {
	saved := s.exits
	for i := len(saved) - 1; i >= depth && !s.isDead(); i-- {
		s.exits = saved[:i]
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
		s.exits = saved[:i]
		s.runDefers(saved[i].defers)
	}
	s.exits = saved
	if s.isDead() {
		return
	}
	s.jump(s.unwindTarget())
}
