package arm64

import (
	"math"
	"math/bits"
	"sort"
)

// Physical registers: 0 to 30 are x0 to x30, 32 to 63 are d0 to d31.
const fpBase = 32

// Allocatable registers, in order of preference. x15, x16, x17 and d29
// to d31 are scratch registers of the emitter (reloads of spilled
// registers, parallel moves); x18 is reserved by Apple; x29 and x30 are
// the frame pointer and the link register.
var (
	gpCaller = []int{9, 10, 11, 12, 13, 14, 8, 7, 6, 5, 4, 3, 2, 1, 0}
	gpCallee = []int{19, 20, 21, 22, 23, 24, 25, 26, 27, 28}
	fpCaller = []int{fpBase + 16, fpBase + 17, fpBase + 18, fpBase + 19, fpBase + 20, fpBase + 21, fpBase + 22,
		fpBase + 23, fpBase + 24, fpBase + 25, fpBase + 26, fpBase + 27, fpBase + 28,
		fpBase + 7, fpBase + 6, fpBase + 5, fpBase + 4, fpBase + 3, fpBase + 2, fpBase + 1, fpBase + 0}
	fpCallee = []int{fpBase + 8, fpBase + 9, fpBase + 10, fpBase + 11, fpBase + 12, fpBase + 13, fpBase + 14, fpBase + 15}
)

// Go mode (goasm.go): Go's ABI0 saves no register across a call, and
// x18 (platform), x26 (closure context), x27 (the assembler's
// temporary), x28 (g), x29, and x30 are never touched; every other
// register is allocatable and clobbered by calls.
var (
	goGP = []int{9, 10, 11, 12, 13, 14, 8, 7, 6, 5, 4, 3, 2, 1, 0, 19, 20, 21, 22, 23, 24, 25}
	goFP = []int{fpBase + 16, fpBase + 17, fpBase + 18, fpBase + 19, fpBase + 20, fpBase + 21, fpBase + 22,
		fpBase + 23, fpBase + 24, fpBase + 25, fpBase + 26, fpBase + 27, fpBase + 28,
		fpBase + 7, fpBase + 6, fpBase + 5, fpBase + 4, fpBase + 3, fpBase + 2, fpBase + 1, fpBase + 0,
		fpBase + 8, fpBase + 9, fpBase + 10, fpBase + 11, fpBase + 12, fpBase + 13, fpBase + 14, fpBase + 15}
)

func isCallee(r int) bool { return (r >= 19 && r <= 28) || (r >= fpBase+8 && r <= fpBase+15) }

// regSets returns the caller saved and callee saved registers of the
// target convention, in order of preference.
func (s *fstate) regSets() (gpC, gpE, fpC, fpE []int) {
	if s.g.goMode {
		return goGP, nil, goFP, nil
	}
	return gpCaller, gpCallee, fpCaller, fpCallee
}

// loc is where a vreg lives: a physical register, or a spill slot.
type loc struct {
	reg  int // -1 when spilled
	slot int
}

// rng is a closed range of positions. Instruction i reads its uses at
// 4i, the registers a call clobbers die at 4i+1, and it writes its
// definitions at 4i+2 (at 4i when early).
type rng struct{ s, e int }

func (s *fstate) succs(b *mblock) []*mblock {
	t := b.term()
	if t == nil {
		return nil
	}
	switch t.kind {
	case iJump, iCondBr, iSwitch:
		return t.targets
	}
	return nil
}

type bitset []uint64

func newBitset(n int) bitset       { return make(bitset, (n+63)/64) }
func (b bitset) has(i vreg) bool   { return b[i/64]&(1<<(uint(i)%64)) != 0 }
func (b bitset) set(i vreg)        { b[i/64] |= 1 << (uint(i) % 64) }
func (b bitset) clear(i vreg)      { b[i/64] &^= 1 << (uint(i) % 64) }
func (b bitset) copyFrom(o bitset) { copy(b, o) }

