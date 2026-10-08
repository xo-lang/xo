// Package gen lowers checked Xo modules to Go source (P1, decision 0002).
//
// Each Xo module becomes one Go package under the build module `xoprog`
// (`xoprog/m/<name>`), std modules map onto the runtime package `xoprog/rt`
// (std/rt in this repository), and a `package main` calls the program's
// `main` or, for `xo test`, runs the test blocks. Generated code carries
// `//line file.xo:L:C` directives so Go stack traces, the race detector,
// and Delve report Xo positions.
//
// The lowering rules are documented in knowledgebase/infrastructure/
// toolchain.md ("Code generation"). In short: every function takes the
// current *rt.Task first; a function value of type `fn(A) -> R ! E` is a Go
// `func(*rt.Task, A) rt.Result[R, E]` (E is rt.Never when it cannot fail);
// declared functions return R, rt.Result[R, E], or nothing for `()`;
// expressions that need statements (blocks, if, match, `?`, scope) are
// hoisted in evaluation order; blocks that contain `defer` run as an
// immediately invoked closure whose Go defers give block scoped, shielded
// cleanup.
package gen

import (
	"bytes"
	"fmt"
	"go/format"
	"path/filepath"
	"sort"
	"strings"

	"github.com/xo-lang/xo/internal/ast"
	"github.com/xo-lang/xo/internal/check"
)

// Mode selects what is generated.
type Mode int

const (
	// Build generates a program whose main calls the Xo `main`.
	Build Mode = iota
	// Test generates a program that runs the test blocks of the root
	// modules.
	Test
)

// Options configure generation.
type Options struct {
	Mode Mode
	// Holes allows `???`, which faults when evaluated (core 3.7).
	Holes bool
	// LineRoot is the directory //line paths are made relative to; empty
	// means each module's xo.mod root (or the working directory).
	LineRoot string
	// NoMono turns off monomorphization (mono.go, `xo build --no-mono`):
	// by default generic functions are specialized per concrete
	// instantiation and closure literals passed to List higher order
	// methods are inlined; with NoMono each generic function is one Go
	// generic function, which is also the fallback for recursion through
	// type parameters and past the instantiation cap.
	NoMono bool
	// NoRanges turns off the range analysis of Int operations (ranges.go):
	// every operation is checked. Used by tests that compare the two.
	NoRanges bool
}

// Program is the generated Go source of one Xo program: file paths
// relative to the build directory mapped to their content.
type Program struct {
	Files map[string][]byte
	// MainModule is the Xo module whose `main` the program runs (Build).
	MainModule string
	// Specs counts the specialized copies of generic functions.
	Specs int
}

// Error is a generation failure: a construct the P1 generator does not
// support, with its position.
type Error struct {
	Pos string
	Msg string
}

func (e *Error) Error() string {
	if e.Pos == "" {
		return e.Msg
	}
	return e.Pos + ": " + e.Msg
}

// ModulePath is the Go module path of the build directory.
const ModulePath = "xoprog"

// gen holds the state shared by every package of one program.
type gen struct {
	info    *check.Info
	opts    Options
	roots   []*check.Module
	pkgs    map[*check.Module]*pkgGen
	errs    []*Error
	modRoot map[string]string // module dir -> xo.mod root dir
	prelude *check.Module
	boxed   map[*check.FieldObj]bool
	ad      adaptState // interface adapters for generic code (adapt.go)
}

