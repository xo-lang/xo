package arm64

import "github.com/xo-lang/xo/internal/ssa"

// Inlining of small functions, SSA to SSA, after reference counting
// (decisions 0049, 0184). The callee body already carries its own
// retains and releases for its ownership convention (borrowed parameters
// are never released by the callee, owned ones are consumed), so a
// faithful copy with the parameters replaced by the call's arguments
// keeps every count balanced; Generate verifies the module again after
// inlining.
//
// A call `v = call f, args` in block B becomes:
//
//	B:  ...instructions before v; jump f.entry'
//	f's blocks, copied; `ret xs` becomes `jump C(xs)`
//	C(results):  the rest of B (v is C's parameter, or a tuple of them)
//
// Cost model (decision 0184): a function's cost is its instruction and
// block count with every call of an inlinable Xo function counted as
// that function's cost (its body ends up in the copy too) and every
// other call as one instruction. A function is inlinable when both stay
// within the limits, it is not main, and it has no landing pads of its
// own. Recursion: the costs are found by a depth first walk of the call
// graph (functions in module order, callees in instruction order), and
// a function that calls one still being walked (a cycle, or itself) is
// not inlinable, so every cycle has a function that is not and copies
// never nest without end. A forwarder to a large function costs a few
// instructions and is inlined; the large function stays a call.

const (
	inlineMaxInstrs = 40
	inlineMaxBlocks = 12
)

// inlineCost is the expanded size of a function, or bad when it is not
// inlinable.
type inlineCost struct {
	instrs, blocks int
	bad            bool
}

// inlinableSet returns the names of the inlinable functions of m.
func inlinableSet(m *ssa.Module, byName map[string]*ssa.Func) map[string]bool {
	// state: 1 being walked, 2 done
	state := map[string]int{}
	costs := map[string]inlineCost{}
	var walk func(f *ssa.Func) inlineCost
	walk = func(f *ssa.Func) inlineCost {
		state[f.Name] = 1
		c := inlineCost{blocks: len(f.Blocks), bad: f.Name == "main"}
		for _, b := range f.Blocks {
			for _, v := range b.Instrs {
				if v.Pad != nil {
					// it has landing pads of its own (decision 0085)
					c.bad = true
				}
				callee := (*ssa.Func)(nil)
				if v.Op == ssa.OpCall {
					callee = byName[v.Aux.(string)]
				}
				if callee == nil {
					c.instrs++
					continue
				}
				var k inlineCost
				switch state[callee.Name] {
				case 1:
					c.bad = true // a cycle through f
					k.bad = true
				case 2:
					k = costs[callee.Name]
				default:
					k = walk(callee)
				}
				if k.bad {
					c.instrs++
				} else {
					c.instrs += k.instrs
					c.blocks += k.blocks + 1
				}
			}
		}
		if c.instrs > inlineMaxInstrs || c.blocks > inlineMaxBlocks {
			c.bad = true
		}
		state[f.Name] = 2
		costs[f.Name] = c
		return c
	}
	small := map[string]bool{}
	for _, f := range m.Funcs {
		if state[f.Name] == 0 {
			walk(f)
		}
	}
	for _, f := range m.Funcs {
		if !costs[f.Name].bad {
			small[f.Name] = true
		}
	}
	return small
}

// inline inlines the calls of small functions in m and reports whether
// it changed anything.
func inline(m *ssa.Module) bool {
	byName := map[string]*ssa.Func{}
	for _, f := range m.Funcs {
		byName[f.Name] = f
	}
	small := inlinableSet(m, byName)
	changed := false
	for _, f := range m.Funcs {
		for budget := 0; budget < 200; budget++ {
			v := findCall(f, small, byName)
			if v == nil {
				break
			}
			inlineCall(f, v, byName[v.Aux.(string)])
			changed = true
		}
	}
	return changed
}

