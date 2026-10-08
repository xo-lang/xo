package ssa

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/xo-lang/xo/internal/ast"
	"github.com/xo-lang/xo/internal/check"
)

// Error is a construct the builder does not support, with its position.
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
// type arguments). Not yet: recursive types.
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
	info := res.Info()
	b := &builder{opts: opts, info: info, mod: NewModule(), seen: map[string]bool{}, funcs: map[string]*Func{}, thunks: map[string]*Func{}}
	var mainFo *check.FuncObj
	for _, m := range res.Modules {
		if fo, ok := m.Scope["main"].(*check.FuncObj); ok && fo.Recv == nil {
			mainFo, b.main = fo, m
			break
		}
	}
	if mainFo == nil {
		return nil, Errors{{Msg: "no `main` function in the root module"}}
	}
	if failErr(mainFo.Err) != nil {
		b.unsupported(mainFo.Decl, "`main` that returns an error (declare `fn main(os: Os)` and handle errors with match)")
	}
	b.declare(mainFo, nil, mainFo.Decl)
	for len(b.queue) > 0 {
		next := b.queue[0]
		b.queue = b.queue[1:]
		next()
	}
	if len(b.errs) > 0 {
		return nil, b.errs
	}
	for _, f := range b.mod.Funcs {
		f.cleanup()
	}
	return b.mod, nil
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

func (b *builder) unsupported(n ast.Node, what string) {
	p := b.pos(n)
	key := p + what
	if b.seen[key] {
		return
	}
	b.seen[key] = true
	b.errs = append(b.errs, &Error{Pos: p, Msg: "the SSA builder does not support " + what})
}

// ---------------------------------------------------------------------------
// Types

func (b *builder) prelude(t *check.Named, name string) bool {
	return t.Obj.Module != nil && t.Obj.Module.IsPrelude && t.Obj.Name == name
}

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
			case "Str":
				return m.Builtin(KStr)
			case "Never":
				return m.Builtin(KUnit)
			case "Os", "Stdio":
				x, _ := m.Named(obj.Name, KCap)
				return x
			case "List":
				return m.List(b.typ(r.Args[0], at))
			}
		}
		name := r.String()
		switch obj.Kind {
		case check.KStruct:
			if obj.Module != nil && obj.Module.IsStd {
				break
			}
			x, created := m.Named(name, KStruct)
			if created {
				sub := check.NewSubst(obj.TParams, r.Args)
				for _, f := range obj.Fields {
					x.Fields = append(x.Fields, Field{f.Name, b.typ(sub.Apply(f.Type), at)})
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
						nv.Fields = append(nv.Fields, Field{fn, b.typ(sub.Apply(f.Type), at)})
					}
					x.Variants = append(x.Variants, nv)
				}
			}
			return x
		case check.KAlias:
			return b.typ(check.NewSubst(obj.TParams, r.Args).Apply(obj.Target), at)
		}
	}
	b.unsupported(at, "the type `"+t.String()+"`")
	return m.Builtin(KUnit)
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
	v := s.block(fo.Decl.Body)
	s.ret(v, fo.Decl.Body)
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
	want := s.f.Results[0]
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
	s.add(OpReturn, nil, nil, args...)
	s.dead()
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
	return s.b.typ(s.apply(t), e)
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
	if v.Type == t || v.Op == OpUndef {
		if v.Op == OpUndef && v.Type != t {
			return s.undef(t)
		}
		return v
	}
	if t.Kind == KEnum {
		if i := t.VariantIndex("Some"); i >= 0 && len(t.Variants[i].Fields) == 1 && t.Variants[i].Fields[0].Type == v.Type {
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
	return s.coerce(s.expr(e), t, e)
}
