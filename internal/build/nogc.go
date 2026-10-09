package build

import (
	"runtime"

	"github.com/xo-lang/xo/internal/ast"
	"github.com/xo-lang/xo/internal/check"
	"github.com/xo-lang/xo/internal/diag"
	"github.com/xo-lang/xo/internal/gen"
	"github.com/xo-lang/xo/internal/ssa"
)

// NativeBackend is the backend of `--no-gc` builds when none is named
// (decision 0083): arm64 on darwin/arm64 and linux/arm64 hosts, LLVM
// otherwise.
func NativeBackend() string {
	return nativeBackendFor(runtime.GOOS, runtime.GOARCH)
}

func nativeBackendFor(goos, goarch string) string {
	if goarch == "arm64" && (goos == "darwin" || goos == "linux") {
		return BackendArm64
	}
	return BackendLLVM
}

// CheckNoGC is the no GC check of a native build (decision 0083): the
// XO1201 diagnostics of every construct, std API, or effect of the
// checked program res that the native backends cannot lower, derived
// from internal/ssa's builder (ssa.CheckNoGC). res must come from
// CheckNative. A test build reports each test block, since the native
// backends build programs only so far.
func CheckNoGC(dirs []string, res *check.Result, mode gen.Mode) []diag.Diagnostic {
	if mode == gen.Test {
		var out []diag.Diagnostic
		for _, m := range res.Modules {
			for _, f := range m.Files {
				for _, d := range f.AST.Decls {
					if td, ok := d.(*ast.TestDecl); ok {
						out = append(out, ssa.NoGCDiagnostic(&ssa.Error{Feature: "a test block (`xo test`)", Span: td.Span()}))
					}
				}
			}
		}
		return out
	}
	return ssa.CheckNoGC(res, ssa.Options{LineRoot: lineRoot(dirs)})
}

// CheckNoGCDirs checks the modules under dirs as `xo check --no-gc`
// does: the checker with the native implementation modules, then, when
// it reports no error, the no GC check of a program build.
func CheckNoGCDirs(dirs []string, holes bool) (*check.Result, []diag.Diagnostic, error) {
	res, ok, err := CheckNative(dirs, holes)
	if err != nil {
		return nil, nil, err
	}
	ds := append([]diag.Diagnostic(nil), res.Diagnostics...)
	// a library (no main) has no program to lower: the builder starts at
	// main, so only programs are checked
	if ok && hasMain(res) {
		ds = append(ds, CheckNoGC(dirs, res, gen.Build)...)
		diag.Sort(ds)
	}
	return res, ds, nil
}

// hasMain reports whether a root module of res declares `fn main`.
func hasMain(res *check.Result) bool {
	for _, m := range res.Modules {
		if fo, ok := m.Scope["main"].(*check.FuncObj); ok && fo.Recv == nil {
			return true
		}
	}
	return false
}