// Generate lowers the modules of a check result. roots are the modules
// named on the command line; their imports are included as needed.
func Generate(res *check.Result, opts Options) (*Program, error) {
	info := res.Info()
	g := &gen{info: info, opts: opts, roots: res.Modules, pkgs: map[*check.Module]*pkgGen{}, modRoot: map[string]string{}, prelude: info.Prelude()}
	prog := &Program{Files: map[string][]byte{}}
	g.initAdapt()
	var mainMod *check.Module
	for _, m := range info.AllModules() {
		if m.IsStd {
			continue
		}
		pg := g.newPkg(m)
		g.pkgs[m] = pg
	}
	for _, m := range info.AllModules() {
		pg := g.pkgs[m]
		if pg == nil {
			continue
		}
		pg.genModule()
		if fo, ok := m.Scope["main"].(*check.FuncObj); ok && fo.Recv == nil && m.Root {
			if mainMod == nil {
				mainMod = m
			}
		}
	}
	if !opts.NoMono {
		g.drainSpecs()
	}
	if len(g.errs) > 0 {
		return nil, g.errList()
	}
	for _, m := range info.AllModules() {
		pg := g.pkgs[m]
		if pg == nil {
			continue
		}
		prog.Specs += pg.ms.generated
		src, err := pg.file()
		if err != nil {
			return nil, err
		}
		prog.Files[pg.dir+"/xo.go"] = src
	}
	var mainSrc []byte
	var err error
	switch opts.Mode {
	case Build:
		if mainMod == nil {
			return nil, &Error{Msg: "no `fn main` found in the modules given"}
		}
		prog.MainModule = mainMod.Path
		mainSrc, err = g.mainFile(mainMod)
	case Test:
		mainSrc, err = g.testMainFile()
	}
	if err != nil {
		return nil, err
	}
	prog.Files["main.go"] = mainSrc
	adaptSrc, err := g.adaptFile()
	if err != nil {
		return nil, err
	}
	prog.Files["xo_adapt.go"] = adaptSrc
	prog.Files["go.mod"] = []byte("module " + ModulePath + "\n\ngo 1.26\n")
	return prog, nil
}

func (g *gen) errList() error {
	var b strings.Builder
	for i, e := range g.errs {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(e.Error())
	}
	return &Error{Msg: b.String()}
}

func (g *gen) errorf(n ast.Node, format string, args ...any) {
	pos := ""
	if n != nil {
		sp := n.Span()
		pos = fmt.Sprintf("%s:%d:%d", sp.File, sp.Start.Line, sp.Start.Col)
	}
	g.errs = append(g.errs, &Error{Pos: pos, Msg: fmt.Sprintf(format, args...)})
}

// ---------------------------------------------------------------------------
// Packages

// pkgGen generates one Go package for one Xo module.
type pkgGen struct {
	g       *gen
	m       *check.Module
	dir     string            // relative to the build dir: m/<name>
	name    string            // Go package name
	imports map[string]string // import path -> local name
	out     bytes.Buffer
	cur     *check.File // file being generated
	tmp     int
	adapt   map[string]bool // interface adapter types already emitted
	tests   []testRef
	// inRT is set when generating the std type declarations into package
	// rt itself.
	inRT bool
	// ms is the specialization state (mono.go).
	ms monoState
	// noMono turns specialization off for this package (generated
	// adapters in package main); noAdapt keeps it from recording types
	// for the adapter registry (adapt.go).
	noMono, noAdapt bool
}

func pkgIdent(path string) string {
	var b strings.Builder
	for _, r := range path {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r + 'a' - 'A')
		default:
			b.WriteByte('_')
		}
	}
	s := strings.Trim(b.String(), "_")
	if s == "" || s[0] >= '0' && s[0] <= '9' {
		s = "m_" + s
	}
	return "x_" + s
}

func (g *gen) newPkg(m *check.Module) *pkgGen {
	name := pkgIdent(m.Path)
	return &pkgGen{g: g, m: m, dir: "m/" + name, name: name, imports: map[string]string{}, adapt: map[string]bool{},
		ms: monoState{byKey: map[string]string{}, names: map[string]bool{}, perFunc: map[*check.FuncObj]int{}}}
}

// pkgRef returns the qualifier for module m as seen from pg ("" when m is
// pg's own module), importing it.
func (pg *pkgGen) pkgRef(m *check.Module) string {
	if m == nil || m.IsStd {
		return pg.rt()
	}
	if m == pg.m {
		return ""
	}
	other := pg.g.pkgs[m]
	if other == nil {
		return "MISSING_PKG_" + pkgIdent(m.Path)
	}
	path := ModulePath + "/" + other.dir
	pg.imports[path] = other.name
	return other.name + "."
}