func (s *fstate) regalloc() {
	s.order = append(append([]*mblock(nil), s.hot...), s.cold...)
	for _, b := range s.order {
		b.succs = s.succs(b)
		for _, t := range b.succs {
			t.preds = append(t.preds, b)
		}
	}
	// positions
	i := 0
	for _, b := range s.order {
		b.start = 4 * i
		i += len(b.insts)
		b.end = 4*(i-1) + 3
	}
	s.ninst = i
	n := len(s.fp)

	// liveness
	gen := make([]bitset, len(s.order))
	kill := make([]bitset, len(s.order))
	liveIn := make([]bitset, len(s.order))
	liveOut := make([]bitset, len(s.order))
	idx := map[*mblock]int{}
	for k, b := range s.order {
		idx[b] = k
		gen[k], kill[k], liveIn[k], liveOut[k] = newBitset(n), newBitset(n), newBitset(n), newBitset(n)
		for _, in := range b.insts {
			for _, u := range in.uses {
				if u != 0 && !kill[k].has(u) {
					gen[k].set(u)
				}
			}
			for _, d := range in.defs {
				if d != 0 {
					kill[k].set(d)
				}
			}
		}
	}
	for changed := true; changed; {
		changed = false
		for k := len(s.order) - 1; k >= 0; k-- {
			b := s.order[k]
			out := liveOut[k]
			for _, t := range b.succs {
				for w, x := range liveIn[idx[t]] {
					out[w] |= x
				}
			}
			for w := range out {
				nv := gen[k][w] | (out[w] &^ kill[k][w])
				if nv != liveIn[k][w] {
					liveIn[k][w] = nv
					changed = true
				}
			}
		}
	}

	// a register live into the entry is read before anything writes it
	// (gotchas/arm64-mir-param-without-moves.md)
	for w, x := range liveIn[0] {
		if x != 0 {
			fail("internal: %s reads virtual register %d before it is written", s.f.Name, w*64+bits.TrailingZeros64(x))
		}
	}

	// a landing pad is entered by the unwinder with the callee saved
	// registers of the faulting point and the others lost (decision
	// 0085): what it reads lives in callee saved registers or slots. A
	// call already clobbers the others; a fault stub's call does not
	// count as one, so the values live into its pad are marked.
	noCaller := newBitset(n)
	for _, b := range s.order {
		for _, in := range b.insts {
			if in.pad == nil || in.kind == iCall {
				continue
			}
			if k, ok := idx[s.padOf(in)]; ok {
				for w, x := range liveIn[k] {
					noCaller[w] |= x
				}
			}
		}
	}

	// loop depth, for spill weights
	s.loopDepth()

	// live ranges
	ranges := make([][]rng, n)
	weight := make([]float64, n)
	end := make([]int, n)
	live := newBitset(n)
	var calls []int
	callAt := map[int]*inst{}
	for k, b := range s.order {
		live.copyFrom(liveOut[k])
		for w, x := range live {
			for x != 0 {
				bit := 0
				for x&(1<<uint(bit)) == 0 {
					bit++
				}
				x &^= 1 << uint(bit)
				end[w*64+bit] = b.end
			}
		}
		first := b.start / 4
		wt := math.Pow(8, float64(min(b.depth, 6)))
		if b.cold {
			wt = 0.1
		}
		for j := len(b.insts) - 1; j >= 0; j-- {
			in := b.insts[j]
			at := 4 * (first + j)
			if in.kind == iCall {
				calls = append(calls, at+1)
				callAt[at+1] = in
			}
			for _, d := range in.defs {
				if d == 0 {
					continue
				}
				dp := at + 2
				if in.early {
					dp = at
				}
				weight[d] += wt
				if live.has(d) {
					ranges[d] = append(ranges[d], rng{dp, end[d]})
					live.clear(d)
				} else {
					ranges[d] = append(ranges[d], rng{dp, dp})
				}
			}
			for _, u := range in.uses {
				if u == 0 {
					continue
				}
				weight[u] += wt
				if !live.has(u) {
					live.set(u)
					end[u] = at
				}
			}
		}
		for w, x := range live {
			for x != 0 {
				bit := 0
				for x&(1<<uint(bit)) == 0 {
					bit++
				}
				x &^= 1 << uint(bit)
				v := w*64 + bit
				ranges[v] = append(ranges[v], rng{b.start, end[v]})
			}
		}
	}
	for v := range ranges {
		ranges[v] = mergeRanges(ranges[v])
	}

	// hints: registers a vreg would like (ABI registers), and pairs of
	// registers joined by a move
	fixed := make([][]int, n)
	type pair struct {
		a, b vreg
		wt   float64
	}
	var pairs []pair
	for _, b := range s.order {
		wt := math.Pow(8, float64(min(b.depth, 6)))
		for _, in := range b.insts {
			switch in.kind {
			case iPmove:
				for k, d := range in.defs {
					if a := in.srcs[k]; a.kind == sReg && a.v != 0 {
						pairs = append(pairs, pair{d, a.v, wt})
					}
				}
			case iCall:
				if s.g.goMode {
					continue // arguments and results go through memory
				}
				locs := s.abiLocs(in.srcs)
				for k, a := range in.srcs {
					if a.kind == sReg && a.v != 0 && locs[k].reg >= 0 {
						fixed[a.v] = append(fixed[a.v], locs[k].reg)
					}
				}
				for k, l := range s.abiLocs(regs(in.defs...)) {
					if l.reg >= 0 {
						fixed[in.defs[k]] = append(fixed[in.defs[k]], l.reg)
					}
				}
			case iEntry:
				if s.g.goMode {
					continue
				}
				for k, l := range s.abiLocs(regs(in.defs...)) {
					if l.reg >= 0 {
						fixed[in.defs[k]] = append(fixed[in.defs[k]], l.reg)
					}
				}
			case iRet:
				if s.g.goMode {
					continue
				}
				for k, l := range s.abiLocs(in.srcs) {
					if a := in.srcs[k]; a.kind == sReg && a.v != 0 && l.reg >= 0 {
						fixed[a.v] = append(fixed[a.v], l.reg)
					}
				}
			}
		}
	}

	// coalescing: registers joined by a move whose ranges do not overlap
	// share a location (most frequent moves first)
	leader := make([]vreg, n)
	for v := range leader {
		leader[v] = vreg(v)
	}
	var find func(v vreg) vreg
	find = func(v vreg) vreg {
		for leader[v] != v {
			leader[v] = leader[leader[v]]
			v = leader[v]
		}
		return v
	}
	sort.SliceStable(pairs, func(i, j int) bool { return pairs[i].wt > pairs[j].wt })
	var partners [][]vreg = make([][]vreg, n)
	for _, p := range pairs {
		a, b := find(p.a), find(p.b)
		if a == b || s.fp[a] != s.fp[b] {
			continue
		}
		if overlaps(ranges[a], ranges[b]) {
			partners[a] = append(partners[a], b)
			partners[b] = append(partners[b], a)
			continue
		}
		if b < a {
			a, b = b, a
		}
		leader[b] = a
		ranges[a] = mergeRanges(append(ranges[a], ranges[b]...))
		ranges[b] = nil
		weight[a] += weight[b]
		fixed[a] = append(fixed[a], fixed[b]...)
		partners[a] = append(partners[a], partners[b]...)
		if noCaller.has(b) {
			noCaller.set(a)
		}
	}

	// allocation: greedy first fit in order of weight per length
	var order []vreg
	for v := 1; v < n; v++ {
		if find(vreg(v)) == vreg(v) && len(ranges[v]) > 0 {
			order = append(order, vreg(v))
		}
	}
	prio := func(v vreg) float64 {
		l := 0
		for _, r := range ranges[v] {
			l += r.e - r.s + 1
		}
		return weight[v] / float64(l+4)
	}
	sort.SliceStable(order, func(a, b int) bool {
		pa, pb := prio(order[a]), prio(order[b])
		if pa != pb {
			return pa > pb
		}
		return order[a] < order[b]
	})
	gpC, gpE, fpC, fpE := s.regSets()
	var usedCallee map[int]bool
	for {
		occ := make([][]rng, 64)
		if s.base != 0 {
			// the base register of the frame's far slots (emit.go farSlots)
			occ[s.base] = []rng{{0, 4*s.ninst + 3}}
		}
		for _, c := range calls {
			if s.g.goMode {
				// no register survives a call: the values live across one
				// keep their registers and are saved around it (below)
				break
			}
			for _, r := range gpC {
				occ[r] = append(occ[r], rng{c, c})
			}
			for _, r := range fpC {
				occ[r] = append(occ[r], rng{c, c})
			}
		}
		for r := range occ {
			occ[r] = mergeRanges(occ[r])
		}
		s.loc = make([]loc, n)
		for v := range s.loc {
			s.loc[v] = loc{reg: -1, slot: -1}
		}
		usedCallee = map[int]bool{}
		s.slots = 0
		var slotOcc [][]rng // the ranges each spill slot holds
		for _, v := range order {
			fp := s.fp[v]
			var cands []int
			for _, r := range fixed[v] {
				if (r >= fpBase) == fp {
					cands = append(cands, r)
				}
			}
			for _, p := range partners[v] {
				if r := s.loc[find(p)].reg; r >= 0 {
					cands = append(cands, r)
				}
			}
			if fp {
				cands = append(cands, fpC...)
				cands = append(cands, fpE...)
			} else {
				cands = append(cands, gpC...)
				cands = append(cands, gpE...)
			}
			chosen := -1
			for _, r := range cands {
				if overlaps(occ[r], ranges[v]) || (noCaller.has(v) && !isCallee(r)) {
					continue
				}
				if !s.g.goMode && isCallee(r) && !usedCallee[r] {
					// a callee saved register already saved before a fresh one
					for _, r2 := range cands {
						if isCallee(r2) && usedCallee[r2] && !overlaps(occ[r2], ranges[v]) {
							r = r2
							break
						}
					}
				}
				chosen = r
				break
			}
			if chosen < 0 {
				// a spill slot no value live at the same time holds (first
				// fit, like the registers): one slot per spilled value made
				// long functions' frames grow with their length
				// (gotchas/arm64-large-frame.md)
				k := 0
				for k < len(slotOcc) && overlaps(slotOcc[k], ranges[v]) {
					k++
				}
				if k == len(slotOcc) {
					slotOcc = append(slotOcc, nil)
					s.slots++
				}
				slotOcc[k] = mergeRanges(append(slotOcc[k], ranges[v]...))
				s.loc[v] = loc{reg: -1, slot: k}
				continue
			}
			s.loc[v] = loc{reg: chosen}
			if !s.g.goMode && isCallee(chosen) {
				usedCallee[chosen] = true
			}
			occ[chosen] = mergeRanges(append(occ[chosen], ranges[v]...))
		}
		// a frame past the reach of an sp relative ldr or str: allocate
		// again with x28 kept free, for the addresses of the far slots
		if s.g.goMode || s.base != 0 || s.frameBound(len(usedCallee)) <= maxFrame {
			break
		}
		s.base = farBase
	}
	if s.base != 0 {
		usedCallee[s.base] = true
	}
	if s.g.goMode {
		// Go saves no register across a call: a value in a register
		// that lives across one is stored to its own slot before the
		// call and loaded after it (goEmitCall), so it keeps its
		// register everywhere else (a whole life spill would put the
		// loop counters of a recursive function in memory)
		slot := map[vreg]int{}
		for _, c := range calls {
			in := callAt[c]
			if in.noret {
				continue
			}
			// (a poll's saves are stored only on its cold path)
			for _, v := range order {
				if s.loc[v].reg < 0 || !overlaps(ranges[v], []rng{{c, c}}) {
					continue
				}
				k, ok := slot[v]
				if !ok {
					k = s.slots
					s.slots++
					slot[v] = k
				}
				in.saves = append(in.saves, v)
				in.saveSlots = append(in.saveSlots, k)
			}
		}
	}
	for v := 1; v < n; v++ {
		s.loc[v] = s.loc[find(vreg(v))]
	}
	for r := 0; r < 64; r++ {
		if usedCallee[r] {
			s.saved = append(s.saved, r)
		}
	}
}

