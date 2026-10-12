package ssa

// Borrowed projections (decision 0186).
//
// A projection (`list.index`, `list.get`, `payload`, `capture`, `unbox`)
// out of a tracked container is retained after its definition when it
// lives on and released after its last use: `for x in g.blocks[b].insts`
// dups and drops every heap field of each element, and `a[i][j]` the
// inner list, per iteration. borrowProjections marks a projection
// borrowed instead when its owner (the container leaf, or the owner of a
// borrowed container) stays alive and unchanged wherever the projection
// is alive:
//
//   - no terminator uses it (it is not returned, passed on an edge, or
//     given to a fault), so every use is an instruction the walk can keep
//     the owner alive across;
//   - no instruction from its definition to its last use, on any path,
//     consumes an operand whose root is the owner's root, or passes one
//     on an edge: nothing updates the container in place (an update
//     consumes it: list.set, list.push, list.take of decision 0169, a
//     `setfield` of a `var` binding, a `var` call), moves it, or frees it.
//
// A borrowed projection is never retained or released; a use that
// consumes it retains it first (as a borrowed parameter's), and every
// use borrows its owner too, which extends the owner's liveness to the
// projection's last use. The verifier needs nothing new: it checks such
// a projection as any projection held zero times, alive while its
// container is held.
func (r *rcInfo) borrowProjections(f *Func) {
	r.bproj = map[*Value]leaf{}
	type site struct {
		b *Block
		i int
	}
	pos := map[*Value]int{}
	uses := map[*Value][]site{}
	all := map[*Value][]site{} // uses, terminators and edges included
	cons := map[*Value][]site{}
	inTerm := map[*Value]bool{}
	cand := map[*Value]bool{}
	var cands []*Value
	for _, b := range f.Blocks {
		for i, v := range b.Instrs {
			pos[v] = i
			if isRoot(v) && len(v.Args) > 0 && r.kind(v) == rkProj {
				cand[v] = true
				cands = append(cands, v)
			}
			for k, m := range r.modes(v) {
				if m == mConsume {
					root, _ := base(v.Args[k])
					cons[root] = append(cons[root], site{b, i})
				}
			}
			for _, e := range v.Succs {
				for _, a := range e.Args {
					root, _ := base(a)
					cons[root] = append(cons[root], site{b, i})
					all[root] = append(all[root], site{b, i})
					inTerm[root] = true
				}
			}
			if v.Op.IsTerminator() {
				for _, u := range r.uses(v) {
					all[u.l.root] = append(all[u.l.root], site{b, i})
					inTerm[u.l.root] = true
				}
				continue
			}
			for _, u := range r.uses(v) {
				uses[u.l.root] = append(uses[u.l.root], site{b, i})
				all[u.l.root] = append(all[u.l.root], site{b, i})
			}
		}
	}
	// usedAfter reports whether root is used after instruction s on some
	// path (by its own uses, not those a borrowed projection lends it)
	usedAfter := func(root *Value, s site) bool {
		in := map[*Block]bool{}
		for _, u := range all[root] {
			if u.b == s.b && u.i > s.i {
				return true
			}
			in[u.b] = true
		}
		seen := map[*Block]bool{}
		work := []*Block{s.b}
		for len(work) > 0 {
			b := work[len(work)-1]
			work = work[:len(work)-1]
			for _, e := range b.Term().Succs {
				if seen[e.To] {
					continue
				}
				if in[e.To] {
					return true
				}
				seen[e.To] = true
				work = append(work, e.To)
			}
		}
		return false
	}
	done := map[*Value]bool{}
	var decide func(v *Value)
	decide = func(v *Value) {
		if done[v] {
			return
		}
		done[v] = true
		p := r.parent(v)
		if cand[p.root] {
			decide(p.root)
		}
		owner := p
		if o, ok := r.bproj[p.root]; ok {
			owner = o
		}
		if inTerm[v] || !r.tracked(owner) {
			return
		}
		// the blocks v is live into and out of, and its last use in each
		db, di := v.Block, pos[v]
		liveIn := map[*Block]bool{}
		liveOut := map[*Block]bool{}
		last := map[*Block]int{}
		var work []*Block
		for _, s := range uses[v] {
			if n, ok := last[s.b]; !ok || s.i > n {
				last[s.b] = s.i
			}
			if s.b != db && !liveIn[s.b] {
				liveIn[s.b] = true
				work = append(work, s.b)
			}
		}
		for len(work) > 0 {
			b := work[len(work)-1]
			work = work[:len(work)-1]
			for _, e := range b.Preds {
				pb := e.From
				liveOut[pb] = true
				if pb != db && !liveIn[pb] {
					liveIn[pb] = true
					work = append(work, pb)
				}
			}
		}
		lastIn := func(b *Block) int {
			if n, ok := last[b]; ok {
				return n
			}
			return -1
		}
		for _, s := range cons[owner.root] {
			in := false
			switch {
			case s.b == db:
				in = s.i > di && (liveOut[db] || s.i <= lastIn(db))
			case liveIn[s.b]:
				in = liveOut[s.b] || s.i <= lastIn(s.b)
			}
			if in {
				return
			}
		}
		// a projection consumed where its owner would be dead stays
		// counted: the owner dying first left it unique (a list passed
		// on is updated in place), while borrowing would keep the owner
		// alive and the list shared at the consume
		for _, s := range cons[v] {
			if !usedAfter(owner.root, s) {
				return
			}
		}
		r.bproj[v] = owner
	}
	for _, v := range cands {
		decide(v)
	}
}
