package llvm

import "github.com/xo-lang/xo/internal/ssa"

// Values kept in the memory of a `var` parameter passed by reference
// (decision 0148).
//
// A `var` parameter passed by reference (decision 0145) arrives in the
// caller's memory, which the function also uses as the slot of every
// call that takes a value of that type by reference (its home), and
// which it writes back at return. Decision 0145 loaded the whole value at
// entry, stored it before each call, loaded it again after, and stored
// it at return; field reads were extractvalues of those loads, so every
// field stayed live across every call (the landing pads release them).
//
// held computes, for each instruction, which SSA value that memory holds
// (a forward dataflow over the function: a call's store into its slot,
// the callee's write back of a `var` parameter, a `setfield` done in
// place, block parameters that every edge passes in place). Then:
//
//   - a field read of a held value loads that field from memory;
//   - a `setfield` of the held value stores the field in place;
//   - a call or return that would store the held value into the memory
//     it is already in stores nothing;
//   - a value whose every use finds it held (the parameter, a value
//     updated in place, a `var` result of a call, a block parameter
//     passed in place) is never materialized: a use that needs the whole
//     value loads it there.

// hkey names the value a memory holds: an SSA value, or result k of a
// call (the `var` parameter it wrote back), which the SSA reads with a
// field of the call.
type hkey struct {
	v *ssa.Value
	k int // -1: v itself
}

var (
	hTop = hkey{k: -2} // not reached yet (the dataflow's start)
	hBot = hkey{k: -3} // unknown
)

// keyOf is the key of value v.
func keyOf(v *ssa.Value) hkey {
	if v.Op == ssa.OpField && v.Args[0].Op == ssa.OpCall {
		return hkey{v.Args[0], v.Aux.(int)}
	}
	return hkey{v, -1}
}

// heldMem is a tracked memory: the pointer of a `var` parameter passed
// by reference that is the home of its LLVM type.
type heldMem struct {
	ptr   string
	lt    string
	param *ssa.Value
}

type heldInfo struct {
	mems   []heldMem
	before map[*ssa.Value][]hkey // the state before each instruction
	in     map[*ssa.Block][]hkey
	edge   map[*ssa.Edge][]hkey
	// inPlace marks the setfields done in memory, by memory index
	inPlace map[*ssa.Value]int
	// memOnly marks the values that are never materialized, by memory
	// index
	memOnly map[*ssa.Value]int
}

func (h *heldInfo) memOf(lt string) int {
	for i, m := range h.mems {
		if m.lt == lt {
			return i
		}
	}
	return -1
}

// heldAt reports the memory that holds u before instruction v, or -1.
func (s *sfn) heldAt(v, u *ssa.Value) int {
	h := s.held
	if h == nil {
		return -1
	}
	st, ok := h.before[v]
	if !ok {
		return -1
	}
	m := h.memOf(s.g.ll(u.Type))
	if m >= 0 && st[m] == keyOf(u) {
		return m
	}
	return -1
}

// slotMems returns, for each argument of call v to callee, the tracked
// memory it is stored into (-1 for none), as call lowers it: the first
// argument of each type passed by reference and not by address takes
// the home of that type.
func (s *sfn) slotMems(v *ssa.Value, callee *ssa.Func) []int {
	g := s.g
	br := g.byRefOf(callee)
	out := make([]int, len(v.Args))
	used := map[string]bool{}
	for i, x := range v.Args {
		out[i] = -1
		if br == nil || !br[i] {
			continue
		}
		if s.stable(x) && !isVarParam(callee, i) {
			continue
		}
		lt := g.ll(x.Type)
		if used[lt] {
			continue
		}
		used[lt] = true
		out[i] = s.held.memOf(lt)
	}
	return out
}

// stable reports whether v sits in memory nothing writes during the call
// (sfn.addr, decision 0147), computed before lowering.
func (s *sfn) stable(v *ssa.Value) bool {
	if r, ok := s.stableMemo[v]; ok {
		return r
	}
	r := false
	switch v.Op {
	case ssa.OpParam:
		if v.Block == s.f.Blocks[0] {
			br := s.g.byRefOf(s.f)
			for i, p := range s.f.Params() {
				if p == v && br != nil && br[i] && !isVarParam(s.f, i) {
					r = true
				}
			}
		}
	case ssa.OpPayload:
		r = isAgg(v.Type) && s.stable(v.Args[0])
	case ssa.OpField:
		r = isAgg(v.Type) && s.stable(v.Args[0])
	case ssa.OpUnbox:
		r = isAgg(v.Type) && s.rootBorrowed(v.Args[0])
	case ssa.OpCall:
		if n, _ := v.Aux.(string); n == "list.index" {
			r = isAgg(v.Type) && v.Type.Kind != ssa.KUnit && s.rootBorrowed(v.Args[0])
		}
	}
	s.stableMemo[v] = r
	return r
}