func mergeRanges(rs []rng) []rng {
	if len(rs) < 2 {
		return rs
	}
	sort.Slice(rs, func(i, j int) bool { return rs[i].s < rs[j].s })
	out := rs[:1]
	for _, r := range rs[1:] {
		last := &out[len(out)-1]
		if r.s <= last.e+1 {
			if r.e > last.e {
				last.e = r.e
			}
			continue
		}
		out = append(out, r)
	}
	return out
}

// overlaps reports whether two sorted, merged range lists share a
// position.
func overlaps(a, b []rng) bool {
	i, j := 0, 0
	for i < len(a) && j < len(b) {
		if a[i].e < b[j].s {
			i++
		} else if b[j].e < a[i].s {
			j++
		} else {
			return true
		}
	}
	return false
}

// loopDepth sets the loop nesting depth of every block: a natural loop
// per back edge found by depth first search.
func (s *fstate) loopDepth() {
	state := map[*mblock]int{} // 1 on stack, 2 done
	type be struct{ from, to *mblock }
	var backs []be
	var dfs func(b *mblock)
	dfs = func(b *mblock) {
		state[b] = 1
		for _, t := range b.succs {
			switch state[t] {
			case 0:
				dfs(t)
			case 1:
				backs = append(backs, be{b, t})
			}
		}
		state[b] = 2
	}
	if len(s.order) > 0 {
		dfs(s.order[0])
	}
	for _, e := range backs {
		body := map[*mblock]bool{e.to: true}
		work := []*mblock{e.from}
		for len(work) > 0 {
			b := work[len(work)-1]
			work = work[:len(work)-1]
			if body[b] {
				continue
			}
			body[b] = true
			work = append(work, b.preds...)
		}
		for b := range body {
			b.depth++
		}
	}
}

// abiLocs assigns AAPCS64 locations to call operands: integer words to
// x0 to x7, floating point words to d0 to d7, the rest to 8 byte stack
// slots (reg -1, slot k is [sp + 8k] at the call, [x29 + 16 + 8k] in the
// callee).
func (s *fstate) abiLocs(srcs []src) []loc {
	out := make([]loc, len(srcs))
	ngp, nfp, nst := 0, 0, 0
	for k, a := range srcs {
		fp := a.kind == sReg && s.fp[a.v]
		switch {
		case fp && nfp < 8:
			out[k] = loc{reg: fpBase + nfp}
			nfp++
		case !fp && ngp < 8:
			out[k] = loc{reg: ngp}
			ngp++
		default:
			out[k] = loc{reg: -1, slot: nst}
			nst++
		}
	}
	return out
}