// rt returns the runtime qualifier "rt.", importing the runtime.
func (pg *pkgGen) rt() string {
	if pg.inRT {
		return ""
	}
	pg.imports[ModulePath+"/rt"] = "rt"
	return "rt."
}

func (pg *pkgGen) file() ([]byte, error) {
	var b bytes.Buffer
	b.WriteString("// Code generated by xo from module " + pg.m.Path + ". DO NOT EDIT.\n\n")
	b.WriteString("package " + pg.name + "\n\n")
	if len(pg.imports) > 0 {
		var paths []string
		for p := range pg.imports {
			paths = append(paths, p)
		}
		sort.Strings(paths)
		b.WriteString("import (\n")
		for _, p := range paths {
			fmt.Fprintf(&b, "\t%s %q\n", pg.imports[p], p)
		}
		b.WriteString(")\n\n")
	}
	b.Write(pg.out.Bytes())
	src, err := format.Source(b.Bytes())
	if err != nil {
		return b.Bytes(), fmt.Errorf("gen: generated Go for module %s does not parse: %v", pg.m.Path, err)
	}
	return src, nil
}

// linePath is the path written in //line directives for a source file:
// relative to the module's xo.mod root when there is one.
func (g *gen) linePath(file string) string {
	if g.opts.LineRoot != "" {
		if abs, err := filepath.Abs(file); err == nil {
			if rel, err := filepath.Rel(g.opts.LineRoot, abs); err == nil && !strings.HasPrefix(rel, "..") {
				return filepath.ToSlash(rel)
			}
		}
	}
	return filepath.ToSlash(filepath.Clean(file))
}

// ---------------------------------------------------------------------------
// Names

func goField(name string) string  { return "F_" + name }
func goMethod(name string) string { return "M_" + name }
func goTParam(p *check.TParam) string {
	return "tp_" + p.Name
}

// stdPrefix is the Go name prefix of std module m's declarations in the
// runtime: "" for the prelude, "Http_" for std/http.
func stdPrefix(m *check.Module) string {
	if m == nil || m.IsPrelude {
		return ""
	}
	n := m.Name
	return strings.ToUpper(n[:1]) + n[1:] + "_"
}

// typeObjName is the Go name of a declared type, qualified for pg.
func (pg *pkgGen) typeObjName(o *check.TypeObj) string {
	if o.Module.IsStd {
		return pg.rt() + stdPrefix(o.Module) + o.Name
	}
	return pg.pkgRef(o.Module) + o.Name
}

// ---------------------------------------------------------------------------
// Types

var scalarGo = map[string]string{
	"Bool": "bool", "Int": "int64", "Int8": "int8", "Int16": "int16", "Int32": "int32", "Int64": "int64",
	"UInt8": "uint8", "UInt16": "uint16", "UInt32": "uint32", "UInt64": "uint64",
	"Float": "float64", "Float32": "float32", "Str": "string",
}

// isPreludeType reports whether t is the prelude type name.
func (g *gen) isPrelude(t check.Type, name string) bool {
	n, ok := check.Resolve(t).(*check.Named)
	return ok && n.Obj.Module != nil && n.Obj.Module.IsPrelude && n.Obj.Name == name
}

func (g *gen) named(t check.Type) *check.Named {
	n, _ := check.Resolve(t).(*check.Named)
	return n
}

// optionElem returns T for T? and nil otherwise.
func (g *gen) optionElem(t check.Type) check.Type {
	if n := g.named(t); n != nil && n.Obj.Module.IsPrelude && n.Obj.Name == "Option" {
		return n.Args[0]
	}
	return nil
}

// resultParts returns T and E for Result[T, E].
func (g *gen) resultParts(t check.Type) (check.Type, check.Type, bool) {
	if n := g.named(t); n != nil && n.Obj.Module.IsPrelude && n.Obj.Name == "Result" {
		return n.Args[0], n.Args[1], true
	}
	return nil, nil, false
}

func (g *gen) isInterface(t check.Type) bool {
	n := g.named(t)
	return n != nil && n.Obj.Kind == check.KInterface
}

