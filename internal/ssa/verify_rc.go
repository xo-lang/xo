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
	var alive func(st rcState, l leaf) bool
	alive = func(st rcState, l leaf) bool {
		switch r.kind(l.root) {
		case rkBorrowed:
			return true
		case rkProj:
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
				for _, e := range v.Succs {
					est := st.clone()
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
					if old, ok := entry[e.To]; ok {
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
