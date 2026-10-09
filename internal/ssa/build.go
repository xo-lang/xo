package ssa

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/xo-lang/xo/internal/ast"
	"github.com/xo-lang/xo/internal/check"
	"github.com/xo-lang/xo/internal/token"
)

// Error is a construct the builder does not support, with its position.
// It is also the source of the `--no-gc` check (nogc.go, diagnostic
// XO1201): Feature names the construct, Span locates it, and Impossible
// marks what no native backend can ever support (Go code needs Go's
// runtime), as opposed to what is planned.
type Error struct {
	Pos        string
	Msg        string
	Feature    string
	Span       token.Span
	Impossible bool
}

func (e *Error) Error() string {
	if e.Pos == "" {
		return e.Msg
	}
	return e.Pos + ": " + e.Msg
}

// Errors is every error of one build.
type Errors []*Error

func (es Errors) Error() string {
	var s []string
	for _, e := range es {
		s = append(s, e.Error())
	}
	return strings.Join(s, "\n")
}

// builder lowers a checked program.
type builder struct {
	opts   Options
	info   *check.Info
	mod    *Module
	main   *check.Module
	errs   Errors
	seen   map[string]bool
	funcs  map[string]*Func // by instance key (instKey)
	queue  []func()         // bodies to build
	nclos  int
	thunks map[string]*Func // named functions used as values, by instance key

	// concurrency (build_conc.go): conc builds with unwinding checks;
	// needConc records that the program uses a concurrency construct
	conc     bool
	needConc bool
	checks   []unwindCheck
	sites    map[*ast.CallExpr]int           // spawns that can fail a scope
	boundTo  map[*ast.CallExpr]*check.VarObj // spawn calls bound with let

	// natives are the implementation modules of the native backends by
	// the std path they implement (build_native.go)
	natives map[string]*check.Module

	// consts are the ids of the constants of heap types read so far
	// (build_int.go constValue)
	consts map[*check.ConstObj]int
}

// Build lowers the functions reachable from `main` of the root module of
// res into SSA, then prunes unreachable blocks, removes trivial block
// parameters, and renumbers. It does not verify; call Module.Verify.
//
// The subset is the core of the LLVM prototype's (bench/perf/llvm):
// Int, Float, Bool, Str (literals, interpolation, `+`, comparison),
// printing with os.stdio, structs (literals with defaults, `with`, field
// reads, nested field and element assignment), tuples, enums with
// payloads (Option included), `match` and `if let` with wildcard, binding,
// literal, range, `|`, tuple, variant, and struct patterns and guards,
// `if`, `while`, `for` over ranges and lists, `break` and `continue`
// (labeled too), `return`, functions and methods (`var`
// parameters and receivers), recursion, `const`, `??`, `Float(i)`,
// `Int(f)`, `fault`, and the List methods push, pop, set, get, len,
// is_empty, failing functions (a Result return, `Ok`, `Err`, `?` on
// Result with `from` conversion and on Option, the Result methods,
// `or_fault`), `==` and `!=` on every type (structural), `is`, and local
// `const`, `first`, `last`, `for (i, x) in xs.indexed()`, and the
// `if let x = opt` shorthand, closures and named functions as values
// (decision 0048), and generics (monomorphized: one Func per list of
// type arguments), and recursive types (recursive fields are boxed), and
// structured concurrency (decision 0068, build_conc.go): scopes, tasks,
// channels, select without `after`, Mutex, and Atomic, and (decision
// 0081, build_int.go) the sized integers and Rune, rune literals as
// integers, Bytes indexing, slicing of Str, Bytes, and List, StrBuilder
// and BytesBuilder, `os.fs.read`, `os.env.args`, `os.proc.exit`, and
// constants of collection types built once.
func Build(res *check.Result) (*Module, error) {
	return BuildOpts(res, Options{})
}

// Options configure Build.
type Options struct {
	// LineRoot is the directory fault positions are made relative to (as
	// internal/llvm prints them). Empty: positions use the file's base
	// name.
	LineRoot string
}

// BuildOpts is Build with options.
func BuildOpts(res *check.Result, opts Options) (*Module, error) {
	b, err := buildModule(res, opts, false)
	if !b.needConc {
		if err != nil {
			return nil, err
		}
		return b.mod, nil
	}
	// the program uses tasks: build again with unwinding checks
	b, err = buildModule(res, opts, true)
	if err != nil {
		return nil, err
	}
	return b.mod, nil
}

