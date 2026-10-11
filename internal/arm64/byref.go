package arm64

import (
	"fmt"

	"github.com/xo-lang/xo/internal/ssa"
)

// Large `var` struct parameters passed by reference and kept in memory
// (decision 0151, the arm64 port of decisions 0145 and 0148).
//
// The plain convention passes a struct as its words (x0 to x7, d0 to
// d7, then stack slots) and returns a `var` parameter's new value as
// more result words, which the callee stores in the caller's stack
// argument area. A struct of 300 words (the self hosted SSA builder's
// Builder, a `var` receiver of almost every function) is copied through
// the stack twice per call.
//
// A `var` parameter of a direct call whose type is a struct of byRefMin
// bytes or more is passed instead as one integer word: the address of
// memory that holds the value, 8 bytes per word in layout order. The
// callee updates that memory and its result is no longer among the
// result words. In the callee, the first such parameter of each type is
// the home of the type: every call of the function that passes a value
// of that type by reference passes the home's address (a chain of `var`
// calls updates one memory). A forward dataflow (computeHeld, ported
// from internal/llvm/held.go) tells which SSA value the home holds
// before each instruction; then:
//
//   - a field read of a value held at every use (memOnly) loads that
//     field from memory;
//   - a `setfield` of the held value stores the field in place;
//   - a call or return that would store the held value into the memory
//     it is already in stores nothing;
//   - a memOnly value is never loaded whole into registers; a use that
//     needs it whole (a by value argument, a release) loads it there.
//
// Other by reference arguments (a function with no home of the type, a
// second argument of the type in one call) are stored into memory of the
// caller's frame (frameInfo.memOff) and loaded back after the call. A
// second by reference parameter of one type is loaded at entry and
// stored back at every return.

// byRefMin is the size in bytes from which a `var` struct parameter of a
// direct call is passed by reference.
const byRefMin = 256

// byRefOf returns which parameters of f are passed by reference, or nil.
// Functions called through function values (closure bodies, wrappers,
// task entries, functions used as values) and main keep the plain
// convention, as does Go mode.
func (g *gen) byRefOf(f *ssa.Func) []bool {
	if g.goMode || g.byRef == nil {
		return nil
	}
	if br, ok := g.byRef[f]; ok {
		return br
	}
	var br []bool
	if !f.Indirect && !f.Entry && !g.addrTaken[f.Name] && f.Name != "main" {
		for i, p := range f.Params() {
			t := p.Type
			if t.Kind == ssa.KStruct && 8*g.nwords(t) >= byRefMin {
				if br == nil {
					br = make([]bool, len(f.Params()))
				}
				br[i] = true
			}
		}
	}
	g.byRef[f] = br
	return br
}

// keptResults are the indexes of the results of f that come back in
// result words: all but those of `var` parameters passed by reference.
func (g *gen) keptResults(f *ssa.Func) []int {
	br := g.byRefOf(f)
	var out []int
	for k := range f.Results {
		if k > 0 && br != nil && br[f.VarParams[k-1]] {
			continue
		}
		out = append(out, k)
	}
	return out
}

// findAddrTaken records the functions used as values.
func (g *gen) findAddrTaken() {
	g.addrTaken = map[string]bool{}
	g.byRef = map[*ssa.Func][]bool{}
	for _, f := range g.m.Funcs {
		for _, b := range f.Blocks {
			for _, v := range b.Instrs {
				switch v.Op {
				case ssa.OpClosure, ssa.OpFuncAddr, ssa.OpRebind:
					if n, ok := v.Aux.(string); ok {
						g.addrTaken[n] = true
					}
				}
			}
		}
	}
}

// varParamOf returns the index of the `var` parameter whose new value is
// result k (k >= 1) of f.
func varParamOf(f *ssa.Func, k int) int { return f.VarParams[k-1] }

