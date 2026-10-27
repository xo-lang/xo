package gobind

import (
	"bytes"
	"fmt"
	"go/constant"
	"go/format"
	"go/types"
	"io/fs"
	"math"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/xo-lang/xo/std"
)

// Reason categories of skipped declarations (Stats.Reasons).
const (
	catGeneric   = "generic function or type"
	catAny       = "interface{} or any"
	catIface     = "unnamed interface or struct"
	catChan      = "channel"
	catFunc      = "function result or unmappable callback"
	catPointer   = "pointer to a non handle type"
	catBasic     = "unsupported basic type (complex, uintptr, unsafe.Pointer)"
	catError     = "error outside the final result"
	catArray     = "array of non byte elements"
	catResults   = "more than 6 results"
	catForeign   = "type from another package beyond the binding depth"
	catUnexport  = "unexported type"
	catPtrRecv   = "pointer receiver on a value type"
	catCollision = "name collision after snake_case"
	catCtx       = "context.Context outside the first parameter"
	catNamedBase = "method of a named basic type (the type maps to its basic type)"
)

// skipErr is a reason a declaration cannot be mapped.
type skipErr struct {
	cat    string
	detail string
}

func (e *skipErr) Error() string { return e.detail }

func skipf(cat, format string, args ...any) *skipErr {
	return &skipErr{cat: cat, detail: fmt.Sprintf(format, args...)}
}

// conv maps one Go type to Xo.
type conv struct {
	xo   string              // Xo type syntax
	rt   string              // Go type of the Xo representation (package rt)
	gt   string              // Go type in the bound package's terms
	id   bool                // the representations are the same Go type
	toXo func(string) string // Go value (an addressable expression) to Xo
	toGo func(string) string // Xo value to Go
	// nilable is set for pointer handles: a result becomes `T?`.
	nilable bool
	// hnd is the handle of a pointer handle conversion.
	hnd *handle
	// hpkgs are the Go import paths of the handles the type holds (none:
	// a boundary value, which record and replay copy, decision 0093).
	hpkgs []string
}

func identity(e string) string { return e }

// handle is an opaque Xo type wrapping a Go value: `struct{ P *T }` for
// structs and arrays (held by pointer), `struct{ V T }` for interfaces,
// maps, slices, and functions.
type handle struct {
	obj    *types.TypeName
	named  *types.Named
	xo     string // Xo type name
	goName string // Go type name in package rt
	gt     string // qualified Go type (without pointer)
	byPtr  bool
	// byValue: some bound signature or field has the Go type T itself (not
	// *T): Go hands it out as a value, and Go's == compares it so
	byValue bool
	foreign bool
	names   map[string]bool // Xo method names in use
	decls   []string
}

// gen generates one binding.
type gen struct {
	b       *Binding
	pkg     *types.Package
	imports map[string]string // Go path -> alias in the adapter
	handles map[*types.TypeName]*handle
	order   []*handle
	queue   []*handle
	scope   map[string]bool // Xo names in the module scope
	consts  []string
	funcs   []string
	vars    []string
	adapt   bytes.Buffer
	// rerun names the functions (`name`, `Type.method`) that an xo.mod
	// `replay rerun go` line marks; rerunAll marks every one.
	rerun    map[string]bool
	rerunAll bool
	rerunKey string
	skips    []string // "name: reason" in declaration order
	stats    Stats
}

var preludeTypes = func() map[string]bool {
	m := map[string]bool{}
	src, err := fs.ReadFile(std.Files, "prelude.xo")
	if err != nil {
		return m
	}
	re := regexp.MustCompile(`(?m)^(?:struct|enum|interface|type)\s+([A-Z][A-Za-z0-9]*)`)
	for _, s := range re.FindAllStringSubmatch(string(src), -1) {
		m[s[1]] = true
	}
	return m
}()

func newGen(b *Binding, pkg *types.Package) *gen {
	g := &gen{b: b, pkg: pkg, imports: map[string]string{}, handles: map[*types.TypeName]*handle{}, scope: map[string]bool{"go": true}, rerun: map[string]bool{}}
	g.stats.Reasons = map[string]int{}
	return g
}

// alias returns the adapter's import name of a Go package.
func (g *gen) alias(p *types.Package) string { return g.aliasPath(p.Path()) }

func (g *gen) aliasPath(path string) string {
	if a, ok := g.imports[path]; ok {
		return a
	}
	a := fmt.Sprintf("g%d", len(g.imports))
	g.imports[path] = a
	return a
}

func (g *gen) qual(t types.Type) string {
	return types.TypeString(t, func(p *types.Package) string { return g.alias(p) })
}

// ---------------------------------------------------------------------------
// Names