func buildModule(res *check.Result, opts Options, conc bool) (*builder, error) {
	info := res.Info()
	b := &builder{opts: opts, info: info, mod: NewModule(), seen: map[string]bool{}, funcs: map[string]*Func{}, thunks: map[string]*Func{},
		conc: conc, sites: map[*ast.CallExpr]int{}, boundTo: map[*ast.CallExpr]*check.VarObj{}}
	var mainFo *check.FuncObj
	for _, m := range res.Modules {
		if fo, ok := m.Scope["main"].(*check.FuncObj); ok && fo.Recv == nil {
			mainFo, b.main = fo, m
			break
		}
	}
	if mainFo == nil {
		return b, Errors{{Msg: "no `main` function in the root module"}}
	}
	mf := b.declare(mainFo, nil, mainFo.Decl)
	if et := failErr(mainFo.Err); et != nil {
		b.mainWrapper(mf, et, mainFo.Decl)
	}
	for len(b.queue) > 0 {
		next := b.queue[0]
		b.queue = b.queue[1:]
		next()
	}
	if len(b.errs) > 0 {
		return b, b.errs
	}
	if b.conc {
		b.pruneUnwind()
	}
	for _, f := range b.mod.Funcs {
		f.cleanup()
	}
	return b, nil
}

func (b *builder) pos(n ast.Node) string {
	if n == nil {
		return ""
	}
	sp := n.Span()
	return fmt.Sprintf("%s:%d:%d", b.file(sp.File), sp.Start.Line, sp.Start.Col)
}

// posLine is `file:line`, for the faults the Go backend locates from the
// stack (`List.set`, `Int.abs`), so every backend prints the same text.
func (b *builder) posLine(n ast.Node) string {
	sp := n.Span()
	return fmt.Sprintf("%s:%d", b.file(sp.File), sp.Start.Line)
}

func (b *builder) file(f string) string {
	if b.opts.LineRoot != "" {
		if abs, err := filepath.Abs(f); err == nil {
			if rel, err := filepath.Rel(b.opts.LineRoot, abs); err == nil && !strings.HasPrefix(rel, "..") {
				return filepath.ToSlash(rel)
			}
		}
		return filepath.ToSlash(filepath.Clean(f))
	}
	return filepath.Base(f)
}

// unsupported records a construct the native backends do not support
// yet (support is planned).
func (b *builder) unsupported(n ast.Node, what string) {
	b.unsupportedKind(n, what, false)
}

// impossible records a construct no native backend can support, such
// as a call into a `use go` package (Go code needs Go's runtime).
func (b *builder) impossible(n ast.Node, what string) {
	b.unsupportedKind(n, what, true)
}

func (b *builder) unsupportedKind(n ast.Node, what string, never bool) {
	p := b.pos(n)
	key := p + what
	if b.seen[key] {
		return
	}
	b.seen[key] = true
	e := &Error{Pos: p, Msg: "the SSA builder does not support " + what, Feature: what, Impossible: never}
	if n != nil {
		e.Span = n.Span()
	}
	b.errs = append(b.errs, e)
}

// ---------------------------------------------------------------------------
// Types

