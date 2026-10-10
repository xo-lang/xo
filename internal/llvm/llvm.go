// Package llvm is the LLVM backend (decision 0040): it lowers a checked
// Xo program to textual LLVM IR, which internal/build compiles with the
// system clang together with the C runtime in std/llrt (`xo build
// --backend=llvm`).
//
// Generate builds the shared SSA IR (internal/ssa, decision 0045),
// inserts reference counting with in place reuse (decision 0046), checks
// ownership on every path, and lowers the result (fromssa.go). Closures,
// recursive types, and generics are lowered as decision 0048 describes.
// A construct outside the SSA builder's subset is an error naming it and
// its position.
//
// The first prototype lowered from the typed AST with a bump allocator
// that never freed; it was deleted on 2026-10-08 once every program it
// handled took the SSA path (bench/results/llvm-rc-2026-10-08.md).
package llvm

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"

	"github.com/xo-lang/xo/internal/check"
	"github.com/xo-lang/xo/internal/ssa"
)

// Options configure generation.
type Options struct {
	// LineRoot is the directory fault locations are made relative to.
	LineRoot string
	// ShareProbes marks every heap value shared as it is defined
	// (ssa.InsertShareProbes; a test of the atomic count paths).
	ShareProbes bool
	// Tests generates a test binary (ssa.Options.Tests).
	Tests bool
	// Release compiles out debug contracts (ssa.Options.Release).
	Release bool
	// TargetAttrs are LLVM function attributes every generated function
	// gets: the C compiler's own target CPU and features
	// (build.ClangTargetAttrs), so generated code uses the instructions
	// the runtime's C does (LSE atomics for shared counts on arm64,
	// outline atomics where the baseline lacks them; decision 0119) and
	// LTO may inline runtime functions into it. Empty: LLVM's generic CPU.
	TargetAttrs string
}

// Program is the generated IR of one Xo program.
type Program struct {
	IR []byte
	// MainModule is the Xo module whose `main` the program runs.
	MainModule string
	// Funcs counts the functions emitted.
	Funcs int
	// TZ is the zone data std/time needs (internal/build embeds it).
	TZ ssa.TZUse
}

// Generate lowers the program whose roots are the modules of res: it
// builds SSA, verifies it, inserts reference counting, verifies again
// (ownership balance included), and lowers to LLVM IR.
func Generate(res *check.Result, opts Options) (*Program, error) {
	m, err := ssa.BuildOpts(res, ssa.Options{LineRoot: opts.LineRoot, Tests: opts.Tests, Release: opts.Release})
	if err != nil {
		return nil, err
	}
	if err := m.Verify(); err != nil {
		return nil, err
	}
	if opts.ShareProbes {
		m.InsertShareProbes()
	}
	m.InsertRC()
	if err := m.Verify(); err != nil {
		return nil, err
	}
	prog, err := GenerateSSA(m)
	if err != nil {
		return nil, err
	}
	if opts.TargetAttrs != "" {
		prog.IR = withTargetAttrs(prog.IR, opts.TargetAttrs)
	}
	prog.TZ = m.TZ
	for _, mod := range res.Modules {
		if fo, ok := mod.Scope["main"].(*check.FuncObj); ok && fo.Recv == nil {
			prog.MainModule = mod.Path
			break
		}
	}
	return prog, nil
}

// definedAttrs are the attribute groups of generated function
// definitions (fromssa.go's ssaRuntimeDecls).
var definedAttrs = []string{"attributes #1 = { uwtable", "attributes #4 = { nounwind alwaysinline"}

// withTargetAttrs adds attrs to the attribute groups of the generated
// function definitions in ir.
func withTargetAttrs(ir []byte, attrs string) []byte {
	for _, g := range definedAttrs {
		ir = bytes.Replace(ir, []byte(g+" }"), []byte(g+" "+attrs+" }"), 1)
	}
	return ir
}

// irBytes encodes s as the body of an LLVM c"..." string.
func irBytes(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 0x20 && c < 0x7f && c != '"' && c != '\\' {
			b.WriteByte(c)
		} else {
			fmt.Fprintf(&b, "\\%02X", c)
		}
	}
	return b.String()
}

// alignUp rounds n up to a multiple of a.
func alignUp(n, a int64) int64 {
	if a <= 1 {
		return n
	}
	return (n + a - 1) / a * a
}

// constInt parses an operand that is an integer constant.
func constInt(s string) (int64, bool) {
	k, err := strconv.ParseInt(s, 10, 64)
	return k, err == nil
}