// snake converts a Go name to snake_case: FindAllString -> find_all_string,
// ParseRequestURI -> parse_request_uri, Sum256 -> sum256.
func snake(name string) string {
	rs := []rune(name)
	var b strings.Builder
	for i, r := range rs {
		if unicode.IsUpper(r) {
			if i > 0 {
				prev := rs[i-1]
				nextLower := i+1 < len(rs) && unicode.IsLower(rs[i+1])
				if unicode.IsLower(prev) || unicode.IsDigit(prev) || (unicode.IsUpper(prev) && nextLower) {
					b.WriteByte('_')
				}
			}
			b.WriteRune(unicode.ToLower(r))
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// xoName makes a snake_case name usable in Xo: keywords get a `_` suffix.
func xoName(s string) string {
	if s == "" || s == "_" {
		return s
	}
	if xoKeywords[s] || s == "true" || s == "false" || s == "self" || s == "static" || s == "go" {
		return s + "_"
	}
	return s
}

// xoKeywords are the keywords of Xo (core 1.4; the keys of KEYWORDS in
// compiler/lexer/token.xo, TestKeywordsMatchLexer).
var xoKeywords = map[string]bool{
	"as": true, "break": true, "const": true, "continue": true, "defer": true, "else": true, "ensures": true,
	"enum": true, "fn": true, "for": true, "if": true, "in": true, "interface": true, "invariant": true,
	"let": true, "match": true, "pub": true, "requires": true, "return": true, "scope": true, "select": true,
	"struct": true, "test": true, "type": true, "use": true, "uses": true, "var": true, "while": true,
}

func upperSnake(name string) string { return strings.ToUpper(snake(name)) }

// ---------------------------------------------------------------------------
// Type mapping

func isError(t types.Type) bool {
	n, ok := types.Unalias(t).(*types.Named)
	return ok && n.Obj().Pkg() == nil && n.Obj().Name() == "error"
}

func isContext(t types.Type) bool {
	n, ok := types.Unalias(t).(*types.Named)
	return ok && n.Obj().Pkg() != nil && n.Obj().Pkg().Path() == "context" && n.Obj().Name() == "Context"
}

func isByte(t types.Type) bool {
	b, ok := types.Unalias(t).(*types.Basic)
	return ok && b.Kind() == types.Uint8
}

// basicConv maps a basic type; gt is the Go type expression (a named
// basic type keeps its name).
func basicConv(bt *types.Basic, gt string) (conv, *skipErr) {
	var xo, rt string
	switch bt.Kind() {
	case types.String:
		xo, rt = "Str", "string"
	case types.Bool:
		xo, rt = "Bool", "bool"
	case types.Int, types.Int64:
		xo, rt = "Int", "int64"
	case types.Int8:
		xo, rt = "Int8", "int8"
	case types.Int16:
		xo, rt = "Int16", "int16"
	case types.Int32:
		if bt.Name() == "rune" {
			xo, rt = "Rune", "Rune"
		} else {
			xo, rt = "Int32", "int32"
		}
	case types.Uint8:
		xo, rt = "UInt8", "uint8"
	case types.Uint16:
		xo, rt = "UInt16", "uint16"
	case types.Uint32:
		xo, rt = "UInt32", "uint32"
	case types.Uint64, types.Uint:
		xo, rt = "UInt64", "uint64"
	case types.Float64:
		xo, rt = "Float", "float64"
	case types.Float32:
		xo, rt = "Float32", "float32"
	default:
		return conv{}, skipf(catBasic, "Go type %s has no Xo counterpart", bt)
	}
	c := conv{xo: xo, rt: rt, gt: gt}
	if gt == rt {
		c.id, c.toXo, c.toGo = true, identity, identity
		return c, nil
	}
	c.toXo = func(e string) string { return rt + "(" + e + ")" }
	c.toGo = func(e string) string { return gt + "(" + e + ")" }
	return c, nil
}

// mapType maps t. depth is 0 for the bound package's own API and 1 for
// methods of handles of other packages; only depth 0 signatures may
// introduce new foreign handles.
func (g *gen) mapType(t types.Type, depth int) (conv, *skipErr) {
	switch x := t.(type) {
	case *types.Alias:
		return g.mapType(types.Unalias(x), depth)
	case *types.Basic:
		if x.Kind() == types.UnsafePointer {
			return conv{}, skipf(catBasic, "unsafe.Pointer")
		}
		return basicConv(x, x.Name())
	case *types.Named:
		obj := x.Obj()
		if obj.Pkg() == nil {
			if obj.Name() == "error" {
				return conv{}, skipf(catError, "error outside the final result")
			}
			return conv{}, skipf(catIface, "Go type %s", obj.Name())
		}
		if isContext(x) {
			return conv{}, skipf(catCtx, "context.Context outside the first parameter")
		}
		if obj.Pkg().Path() == "time" && obj.Name() == "Duration" {
			a := g.aliasPath("time")
			return conv{xo: "Duration", rt: "Duration", gt: a + ".Duration",
				toXo: func(e string) string { return "Duration(" + e + ")" },
				toGo: func(e string) string { return a + ".Duration(" + e + ")" }}, nil
		}
		if obj.Pkg().Path() == "time" && obj.Name() == "Time" {
			a := g.aliasPath("time")
			return conv{xo: "Time", rt: "Time", gt: a + ".Time",
				toXo: func(e string) string { return "Time(" + e + ".UnixNano())" },
				toGo: func(e string) string { return a + ".Unix(0, int64(" + e + ")).UTC()" }}, nil
		}
		if obj.Pkg().Path() == "iter" && (obj.Name() == "Seq" || obj.Name() == "Seq2") && x.TypeArgs().Len() > 0 {
			return g.seqConv(x, depth)
		}
		if x.TypeArgs().Len() > 0 || x.TypeParams().Len() > 0 {
			return conv{}, skipf(catGeneric, "generic type %s", g.display(x))
		}
		if bt, ok := x.Underlying().(*types.Basic); ok {
			return basicConv(bt, g.qual(x))
		}
		h, err := g.handleFor(x, depth)
		if err != nil {
			return conv{}, err
		}
		h.byValue = true
		return g.handleValue(h), nil
	case *types.Pointer:
		if n, ok := types.Unalias(x.Elem()).(*types.Named); ok {
			if _, basic := n.Underlying().(*types.Basic); !basic && n.Obj().Pkg() != nil && !isContext(n) && n.TypeArgs().Len() == 0 && n.TypeParams().Len() == 0 && !(n.Obj().Pkg().Path() == "time" && !timeHandles[n.Obj().Name()]) {
				h, err := g.handleFor(n, depth)
				if err != nil {
					return conv{}, err
				}
				if h.byPtr {
					return conv{xo: h.xo, rt: h.goName, gt: "*" + h.gt, nilable: true, hnd: h, hpkgs: h.pkgs(),
						toXo: func(e string) string { return h.goName + "{P: " + e + "}" },
						toGo: func(e string) string { return e + ".P" }}, nil
				}
			}
		}
		return conv{}, skipf(catPointer, "pointer type %s", g.display(x))
	case *types.Slice:
		if isByte(x.Elem()) {
			return conv{xo: "Bytes", rt: "Bytes", gt: "[]byte",
				toXo: func(e string) string { return "Bytes(" + e + ")" },
				toGo: func(e string) string { return "[]byte(" + e + ")" }}, nil
		}
		ec, err := g.mapType(x.Elem(), depth)
		if err != nil {
			return conv{}, err
		}
		return listConv(ec), nil
	case *types.Array:
		if isByte(x.Elem()) {
			n := x.Len()
			return conv{xo: "Bytes", rt: "Bytes", gt: fmt.Sprintf("[%d]byte", n),
				toXo: func(e string) string { return "Bytes(" + e + "[:])" },
				toGo: func(e string) string {
					return fmt.Sprintf("func(s Bytes) (a [%d]byte) { GoArrayLen(len(s), %d, \"a [%d]byte value\"); copy(a[:], s); return a }(%s)", n, n, n, e)
				}}, nil
		}
		return conv{}, skipf(catArray, "array type %s", g.display(x))
	case *types.Map:
		kc, err := g.mapType(x.Key(), depth)
		if err != nil {
			return conv{}, err
		}
		vc, err := g.mapType(x.Elem(), depth)
		if err != nil {
			return conv{}, err
		}
		return conv{xo: "Map[" + kc.xo + ", " + vc.xo + "]", rt: "Map[" + kc.rt + ", " + vc.rt + "]", gt: "map[" + kc.gt + "]" + vc.gt, hpkgs: append(append([]string(nil), kc.hpkgs...), vc.hpkgs...),
			toXo: func(e string) string {
				return fmt.Sprintf("GoMapOf(%s, func(k %s) %s { return %s }, func(v %s) %s { return %s })", e, kc.gt, kc.rt, kc.toXo("k"), vc.gt, vc.rt, vc.toXo("v"))
			},
			toGo: func(e string) string {
				return fmt.Sprintf("GoMapToGo(%s, func(k %s) %s { return %s }, func(v %s) %s { return %s })", e, kc.rt, kc.gt, kc.toGo("k"), vc.rt, vc.gt, vc.toGo("v"))
			}}, nil
	case *types.Signature:
		return conv{}, skipf(catFunc, "function type %s", g.display(x))
	case *types.Interface:
		if x.Empty() {
			return conv{}, skipf(catAny, "interface{} (any)")
		}
		return conv{}, skipf(catIface, "unnamed interface %s", g.display(x))
	case *types.Struct:
		return conv{}, skipf(catIface, "unnamed struct")
	case *types.Chan:
		return conv{}, skipf(catChan, "channel type %s", g.display(x))
	case *types.TypeParam:
		return conv{}, skipf(catGeneric, "type parameter %s", x)
	}
	return conv{}, skipf(catIface, "Go type %s", g.display(t))
}

// seqConv maps an iterator result (iter.Seq[T], iter.Seq2[K, V]) to a List
// collected at the boundary. Iterators cannot be passed to Go (toGo is
// nil).
func (g *gen) seqConv(x *types.Named, depth int) (conv, *skipErr) {
	ta := x.TypeArgs()
	var ecs []conv
	for i := 0; i < ta.Len(); i++ {
		c, err := g.mapType(ta.At(i), depth)
		if err != nil {
			return conv{}, err
		}
		if c.toGo == nil {
			return conv{}, skipf(catGeneric, "nested iterator %s", g.display(x))
		}
		ecs = append(ecs, c)
	}
	if len(ecs) == 1 {
		ec := ecs[0]
		lc := listConv(ec)
		return conv{xo: lc.xo, rt: lc.rt, gt: g.qual(x), hpkgs: ec.hpkgs, toXo: func(e string) string {
			return lc.toXo("GoCollect(" + e + ")")
		}}, nil
	}
	k, v := ecs[0], ecs[1]
	xo := "List[(" + k.xo + ", " + v.xo + ")]"
	rt := "Tuple2[" + k.rt + ", " + v.rt + "]"
	return conv{xo: xo, rt: "List[" + rt + "]", gt: g.qual(x), hpkgs: append(append([]string(nil), k.hpkgs...), v.hpkgs...), toXo: func(e string) string {
		return fmt.Sprintf("GoCollect2(%s, func(k %s, v %s) %s { return %s{F0: %s, F1: %s} })", e, k.gt, v.gt, rt, rt, k.toXo("k"), v.toXo("v"))
	}}, nil
}

func listConv(ec conv) conv {
	c := conv{xo: "List[" + ec.xo + "]", rt: "List[" + ec.rt + "]", gt: "[]" + ec.gt, hpkgs: ec.hpkgs}
	if ec.id {
		c.toXo = func(e string) string { return "GoListCopy(" + e + ")" }
		c.toGo = func(e string) string { return "GoSliceCopy(" + e + ")" }
		return c
	}
	c.toXo = func(e string) string {
		return fmt.Sprintf("GoListOf(%s, func(x %s) %s { return %s })", e, ec.gt, ec.rt, ec.toXo("x"))
	}
	c.toGo = func(e string) string {
		return fmt.Sprintf("GoSliceOf(%s, func(x %s) %s { return %s })", e, ec.rt, ec.gt, ec.toGo("x"))
	}
	return c
}

// handleValue is the conversion of a handle's Go type used by value.
func (g *gen) handleValue(h *handle) conv {
	if h.byPtr {
		return conv{xo: h.xo, rt: h.goName, gt: h.gt, hpkgs: h.pkgs(),
			toXo: func(e string) string { return h.goName + "{P: GoPtrTo(" + e + ")}" },
			toGo: func(e string) string { return "*" + e + ".P" }}
	}
	return conv{xo: h.xo, rt: h.goName, gt: h.gt, hpkgs: h.pkgs(),
		toXo: func(e string) string { return h.goName + "{V: " + e + "}" },
		toGo: func(e string) string { return e + ".V" }}
}

// pkgs is the import path of the handle's Go type, as conv.hpkgs.
func (h *handle) pkgs() []string { return []string{h.obj.Pkg().Path()} }

// display is a Go type as the Go documentation writes it.
func (g *gen) display(t types.Type) string {
	return types.TypeString(t, func(p *types.Package) string { return p.Name() })
}

// handleFor returns the handle of named type n, creating it.
func (g *gen) handleFor(n *types.Named, depth int) (*handle, *skipErr) {
	obj := n.Obj()
	if h, ok := g.handles[obj]; ok {
		return h, nil
	}
	foreign := obj.Pkg() != g.pkg
	if !obj.Exported() {
		return nil, skipf(catUnexport, "unexported type %s", g.display(n))
	}
	if foreign && depth > 0 {
		return nil, skipf(catForeign, "type %s is from another package (only the package's own signatures may introduce foreign handles)", g.display(n))
	}
	name := obj.Name()
	if preludeTypes[name] || g.scope[name] {
		pn := obj.Pkg().Name()
		name = strings.ToUpper(pn[:1]) + pn[1:] + name
		if preludeTypes[name] || g.scope[name] {
			name = "Go" + name
		}
	}
	if preludeTypes[name] || g.scope[name] {
		return nil, skipf(catCollision, "type name %s collides", obj.Name())
	}
	byPtr := false
	switch n.Underlying().(type) {
	case *types.Struct, *types.Array:
		byPtr = true
	}
	h := &handle{obj: obj, named: n, xo: name, goName: g.b.Prefix + name, gt: g.qual(n), byPtr: byPtr, foreign: foreign, names: map[string]bool{}}
	g.handles[obj] = h
	g.scope[name] = true
	g.order = append(g.order, h)
	g.queue = append(g.queue, h)
	return h, nil
}

// ---------------------------------------------------------------------------
// Functions and methods

// fnSpec describes one function or method to bind.
type fnSpec struct {
	xoName  string
	display string // Go name for messages: regexp.Compile, (*regexp.Regexp).FindString
	sig     *types.Signature
	recv    *handle
	goCall  string // the callee in the adapter
	depth   int
	// ctor marks a package function named New* or Must*: a pointer result
	// is taken to be non nil (checked, a nil faults) instead of `T?`.
	ctor bool
}

func (g *gen) paramName(name string, i int, used map[string]bool) string {
	n := xoName(snake(name))
	if n == "" || n == "_" {
		n = fmt.Sprintf("a%d", i)
	}
	for used[n] {
		n += "_"
	}
	used[n] = true
	return n
}

// callbackConv maps a func parameter to an Xo function type with an effect
// parameter `fx`.
func (g *gen) callbackConv(s *types.Signature, gt string, depth int) (conv, *skipErr) {
	if s.Variadic() || s.TypeParams().Len() > 0 {
		return conv{}, skipf(catFunc, "callback %s", g.display(s))
	}
	if s.Results().Len() > 1 {
		return conv{}, skipf(catFunc, "callback %s has several results", g.display(s))
	}
	var xps, rps, gps []string
	var pcs []conv
	for i := 0; i < s.Params().Len(); i++ {
		pc, err := g.mapType(s.Params().At(i).Type(), depth)
		if err != nil {
			return conv{}, skipf(catFunc, "callback %s: %s", g.display(s), err.detail)
		}
		xps = append(xps, pc.xo)
		rps = append(rps, pc.rt)
		gps = append(gps, fmt.Sprintf("b%d %s", i, pc.gt))
		pcs = append(pcs, pc)
	}
	var rc *conv
	if s.Results().Len() == 1 {
		c, err := g.mapType(s.Results().At(0).Type(), depth)
		if err != nil {
			return conv{}, skipf(catFunc, "callback %s: %s", g.display(s), err.detail)
		}
		rc = &c
	}
	xo := "fn(" + strings.Join(xps, ", ") + ")"
	rres := "Unit"
	gres := ""
	if rc != nil {
		xo += " -> " + rc.xo
		rres = rc.rt
		gres = " " + rc.gt
	}
	xo += " uses fx"
	rt := "func(" + strings.Join(append([]string{"*Task"}, rps...), ", ") + ") Result[" + rres + ", Never]"
	// Go may call the callback on the passing task's goroutine during a Go
	// call (strings.Map) or on another goroutine, during or after the call
	// that passed it (sync.WaitGroup.Go, a timer): rt.GoCallback decides
	// per call which task it runs with, the passing task (without the
	// request's arena, decision 0066) or a task of its own (decision 0067).
	// The arguments are converted before Begin, which hands them to the
	// deterministic scheduler for a call on another goroutine (decision
	// 0073: calls are ordered and recorded by them).
	return conv{xo: xo, rt: rt, gt: gt, toGo: func(e string) string {
		args := []string{"ctk"}
		var xs, conv []string
		for i, pc := range pcs {
			a := fmt.Sprintf("c%d", i)
			xs = append(xs, a)
			conv = append(conv, a+" := "+pc.toXo(fmt.Sprintf("b%d", i))+"; ")
			args = append(args, a)
		}
		call := e + "(" + strings.Join(args, ", ") + ")"
		argf := "nil"
		if len(xs) > 0 {
			argf = "func() []any { return []any{" + strings.Join(xs, ", ") + "} }"
		}
		begin := strings.Join(conv, "") + "ctk := cb.Begin(" + argf + "); defer cb.End(ctk); "
		fn := "func(" + strings.Join(gps, ", ") + ") { " + begin + call + " }"
		if rc != nil {
			fn = "func(" + strings.Join(gps, ", ") + ")" + gres + " { " + begin + "return " + rc.toGo(call+".V") + " }"
		}
		return "func(cb *GoCallback) " + gt + " { return " + fn + " }(NewGoCallback(tk))"
	}}, nil
}

// bindFunc writes the declaration and adapter of one function or method.
func (g *gen) bindFunc(s fnSpec) *skipErr {
	sig := s.sig
	if sig.TypeParams().Len() > 0 {
		return skipf(catGeneric, "generic function")
	}
	used := map[string]bool{}
	recvName := "h"
	if s.recv != nil {
		recvName = "recv"
		for i := 0; i < sig.Params().Len(); i++ {
			if xoName(snake(sig.Params().At(i).Name())) == "recv" {
				recvName = "recv_"
			}
		}
		used[recvName] = true
	}
	var xps, gps, args, argNames []string
	var hpkgs []string // packages of the handles in parameters and results
	fx := false
	n := sig.Params().Len()
	for i := 0; i < n; i++ {
		p := sig.Params().At(i)
		pt := p.Type()
		if i == 0 && isContext(pt) {
			args = append(args, "GoContext(tk)")
			continue
		}
		var c conv
		var err *skipErr
		if fs, ok := pt.Underlying().(*types.Signature); ok && !(sig.Variadic() && i == n-1) {
			c, err = g.callbackConv(fs, g.qual(pt), s.depth)
			fx = true
		} else {
			c, err = g.mapType(pt, s.depth)
		}
		if err != nil {
			pn := p.Name()
			if pn == "" {
				pn = fmt.Sprintf("#%d", i+1)
			}
			return skipf(err.cat, "parameter %s: %s", pn, err.detail)
		}
		if c.toGo == nil {
			return skipf(catGeneric, "parameter %s: iterator %s cannot be passed to Go", p.Name(), g.display(pt))
		}
		pname := g.paramName(p.Name(), i, used)
		xps = append(xps, pname+": "+c.xo)
		an := fmt.Sprintf("a%d", i)
		hpkgs = append(hpkgs, c.hpkgs...)
		argNames = append(argNames, an)
		gps = append(gps, an+" "+c.rt)
		arg := c.toGo(an)
		if sig.Variadic() && i == n-1 {
			arg += "..."
		}
		args = append(args, arg)
	}
	res := sig.Results()
	nr := res.Len()
	hasErr := nr > 0 && isError(res.At(nr-1).Type())
	nv := nr
	if hasErr {
		nv--
	}
	if nv > 6 {
		return skipf(catResults, "%d results", nv)
	}
	var rcs []conv
	for i := 0; i < nv; i++ {
		c, err := g.mapType(res.At(i).Type(), s.depth)
		if err != nil {
			return skipf(err.cat, "result %d: %s", i+1, err.detail)
		}
		rcs = append(rcs, c)
		hpkgs = append(hpkgs, c.hpkgs...)
	}
	mode := g.replayMode(s, fx, hpkgs)
	// result types and the Xo value of the results r0..rn
	var xoRes, rtRes, val string
	var checks []string
	optional := func(c conv) conv {
		if !c.nilable {
			return c
		}
		h := c.hnd
		return conv{xo: c.xo + "?", rt: "Option[" + c.rt + "]", hpkgs: c.hpkgs, toXo: func(e string) string {
			return "Option[" + h.goName + "]{Some: " + e + " != nil, V: " + h.goName + "{P: " + e + "}}"
		}}
	}
	switch {
	case nv == 0:
		xoRes, rtRes, val = "", "Unit", "Unit{}"
	case nv == 1:
		c := rcs[0]
		if hasErr || c.nilable && s.ctor {
			if c.nilable {
				checks = append(checks, fmt.Sprintf("GoNotNil(r0 != nil, %q)", s.display))
			}
		} else {
			c = optional(c)
		}
		xoRes, rtRes, val = c.xo, c.rt, c.toXo("r0")
	default:
		var xs, rs, fs []string
		for i, c := range rcs {
			c = optional(c)
			xs = append(xs, c.xo)
			rs = append(rs, c.rt)
			fs = append(fs, fmt.Sprintf("F%d: %s", i, c.toXo(fmt.Sprintf("r%d", i))))
		}
		xoRes = "(" + strings.Join(xs, ", ") + ")"
		rtRes = fmt.Sprintf("Tuple%d[%s]", nv, strings.Join(rs, ", "))
		val = rtRes + "{" + strings.Join(fs, ", ") + "}"
	}
	// declaration
	var d strings.Builder
	fmt.Fprintf(&d, "/// Go: %s\n", s.goDoc(g))
	fmt.Fprintf(&d, "/// Replay: %s\n", replayDoc[mode])
	d.WriteString("fn ")
	if s.recv != nil {
		fmt.Fprintf(&d, "(%s: %s) ", recvName, s.recv.xo)
	}
	d.WriteString(s.xoName)
	if fx {
		d.WriteString("[fx: Effects]")
	}
	d.WriteString("(" + strings.Join(xps, ", ") + ")")
	if xoRes != "" {
		d.WriteString(" -> " + xoRes)
	}
	if hasErr {
		d.WriteString(" ! go.Error")
	}
	if fx {
		d.WriteString(" uses go, fx")
	} else {
		d.WriteString(" uses go")
	}
	// adapter
	var a strings.Builder
	goRet := ""
	switch {
	case hasErr:
		goRet = " Result[" + rtRes + ", Go_Error]"
	case nv > 0:
		goRet = " " + rtRes
	}
	params := strings.Join(append([]string{"tk *Task"}, gps...), ", ")
	if s.recv != nil {
		fmt.Fprintf(&a, "func (h %s) M_%s(%s)%s {\n", s.recv.goName, s.xoName, params, goRet)
	} else {
		fmt.Fprintf(&a, "func %s%s(%s)%s {\n", g.b.Prefix, s.xoName, params, goRet)
	}
	// record and replay (decision 0093, std/rt gofx.go)
	qual := g.b.Name + "." + s.xoName
	if s.recv != nil {
		qual = g.b.Name + "." + s.recv.xo + "." + s.xoName
	}
	switch mode {
	case replayHandle, replayCallback:
		fmt.Fprintf(&a, "\tif GoFxOn() {\n\t\tGoFxRerun(tk, %q, %q)\n\t}\n", qual, mode)
	case replayValue:
		// the Go call moves into a function of its own that the wrapper
		// calls directly or through GoFxValue
		inner := "goCall_" + g.b.Prefix + s.xoName
		callArgs := strings.Join(append([]string{"tk"}, argNames...), ", ")
		argList := "nil"
		if len(argNames) > 0 {
			argList = "[]any{" + strings.Join(argNames, ", ") + "}"
		}
		rt := strings.TrimSpace(goRet)
		if rt == "" {
			fmt.Fprintf(&a, "\tif GoFxOn() {\n\t\tGoFxValue(tk, %q, %s, func() Unit { %s(%s); return Unit{} })\n\t\treturn\n\t}\n\t%s(%s)\n}\n\n", qual, argList, inner, callArgs, inner, callArgs)
		} else {
			fmt.Fprintf(&a, "\tif GoFxOn() {\n\t\treturn GoFxValue(tk, %q, %s, func() %s { return %s(%s) })\n\t}\n\treturn %s(%s)\n}\n\n", qual, argList, rt, inner, callArgs, inner, callArgs)
		}
		fmt.Fprintf(&a, "func %s(%s)%s {\n", inner, params, goRet)
	}
	fmt.Fprintf(&a, "\tGoEnter(tk)\n\tdefer GoExit(tk, %q)\n", s.display)
	call := s.goCall + "(" + strings.Join(args, ", ") + ")"
	var lhs []string
	for i := 0; i < nv; i++ {
		lhs = append(lhs, fmt.Sprintf("r%d", i))
	}
	if hasErr {
		lhs = append(lhs, "err")
	}
	if len(lhs) > 0 {
		fmt.Fprintf(&a, "\t%s := %s\n", strings.Join(lhs, ", "), call)
	} else {
		fmt.Fprintf(&a, "\t%s\n", call)
	}
	if hasErr {
		fmt.Fprintf(&a, "\tif err != nil {\n\t\treturn Result[%s, Go_Error]{E: GoErr(err)}\n\t}\n", rtRes)
	}
	for _, c := range checks {
		fmt.Fprintf(&a, "\t%s\n", c)
	}
	switch {
	case hasErr:
		fmt.Fprintf(&a, "\treturn Result[%s, Go_Error]{Ok: true, V: %s}\n", rtRes, val)
	case nv > 0:
		fmt.Fprintf(&a, "\treturn %s\n", val)
	}
	a.WriteString("}\n\n")
	g.adapt.WriteString(a.String())
	if s.recv != nil {
		s.recv.decls = append(s.recv.decls, d.String())
	} else {
		g.funcs = append(g.funcs, d.String())
	}
	return nil
}

// Replay modes of a bound function (decision 0093 stage 1).
const (
	replayValue    = "value"    // recorded; a replay returns the recorded results
	replayHandle   = "handle"   // a handle crosses: re executed and reported
	replayCallback = "callback" // takes an Xo callback: re executed and reported
	replayPure     = "pure"     // `replay rerun`: re executed, not recorded
)

var replayDoc = map[string]string{
	replayValue:    "recorded; `xo replay` returns the recorded results",
	replayHandle:   "a Go handle crosses: re executed by `xo replay`",
	replayCallback: "takes an Xo callback: re executed by `xo replay`",
	replayPure:     "rerun (pure): re executed by `xo replay`, not recorded",
}

// purePackages are the std Go packages whose functions are `replay
// rerun`: they compute from their arguments only (no I/O, clock,
// randomness, or global state that changes). A function or method stays
// pure only when every handle it takes or returns is of one of these
// packages too (a regexp method reading an io.RuneReader is not).
var purePackages = map[string]bool{
	"strconv": true, "strings": true, "bytes": true, "unicode": true,
	"unicode/utf8": true, "unicode/utf16": true, "math": true,
	"math/bits": true, "regexp": true, "regexp/syntax": true, "path": true,
	"html": true, "net/url": true, "encoding/hex": true,
	"encoding/base64": true, "encoding/base32": true,
}

// PurePackage reports whether the Go package at path is one of the pure
// std packages: a dependency's `use go` of it is not an escape hatch
// that needs the consumer's vouch (decision 0156).
func PurePackage(path string) bool { return purePackages[path] }

// replayMode decides how record and replay treat a function or method:
// fx marks an Xo callback parameter, hpkgs the packages of the handles
// among its parameters and results.
func (g *gen) replayMode(s fnSpec, fx bool, hpkgs []string) string {
	name := s.xoName
	if s.recv != nil {
		name = s.recv.xo + "." + s.xoName
	}
	if g.rerunAll || g.rerun[name] {
		return replayPure
	}
	if purePackages[g.pkg.Path()] {
		pure := s.recv == nil || purePackages[s.recv.obj.Pkg().Path()]
		for _, p := range hpkgs {
			pure = pure && purePackages[p]
		}
		if pure {
			return replayPure
		}
	}
	switch {
	case fx:
		return replayCallback
	case s.recv != nil || len(hpkgs) > 0:
		return replayHandle
	}
	return replayValue
}

// goDoc is the Go signature for the doc comment.
func (s fnSpec) goDoc(g *gen) string {
	sig := types.TypeString(s.sig, func(p *types.Package) string { return p.Name() })
	return s.display + strings.TrimPrefix(sig, "func")
}

func (g *gen) skip(name, display string, err *skipErr, counted bool) {
	g.skips = append(g.skips, fmt.Sprintf("%s: %s (%s)", name, err.detail, display))
	g.b.Skipped[name] = err.detail
	if counted {
		g.stats.Reasons[err.cat]++
	}
}

// ---------------------------------------------------------------------------
// The package

func (g *gen) run() {
	scope := g.pkg.Scope()
	names := scope.Names()
	// types first, so functions see every handle of the package
	for _, name := range names {
		tn, ok := scope.Lookup(name).(*types.TypeName)
		if !ok || !tn.Exported() || tn.IsAlias() {
			continue
		}
		n, ok := tn.Type().(*types.Named)
		if !ok || n.TypeParams().Len() > 0 {
			continue
		}
		if _, basic := n.Underlying().(*types.Basic); basic {
			continue
		}
		if tn.Pkg().Path() == "time" && (name == "Duration" || name == "Time") {
			continue
		}
		g.handleFor(n, 0)
	}
	for _, name := range names {
		switch o := scope.Lookup(name).(type) {
		case *types.Func:
			if !o.Exported() {
				continue
			}
			g.stats.Funcs++
			xn := xoName(snake(name))
			display := g.pkg.Name() + "." + name
			if g.scope[xn] {
				g.skip(xn, display, skipf(catCollision, "name `%s` is taken", xn), true)
				continue
			}
			sig := o.Type().(*types.Signature)
			ctor := strings.HasPrefix(name, "New") || strings.HasPrefix(name, "Must")
			if err := g.bindFunc(fnSpec{xoName: xn, display: display, sig: sig, goCall: g.alias(g.pkg) + "." + name, ctor: ctor}); err != nil {
				g.skip(xn, display, err, true)
				continue
			}
			g.scope[xn] = true
			g.stats.FuncsMapped++
		case *types.Const:
			if o.Exported() {
				g.bindConst(o)
			}
		case *types.Var:
			if o.Exported() {
				g.bindVar(o)
			}
		case *types.TypeName:
			if !o.Exported() {
				continue
			}
			n, ok := o.Type().(*types.Named)
			if !ok {
				continue
			}
			if _, basic := n.Underlying().(*types.Basic); basic || n.TypeParams().Len() > 0 {
				// methods of named basic and generic types are not bound
				ms := types.NewMethodSet(types.NewPointer(n))
				for i := 0; i < ms.Len(); i++ {
					f := ms.At(i).Obj()
					if !f.Exported() {
						continue
					}
					g.stats.Methods++
					cat := catNamedBase
					if !basic {
						cat = catGeneric
					}
					g.skip(o.Name()+"."+xoName(snake(f.Name())), "("+g.display(n)+")."+f.Name(), skipf(cat, "method of %s", g.display(n)), true)
				}
			}
		}
	}
	for len(g.queue) > 0 {
		h := g.queue[0]
		g.queue = g.queue[1:]
		g.bindHandle(h)
	}
}

func (g *gen) bindHandle(h *handle) {
	depth := 0
	if h.foreign {
		depth = 1
	}
	count := func(ok bool) {
		if h.foreign {
			g.stats.Foreign++
			if ok {
				g.stats.ForeignMapped++
			}
			return
		}
		g.stats.Methods++
		if ok {
			g.stats.MethodsMapped++
		}
	}
	var ms *types.MethodSet
	if h.byPtr {
		ms = types.NewMethodSet(types.NewPointer(h.named))
	} else {
		ms = types.NewMethodSet(h.named)
	}
	_, isStruct := h.named.Underlying().(*types.Struct)
	field := "V"
	if h.byPtr {
		field = "P"
	}
	for i := 0; i < ms.Len(); i++ {
		f, ok := ms.At(i).Obj().(*types.Func)
		if !ok || !f.Exported() {
			continue
		}
		sig := f.Type().(*types.Signature)
		xn := xoName(snake(f.Name()))
		display := "(" + g.display(types.NewPointer(h.named)) + ")." + f.Name()
		if !h.byPtr {
			display = g.display(h.named) + "." + f.Name()
		}
		qname := h.xo + "." + xn
		if h.byPtr && !isStruct {
			if r := sig.Recv(); r != nil {
				if _, ptr := r.Type().(*types.Pointer); ptr {
					g.skip(qname, display, skipf(catPtrRecv, "pointer receiver would mutate a value held by a shared handle"), !h.foreign)
					count(false)
					continue
				}
			}
		}
		if h.names[xn] {
			g.skip(qname, display, skipf(catCollision, "method name `%s` is taken", xn), !h.foreign)
			count(false)
			continue
		}
		err := g.bindFunc(fnSpec{xoName: xn, display: display, sig: sig, recv: h, goCall: "h." + field + "." + f.Name(), depth: depth})
		if err != nil {
			g.skip(qname, display, err, !h.foreign)
			count(false)
			continue
		}
		h.names[xn] = true
		count(true)
	}
	if !h.byPtr || !isStruct {
		return
	}
	// exported fields: getters and setters
	st := h.named.Underlying().(*types.Struct)
	for i := 0; i < st.NumFields(); i++ {
		fv := st.Field(i)
		if !fv.Exported() || fv.Embedded() {
			continue
		}
		g.stats.Fields++
		xn := xoName(snake(fv.Name()))
		qname := h.xo + "." + xn
		display := g.display(h.named) + "." + fv.Name()
		c, err := g.mapType(fv.Type(), depth)
		if err == nil && (h.names[xn] || h.names["set_"+snake(fv.Name())]) {
			err = skipf(catCollision, "field accessor `%s` collides with a method", xn)
		}
		if err != nil {
			g.skip(qname, display, &skipErr{cat: err.cat, detail: "field: " + err.detail}, false)
			continue
		}
		g.stats.FieldsMapped++
		h.names[xn] = true
		rc := c
		if c.nilable {
			hh := c.hnd
			rc = conv{xo: c.xo + "?", rt: "Option[" + c.rt + "]", toXo: func(e string) string {
				return "Option[" + hh.goName + "]{Some: " + e + " != nil, V: " + hh.goName + "{P: " + e + "}}"
			}}
		}
		h.decls = append(h.decls, fmt.Sprintf("/// Go: field %s %s\nfn (recv: %s) %s() -> %s uses go", fv.Name(), g.display(fv.Type()), h.xo, xn, rc.xo))
		fmt.Fprintf(&g.adapt, "func (h %s) M_%s(tk *Task) %s {\n\tdefer GoBoundary(%q)\n\tv := h.P.%s\n\treturn %s\n}\n\n", h.goName, xn, rc.rt, display, fv.Name(), rc.toXo("v"))
		sn := "set_" + snake(fv.Name())
		h.names[sn] = true
		h.decls = append(h.decls, fmt.Sprintf("/// Go: assigns field %s\nfn (recv: %s) %s(v: %s) uses go", fv.Name(), h.xo, sn, c.xo))
		fmt.Fprintf(&g.adapt, "func (h %s) M_%s(tk *Task, a0 %s) {\n\tdefer GoBoundary(%q)\n\th.P.%s = %s\n}\n\n", h.goName, sn, c.rt, display, fv.Name(), c.toGo("a0"))
	}
	if !h.names["new"] {
		h.names["new"] = true
		h.decls = append(h.decls, fmt.Sprintf("/// Go: new(%s), the zero value\nfn (static: %s) new() -> %s uses go", g.display(h.named), h.xo, h.xo))
		fmt.Fprintf(&g.adapt, "func %s_new(tk *Task) %s {\n\treturn %s{P: new(%s)}\n}\n\n", h.goName, h.goName, h.goName, h.gt)
	}
}

func (g *gen) bindConst(o *types.Const) {
	g.stats.Consts++
	name := upperSnake(o.Name())
	display := g.pkg.Name() + "." + o.Name()
	if g.scope[name] {
		g.skip(name, display, skipf(catCollision, "name `%s` is taken", name), false)
		return
	}
	bt, ok := o.Type().Underlying().(*types.Basic)
	if !ok {
		g.skip(name, display, skipf(catIface, "constant of type %s", g.display(o.Type())), false)
		return
	}
	v := o.Val()
	var xoType, lit, rt string
	switch {
	case bt.Info()&types.IsBoolean != 0:
		xoType, rt, lit = "Bool", "bool", strconv.FormatBool(constant.BoolVal(v))
	case bt.Info()&types.IsString != 0:
		s := constant.StringVal(v)
		l, ok := xoString(s)
		if !ok {
			g.skip(name, display, skipf(catBasic, "string constant with characters the binder does not escape"), false)
			return
		}
		xoType, rt, lit = "Str", "string", l
	case bt.Info()&types.IsInteger != 0:
		c, err := basicConv(bt, bt.Name())
		if bt.Kind() == types.UntypedInt || bt.Kind() == types.UntypedRune {
			c, err = conv{xo: "Int", rt: "int64"}, nil
		}
		if err != nil {
			g.skip(name, display, err, false)
			return
		}
		if c.xo == "Rune" {
			c = conv{xo: "Int32", rt: "int32"}
		}
		i, exact := constant.Int64Val(v)
		if !exact {
			u, exactU := constant.Uint64Val(v)
			if !exactU || c.xo != "UInt64" {
				g.skip(name, display, skipf(catBasic, "integer constant out of range"), false)
				return
			}
			lit = strconv.FormatUint(u, 10)
		} else {
			lit = strconv.FormatInt(i, 10)
		}
		xoType, rt = c.xo, c.rt
	case bt.Info()&types.IsFloat != 0:
		f, _ := constant.Float64Val(v)
		if math.IsInf(f, 0) || math.IsNaN(f) {
			g.skip(name, display, skipf(catBasic, "float constant out of range"), false)
			return
		}
		lit = strconv.FormatFloat(f, 'g', -1, 64)
		if !strings.ContainsAny(lit, ".e") {
			lit += ".0"
		}
		lit = strings.Replace(lit, "e+", "e", 1)
		if strings.Contains(lit, "e") && !strings.Contains(lit, ".") {
			mant, exp, _ := strings.Cut(lit, "e")
			lit = mant + ".0e" + exp
		}
		xoType, rt = "Float", "float64"
		if bt.Kind() == types.Float32 {
			xoType, rt = "Float32", "float32"
		}
	default:
		g.skip(name, display, skipf(catBasic, "constant of type %s", bt), false)
		return
	}
	g.scope[name] = true
	g.stats.ConstsMapped++
	g.consts = append(g.consts, fmt.Sprintf("/// Go: %s\nconst %s: %s = %s", display, name, xoType, lit))
	fmt.Fprintf(&g.adapt, "const %s%s %s = %s(%s.%s)\n\n", g.b.Prefix, name, rt, rt, g.alias(g.pkg), o.Name())
}

// xoString writes s as an Xo string literal when it has only printable
// ASCII.
func xoString(s string) (string, bool) {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch {
		case r == '"' || r == '\\' || r == '$':
			b.WriteByte('\\')
			b.WriteRune(r)
		case r == '\n':
			b.WriteString(`\n`)
		case r == '\t':
			b.WriteString(`\t`)
		case r >= 0x20 && r < 0x7f:
			b.WriteRune(r)
		default:
			return "", false
		}
	}
	b.WriteByte('"')
	return b.String(), true
}

func (g *gen) bindVar(o *types.Var) {
	g.stats.Vars++
	name := xoName(snake(o.Name()))
	display := g.pkg.Name() + "." + o.Name()
	if g.scope[name] {
		g.skip(name, display, skipf(catCollision, "name `%s` is taken", name), false)
		return
	}
	if isError(o.Type()) {
		g.skip(name, display, skipf(catError, "error variable (sentinel errors are not bound)"), false)
		return
	}
	c, err := g.mapType(o.Type(), 0)
	if err != nil {
		g.skip(name, display, &skipErr{cat: err.cat, detail: "variable: " + err.detail}, false)
		return
	}
	if c.nilable {
		hh := c.hnd
		c = conv{xo: c.xo + "?", rt: "Option[" + c.rt + "]", toXo: func(e string) string {
			return "Option[" + hh.goName + "]{Some: " + e + " != nil, V: " + hh.goName + "{P: " + e + "}}"
		}}
	}
	g.scope[name] = true
	g.stats.VarsMapped++
	g.vars = append(g.vars, fmt.Sprintf("/// Go: reads variable %s %s\nfn %s() -> %s uses go", display, g.display(o.Type()), name, c.xo))
	fmt.Fprintf(&g.adapt, "func %s%s(tk *Task) %s {\n\tv := %s.%s\n\treturn %s\n}\n\n", g.b.Prefix, name, c.rt, g.alias(g.pkg), o.Name(), c.toXo("v"))
}

// ---------------------------------------------------------------------------
// Output

func (g *gen) declFile() []byte {
	g.b.Stats = g.stats
	var b bytes.Buffer
	fmt.Fprintf(&b, "// Code generated by xo bind from Go package %s. DO NOT EDIT.\n", g.b.GoPath)
	fmt.Fprintf(&b, "// xo-bind: v%d %s\n", Version, runtime.Version())
	fmt.Fprintf(&b, "// go-package: %s\n", g.b.Name)
	if g.b.Standard {
		b.WriteString("// go-standard: true\n")
	}
	if g.b.Module != "" {
		fmt.Fprintf(&b, "// go-module: %s %s\n", g.b.Module, g.b.ModVersion)
	}
	if g.rerunKey != "" {
		fmt.Fprintf(&b, "// replay-rerun: %s\n", g.rerunKey)
	}
	s := g.stats
	b.WriteString("//\n")
	fmt.Fprintf(&b, "// Coverage: functions %d/%d, methods %d/%d, fields %d/%d, constants %d/%d,\n", s.FuncsMapped, s.Funcs, s.MethodsMapped, s.Methods, s.FieldsMapped, s.Fields, s.ConstsMapped, s.Consts)
	fmt.Fprintf(&b, "// variables %d/%d, methods of foreign handles %d/%d.\n", s.VarsMapped, s.Vars, s.ForeignMapped, s.Foreign)
	b.WriteString("// Every function carries the `go` effect. Handles are opaque and shared:\n")
	b.WriteString("// a method that mutates the Go value is seen through every copy.\n")
	b.WriteString("// Replay (decision 0093): `--record` records calls whose arguments and\n")
	b.WriteString("// results are values and `xo replay` returns the recorded results; calls\n")
	b.WriteString("// with a handle or a callback run again and are reported; pure functions\n")
	b.WriteString("// (`replay rerun`) run again and are not recorded.\n")
	if len(g.skips) > 0 {
		b.WriteString("//\n// Skipped (signatures that do not map):\n")
		for _, l := range g.skips {
			b.WriteString("// skip: " + l + "\n")
		}
	}
	b.WriteString("\nuse std/go\n")
	if len(g.consts) > 0 {
		b.WriteString("\n// Constants\n\n")
		for _, c := range g.consts {
			b.WriteString(c + "\n")
		}
	}
	if len(g.order) > 0 {
		b.WriteString("\n// Handles (opaque Go values)\n\n")
		for _, h := range g.order {
			kind := "*" + g.display(h.named)
			if !h.byPtr {
				kind = g.display(h.named)
			}
			fmt.Fprintf(&b, "/// Go: %s\nstruct %s {}\n", kind, h.xo)
		}
	}
	if len(g.funcs) > 0 {
		b.WriteString("\n// Functions\n\n")
		for _, f := range g.funcs {
			b.WriteString(f + "\n")
		}
	}
	if len(g.vars) > 0 {
		b.WriteString("\n// Variables (read on each call)\n\n")
		for _, f := range g.vars {
			b.WriteString(f + "\n")
		}
	}
	for _, h := range g.order {
		if len(h.decls) == 0 {
			continue
		}
		fmt.Fprintf(&b, "\n// Methods of %s\n\n", h.xo)
		for _, d := range h.decls {
			b.WriteString(d + "\n")
		}
	}
	return b.Bytes()
}

func (g *gen) adapterFile() ([]byte, error) {
	var body bytes.Buffer
	for _, h := range g.order {
		field := "V " + h.gt
		if h.byPtr {
			field = "P *" + h.gt
		}
		disp := g.display(h.named)
		if h.byPtr {
			disp = "*" + disp
		}
		fmt.Fprintf(&body, "// %s is the handle of Go %s.\ntype %s struct{ %s }\n\n", h.goName, disp, h.goName, field)
		fmt.Fprintf(&body, "func (h %s) XoDebug(b *strings.Builder) { GoDebugHandle(b, %q) }\n\n", h.goName, disp)
		switch {
		case h.byPtr && types.Comparable(h.named) && (!isStructType(h.named) || h.byValue):
			// Go's equality (core 2.9): a comparable type Go hands out as a
			// value (netip.Addr, language.Tag, arrays such as uuid.UUID)
			// compares by value; a struct the API passes only as *T
			// (url.Parse's *url.URL) compares by pointer, as Go's == on
			// the pointers does (Intel b15, 0.33.0: two equal netip.Addr
			// were unequal)
			fmt.Fprintf(&body, "func (h %s) XoEq(o any) bool {\n\tx, ok := o.(%s)\n\treturn ok && (h.P == x.P || h.P != nil && x.P != nil && *h.P == *x.P)\n}\n\n", h.goName, h.goName)
			fmt.Fprintf(&body, "func (h %s) XoHash() uint64 {\n\tif h.P == nil {\n\t\treturn 0\n\t}\n\treturn GoHashComparable(*h.P)\n}\n\n", h.goName)
		case h.byPtr:
			fmt.Fprintf(&body, "func (h %s) XoEq(o any) bool {\n\tx, ok := o.(%s)\n\treturn ok && h.P == x.P\n}\n\n", h.goName, h.goName)
			fmt.Fprintf(&body, "func (h %s) XoHash() uint64 { return GoHashComparable(h.P) }\n\n", h.goName)
		case isInterfaceType(h.named):
			fmt.Fprintf(&body, "func (h %s) XoEq(o any) bool {\n\tx, ok := o.(%s)\n\treturn ok && GoEqIface(h.V, x.V)\n}\n\n", h.goName, h.goName)
			fmt.Fprintf(&body, "func (h %s) XoHash() uint64 { return GoHashIface(h.V) }\n\n", h.goName)
		}
	}
	body.Write(g.adapt.Bytes())
	var b bytes.Buffer
	fmt.Fprintf(&b, "// Code generated by xo bind from Go package %s. DO NOT EDIT.\n\n", g.b.GoPath)
	b.WriteString("package rt\n\n")
	text := body.String()
	var imps []string
	for _, p := range sortedKeys(g.imports) {
		if strings.Contains(text, g.imports[p]+".") {
			imps = append(imps, fmt.Sprintf("\t%s %q\n", g.imports[p], p))
		}
	}
	if len(g.order) > 0 {
		imps = append(imps, "\t\"strings\"\n")
	}
	if len(imps) > 0 {
		b.WriteString("import (\n" + strings.Join(imps, "") + ")\n\n")
	}
	b.WriteString(text)
	src, err := format.Source(b.Bytes())
	if err != nil {
		return b.Bytes(), err
	}
	return src, nil
}

func isStructType(n *types.Named) bool {
	_, ok := n.Underlying().(*types.Struct)
	return ok
}

func isInterfaceType(n *types.Named) bool {
	_, ok := n.Underlying().(*types.Interface)
	return ok
}

// Summary is a one paragraph text of the coverage.
func (b *Binding) Summary() string {
	s := b.Stats
	var w strings.Builder
	fmt.Fprintf(&w, "%s: functions %d/%d, methods %d/%d, fields %d/%d, constants %d/%d, variables %d/%d, foreign handle methods %d/%d",
		b.GoPath, s.FuncsMapped, s.Funcs, s.MethodsMapped, s.Methods, s.FieldsMapped, s.Fields, s.ConstsMapped, s.Consts, s.VarsMapped, s.Vars, s.ForeignMapped, s.Foreign)
	if len(s.Reasons) > 0 {
		var parts []string
		for _, k := range sortedKeys(s.Reasons) {
			parts = append(parts, fmt.Sprintf("%s %d", k, s.Reasons[k]))
		}
		sort.Strings(parts)
		fmt.Fprintf(&w, "; skipped functions and methods: %s", strings.Join(parts, ", "))
	}
	return w.String()
}

// timeHandles are the types of package time whose pointers are handles
// (time.AfterFunc returns a *Timer, decision 0073); pointers to the
// others (Time, Location) stay unbound.
var timeHandles = map[string]bool{"Timer": true, "Ticker": true}