// typeArgs returns the non effect type arguments of n.
func typeArgs(n *check.Named) []check.Type {
	var out []check.Type
	for i, a := range n.Args {
		if _, ok := a.(*check.EffArg); ok {
			continue
		}
		if i < len(n.Obj.TParams) && n.Obj.TParams[i].IsEffect {
			continue
		}
		out = append(out, a)
	}
	return out
}

// typeParams returns the non effect type parameters.
func typeParams(ps []*check.TParam) []*check.TParam {
	var out []*check.TParam
	for _, p := range ps {
		if !p.IsEffect {
			out = append(out, p)
		}
	}
	return out
}

func (pg *pkgGen) typeList(ts []check.Type) string {
	parts := make([]string, len(ts))
	for i, t := range ts {
		parts[i] = pg.goType(t)
	}
	return strings.Join(parts, ", ")
}

// goType is the Go type of an Xo type.
func (pg *pkgGen) goType(t check.Type) string {
	t = check.Resolve(t)
	switch x := t.(type) {
	case nil:
		return pg.rt() + "Unit"
	case *check.Var:
		switch x.LitKind() {
		case "float":
			return "float64"
		case "int":
			return "int64"
		}
		return pg.rt() + "Unit"
	case *check.BadType:
		return "any"
	case *check.ParamType:
		if pg.ms.sub != nil {
			if c := pg.ms.sub.Type(x.P); c != nil {
				return pg.goType(c)
			}
		}
		return goTParam(x.P)
	case *check.Tuple:
		if len(x.Elems) == 0 {
			return pg.rt() + "Unit"
		}
		return fmt.Sprintf("%sTuple%d[%s]", pg.rt(), len(x.Elems), pg.typeList(x.Elems))
	case *check.Func:
		return pg.funcType(x)
	case *check.Named:
		pg.noteType(x)
		return pg.namedType(x)
	}
	return "any"
}

// funcType is the Go type of a function value: func(*rt.Task, params)
// rt.Result[R, E].
func (pg *pkgGen) funcType(f *check.Func) string {
	var b strings.Builder
	b.WriteString("func(*" + pg.rt() + "Task")
	for _, p := range f.Params {
		b.WriteString(", ")
		if p.Var {
			b.WriteByte('*')
		}
		b.WriteString(pg.goType(p.T))
	}
	b.WriteString(") ")
	b.WriteString(pg.resultOf(f.Result, f.Err))
	return b.String()
}

// resultOf is rt.Result[goType(r), goType(e) or rt.Never].
func (pg *pkgGen) resultOf(r, e check.Type) string {
	et := pg.rt() + "Never"
	if e != nil {
		et = pg.goType(e)
	}
	return fmt.Sprintf("%sResult[%s, %s]", pg.rt(), pg.goType(r), et)
}

func (pg *pkgGen) namedType(n *check.Named) string {
	o := n.Obj
	args := typeArgs(n)
	if o.Module.IsPrelude {
		if s, ok := scalarGo[o.Name]; ok {
			return s
		}
		r := pg.rt()
		switch o.Name {
		case "Rune", "Bytes", "Duration", "Time", "Never", "StrBuilder", "Listener", "Conn":
			return r + o.Name
		case "List", "Map", "Set", "Buf", "Secret", "Range", "Option", "Result":
			return fmt.Sprintf("%s%s[%s]", r, o.Name, pg.typeList(args))
		case "Chan", "Mutex", "Atomic":
			return fmt.Sprintf("*%s%s[%s]", r, o.Name, pg.typeList(args))
		case "Scope":
			return "*" + r + "Scope"
		case "Task":
			return fmt.Sprintf("*%sHandle[%s]", r, pg.typeList(args))
		case "Display":
			return r + "IDisplay"
		case "Error":
			return r + "IError"
		case "Debug":
			return r + "IDebug"
		case "Eq", "Hash", "Ord", "Json", "OpenApi", "Proto":
			return "any"
		}
	}
	name := pg.typeObjName(o)
	if len(args) == 0 {
		return name
	}
	return name + "[" + pg.typeList(args) + "]"
}