// computeHeld runs the dataflow for the function (nil when it has no
// tracked memory).
func (s *sfn) computeHeld(mems []heldMem) *heldInfo {
	if len(mems) == 0 {
		return nil
	}
	h := &heldInfo{mems: mems, before: map[*ssa.Value][]hkey{}, in: map[*ssa.Block][]hkey{},
		edge: map[*ssa.Edge][]hkey{}, inPlace: map[*ssa.Value]int{}, memOnly: map[*ssa.Value]int{}}
	s.held = h
	n := len(mems)
	top := func() []hkey {
		st := make([]hkey, n)
		for i := range st {
			st[i] = hTop
		}
		return st
	}
	same := func(a, b []hkey) bool {
		for i := range a {
			if a[i] != b[i] {
				return false
			}
		}
		return true
	}
	landed := map[*ssa.Value][]hkey{} // the pad state of each landing
	order := reversePostorder(s.f)
	for changed := true; changed; {
		changed = false
		for _, b := range order {
			st := s.heldIn(b, top)
			if old, ok := h.in[b]; !ok || !same(old, st) {
				h.in[b] = append([]hkey(nil), st...)
				changed = true
			}
			st = append([]hkey(nil), st...)
			for _, v := range b.Instrs {
				h.before[v] = append([]hkey(nil), st...)
				pad := st
				switch v.Op {
				case ssa.OpCall:
					if callee := s.g.byName[v.Aux.(string)]; callee != nil {
						sm := s.slotMems(v, callee)
						for i, m := range sm {
							if m >= 0 {
								st[m] = keyOf(v.Args[i])
							}
						}
						pad = append([]hkey(nil), st...)
						for i, m := range sm {
							if m >= 0 && isVarParam(callee, i) {
								pad[m] = hBot
								for j, vi := range callee.VarParams {
									if vi == i {
										st[m] = hkey{v, 1 + j}
									}
								}
							}
						}
					}
				case ssa.OpSetField:
					u := v.Args[0]
					if m := h.memOf(s.g.ll(u.Type)); m >= 0 && st[m] == keyOf(u) {
						h.inPlace[v] = m
						st[m] = hkey{v, -1}
					}
				case ssa.OpJump:
					s.setEdge(v.Succs[0], st, &changed)
				case ssa.OpBranch:
					if l := v.Args[0]; l.Op == ssa.OpCall && l.Aux == "rt.landing" {
						if ps, ok := landed[l]; ok {
							s.setEdge(v.Succs[0], ps, &changed)
						} else {
							s.setEdge(v.Succs[0], st, &changed)
						}
						s.setEdge(v.Succs[1], st, &changed)
					} else {
						s.setEdge(v.Succs[0], st, &changed)
						s.setEdge(v.Succs[1], st, &changed)
					}
				}
				if v.Pad != nil {
					landed[v.Pad] = append([]hkey(nil), pad...)
				}
			}
		}
	}
	s.findMemOnly()
	return h
}

func (s *sfn) setEdge(e *ssa.Edge, st []hkey, changed *bool) {
	h := s.held
	old, ok := h.edge[e]
	if ok {
		eq := true
		for i := range old {
			if old[i] != st[i] {
				eq = false
			}
		}
		if eq {
			return
		}
	}
	h.edge[e] = append([]hkey(nil), st...)
	*changed = true
}

