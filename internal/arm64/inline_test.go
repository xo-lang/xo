package arm64

import (
	"testing"

	"github.com/xo-lang/xo/internal/ssa"
)

// TestInlineCostModel checks the inliner's cost model (decision 0184) on
// hand built functions: small functions are inlinable whether they call
// others or not, a cycle (mutual or a call of itself) leaves one of its
// functions not inlinable, a large function is inlinable only with few
// call sites and copies within the budget, and main never is.
func TestInlineCostModel(t *testing.T) {
	m := ssa.NewModule()
	intT := m.Builtin(ssa.KInt)
	// fn name(x: Int) -> Int: pad plain instructions, then calls, then
	// return
	fn := func(name string, pad int, calls ...string) {
		f := m.NewFunc(name, []*ssa.Type{intT}, []*ssa.Type{intT})
		b := f.Blocks[0]
		x := f.Params()[0]
		for i := 0; i < pad; i++ {
			x = b.Add(ssa.OpAdd, intT, nil, x, x)
		}
		for _, c := range calls {
			x = b.Add(ssa.OpCall, intT, c, x)
		}
		b.Add(ssa.OpReturn, nil, nil, x)
	}
	fn("main", 1, "chain", "self", "even", "big1", "big3", "big3", "big3", "forward", "huge2", "huge2")
	fn("leaf", 3)
	fn("chain", 3, "leaf")   // calls a small function: inlinable
	fn("self", 3, "self")    // calls itself: never
	fn("even", 3, "odd")     // even and odd call each other: the walk
	fn("odd", 3, "even")     // reaches odd from even, so odd is not
	fn("big1", 100)          // one site, 100 instructions: within 500
	fn("big3", 100)          // three sites: too many
	fn("forward", 1, "big3") // a forwarder to a large function
	fn("huge2", 300)         // two sites of 300: 600 over the budget
	byName := map[string]*ssa.Func{}
	for _, f := range m.Funcs {
		byName[f.Name] = f
	}
	got := inlinableSet(m, byName)
	want := map[string]bool{"leaf": true, "chain": true, "even": true, "big1": true, "forward": true}
	for _, f := range m.Funcs {
		if got[f.Name] != want[f.Name] {
			t.Errorf("%s: inlinable %v, want %v", f.Name, got[f.Name], want[f.Name])
		}
	}
}
