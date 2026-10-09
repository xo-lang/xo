package build

import (
	"fmt"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

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
// CheckNative. A test build checks the test binary: every test block
// (and what it reaches) instead of main.
func CheckNoGC(dirs []string, res *check.Result, mode gen.Mode) []diag.Diagnostic {
	return ssa.CheckNoGC(res, ssa.Options{LineRoot: lineRoot(dirs), Tests: mode == gen.Test})
}

// TestInfo describes a test block of a native test binary (the harness
// runs it with XO_TEST set to Index).
type TestInfo struct {
	Index  int
	Name   string
	Module string
	Dir    string // the module's directory: where the test runs
	Loc    string // file:line:col, relative to the line root
	// Seed is the pinned seed (`test "x" seed N`), or 0.
	Seed int64
	// Uses lists the effects of `test "x" uses ...` (skipped without
	// --real).
	Uses []string
	// Skip is why the test does not run natively (a contract fuzz test,
	// decision 0085), or "".
	Skip string
}

// fuzzSkip is the report of a contract fuzz test of a native test binary.
const fuzzSkip = "contract fuzzing is not available without a garbage collector (`--no-gc`) yet (support is planned)"

// testInfos lists the test blocks of a native test build in the order
// the test binary numbers them (ssa.TestDecls).
func testInfos(dirs []string, res *check.Result) []TestInfo {
	root := lineRoot(dirs)
	modOf := map[*ast.TestDecl]*check.Module{}
	for _, m := range res.Modules {
		for _, f := range m.Files {
			for _, d := range f.AST.Decls {
				if td, ok := d.(*ast.TestDecl); ok {
					modOf[td] = m
				}
			}
		}
	}
	// the contract fuzz tests the Go backend runs (core 9.1) are listed
	// where it lists them, before the test blocks of their file that
	// follow them, as skipped
	fuzzAt := map[ast.Node][]TestInfo{}
	for _, ft := range gen.ContractFuzzTargets(res) {
		ti := TestInfo{Index: -1, Name: ft.Name, Loc: relLoc(root, ft.Fn.Decl), Module: ft.Fn.Module.Path, Skip: fuzzSkip}
		ti.Dir, _ = filepath.Abs(ft.Fn.Module.Dir)
		fuzzAt[ft.Fn.Decl] = append(fuzzAt[ft.Fn.Decl], ti)
	}
	var out []TestInfo
	pending := map[*ast.TestDecl][]TestInfo{}
	var held []TestInfo // in declaration order, until the next test block
	for _, m := range res.Modules {
		for _, f := range m.Files {
			for _, d := range f.AST.Decls {
				if fd, ok := d.(*ast.FnDecl); ok {
					held = append(held, fuzzAt[fd]...)
				}
				if td, ok := d.(*ast.TestDecl); ok && len(held) > 0 {
					pending[td] = held
					held = nil
				}
			}
		}
	}
	tail := held
	for i, td := range ssa.TestDecls(res) {
		out = append(out, pending[td]...)
		ti := TestInfo{Index: i, Name: ssa.TestName(td), Loc: relLoc(root, td)}
		if m := modOf[td]; m != nil {
			ti.Module = m.Path
			ti.Dir, _ = filepath.Abs(m.Dir)
		}
		if td.Seed != nil {
			ti.Seed, _ = strconv.ParseInt(td.Seed.Value, 10, 64)
		}
		for _, e := range td.Uses {
			u := e.Name.Name
			if e.Sub != nil {
				u += "." + e.Sub.Name
			}
			ti.Uses = append(ti.Uses, u)
		}
		out = append(out, ti)
	}
	return append(out, tail...)
}

// relLoc is `file:line:col` of n with the file relative to root (as the
// Go backend's test locations).
func relLoc(root string, n ast.Node) string {
	sp := n.Span()
	f := sp.File
	if abs, err := filepath.Abs(f); err == nil && root != "" {
		if rel, err := filepath.Rel(root, abs); err == nil && !strings.HasPrefix(rel, "..") {
			f = rel
		}
	}
	return fmt.Sprintf("%s:%d:%d", filepath.ToSlash(f), sp.Start.Line, sp.Start.Col)
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
