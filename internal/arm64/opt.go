package arm64

// Local optimizations of instruction selection and MIR.
//
//   - Per MIR block memos: constants, list lengths, bounds checks done,
//     and element addresses are computed once per block. They are keyed
//     by vreg, which is sound because a vreg holds one SSA value at a
//     time within a block (list.set results alias their list: same cell,
//     same length; list.pop also aliases its list but changes the
//     length, so it drops the length memos of that list).
//   - Stores of list.set that write back a word loaded from the same
//     address with no store or call in between are dropped (updating one
//     field of a struct element writes one word).
//   - Dead code: instructions without side effects whose definitions are
//     never used are deleted (a field read of a list element loads one
//     word, not the whole element).

type memAt struct {
	base  vreg
	off   int
	epoch int
}

type memo struct {
	blk     *mblock
	consts  map[any]vreg
	lens    map[vreg]vreg
	checked map[[2]vreg]bool
	bases   map[[3]int64]vreg
	loaded  map[vreg]memAt
	epoch   int
}

// m returns the memo of the current block.
func (s *fstate) m() *memo {
	if s.mm == nil || s.mm.blk != s.cur {
		s.mm = &memo{blk: s.cur, consts: map[any]vreg{}, lens: map[vreg]vreg{}, checked: map[[2]vreg]bool{},
			bases: map[[3]int64]vreg{}, loaded: map[vreg]memAt{}}
	}
	return s.mm
}

// clobber records a store or a call: loads before it may be stale.
func (s *fstate) clobber() { s.m().epoch++ }

// store adds a memory write.
func (s *fstate) store(text string, uses ...vreg) {
	for i := 1; i < len(uses); i++ {
		uses[i] = s.real(uses[i]) // the address; the value may be xzr
	}
	s.add(&inst{kind: iPlain, text: text, uses: uses})
	s.clobber()
}

// thread retargets branches to blocks that only jump.
func (s *fstate) thread() {
	blocks := append(append([]*mblock(nil), s.hot...), s.cold...)
	final := func(b *mblock) *mblock {
		for i := 0; i < 16; i++ {
			if len(b.insts) != 1 || b.insts[0].kind != iJump || b.insts[0].targets[0] == b {
				return b
			}
			b = b.insts[0].targets[0]
		}
		return b
	}
	for _, b := range blocks {
		if t := b.term(); t != nil && (t.kind == iJump || t.kind == iCondBr) {
			for i, x := range t.targets {
				t.targets[i] = final(x)
			}
		}
	}
	// drop blocks nothing reaches
	seen := map[*mblock]bool{}
	var walk func(b *mblock)
	walk = func(b *mblock) {
		if seen[b] {
			return
		}
		seen[b] = true
		if t := b.term(); t != nil && (t.kind == iJump || t.kind == iCondBr) {
			for _, x := range t.targets {
				walk(x)
			}
		}
	}
	walk(s.hot[0])
	keep := func(bs []*mblock) []*mblock {
		out := bs[:0]
		for _, b := range bs {
			if seen[b] {
				out = append(out, b)
			}
		}
		return out
	}
	s.hot, s.cold = keep(s.hot), keep(s.cold)
}

// dce deletes plain instructions without a fault whose definitions are
// all unused, to a fixpoint.
func (s *fstate) dce() {
	blocks := append(append([]*mblock(nil), s.hot...), s.cold...)
	for {
		used := make([]int, len(s.fp))
		for _, b := range blocks {
			for _, in := range b.insts {
				for _, u := range in.uses {
					used[u]++
				}
			}
		}
		changed := false
		for _, b := range blocks {
			out := b.insts[:0]
			for _, in := range b.insts {
				dead := in.kind == iPlain && in.fault == nil && len(in.defs) > 0
				for _, d := range in.defs {
					if used[d] > 0 {
						dead = false
					}
				}
				if in.kind == iPmove {
					// drop moves into unused registers
					k := 0
					for i, d := range in.defs {
						if used[d] > 0 {
							in.defs[k] = d
							in.srcs[k] = in.srcs[i]
							k++
						}
					}
					if k < len(in.defs) {
						changed = true
						in.defs, in.srcs = in.defs[:k], in.srcs[:k]
						in.uses = in.uses[:0]
						for _, a := range in.srcs {
							if a.kind == sReg && a.v != 0 {
								in.uses = append(in.uses, a.v)
							}
						}
					}
					if k == 0 {
						continue
					}
				}
				if dead {
					changed = true
					continue
				}
				out = append(out, in)
			}
			b.insts = out
		}
		if !changed {
			return
		}
	}
}
