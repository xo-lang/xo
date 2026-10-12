package ssa

import (
	"fmt"
	"strings"
)

// rcState counts the references a function holds per leaf at one point.
type rcState map[leaf]int

func (s rcState) clone() rcState {
	out := rcState{}
	for l, n := range s {
		if n != 0 {
			out[l] = n
		}
	}
	return out
}

func (s rcState) String() string {
	var parts []string
	for _, l := range sortedLeaves(func() leafSet {
		ls := leafSet{}
		for l, n := range s {
			if n != 0 {
				ls[l] = true
			}
		}
		return ls
	}()) {
		parts = append(parts, fmt.Sprintf("%s:%d", l, s[l]))
	}
	return "{" + strings.Join(parts, " ") + "}"
}

func (s rcState) equal(t rcState) bool {
	for l, n := range s {
		if n != 0 && t[l] != n {
			return false
		}
	}
	for l, n := range t {
		if n != 0 && s[l] != n {
			return false
		}
	}
	return true
}

// stops reports a block that ends the program (unreachable or a fault).
func stops(b *Block) bool {
	t := b.Term()
	return t != nil && (t.Op == OpUnreachable || t.Op == OpFault)
}

// verifyRC checks that a function after InsertRC balances ownership on
// every path: each owned leaf is consumed or released exactly once
// before `ret` (paths that end in a fault are exempt), nothing is used
// after it is consumed, predecessors agree on what is held at each
// block, and every list update gets a list held exactly once that is
// unique (a `reuse` result or provably unique).
func (m *Module) verifyRC(f *Func) []string {
	r := newRCInfo(m)
	var errs []string
	bad := func(b *Block, format string, args ...any) {
		errs = append(errs, f.Name+": "+b.Name()+": "+fmt.Sprintf(format, args...))
	}
	uniq := r.unique(f)
	var faults map[*Func]bool
	if m.FaultUnwind {
		faults = m.faults
		if faults == nil {
			faults = m.mayFault()
		}
	}
	var alive func(st rcState, l leaf) bool
	alive = func(st rcState, l leaf) bool {
		switch r.kind(l.root) {
		case rkBorrowed:
			return true
		case rkProj, rkBProj:
			if st[l] > 0 {
				return true
			}
			p := r.parent(l.root)
			return alive(st, p)
		}
		return st[l] > 0
	}
	entry := map[*Block]rcState{}
	st0 := rcState{}
	for _, p := range f.Params() {
		if isRoot(p) && r.kind(p) == rkOwned {
			for _, l := range r.leaves(p) {
				st0[l] = 1
			}
		}
	}
	entry[f.Blocks[0]] = st0
	var order []*Block
	seen := map[*Block]bool{}
	var walk func(b *Block)
	walk = func(b *Block) {
		seen[b] = true
		if t := b.Term(); t != nil {
			for _, e := range t.Succs {
				if !seen[e.To] {
					walk(e.To)
				}
			}
		}
		order = append(order, b)
	}
	walk(f.Blocks[0])
	for i := len(order) - 1; i >= 0; i-- {
		b := order[i]
		st := entry[b].clone()
		for _, v := range b.Instrs {
			switch v.Op {
			case OpRetain:
				for _, l := range r.leaves(v.Args[0]) {
					if !alive(st, l) {
						bad(b, "retain of %s, which is not alive", l)
					}
					st[l]++
				}
				continue
			case OpRelease:
				for _, l := range r.leaves(v.Args[0]) {
					if st[l] < 1 {
						bad(b, "release of %s, which is not held", l)
					}
					st[l]--
				}
				continue
			}
			if v.Op == OpCall && updateCalls[v.Aux.(string)] {
				a := v.Args[0]
				for _, l := range r.leaves(a) {
					if st[l] != 1 {
						bad(b, "%s updates %s held %d times, want once", v.Name(), l, st[l])
					}
					if a.Op != OpReuse && !uniq(l) {
						bad(b, "%s updates %s, which is neither a reuse result nor provably unique", v.Name(), l)
					}
				}
			}
			for _, u := range r.uses(v) {
				if u.m == mBorrow && !alive(st, u.l) {
					bad(b, "%s uses %s, which is not alive", v.Name(), u.l)
				}
			}
			for _, u := range r.uses(v) {
				if u.m != mConsume {
					continue
				}
				if st[u.l] < 1 {
					bad(b, "%s consumes %s, which is not held", v.Name(), u.l)
				}
				st[u.l]--
			}
			if faults != nil && v.Pad == nil && !v.Op.IsTerminator() && raises(m, v, faults) {
				// a fault here leaves the function without cleanup
				// (decision 0085): nothing may be held
				for l, n := range st {
					if n > 0 {
						bad(b, "%s can fault while %s is held %d times (no landing releases it)", v.Name(), l, n)
					}
				}
				if keepsOperands(v) {
					for _, l := range consumedLeaves(r, v) {
						bad(b, "%s can fault before it takes %s (no landing releases it)", v.Name(), l)
					}
				}
			}
			if isRoot(v) {
				for _, l := range r.leaves(v) {
					if st[l] != 0 {
						bad(b, "%s is still held %d times when it is defined again (a leak around a loop)", l, st[l])
					}
					st[l] = 0
					if r.kind(v) == rkOwned {
						st[l] = 1
					}
				}
			}
			switch v.Op {
			case OpReturn:
				for l, n := range st {
					if n != 0 {
						bad(b, "%s is held %d times at ret (leak or imbalance)", l, n)
					}
				}
			case OpJump, OpBranch:
				site, landing := landingSite(b)
				for i, e := range v.Succs {
					est := st.clone()
					if i == 0 && landing != nil && v.Op == OpBranch {
						// the cleanup path of a landing check: the operation
						// checked produced nothing (decision 0085); a call
						// inlined since (internal/arm64 inline.go) left its
						// results as the block's parameters (not the entry
						// block's: those are the function's, and a check
						// of rt.unwinding without a landing, as in a task
						// runner built without fault unwinding, keeps them)
						for l := range est {
							if l.root == site || (site == nil && l.root.Op == OpParam && l.root.Block == b && b != f.Blocks[0]) {
								delete(est, l)
							}
						}
						if site != nil && keepsOperands(site) {
							// it faulted before taking its operands
							for _, l := range consumedLeaves(r, site) {
								est[l]++
							}
						}
					}
					for _, l := range r.edgeUses(e) {
						if est[l] < 1 {
							bad(b, "edge to %s passes %s, which is not held", e.To.Name(), l)
						}
						est[l]--
					}
					for _, p := range e.To.Params {
						if isRoot(p) {
							for _, l := range r.leaves(p) {
								if est[l] != 0 {
									bad(b, "edge to %s: the previous %s is still held %d times (a leak around a loop)", e.To.Name(), l, est[l])
								}
								est[l] = 1
							}
						}
					}
					if old, ok := entry[e.To]; ok && stops(e.To) {
						// the program stops there: paths need not agree
						// (InsertRC releases nothing on them)
						for l, n := range est {
							if n > old[l] {
								old[l] = n
							}
						}
					} else if ok {
						if !old.equal(est) {
							bad(b, "edge to %s holds %s, another predecessor %s", e.To.Name(), est, old)
						}
					} else {
						entry[e.To] = est.clone()
					}
				}
			}
		}
	}
	return errs
}
