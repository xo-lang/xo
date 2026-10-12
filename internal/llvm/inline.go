package llvm

import "github.com/xo-lang/xo/internal/ssa"

// Inlining (decision 0192): a call of a function the arm64 backend's
// cost model (internal/arm64 inline.go, decision 0184) finds inlinable
// is marked `alwaysinline` at the call site (attribute group #6), so
// LLVM inlines what arm64 inlines. Its own inliner stops at a cost of
// about 225 and does not count how few call sites a larger function
// has: the UTF-8 decoder and the case mapping of
// std/native/llrt/unicode.xo, each called from two loops, stayed calls
// in the runtime's text loops, and the facts of ranges.go did not reach
// the table reads inside them.
//
// Cost model: a function's cost is its instruction and block count,
// where a call of an inlinable Xo function counts that function's cost
// and any other call one instruction. A function is inlinable when its
// cost is at most inlineMaxInstrs instructions and inlineMaxBlocks
// blocks, or when it has at most inlineFewSites call sites in the module
// (its own calls not counted), sites times instructions is at most
// inlineFewBudget, and it has at most inlineFewBlocks blocks. Never
// main, never a function with landing pads of its own. Costs come from a
// depth first walk of the call graph (functions in module order, callees
// in instruction order); a function that calls one still being walked (a
// cycle, or itself) is not inlinable, so every cycle has a function that
// is not and copies never nest without end.

const (
	inlineMaxInstrs = 40
	inlineMaxBlocks = 12
	inlineFewSites  = 2
	inlineFewBudget = 500
	inlineFewBlocks = 80
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
	sites := map[string]int{}
	for _, f := range m.Funcs {
		for _, b := range f.Blocks {
			for _, v := range b.Instrs {
				if v.Op == ssa.OpCall && v.Aux.(string) != f.Name {
					sites[v.Aux.(string)]++
				}
			}
		}
	}
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
			n := sites[f.Name]
			if n > inlineFewSites || n*c.instrs > inlineFewBudget || c.blocks > inlineFewBlocks {
				c.bad = true
			}
		}
		state[f.Name] = 2
		costs[f.Name] = c
		return c
	}
	for _, f := range m.Funcs {
		if state[f.Name] == 0 {
			walk(f)
		}
	}
	small := map[string]bool{}
	for _, f := range m.Funcs {
		if !costs[f.Name].bad {
			small[f.Name] = true
		}
	}
	return small
}
