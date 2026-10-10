package ssa

import (
	"runtime"

	"github.com/xo-lang/xo/internal/ast"
)

// target is the value of the predeclared `target` (core 12.2): a
// constant of the build's target, so a `match target.os` keeps only the
// chosen arm after constant folding. The variants follow std/rt's
// CurrentTarget (an unknown OS is Linux, an unknown architecture Amd64).
// nil in Go mode assembly (Options.NoFaultUnwind), whose target the
// builder does not know.
func (s *fn) target(x *ast.Ident) *Value {
	b := s.b
	if b.opts.NoFaultUnwind {
		return nil
	}
	t := s.natOf(x)
	if t.Kind != KStruct || len(t.Fields) != 2 || t.Fields[0].Type.Kind != KEnum || t.Fields[1].Type.Kind != KEnum {
		return nil
	}
	goos, goarch := b.opts.GOOS, b.opts.GOARCH
	if goos == "" {
		goos = runtime.GOOS
	}
	if goarch == "" {
		goarch = runtime.GOARCH
	}
	osName := "Linux"
	switch goos {
	case "darwin":
		osName = "Darwin"
	case "windows":
		osName = "Windows"
	case "wasip1", "wasi":
		osName = "Wasi"
	}
	archName := "Amd64"
	switch goarch {
	case "arm64":
		archName = "Arm64"
	case "wasm":
		archName = "Wasm32"
	}
	ot, at := t.Fields[0].Type, t.Fields[1].Type
	oi, ai := ot.VariantIndex(osName), at.VariantIndex(archName)
	if oi < 0 || ai < 0 {
		return nil
	}
	return s.add(OpStruct, t, nil, s.add(OpVariant, ot, oi), s.add(OpVariant, at, ai))
}