// typ lowers a checker type.
func (b *builder) typ(t check.Type, at ast.Node) *Type {
	m := b.mod
	if t == nil {
		return m.Builtin(KUnit)
	}
	t = check.Deep(t)
	switch r := check.Resolve(t).(type) {
	case *check.Tuple:
		var es []*Type
		for _, e := range r.Elems {
			es = append(es, b.typ(e, at))
		}
		return m.Tuple(es...)
	case *check.Func:
		var ps []*Type
		for _, p := range r.Params {
			if p.Var {
				b.unsupported(at, "function values with `var` parameters")
			}
			ps = append(ps, b.typ(p.T, at))
		}
		return m.FuncType(ps, b.typ(b.resultOf(r.Result, failErr(r.Err)), at))
	case *check.Var:
		switch r.LitKind() {
		case "float":
			return m.Builtin(KFloat)
		case "int":
			return m.Builtin(KInt)
		}
	case *check.Named:
		obj := r.Obj
		if obj.Module != nil && obj.Module.IsPrelude {
			switch obj.Name {
			case "Int", "Int64":
				return m.Builtin(KInt)
			case "Float":
				return m.Builtin(KFloat)
			case "Bool":
				return m.Builtin(KBool)
			case "Str", "Bytes":
				// Bytes is an immutable byte string like Str (decision 0069)
				return m.Builtin(KStr)
			case "StrBuilder", "BytesBuilder":
				// the text built so far: `write` appends in place while
				// the builder, move only, owns the only reference
				// (decision 0081)
				return m.Builtin(KStr)
			case "Int8", "Int16", "Int32", "UInt8", "UInt16", "UInt32", "UInt64", "Rune":
				// sized integers in the Int word (decision 0081)
				return m.IntType(obj.Name)
			case "Duration", "Time":
				// nanoseconds (decision 0069)
				return m.Builtin(KInt)
			case "Never":
				return m.Builtin(KUnit)
			case "Os", "Stdio", "Clock", "Net", "Proc", "Env", "Fs":
				x, _ := m.Named(obj.Name, KCap)
				return x
			case "List":
				return m.List(b.typ(r.Args[0], at))
			case "Map":
				return b.mapType(false, b.typ(r.Args[0], at), b.typ(r.Args[1], at))
			case "Set":
				return b.mapType(true, b.typ(r.Args[0], at), m.Builtin(KUnit))
			case "Result":
				// `Result[T, Never]` is T: what a function of a generic
				// error type returns when that type is Never (decision
				// 0074; `T ! Never` is T, see typeOf)
				if len(r.Args) == 2 && check.IsNever(r.Args[1]) {
					return b.typ(r.Args[0], at)
				}
			}
		}
		name := r.String()
		if obj.Module != nil && obj.Module.Go != nil {
			b.impossible(at, "the type `"+name+"` of the Go package `"+obj.Module.Go.GoPath+"` (`use go`)")
			return m.Builtin(KUnit)
		}
		switch obj.Kind {
		case check.KStruct:
			if x := b.concType(obj, r, at); x != nil {
				return x
			}
			if it := b.implType(obj); it != nil {
				// the native implementation's struct (decision 0074)
				return b.typ(&check.Named{Obj: it, Args: r.Args}, at)
			}
			if obj.Module != nil && strings.HasPrefix(obj.Module.Path, "native/") && obj.Opaque {
				// an object of the native runtime (decision 0074); a
				// Loc is the `file:line` of a call
				if obj.Name == "Loc" {
					return m.Builtin(KStr)
				}
				return m.Handle(strings.ReplaceAll(obj.Module.Path, "/", ".") + "." + obj.Name)
			}
			if obj.Module != nil && obj.Module.IsStd && (obj.Opaque || obj.Module.IsPrelude) {
				break
			}
			x, created := m.Named(name, KStruct)
			if created {
				sub := check.NewSubst(obj.TParams, r.Args)
				for _, f := range obj.Fields {
					ft := sub.Apply(f.Type)
					x.Fields = append(x.Fields, Field{f.Name, b.boxed(ft, b.typ(ft, at), name)})
				}
			}
			return x
		case check.KEnum:
			x, created := m.Named(name, KEnum)
			if created {
				sub := check.NewSubst(obj.TParams, r.Args)
				for _, v := range obj.Variants {
					nv := &Variant{Name: v.Name}
					for i, f := range v.Fields {
						fn := f.Name
						if fn == "" {
							fn = strconv.Itoa(i)
						}
						ct := sub.Apply(f.Type)
						ft := b.typ(ct, at)
						if obj.Module == nil || !obj.Module.IsPrelude {
							ft = b.boxed(ct, ft, name)
						}
						nv.Fields = append(nv.Fields, Field{fn, ft})
					}
					x.Variants = append(x.Variants, nv)
				}
			}
			return x
		case check.KAlias:
			return b.typ(check.NewSubst(obj.TParams, r.Args).Apply(obj.Target), at)
		case check.KInterface:
			if dynIface(obj) {
				// its text (build_show.go)
				return b.dynType()
			}
			b.unsupported(at, "interface types (`"+obj.Name+"`)")
			return m.Builtin(KUnit)
		}
	}
	b.unsupported(at, "the type `"+t.String()+"`")
	return m.Builtin(KUnit)
}

// boxed returns the type of a field of the user struct or enum named
// owner: Box[ft] when the field's checker type ct reaches owner by
// value (a recursive type), else ft (the lowered ct).
//
// The decision is made on the type definition graph (the checker's
// types, with type arguments substituted), not on the SSA types built
// so far, so it does not depend on which type of a cycle the program
// happens to use first (a `Node?` met before `Node` used to leave the
// cycle unboxed). Each by-value cycle is boxed once, at the edge out of
// its least user type by name: a field of owner is boxed when it
// reaches owner through user types whose names sort after owner (and
// through prelude enums and tuples, which are never boxed). Every cycle
// has such an edge, and the choice is a property of the cycle alone
// (decision 0052).
func (b *builder) boxed(ct check.Type, ft *Type, owner string) *Type {
	if reachesByValue(ct, owner, map[string]bool{}) {
		return b.mod.Box(ft)
	}
	return ft
}