func findCall(f *ssa.Func, small map[string]bool, byName map[string]*ssa.Func) *ssa.Value {
	for _, b := range f.Blocks {
		for _, v := range b.Instrs {
			if v.Op == ssa.OpCall && small[v.Aux.(string)] && (v.Pad == nil || len(byName[v.Aux.(string)].Results) == 1) {
				return v
			}
		}
	}
	return nil
}

func inlineCall(f *ssa.Func, call *ssa.Value, callee *ssa.Func) {
	b := call.Block
	at := -1
	for i, v := range b.Instrs {
		if v == call {
			at = i
		}
	}
	// the continuation takes the rest of b
	cont := f.NewBlock()
	rest := b.Instrs[at+1:]
	b.Instrs = b.Instrs[:at]
	for _, v := range rest {
		v.Block = cont
		for _, e := range v.Succs {
			e.From = cont
		}
	}
	var res []*ssa.Value
	for _, r := range callee.Results {
		res = append(res, cont.AddParam(r))
	}
	var repl *ssa.Value
	if len(res) == 1 {
		repl = res[0]
	} else {
		repl = cont.Add(ssa.OpTuple, call.Type, nil, res...)
	}
	cont.Instrs = append(cont.Instrs, rest...)
	replaceUses(f, call, repl)

	// copy the callee
	vm := map[*ssa.Value]*ssa.Value{}
	bm := map[*ssa.Block]*ssa.Block{}
	for i, p := range callee.Params() {
		vm[p] = call.Args[i]
	}
	order := rpo(callee)
	for _, cb := range order {
		nb := f.NewBlock()
		bm[cb] = nb
		if cb != callee.Blocks[0] {
			for _, p := range cb.Params {
				vm[p] = nb.AddParam(p.Type)
			}
		}
	}
	mapv := func(v *ssa.Value) *ssa.Value {
		if x, ok := vm[v]; ok {
			return x
		}
		return v
	}
	mapArgs := func(xs []*ssa.Value) []*ssa.Value {
		out := make([]*ssa.Value, len(xs))
		for i, x := range xs {
			out[i] = mapv(x)
		}
		return out
	}
	// edges to blocks not copied yet refer to values defined later only
	// through edge arguments, which are mapped after all bodies
	type pending struct {
		e    *ssa.Edge
		args []*ssa.Value
	}
	var edges []pending
	for _, cb := range order {
		nb := bm[cb]
		for _, v := range cb.Instrs {
			switch v.Op {
			case ssa.OpJump:
				t := nb.Jump(bm[v.Succs[0].To])
				edges = append(edges, pending{t.Succs[0], v.Succs[0].Args})
			case ssa.OpBranch:
				t := nb.Branch(mapv(v.Args[0]), bm[v.Succs[0].To], bm[v.Succs[1].To])
				edges = append(edges, pending{t.Succs[0], v.Succs[0].Args}, pending{t.Succs[1], v.Succs[1].Args})
			case ssa.OpReturn:
				t := nb.Jump(cont)
				edges = append(edges, pending{t.Succs[0], v.Args})
			default:
				nv := nb.Add(v.Op, v.Type, v.Aux, mapArgs(v.Args)...)
				nv.Pos = v.Pos
				// a fault in the copy lands where one in the call would
				// (decision 0085): the callee holds nothing then
				nv.Pad = call.Pad
				vm[v] = nv
			}
		}
	}
	for _, p := range edges {
		p.e.Args = mapArgs(p.args)
	}
	b.Jump(bm[callee.Blocks[0]])
}

// replaceUses replaces every use of old in f by x.
func replaceUses(f *ssa.Func, old, x *ssa.Value) {
	for _, b := range f.Blocks {
		for _, v := range b.Instrs {
			for i, a := range v.Args {
				if a == old {
					v.Args[i] = x
				}
			}
			for _, e := range v.Succs {
				for i, a := range e.Args {
					if a == old {
						e.Args[i] = x
					}
				}
			}
		}
	}
}
