package build

import (
	"errors"
	"sort"
	"strings"

	"github.com/xo-lang/xo/internal/arm64"
	"github.com/xo-lang/xo/internal/check"
	"github.com/xo-lang/xo/internal/gen"
	"github.com/xo-lang/xo/internal/ssa"
)

// HybridInfo is what `xo build --hybrid` (decision 0084) compiled to Go
// assembler.
type HybridInfo struct {
	// Asm are the functions whose Go version calls assembly; Called the
	// ones emitted only for other assembly functions.
	Asm, Called []string
	// Rejected gives, per function of the program, why it stayed Go.
	Rejected map[string]string
	// Err is why no function was compiled (the program is beyond the
	// native backends' subset), or "".
	Err string
}

// hybridAsm runs the arm64 backend's Go mode over the program: the
// native check (std/native), SSA with reference counting, and
// arm64.GenerateGo. Any failure leaves the build plain Go (nil).
func hybridAsm(dirs []string, root string, opts Options) (*gen.Hybrid, *HybridInfo) {
	info := &HybridInfo{Rejected: map[string]string{}}
	res, ok, err := checkOpts(dirs, opts.Holes, true)
	if err != nil || !ok {
		info.Err = "the native check failed"
		return nil, info
	}
	gopts := arm64.GoOptions{All: opts.HybridAll, NoPoll: opts.HybridNoPoll}
	gp, err := hybridGo(res, root, gopts, opts.release())
	var ie *arm64.InlineError
	if errors.As(err, &ie) {
		gopts.NoInline = true
		gp, err = hybridGo(res, root, gopts, opts.release())
	}
	if err != nil {
		info.Err = err.Error()
		return nil, info
	}
	info.Rejected = gp.Rejected
	if len(gp.Funcs) == 0 {
		return nil, info
	}
	h := &gen.Hybrid{Funcs: map[string]gen.HybridFunc{}, Positions: gp.Positions, Asm: gp.Asm}
	for name, f := range gp.Funcs {
		h.Funcs[name] = gen.HybridFunc{Sym: f.Sym, Wrap: f.Wrap, NoEscape: f.NoEscape, Unshare: f.Unshare}
		if f.Wrap {
			info.Asm = append(info.Asm, name)
		} else {
			info.Called = append(info.Called, name)
		}
	}
	sort.Strings(info.Asm)
	sort.Strings(info.Called)
	return h, info
}

func hybridGo(res *check.Result, root string, gopts arm64.GoOptions, release bool) (*arm64.GoProgram, error) {
	m, err := ssa.BuildOpts(res, ssa.Options{LineRoot: root, NoFaultUnwind: true, Release: release})
	if err != nil {
		return nil, err
	}
	if err := m.Verify(); err != nil {
		return nil, err
	}
	m.InsertRC()
	if err := m.Verify(); err != nil {
		return nil, err
	}
	// contracts are checked in SSA as by the Go backend (ensures and
	// invariants since decision 0085): every function is eligible
	return arm64.GenerateGo(m, gopts)
}

// withTag adds a build tag to go build arguments (to a -tags= there, or
// as a new one).
func withTag(args []string, tag string) []string {
	out := append([]string(nil), args...)
	for i, a := range out {
		if strings.HasPrefix(a, "-tags=") {
			out[i] = a + "," + tag
			return out
		}
	}
	return append(out, "-tags="+tag)
}

// HybridPlan reports what `xo build --hybrid` would compile to assembly
// (every eligible function when all is set) without building.
func HybridPlan(dirs []string, all bool) *HybridInfo {
	_, info := hybridAsm(dirs, lineRoot(dirs), Options{HybridAll: all})
	return info
}