// isScalar reports whether values of t are compared with Go's == and <.
func (g *gen) isScalar(t check.Type) bool {
	n := g.named(t)
	if n == nil || !n.Obj.Module.IsPrelude {
		return false
	}
	switch n.Obj.Name {
	case "Int", "Int8", "Int16", "Int32", "Int64", "UInt8", "UInt16", "UInt32", "UInt64",
		"Float", "Float32", "Str", "Rune", "Bytes", "Duration", "Time", "Bool":
		return true
	}
	return false
}

func (g *gen) isSigned(t check.Type) bool {
	n := g.named(t)
	if n == nil || !n.Obj.Module.IsPrelude {
		return false
	}
	switch n.Obj.Name {
	case "Int", "Int8", "Int16", "Int32", "Int64", "Duration":
		return true
	}
	return false
}

func (g *gen) isUnsigned(t check.Type) bool {
	n := g.named(t)
	if n == nil || !n.Obj.Module.IsPrelude {
		return false
	}
	switch n.Obj.Name {
	case "UInt8", "UInt16", "UInt32", "UInt64":
		return true
	}
	return false
}

func (g *gen) isFloat(t check.Type) bool {
	return g.isPrelude(t, "Float") || g.isPrelude(t, "Float32")
}

// isCapability reports whether t is a capability or a test fake: Os,
// TestOs, the capability interfaces, and the fakes (core 6.3, std.md 8).
func (g *gen) isCapability(t check.Type) bool {
	n := g.named(t)
	if n == nil || !n.Obj.Module.IsStd {
		return false
	}
	switch n.Obj.Name {
	case "Os", "Net", "Fs", "Clock", "Rand", "Env", "Proc", "Stdio",
		"TestOs", "FakeNet", "MemFs", "FakeClock", "FakeEnv", "FakeStdio", "FakeProc":
		return true
	}
	return false
}

// isBoxedField reports whether field f of struct obj is stored in an
// rt.Box: its type contains obj by value (`struct Node { next: Node? }`),
// which Go cannot lay out inline.
func (g *gen) isBoxedField(obj *check.TypeObj, f *check.FieldObj) bool {
	if g.boxed == nil {
		g.boxed = map[*check.FieldObj]bool{}
	}
	if b, ok := g.boxed[f]; ok {
		return b
	}
	b := g.containsByValue(f.Type, obj, map[*check.TypeObj]bool{})
	g.boxed[f] = b
	return b
}

// boxedFieldNamed is isBoxedField by field name.
func (g *gen) boxedFieldNamed(t check.Type, name string) bool {
	n := g.named(t)
	if n == nil || n.Obj.Kind != check.KStruct {
		return false
	}
	for _, f := range n.Obj.Fields {
		if f.Name == name {
			return g.isBoxedField(n.Obj, f)
		}
	}
	return false
}

// containsByValue reports whether a value of type t holds a value of the
// struct target inline: through Option, Result, tuples, newtypes, and
// struct fields, but not through collections, enums, or handles.
func (g *gen) containsByValue(t check.Type, target *check.TypeObj, seen map[*check.TypeObj]bool) bool {
	switch x := check.Resolve(t).(type) {
	case *check.Tuple:
		for _, e := range x.Elems {
			if g.containsByValue(e, target, seen) {
				return true
			}
		}
	case *check.Named:
		if x.Obj == target {
			return true
		}
		if x.Obj.Module.IsPrelude {
			switch x.Obj.Name {
			case "Option", "Result", "Secret":
				for _, a := range typeArgs(x) {
					if g.containsByValue(a, target, seen) {
						return true
					}
				}
			}
			return false
		}
		if seen[x.Obj] {
			return false
		}
		seen[x.Obj] = true
		s := check.NewSubst(x.Obj.TParams, x.Args)
		switch x.Obj.Kind {
		case check.KStruct:
			for _, f := range x.Obj.Fields {
				if g.containsByValue(s.Apply(f.Type), target, seen) {
					return true
				}
			}
		case check.KNewtype:
			return g.containsByValue(s.Apply(x.Obj.Target), target, seen)
		}
	}
	return false
}