// isVarParam reports whether parameter i of f is `var`.
func isVarParam(f *ssa.Func, i int) bool {
	for _, j := range f.VarParams {
		if j == i {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// The dataflow (internal/llvm/held.go)

// hkey names the value a memory holds: an SSA value, or result k of a
// call (the `var` parameter it wrote back), which the SSA reads with a
// field of the call.
type hkey struct {
	v *ssa.Value
	k int // -1: v itself
}

var (
	hTop = hkey{k: -2} // not reached yet
	hBot = hkey{k: -3} // unknown
)

func keyOf(v *ssa.Value) hkey {
	if v.Op == ssa.OpField && v.Args[0].Op == ssa.OpCall {
		return hkey{v.Args[0], v.Aux.(int)}
	}
	return hkey{v, -1}
}

// hmem is a tracked memory: a `var` parameter passed by reference that
// is the home of its type.
type hmem struct {
	t     *ssa.Type
	param *ssa.Value
	ptr   vreg // the address, defined at entry
}

type heldInfo struct {
	mems    []hmem
	before  map[*ssa.Value][]hkey
	in      map[*ssa.Block][]hkey
	edge    map[*ssa.Edge][]hkey
	inPlace map[*ssa.Value]int
	memOnly map[*ssa.Value]int
	uses    map[*ssa.Value][]*ssa.Value
}

func (h *heldInfo) memOf(t *ssa.Type) int {
	for i, m := range h.mems {
		if m.t == t {
			return i
		}
	}
	return -1
}

// inPlaceOf reports whether setfield v is done in place, and its
// memory.
func (h *heldInfo) inPlaceOf(v *ssa.Value) (int, bool) {
	if h == nil {
		return -1, false
	}
	m, ok := h.inPlace[v]
	return m, ok
}

// stableOf reports whether v is a read only parameter passed by
// reference, and its address.
func (s *fstate) stableOf(v *ssa.Value) (vreg, bool) {
	ptr, ok := s.stable[v]
	return ptr, ok
}

// heldAt reports the memory that holds u before instruction v, or -1.
func (s *fstate) heldAt(v, u *ssa.Value) int {
	h := s.held
	if h == nil {
		return -1
	}
	st, ok := h.before[v]
	if !ok {
		return -1
	}
	m := h.memOf(u.Type)
	if m >= 0 && st[m] == keyOf(u) {
		return m
	}
	return -1
}

// slotMems returns, for each argument of call v to callee, the tracked
// memory it is passed in (-1: memory of the frame, or not by reference):
// the first argument of each type passed by reference takes the home of
// that type.
func (s *fstate) slotMems(v *ssa.Value, callee *ssa.Func) []int {
	br := s.g.byRefOf(callee)
	out := make([]int, len(v.Args))
	used := map[*ssa.Type]bool{}
	for i, x := range v.Args {
		out[i] = -1
		if br == nil || !br[i] || !isVarParam(callee, i) || used[x.Type] {
			continue
		}
		used[x.Type] = true
		if s.held != nil {
			out[i] = s.held.memOf(x.Type)
		}
	}
	return out
}

// computeHeld runs the dataflow (nil when the function has no home).
func (s *fstate) computeHeld(mems []hmem) *heldInfo {
	if len(mems) == 0 {
		return nil
	}
	h := &heldInfo{mems: mems, before: map[*ssa.Value][]hkey{}, in: map[*ssa.Block][]hkey{},
		edge: map[*ssa.Edge][]hkey{}, inPlace: map[*ssa.Value]int{}, memOnly: map[*ssa.Value]int{}}
	s.held = h
	n := len(mems)
	same := func(a, b []hkey) bool {
		for i := range a {
			if a[i] != b[i] {
				return false
			}
		}
		return true
	}
	landed := map[*ssa.Value][]hkey{} // the pad state of each landing
	order := rpo(s.f)
	for changed := true; changed; {
		changed = false
		for _, b := range order {
			st := s.heldIn(b, n)
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
							if m >= 0 {
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
					if m := h.memOf(u.Type); m >= 0 && st[m] == keyOf(u) {
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

func (s *fstate) setEdge(e *ssa.Edge, st []hkey, changed *bool) {
	h := s.held
	if old, ok := h.edge[e]; ok {
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

// heldIn is the state at the start of block b: the parameter's own key
// in the entry block; else, per memory, the block parameter every
// incoming edge passes in place, or the key every edge agrees on.
func (s *fstate) heldIn(b *ssa.Block, n int) []hkey {
	h := s.held
	st := make([]hkey, n)
	for i := range st {
		st[i] = hTop
	}
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
			continue
		}
		found := false
		for pi, p := range b.Params {
			if p.Type != m.t {
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
func (s *fstate) findMemOnly() {
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
				if m := h.memOf(v.Type); m >= 0 {
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

// memOnlyOf reports whether v is never in registers, and its memory.
func (s *fstate) memOnlyOf(v *ssa.Value) (int, bool) {
	if s.held == nil {
		return -1, false
	}
	m, ok := s.held.memOnly[v]
	return m, ok
}

// lazyResult reports whether result k of call v (a `var` parameter the
// callee wrote back into a tracked memory) needs no load after the call:
// the call is only read by field reads, and those of result k are held
// at every use.
func (s *fstate) lazyResult(v *ssa.Value, k int) bool {
	if s.held == nil {
		return false
	}
	h := s.held
	if h.uses == nil {
		h.uses = map[*ssa.Value][]*ssa.Value{}
		for _, b := range s.f.Blocks {
			for _, x := range b.Instrs {
				for _, a := range x.Args {
					h.uses[a] = append(h.uses[a], x)
				}
				for _, e := range x.Succs {
					for _, a := range e.Args {
						h.uses[a] = append(h.uses[a], x)
					}
				}
			}
		}
	}
	for _, u := range h.uses[v] {
		if u.Op != ssa.OpField {
			return false
		}
		if u.Aux.(int) == k {
			if _, ok := s.memOnlyOf(u); !ok {
				return false
			}
		}
	}
	return true
}

// ---------------------------------------------------------------------------
// Lowering helpers

// memRef returns a base register and an offset that reach byte off of
// the memory at base (ldr and str scale a 12 bit offset by 8).
func (s *fstate) memAt(base vreg, off int) (vreg, int) {
	if off <= 32760 {
		return base, off
	}
	hi := off &^ 4095
	r := s.def1(false, fmt.Sprintf("add $d0, $u0, #%d, lsl #12", hi>>12), base)
	return r, off - hi
}

// loadWords loads n words of layout lay starting at word w0 of the
// memory at base.
func (s *fstate) loadWords(base vreg, w0 int, lay []bool) []vreg {
	out := make([]vreg, len(lay))
	for k, fp := range lay {
		b, off := s.memAt(base, 8*(w0+k))
		out[k] = s.def1(fp, fmt.Sprintf("ldr $d0, [$u0, #%d]", off), b)
	}
	return out
}

// storeWordsAt stores ws as words w0, w0+1, ... of the memory at base.
func (s *fstate) storeWordsAt(base vreg, w0 int, ws []vreg) {
	for k, w := range ws {
		b, off := s.memAt(base, 8*(w0+k))
		s.store(fmt.Sprintf("str $u0, [$u1, #%d]", off), w, b)
	}
}

// frameArea returns the offset in the frame's memory of the n-th area
// for values of type t (one per argument of the type in a call).
func (s *fstate) frameArea(t *ssa.Type, n int) int {
	if s.areas == nil {
		s.areas = map[*ssa.Type][]int{}
	}
	for len(s.areas[t]) <= n {
		s.areas[t] = append(s.areas[t], s.memBytes)
		s.memBytes += 8 * s.g.nwords(t)
	}
	return s.areas[t][n]
}

// areaAddr puts the address of byte off of the frame's memory in a
// register.
func (s *fstate) areaAddr(off int) vreg {
	d := s.newVreg(false)
	s.pmoveSrcs([]vreg{d}, []src{{kind: sMem, imm: int64(off)}})
	return d
}

// byRefEntry sets up the parameters passed by reference at entry: the
// homes (computeHeld), and the words of the parameters that are not
// held at every use, loaded from their memory. ptrs are the address
// registers by parameter index.
func (s *fstate) byRefEntry(ptrs map[int]vreg) {
	params := s.f.Params()
	for i, p := range params {
		ptr, ok := ptrs[i]
		if !ok {
			continue
		}
		if !isVarParam(s.f, i) {
			// read only: nothing writes its memory during the call
			s.stable[p] = ptr
			continue
		}
		m := s.held.memOf(p.Type)
		if m >= 0 && s.held.mems[m].param == p {
			if _, only := s.memOnlyOf(p); only {
				continue
			}
		}
		s.vals[p] = s.loadWords(ptr, 0, s.g.layout(p.Type))
	}
}

// planHeld finds the homes of the function and runs the dataflow; it
// returns the address registers of the parameters passed by reference.
func (s *fstate) planHeld() map[int]vreg {
	br := s.g.byRefOf(s.f)
	if br == nil {
		return nil
	}
	ptrs := map[int]vreg{}
	var mems []hmem
	seen := map[*ssa.Type]bool{}
	for i, p := range s.f.Params() {
		if !br[i] {
			continue
		}
		ptrs[i] = s.newVreg(false)
		if !isVarParam(s.f, i) {
			continue
		}
		if !seen[p.Type] {
			seen[p.Type] = true
			mems = append(mems, hmem{t: p.Type, param: p, ptr: ptrs[i]})
		}
	}
	s.stable = map[*ssa.Value]vreg{}
	s.computeHeld(mems)
	if s.held == nil {
		s.held = &heldInfo{}
	}
	s.refPtr = ptrs
	return ptrs
}

// byRefCall lowers a direct call of callee, which takes parameters by
// reference.
func (s *fstate) byRefCall(v *ssa.Value, callee *ssa.Func) {
	g := s.g
	br := g.byRefOf(callee)
	sm := s.slotMems(v, callee)
	// a read only argument already in memory nothing writes during the
	// call is passed in place: a read only parameter of this function,
	// or a home no `var` argument of the call is passed in
	inPlace := map[int]vreg{}
	for i, x := range v.Args {
		if !br[i] || isVarParam(callee, i) {
			continue
		}
		if ptr, ok := s.stable[x]; ok {
			inPlace[i] = ptr
			continue
		}
		if m := s.heldAt(v, x); m >= 0 {
			free := true
			for _, m2 := range sm {
				free = free && m2 != m
			}
			if free {
				inPlace[i] = s.held.mems[m].ptr
			}
		}
	}
	// read every operand before anything is stored
	words := make([][]vreg, len(v.Args))
	for i, x := range v.Args {
		if br[i] && sm[i] >= 0 && s.heldAt(v, x) == sm[i] {
			continue // already in the memory it is passed in
		}
		if _, ok := inPlace[i]; ok {
			continue
		}
		words[i] = s.words(x)
	}
	var args []src
	area := map[int]int{} // frame memory offset by argument
	nth := map[*ssa.Type]int{}
	for i, x := range v.Args {
		if !br[i] {
			args = append(args, regs(words[i]...)...)
			continue
		}
		if ptr, ok := inPlace[i]; ok {
			args = append(args, rsrc(ptr))
			continue
		}
		if m := sm[i]; m >= 0 {
			ptr := s.held.mems[m].ptr
			if words[i] != nil {
				s.storeWordsAt(ptr, 0, words[i])
			}
			args = append(args, rsrc(ptr))
			continue
		}
		off := s.frameArea(x.Type, nth[x.Type])
		nth[x.Type]++
		area[i] = off
		addr := s.areaAddr(off)
		s.storeWordsAt(addr, 0, words[i])
		args = append(args, rsrc(addr))
	}
	kept := g.keptResults(callee)
	var res []bool
	for _, k := range kept {
		res = append(res, g.layout(callee.Results[k])...)
	}
	defs := s.call(g.fname(callee.Name), args, res)
	// the words of every result, as the SSA reads them
	var out []vreg
	at := 0
	ki := 0
	for k, rt := range callee.Results {
		n := g.nwords(rt)
		if ki < len(kept) && kept[ki] == k {
			out = append(out, defs[at:at+n]...)
			at += n
			ki++
			continue
		}
		i := varParamOf(callee, k)
		if m := sm[i]; m >= 0 {
			if s.lazyResult(v, k) {
				if s.lazyRes == nil {
					s.lazyRes = map[hkey]bool{}
				}
				s.lazyRes[hkey{v, k}] = true
				out = append(out, make([]vreg, n)...)
				continue
			}
			out = append(out, s.loadWords(s.held.mems[m].ptr, 0, g.layout(rt))...)
			continue
		}
		out = append(out, s.loadWords(s.areaAddr(area[i]), 0, g.layout(rt))...)
	}
	s.set(v, out...)
}

// byRefReturn lowers a return of a function with parameters passed by
// reference: the new values of those are stored through their addresses
// (unless the memory already holds them), the other results returned
// in words.
func (s *fstate) byRefReturn(v *ssa.Value) {
	g := s.g
	f := s.f
	kept := g.keptResults(f)
	params := f.Params()
	type wb struct {
		ptr vreg
		ws  []vreg
	}
	var wbs []wb
	isKept := map[int]bool{}
	for _, k := range kept {
		isKept[k] = true
	}
	// read everything first: two parameters of one type may swap through
	// the home
	for k, x := range v.Args {
		if isKept[k] {
			continue
		}
		i := varParamOf(f, k)
		ptr := s.refPtr[i]
		if m := s.held.memOf(params[i].Type); m >= 0 && s.held.mems[m].param == params[i] && s.heldAt(v, x) == m {
			continue
		}
		wbs = append(wbs, wb{ptr, s.words(x)})
	}
	var uses []vreg
	for _, k := range kept {
		uses = append(uses, s.words(v.Args[k])...)
	}
	for _, w := range wbs {
		s.storeWordsAt(w.ptr, 0, w.ws)
	}
	in := &inst{kind: iRet, srcs: regs(uses...)}
	for _, u := range uses {
		if u != 0 {
			in.uses = append(in.uses, u)
		}
	}
	s.add(in)
}