// heldIn is the state at the start of block b: the parameter's own key in
// the entry block; else, per memory, the block parameter every incoming
// edge passes in place, or the key every edge agrees on.
func (s *sfn) heldIn(b *ssa.Block, top func() []hkey) []hkey {
	h := s.held
	st := top()
	if b == s.f.Blocks[0] {
		for i, m := range h.mems {
			st[i] = keyOf(m.param)
		}
		return st
	}
	for i, m := range h.mems {
		var states []*ssa.Edge
		for _, e := range b.Preds {
			if es, ok := h.edge[e]; ok && es[i] != hTop {
				states = append(states, e)
			}
		}
		if len(states) == 0 {
			continue // hTop
		}
		found := false
		for pi, p := range b.Params {
			if s.g.ll(p.Type) != m.lt {
				continue
			}
			all := true
			for _, e := range states {
				if h.edge[e][i] != keyOf(e.Args[pi]) {
					all = false
				}
			}
			if all {
				st[i] = hkey{p, -1}
				found = true
				break
			}
		}
		if found {
			continue
		}
		k := h.edge[states[0]][i]
		for _, e := range states[1:] {
			if h.edge[e][i] != k {
				k = hBot
			}
		}
		st[i] = k
	}
	return st
}

// findMemOnly marks the values held in a memory at every use: the
// parameter, values updated in place, `var` results of calls, and block
// parameters passed in place.
func (s *sfn) findMemOnly() {
	h := s.held
	cands := map[*ssa.Value]int{}
	for i, m := range h.mems {
		cands[m.param] = i
	}
	for v, m := range h.inPlace {
		cands[v] = m
	}
	for _, b := range s.f.Blocks {
		if st, ok := h.in[b]; ok && b != s.f.Blocks[0] {
			for i, k := range st {
				if k.k == -1 && k.v != nil && k.v.Op == ssa.OpParam && k.v.Block == b {
					cands[k.v] = i
				}
			}
		}
		for _, v := range b.Instrs {
			if v.Op == ssa.OpField && v.Args[0].Op == ssa.OpCall {
				if m := h.memOf(s.g.ll(v.Type)); m >= 0 {
					cands[v] = m
				}
			}
		}
	}
	bad := map[*ssa.Value]bool{}
	for _, b := range s.f.Blocks {
		for _, v := range b.Instrs {
			for _, a := range v.Args {
				m, ok := cands[a]
				if !ok {
					continue
				}
				if st, ok := h.before[v]; !ok || st[m] != keyOf(a) {
					bad[a] = true
				}
			}
			for _, e := range v.Succs {
				for pi, a := range e.Args {
					m, ok := cands[a]
					if !ok {
						continue
					}
					es, ok1 := h.edge[e]
					in, ok2 := h.in[e.To]
					if !ok1 || !ok2 || es[m] != keyOf(a) || in[m] != (hkey{e.To.Params[pi], -1}) {
						bad[a] = true
					}
				}
			}
		}
	}
	for v, m := range cands {
		if !bad[v] {
			h.memOnly[v] = m
		}
	}
}

// memOnly reports whether v is never materialized, and its memory.
func (s *sfn) memOnly(v *ssa.Value) (int, bool) {
	if s.held == nil {
		return -1, false
	}
	m, ok := s.held.memOnly[v]
	return m, ok
}

// heldParam returns the memory that holds block parameter p of b at the
// block's start because every edge passes it in place, or -1.
func (s *sfn) heldParam(b *ssa.Block, p *ssa.Value) int {
	h := s.held
	if h == nil || b == s.f.Blocks[0] {
		return -1
	}
	st, ok := h.in[b]
	if !ok {
		return -1
	}
	for i, k := range st {
		if k == (hkey{p, -1}) {
			return i
		}
	}
	return -1
}

// lazyResult reports whether result k of call v (a `var` parameter the
// callee wrote back into a tracked memory) needs no load after the call:
// the call is only read by field reads, and those of result k are held
// at every use.
func (s *sfn) lazyResult(v *ssa.Value, k int) bool {
	if s.held == nil {
		return false
	}
	for _, u := range s.users(v) {
		if u.Op != ssa.OpField {
			return false
		}
		if u.Aux.(int) == k {
			if _, ok := s.memOnly(u); !ok {
				return false
			}
		}
	}
	return true
}

// users lists the instructions and edges (as their terminators) that use
// v.
func (s *sfn) users(v *ssa.Value) []*ssa.Value {
	if s.uses == nil {
		s.uses = map[*ssa.Value][]*ssa.Value{}
		for _, b := range s.f.Blocks {
			for _, x := range b.Instrs {
				for _, a := range x.Args {
					s.uses[a] = append(s.uses[a], x)
				}
				for _, e := range x.Succs {
					for _, a := range e.Args {
						s.uses[a] = append(s.uses[a], x)
					}
				}
			}
		}
	}
	return s.uses[v]
}