// reachesByValue reports whether a value of checker type t holds a
// value of the struct or enum named owner (its SSA name: the
// instantiated type's String) inline: through struct fields, enum
// payloads (prelude Option and Result included), tuple elements, and
// aliases, not through lists, function values, or builtins, and not
// through user types named before owner (their own cycles are boxed at
// their fields). seen holds the named types already walked.
func reachesByValue(t check.Type, owner string, seen map[string]bool) bool {
	if t == nil {
		return false
	}
	t = check.Deep(t)
	switch r := check.Resolve(t).(type) {
	case *check.Tuple:
		for _, e := range r.Elems {
			if reachesByValue(e, owner, seen) {
				return true
			}
		}
	case *check.Named:
		obj := r.Obj
		if obj.Module != nil && obj.Module.IsPrelude {
			switch obj.Name {
			case "Int", "Int64", "Float", "Bool", "Str", "Never", "Os", "Stdio", "List":
				return false
			}
		}
		switch obj.Kind {
		case check.KAlias:
			return reachesByValue(check.NewSubst(obj.TParams, r.Args).Apply(obj.Target), owner, seen)
		case check.KStruct, check.KEnum:
			if obj.Kind == check.KStruct && obj.Module != nil && obj.Module.IsStd {
				return false
			}
			name := r.String()
			if name == owner {
				return true
			}
			prelude := obj.Module != nil && obj.Module.IsPrelude
			if seen[name] || (!prelude && name < owner) {
				return false
			}
			seen[name] = true
			sub := check.NewSubst(obj.TParams, r.Args)
			for _, f := range obj.Fields {
				if reachesByValue(sub.Apply(f.Type), owner, seen) {
					return true
				}
			}
			for _, v := range obj.Variants {
				for _, f := range v.Fields {
					if reachesByValue(sub.Apply(f.Type), owner, seen) {
						return true
					}
				}
			}
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Functions

func (b *builder) funcName(fo *check.FuncObj) string {
	n := fo.DisplayName()
	if fo.Module != nil && fo.Module != b.main {
		n = fo.Module.Name + "." + n
	}
	return n
}

// typeParams returns the type parameters of fo that take types (not
// effects): the implicit receiver parameters first, then the declared.
func typeParams(fo *check.FuncObj) []*check.TParam {
	var out []*check.TParam
	for _, p := range fo.TParams {
		if !p.IsEffect {
			out = append(out, p)
		}
	}
	return out
}

// instKey names the instance of fo for type arguments targs (SSA
// types, one per typeParams entry): its Func name, and the key that
// tells instances of different functions with one name apart.
func (b *builder) instKey(fo *check.FuncObj, targs []*Type) (name, key string) {
	name = b.funcName(fo)
	if len(targs) > 0 {
		var ns []string
		for _, t := range targs {
			ns = append(ns, t.Name)
		}
		name += "[" + strings.Join(ns, ", ") + "]"
	}
	return name, fmt.Sprintf("%p:%s", fo, name)
}

// declare returns the SSA function of fo instantiated with targs (the
// checker types of its type parameters, already concrete; nil for a
// function that is not generic), creating its signature and queueing
// its body on first use. Generic functions are monomorphized here, one
// Func per distinct list of type arguments, as internal/gen's
// monomorphizer and the AST path of internal/llvm do.
func (b *builder) declare(fo *check.FuncObj, targs []check.Type, at ast.Node) *Func {
	tps := typeParams(fo)
	if len(targs) != len(tps) {
		b.unsupported(at, "a generic call whose type arguments are not inferred (`"+fo.DisplayName()+"`)")
		return b.mod.NewFunc(b.funcName(fo)+"?", nil, []*Type{b.mod.Builtin(KUnit)})
	}
	var sub *check.Subst
	var sargs []*Type
	if len(tps) > 0 {
		sub = check.NewSubst(tps, targs)
		for _, t := range targs {
			sargs = append(sargs, b.typ(t, at))
		}
	}
	apply := func(t check.Type) check.Type {
		if t == nil {
			return nil
		}
		if sub != nil {
			t = sub.Apply(t)
		}
		return check.Deep(t)
	}
	name, key := b.instKey(fo, sargs)
	if f := b.funcs[key]; f != nil {
		return f
	}
	var params []*Type
	var varIdx []int
	if fo.Recv != nil {
		params = append(params, b.typ(apply(fo.RecvType), at))
		if fo.RecvVar {
			varIdx = append(varIdx, 0)
		}
	}
	for _, p := range fo.Params {
		if p.Var {
			varIdx = append(varIdx, len(params))
		}
		params = append(params, b.typ(apply(p.Type), at))
	}
	results := []*Type{b.typ(b.resultOf(apply(fo.Result), failErr(apply(fo.Err))), at)}
	for _, i := range varIdx {
		results = append(results, params[i])
	}
	f := b.mod.NewFunc(name, params, results)
	f.VarParams = varIdx
	b.funcs[key] = f
	b.queue = append(b.queue, func() { b.body(fo, f, sub) })
	return f
}

// failErr is the error type of a function that can fail, or nil.
func failErr(e check.Type) check.Type {
	if e == nil || check.IsNever(e) {
		return nil
	}
	return e
}

// preludeType is the prelude type name applied to args.
func (b *builder) preludeType(name string, args ...check.Type) check.Type {
	obj, _ := b.info.Prelude().Scope[name].(*check.TypeObj)
	return &check.Named{Obj: obj, Args: args}
}

// resultOf is what a function with success type res and error type err
// returns: res, or Result[res, err] when err is not nil (core 2.6).
func (b *builder) resultOf(res, err check.Type) check.Type {
	if err == nil {
		return res
	}
	if res == nil {
		res = &check.Tuple{}
	}
	return b.preludeType("Result", res, err)
}

// fn is the state of one function body under construction. Locals are
// renamed into SSA by the algorithm of Braun et al., "Simple and
// Efficient Construction of Static Single Assignment Form" (CC 2013),
// with block parameters as phis.
type fn struct {
	b   *builder
	fo  *check.FuncObj
	f   *Func
	cur *Block

	defs       map[*check.VarObj]map[*Block]*Value
	varType    map[*check.VarObj]*Type
	sealed     map[*Block]bool
	incomplete map[*Block][]*Value // parameters added before the block was sealed
	paramVar   map[*Value]*check.VarObj
	loops      []*loopCtx
	varParams  []*check.VarObj // bindings of `var` parameters, returned updated
	// packVar: the function returns its result and its `var` parameters
	// as one tuple (a closure given to Mutex.with, build_conc.go)
	packVar bool
	// packOrig is the value a packed closure was given: what it hands
	// back when it fails or unwinds (packedValue)
	packOrig *Value
	scopes   []*scopeCtx // the scopes open around the current code
	unwindB  *Block      // the function's unwind block (lazy)
	// exits are the blocks and scopes open around the current code, the
	// innermost last: leaving them runs each block's pending `defer`
	// statements and closes each scope (build_defer.go)
	exits []exitEntry
	// shield counts the deferred code being lowered around the current
	// code: it is shielded from cancellation, so it never unwinds
	shield int

	// res is the success type, errT the error type of a function that
	// can fail (nil otherwise); its SSA result is then Result[res, errT].
	res  check.Type
	errT check.Type
	// sub maps the type parameters of the instance being built (nil
	// when the function is not generic).
	sub *check.Subst
}

// apply resolves a checker type in the instance being built.
func (s *fn) apply(t check.Type) check.Type {
	if t == nil {
		return nil
	}
	if s.sub != nil {
		t = s.sub.Apply(t)
	}
	return check.Deep(t)
}

type loopCtx struct {
	label     string
	brk, cont *Block
	scopes    int // the scopes open around the loop (left by break and continue)
	exits     int // the exits open around the loop (left by break and continue)
}

// newFn starts building the body of f.
func (b *builder) newFn(f *Func, res, errT check.Type) *fn {
	s := &fn{b: b, f: f, cur: f.Blocks[0],
		defs: map[*check.VarObj]map[*Block]*Value{}, varType: map[*check.VarObj]*Type{},
		sealed: map[*Block]bool{}, incomplete: map[*Block][]*Value{}, paramVar: map[*Value]*check.VarObj{},
		res: res, errT: errT}
	s.sealed[s.cur] = true
	return s
}

func (b *builder) body(fo *check.FuncObj, f *Func, sub *check.Subst) {
	s := b.newFn(f, nil, nil)
	s.fo = fo
	s.sub = sub
	s.res, s.errT = s.apply(fo.Result), failErr(s.apply(fo.Err))
	var vars []*check.VarObj
	if fo.Recv != nil {
		vars = append(vars, b.info.RecvVar(fo))
	}
	vars = append(vars, b.info.ParamVars(fo)...)
	for i, vo := range vars {
		if vo != nil && i < len(f.Params()) {
			s.writeVar(vo, f.Params()[i])
		}
	}
	for _, i := range f.VarParams {
		s.varParams = append(s.varParams, vars[i])
	}
	if fo.Decl == nil || fo.Decl.Body == nil {
		b.unsupported(fo.Decl, "a function without a body")
		return
	}
	for _, cl := range fo.Decl.Clauses {
		if c, ok := cl.(*ast.RequiresClause); ok {
			s.requires(c)
		}
	}
	v := s.block(fo.Decl.Body)
	s.ret(v, fo.Decl.Body)
	s.finishFn()
}

// mainWrapper makes `main` of a main that returns an error (type et):
// the program's main becomes main.run, and main calls it; an error is
// printed as `error: <its Display text>` on standard error and the
// program exits with status 1, as on the Go backend (without the error's
// frames and crash bundle).
func (b *builder) mainWrapper(mf *Func, et check.Type, at ast.Node) {
	m := b.mod
	mf.Name = "main.run"
	var params []*Type
	for _, p := range mf.Params() {
		params = append(params, p.Type)
	}
	unit, intT, boolT, st := m.Builtin(KUnit), m.Builtin(KInt), m.Builtin(KBool), m.Builtin(KStr)
	b.genFunc("main", params, []*Type{unit}, func(s *fn, ps []*Value) {
		rt := mf.Results[0]
		r := s.call(mf.Name, rt, "", ps...)
		ei := rt.VariantIndex("Err")
		s.ifThen(s.add(OpEq, boolT, nil, s.add(OpTag, intT, nil, r), s.cint(int64(ei))), func() {
			e := s.add(OpPayload, rt.Variants[ei].Fields[0].Type, PayloadRef{ei, 0}, r)
			msg := s.appendDisplay(s.add(OpConst, st, "error: "), e, et, at)
			s.rt("eprintln", nil, msg)
			s.rt("exit", nil, s.cint(1))
		})
		s.retv(s.unit())
	})
}

// requires lowers a `requires` clause (core 9.1): without `else` a false
// condition faults, with it the function returns Err(else value).
func (s *fn) requires(c *ast.RequiresClause) {
	b := s.b
	boolT := b.mod.Builtin(KBool)
	ok := s.valueAs(c.Cond, boolT)
	fail, good := s.branch(s.add(OpNot, boolT, nil, ok))
	s.cur = fail
	if c.Else == nil {
		f := s.add(OpFault, nil, nil, s.add(OpConst, b.mod.Builtin(KStr), "requires failed: "+b.sourceText(c.Cond)))
		f.Pos = b.pos(c.Cond)
	} else if s.errT == nil {
		b.unsupported(c, "`requires ... else` in a function that cannot fail")
		s.add(OpUnreachable, nil, nil)
	} else {
		ev := s.coerceErr(s.expr(c.Else), s.natCheck(c.Else), s.errT, c.Else)
		want := s.resultT()
		s.ret(s.add(OpVariant, want, want.VariantIndex("Err"), ev), c)
	}
	s.cur = good
}

// sourceText is the source text of n.
func (b *builder) sourceText(n ast.Node) string {
	sp := n.Span()
	for _, m := range b.info.AllModules() {
		for _, f := range m.Files {
			if f.Path == sp.File && sp.Start.Offset >= 0 && sp.End.Offset <= len(f.Src) && sp.Start.Offset <= sp.End.Offset {
				return string(f.Src[sp.Start.Offset:sp.End.Offset])
			}
		}
	}
	return "?"
}

// ---------------------------------------------------------------------------
// SSA construction (Braun et al.)

func (s *fn) newBlock() *Block { return s.f.NewBlock() }

// dead starts an unreachable block for code after a terminator; it is
// pruned after building.
func (s *fn) dead() {
	s.cur = s.newBlock()
	s.sealed[s.cur] = true
}

func (s *fn) writeVar(vo *check.VarObj, v *Value) {
	if s.defs[vo] == nil {
		s.defs[vo] = map[*Block]*Value{}
	}
	s.defs[vo][s.cur] = v
	if _, ok := s.varType[vo]; !ok {
		s.varType[vo] = v.Type
	}
}

func (s *fn) readVar(vo *check.VarObj, bl *Block) *Value {
	if v := s.defs[vo][bl]; v != nil {
		return v
	}
	var v *Value
	switch {
	case !s.sealed[bl]:
		v = s.newParam(bl, vo)
		s.incomplete[bl] = append(s.incomplete[bl], v)
	case len(bl.Preds) == 1:
		v = s.readVar(vo, bl.Preds[0].From)
	default:
		v = s.newParam(bl, vo)
		s.defs[vo][bl] = v
		s.addArgs(vo, v)
	}
	s.defs[vo][bl] = v
	return v
}

func (s *fn) newParam(bl *Block, vo *check.VarObj) *Value {
	t := s.varType[vo]
	if t == nil {
		t = s.b.mod.Builtin(KUnit) // read before any write: only in dead code
	}
	p := bl.AddParam(t)
	s.paramVar[p] = vo
	if s.defs[vo] == nil {
		s.defs[vo] = map[*Block]*Value{}
	}
	return p
}

// addArgs fills the argument of parameter p of its block on every
// incoming edge.
func (s *fn) addArgs(vo *check.VarObj, p *Value) {
	bl := p.Block
	idx := -1
	for i, q := range bl.Params {
		if q == p {
			idx = i
		}
	}
	for _, e := range bl.Preds {
		a := s.readVar(vo, e.From)
		for len(e.Args) <= idx {
			e.Args = append(e.Args, nil)
		}
		e.Args[idx] = a
	}
}

// seal records that every predecessor of bl is known.
func (s *fn) seal(bl *Block) {
	if s.sealed[bl] {
		return
	}
	for i := 0; i < len(s.incomplete[bl]); i++ {
		// the parameter itself, not defs[vo][bl]: a later write in bl
		// replaces that entry (gotchas/ssa-braun-incomplete-phi.md)
		p := s.incomplete[bl][i]
		s.addArgs(s.paramVar[p], p)
	}
	delete(s.incomplete, bl)
	s.sealed[bl] = true
}

// read reads a local in the current block.
func (s *fn) read(vo *check.VarObj) *Value { return s.readVar(vo, s.cur) }

// ---------------------------------------------------------------------------
// Emission helpers

func (s *fn) add(op Op, t *Type, aux any, args ...*Value) *Value {
	return s.cur.Add(op, t, aux, args...)
}

func (s *fn) unit() *Value { return s.add(OpTuple, s.b.mod.Builtin(KUnit), nil) }

func (s *fn) undef(t *Type) *Value { return s.add(OpUndef, t, nil) }

// unbox reads the value out of a Box (a field of a recursive type);
// other values are returned as they are.
func (s *fn) unbox(v *Value) *Value {
	if v.Type.Kind != KBox {
		return v
	}
	return s.add(OpUnbox, v.Type.Elems[0], nil, v)
}

func (s *fn) cint(n int64) *Value { return s.add(OpConst, s.b.mod.Builtin(KInt), n) }

func (s *fn) bool(x bool) *Value { return s.add(OpConst, s.b.mod.Builtin(KBool), x) }

func (s *fn) call(name string, t *Type, pos string, args ...*Value) *Value {
	v := s.add(OpCall, t, name, args...)
	v.Pos = pos
	return v
}

// jump ends the current block with a jump carrying value arguments.
func (s *fn) jump(to *Block, args ...*Value) { s.cur.Jump(to, args...) }

// branch ends the current block and returns the two new single
// predecessor successor blocks, sealed.
func (s *fn) branch(cond *Value) (yes, no *Block) {
	yes, no = s.newBlock(), s.newBlock()
	s.cur.Branch(cond, yes, no)
	s.seal(yes)
	s.seal(no)
	return yes, no
}

// test continues in a new block when cond holds and goes to fail when it
// does not.
func (s *fn) test(cond *Value, fail *Block) {
	ok := s.newBlock()
	s.cur.Branch(cond, ok, fail)
	s.seal(ok)
	s.cur = ok
}

// isDead reports whether the current block is unreachable: code after a
// terminator (return, break, a call that never returns), or the join of
// branches that all left.
func (s *fn) isDead() bool {
	return s.cur != s.f.Blocks[0] && len(s.cur.Preds) == 0 && s.sealed[s.cur]
}

// ret returns v and the updated `var` parameters. v is a value of the
// success type (converted: Some wrapping, unit) or, in a function that
// can fail, its Result as is; a success value is then wrapped in `Ok`.
func (s *fn) ret(v *Value, at ast.Node) {
	want := s.resultT()
	if s.isDead() {
		s.add(OpUnreachable, nil, nil)
		s.dead()
		return
	}
	if s.errT != nil {
		if v.Type != want {
			ok := want.VariantIndex("Ok")
			v = s.add(OpVariant, want, ok, s.coerce(v, want.Variants[ok].Fields[0].Type, at))
		}
	} else {
		v = s.coerce(v, want, at)
	}
	args := []*Value{v}
	for _, vo := range s.varParams {
		args = append(args, s.read(vo))
	}
	if s.packVar {
		if s.packOrig != nil && len(args) == 2 && s.errT != nil {
			args[1] = s.packedValue(v, args[1])
		}
		args = []*Value{s.add(OpTuple, s.f.Results[0], nil, args...)}
	}
	s.exitTo(0)
	s.add(OpReturn, nil, nil, args...)
	s.dead()
}

// packedValue is the value a packed closure (Mutex.with) hands back with
// its result r: the updated value cur when r is a success, the value it
// was given (packOrig) when r is an error, since the mutex then keeps
// the old value (core 5.4). The caller moves the value out of the mutex
// and always stores what comes back, so a closure that cannot fail
// after it changed the value updates it in place: only when the given
// value is still live after a change (an error may follow) does the RC
// pass keep a reference, and the change copies.
func (s *fn) packedValue(r, cur *Value) *Value {
	errI := r.Type.VariantIndex("Err")
	if cur == s.packOrig || errI < 0 {
		return cur
	}
	if r.Op == OpVariant {
		if r.Aux.(int) == errI {
			return s.packOrig
		}
		return cur
	}
	m := s.b.mod
	join := s.newBlock()
	res := join.AddParam(cur.Type)
	isErr := s.add(OpEq, m.Builtin(KBool), nil, s.add(OpTag, m.Builtin(KInt), nil, r), s.cint(int64(errI)))
	yes, no := s.branch(isErr)
	s.cur = yes
	s.jump(join, s.packOrig)
	s.cur = no
	s.jump(join, cur)
	s.seal(join)
	s.cur = join
	return res
}

// resultT is the type the function's body returns (its result; for a
// packed closure the first element of its tuple).
func (s *fn) resultT() *Type {
	if s.packVar {
		return s.f.Results[0].Elems[0]
	}
	return s.f.Results[0]
}

// ---------------------------------------------------------------------------
// Types of expressions

func (s *fn) typeOf(e ast.Expr) *Type {
	t := s.b.info.TypeOf(e)
	if t == nil {
		t = s.b.info.NaturalType(e)
	}
	if check.IsNever(t) {
		return s.b.mod.Builtin(KUnit)
	}
	t = s.apply(t)
	if rt, et, ok := resultParts(t); ok && check.IsNever(et) {
		// `T ! Never` is T: the type a closure body is checked against
		// when it is passed for `fn() -> T ! E` and E is Never (spawn,
		// Mutex.with)
		t = rt
	}
	return s.b.typ(t, e)
}

// checkType is the checker type of e in context.
func (s *fn) checkType(e ast.Expr) check.Type {
	t := s.b.info.TypeOf(e)
	if t == nil {
		t = s.b.info.NaturalType(e)
	}
	return s.apply(t)
}

// natCheck is the checker type of e before implicit conversion.
func (s *fn) natCheck(e ast.Expr) check.Type {
	t := s.b.info.NaturalType(e)
	if t == nil {
		t = s.b.info.TypeOf(e)
	}
	return s.apply(t)
}

func (s *fn) natOf(e ast.Expr) *Type {
	t := s.b.info.NaturalType(e)
	if t == nil {
		t = s.b.info.TypeOf(e)
	}
	return s.b.typ(s.apply(t), e)
}

func (s *fn) isNever(e ast.Expr) bool {
	t := s.b.info.NaturalType(e)
	if t == nil {
		t = s.b.info.TypeOf(e)
	}
	return check.IsNever(t)
}

// coerce converts v to t: the implicit `T` to `Option[T]`.
func (s *fn) coerce(v *Value, t *Type, at ast.Node) *Value {
	if v.Type != t && s.isDead() {
		// the value of code after a `return` (an `if` arm that returns)
		return s.undef(t)
	}
	if v.Type == t || v.Op == OpUndef {
		if v.Op == OpUndef && v.Type != t {
			return s.undef(t)
		}
		return v
	}
	if t.Kind == KBox {
		return s.add(OpBox, t, nil, s.coerce(v, t.Elems[0], at))
	}
	if t.Kind == KEnum {
		if i := t.VariantIndex("Some"); i >= 0 && len(t.Variants[i].Fields) == 1 && t.Variants[i].Fields[0].Type == v.Type {
			return s.add(OpVariant, t, i, v)
		}
		// the success value where the Result is expected (the value of
		// an `if` without `else` that ends a failing function)
		if i := t.VariantIndex("Ok"); i >= 0 && t.VariantIndex("Err") >= 0 && len(t.Variants[i].Fields) == 1 && t.Variants[i].Fields[0].Type == v.Type {
			return s.add(OpVariant, t, i, v)
		}
	}
	if t.Kind == KUnit {
		return s.unit()
	}
	s.b.unsupported(at, "converting `"+v.Type.Name+"` to `"+t.Name+"`")
	return s.undef(t)
}

// value evaluates e in its context type.
func (s *fn) value(e ast.Expr) *Value {
	v := s.expr(e)
	return s.coerce(v, s.typeOf(e), e)
}

func (s *fn) valueAs(e ast.Expr, t *Type) *Value {
	v := s.expr(e)
	if isDyn(t) && !isDyn(v.Type) && v.Op != OpUndef {
		// a concrete value converted to Error, Display, or Debug
		return s.toDyn(v, s.natCheck(e), s.ifaceName(e), e)
	}
	return s.coerce(v, t, e)
}

// ifaceName is the name of the prelude interface e is converted to in
// its context ("Debug" for a log field).
func (s *fn) ifaceName(e ast.Expr) string {
	if n, ok := check.Resolve(check.Deep(s.checkType(e))).(*check.Named); ok && n.Obj.Kind == check.KInterface {
		return n.Obj.Name
	}
	return "Display"
}
