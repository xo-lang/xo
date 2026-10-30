package gobind

import (
	"bytes"
	"fmt"
	"go/constant"
	"go/format"
	"go/token"
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
	catOutBuf    = "output buffer ([]byte Go writes into)"
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
	// ref: a type of another package, bound in that package's binding
	// (decision 0392): this binding names it there and declares nothing
	ref     bool
	foreign bool
	names   map[string]bool // Xo method names in use
	decls   []string
	// a generic type (decision 0415): tps are its type parameters in
	// order, xoT and rtT its Xo and rt types over them (`Cache[K, V]`,
	// `Go_lru_Cache[K, V]`), xoDecl and rtDecl their declarations
	// (`Cache[K: Eq, V]`, `Go_lru_Cache[K comparable, V any]`); for any
	// other type all four are the names
	tps            []*tparam
	xoT, rtT       string
	xoDecl, rtDecl string
}

// gen generates one binding.
type gen struct {
	b       *Binding
	pkg     *types.Package
	imports map[string]string // Go path -> alias in the adapter
	// deps: the Go packages whose bindings this one refers to (decision
	// 0392), Go path -> local name in the declaration file
	deps    map[string]string
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
	// tps: the type parameters of the generic function being bound
	// (decision 0415), nil otherwise
	tps map[*types.TypeParam]*tparam
	// imp imports a dependency's whole API (bindUpcasts): an import's
	// scope holds only what this package's export data names
	imp types.Importer
}

// tparam is a type parameter of a bound generic function (decision 0415):
// a plain one is a type parameter of the Xo declaration and the adapter
// with its constraint mapped; a core one (`S ~[]E`, `M ~map[K]V`) is not
// a parameter in Xo: a value of it is the List or Map of its core type.
type tparam struct {
	name    string     // the Go name, also the Xo name of a plain one
	xoBound string     // the Xo constraint ("" for none)
	goBound string     // the adapter's constraint
	core    types.Type // the core type (a *types.Slice or *types.Map), nil for a plain one
}

// typeParams maps the type parameters of sig (decision 0415): any to no
// constraint, comparable to Eq, cmp.Ordered to Ord, a core type
// constraint ~[]E or ~map[K]V to List[E] or Map[K, V]; any other
// constraint has no Xo form.
func (g *gen) typeParams(list *types.TypeParamList) (map[*types.TypeParam]*tparam, []*types.TypeParam, *skipErr) {
	m := map[*types.TypeParam]*tparam{}
	var order []*types.TypeParam
	for i := 0; i < list.Len(); i++ {
		tp := list.At(i)
		name := tp.Obj().Name()
		if name == "" || name == "_" || !unicode.IsUpper([]rune(name)[0]) {
			return nil, nil, skipf(catGeneric, "type parameter %s has no Xo name", name)
		}
		c := tp.Constraint()
		it, _ := c.Underlying().(*types.Interface)
		t := &tparam{name: name}
		switch {
		case it != nil && it.Empty():
			t.goBound = "any"
		case types.Identical(c, types.Universe.Lookup("comparable").Type()):
			t.xoBound, t.goBound = "Eq", "comparable"
		case isOrdered(c):
			t.xoBound, t.goBound = "Ord", g.qual(c)
		case it != nil && it.NumEmbeddeds() == 1 && it.NumExplicitMethods() == 0:
			u, _ := it.EmbeddedType(0).(*types.Union)
			if u == nil || u.Len() != 1 || !u.Term(0).Tilde() {
				return nil, nil, skipf(catGeneric, "type constraint %s has no Xo form", g.display(c))
			}
			switch ct := u.Term(0).Type().(type) {
			case *types.Slice:
				if _, ok := ct.Elem().(*types.TypeParam); !ok {
					return nil, nil, skipf(catGeneric, "type constraint %s has no Xo form", g.display(c))
				}
				t.core = ct
			case *types.Map:
				_, k := ct.Key().(*types.TypeParam)
				_, v := ct.Elem().(*types.TypeParam)
				if !k || !v {
					return nil, nil, skipf(catGeneric, "type constraint %s has no Xo form", g.display(c))
				}
				t.core = ct
			default:
				return nil, nil, skipf(catGeneric, "type constraint %s has no Xo form", g.display(c))
			}
		default:
			return nil, nil, skipf(catGeneric, "type constraint %s has no Xo form", g.display(c))
		}
		m[tp] = t
		order = append(order, tp)
	}
	return m, order, nil
}

// coreGo is a core type in the adapter's terms: []E, map[K]V.
func (g *gen) coreGo(t types.Type) string {
	switch c := t.(type) {
	case *types.Slice:
		return "[]" + c.Elem().(*types.TypeParam).Obj().Name()
	case *types.Map:
		return "map[" + c.Key().(*types.TypeParam).Obj().Name() + "]" + c.Elem().(*types.TypeParam).Obj().Name()
	}
	return g.qual(t)
}

// isOrdered is cmp.Ordered or golang.org/x/exp/constraints.Ordered.
func isOrdered(t types.Type) bool {
	n, ok := types.Unalias(t).(*types.Named)
	if !ok || n.Obj().Pkg() == nil || n.Obj().Name() != "Ordered" {
		return false
	}
	p := n.Obj().Pkg().Path()
	return p == "cmp" || p == "golang.org/x/exp/constraints"
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
	g := &gen{b: b, pkg: pkg, imports: map[string]string{}, deps: map[string]string{}, handles: map[*types.TypeName]*handle{}, scope: map[string]bool{"go": true}, rerun: map[string]bool{}}
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

// outBuffer reports whether parameter name of type t is a []byte the Go
// function writes into: one of a Read... function or method (io.Reader's
// Read(p), ReadAt, io.ReadFull, rand.Read), or one named dst (hex.Decode,
// base64's Encode), unless the function returns a []byte (the append form,
// whose result carries the bytes). The binding copies a Bytes in only, so
// such a call read n bytes into a copy the program never sees (the Windows
// tester's bug 18, 2026-10-29): it is skipped, as the C binder skips
// mutable buffers (spec 4.5).
func outBuffer(display, name string, t types.Type, sig *types.Signature) bool {
	sl, ok := t.Underlying().(*types.Slice)
	if !ok || !isByte(sl.Elem()) {
		return false
	}
	for i := 0; i < sig.Results().Len(); i++ {
		if rs, ok := sig.Results().At(i).Type().Underlying().(*types.Slice); ok && isByte(rs.Elem()) {
			return false
		}
	}
	fn := display
	if k := strings.LastIndex(fn, "."); k >= 0 {
		fn = fn[k+1:]
	}
	return strings.HasPrefix(fn, "Read") || name == "dst"
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
				// an error that is not the final result (a parameter,
				// a field, a variable such as io.EOF) is `go.Error?`,
				// nil being None (decision 0416): the Go error itself
				// goes back to Go, so errors.Is sees a sentinel
				return conv{xo: "go.Error?", rt: "Option[Go_Error]", gt: "error",
					toXo: func(e string) string { return "GoErrOpt(" + e + ")" },
					toGo: func(e string) string { return "GoErrorOfOpt(" + e + ")" }}, nil
			}
			return conv{}, skipf(catIface, "Go type %s", obj.Name())
		}
		if isContext(x) {
			return conv{}, skipf(catCtx, "context.Context outside the first parameter")
		}
		if it, ok := x.Underlying().(*types.Interface); ok && it.Empty() && x.TypeParams().Len() == 0 {
			// a named empty interface (yaml's Node values, sql.Scanner's
			// src) is Go's any by another name (decision 0412)
			q := g.qual(x)
			return conv{xo: "go.Any", rt: "Go_Any", gt: q,
				toXo: func(e string) string { return "Go_Any{V: " + e + "}" },
				toGo: func(e string) string { return "GoAnyTo[" + q + "](" + e + ")" }}, nil
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
		if x.TypeArgs().Len() > 0 {
			return g.instConv(x, false, depth)
		}
		if x.TypeParams().Len() > 0 {
			return conv{}, skipf(catGeneric, "generic type %s", g.display(x))
		}
		if !obj.Exported() {
			// the adapter would name it (`chi.methodTyp(x)`), which only
			// its own package may (the Windows tester's bug 19,
			// 2026-10-29: chi's MethodNotAllowedHandler(...methodTyp)
			// failed the whole package's go build)
			return conv{}, skipf(catUnexport, "unexported type %s", g.display(x))
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
		if n, ok := types.Unalias(x.Elem()).(*types.Named); ok && n.TypeArgs().Len() > 0 && n.Obj().Pkg() != nil {
			return g.instConv(n, true, depth)
		}
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
		if bt, ok := types.Unalias(x.Elem()).(*types.Basic); ok && bt.Kind() != types.UnsafePointer {
			return g.basicPtrConv(x, bt)
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
			return anyConv(), nil
		}
		return conv{}, skipf(catIface, "unnamed interface %s", g.display(x))
	case *types.Struct:
		return conv{}, skipf(catIface, "unnamed struct")
	case *types.Chan:
		return conv{}, skipf(catChan, "channel type %s", g.display(x))
	case *types.TypeParam:
		tp := g.tps[x]
		if tp == nil {
			return conv{}, skipf(catGeneric, "type parameter %s", x)
		}
		if tp.core != nil {
			// S ~[]E is List[E], M ~map[K]V is Map[K, V]
			return g.mapType(tp.core, depth)
		}
		return conv{xo: tp.name, rt: tp.name, gt: tp.name, id: true, toXo: identity, toGo: identity}, nil
	}
	return conv{}, skipf(catIface, "Go type %s", g.display(t))
}

// basicPtrConv maps a pointer to a basic type (*bool, *string, *int) to
// `T?` (decision 0419): nil is None, else the value read at the
// boundary; from Xo, Some(v) is a pointer to a fresh copy of v. A copy,
// not an alias: what Go writes through the pointer later is not seen.
func (g *gen) basicPtrConv(x *types.Pointer, bt *types.Basic) (conv, *skipErr) {
	gt := g.qual(x.Elem())
	ec, err := basicConv(bt, gt)
	if err != nil {
		return conv{}, err
	}
	rt := "Option[" + ec.rt + "]"
	return conv{xo: ec.xo + "?", rt: rt, gt: "*" + gt,
		toXo: func(e string) string {
			return fmt.Sprintf("func(p *%s) %s {\n\tif p == nil {\n\t\treturn %s{}\n\t}\n\treturn %s{Some: true, V: %s}\n}(%s)", gt, rt, rt, rt, ec.toXo("*p"), e)
		},
		toGo: func(e string) string {
			return fmt.Sprintf("func(o %s) *%s {\n\tif !o.Some {\n\t\treturn nil\n\t}\n\tv := %s\n\treturn &v\n}(%s)", rt, gt, ec.toGo("o.V"), e)
		}}, nil
}

// writesThrough reports whether parameter p (a pointer to a basic type)
// of the function display is one Go writes through or keeps, so the copy
// `T?` would lose the write (decision 0419): sync/atomic, a *Var function
// (flag.BoolVar stores the pointer), and parameters named as outputs.
func writesThrough(display, name string, t types.Type, pkg string) bool {
	pt, ok := types.Unalias(t).(*types.Pointer)
	if !ok {
		return false
	}
	if _, ok := types.Unalias(pt.Elem()).(*types.Basic); !ok {
		return false
	}
	fn := display
	if k := strings.LastIndex(fn, "."); k >= 0 {
		fn = fn[k+1:]
	}
	switch name {
	case "dst", "out", "ptr", "addr", "p", "result", "dest":
		return true
	}
	return pkg == "sync/atomic" || strings.HasSuffix(fn, "Var") || strings.HasPrefix(fn, "Scan")
}

// instConv maps an instance of a generic type (decision 0415), *T when
// ptr: lru.Cache[K, V] is the handle Cache[K, V] when every type argument
// is a type parameter in scope or a type Go and Xo share, since the
// handle's field holds the Go instance itself.
func (g *gen) instConv(x *types.Named, ptr bool, depth int) (conv, *skipErr) {
	disp := g.display(x)
	if ptr {
		disp = "*" + disp
	}
	if !x.Obj().Exported() {
		return conv{}, skipf(catUnexport, "unexported type %s", g.display(x))
	}
	h, err := g.handleFor(x.Origin(), depth)
	if err != nil {
		return conv{}, err
	}
	var xs, rs []string
	for i := 0; i < x.TypeArgs().Len(); i++ {
		a := x.TypeArgs().At(i)
		c, err := g.mapType(a, depth)
		if err != nil {
			return conv{}, err
		}
		if !c.id {
			return conv{}, skipf(catGeneric, "type argument %s of %s is converted at the boundary", g.display(a), disp)
		}
		xs = append(xs, c.xo)
		rs = append(rs, c.rt)
	}
	xo := h.xo + "[" + strings.Join(xs, ", ") + "]"
	rt := h.goName + "[" + strings.Join(rs, ", ") + "]"
	gt := g.qual(x)
	switch {
	case ptr && !h.byPtr:
		return conv{}, skipf(catPointer, "pointer type %s", disp)
	case ptr:
		return conv{xo: xo, rt: rt, gt: "*" + gt, nilable: true, hnd: h, hpkgs: h.pkgs(),
			toXo: func(e string) string { return rt + "{P: " + e + "}" },
			toGo: func(e string) string { return e + ".P" }}, nil
	}
	h.byValue = true
	if h.byPtr {
		return conv{xo: xo, rt: rt, gt: gt, hpkgs: h.pkgs(),
			toXo: func(e string) string { return rt + "{P: GoPtrTo(" + e + ")}" },
			toGo: func(e string) string { return "*" + e + ".P" }}, nil
	}
	return conv{xo: xo, rt: rt, gt: gt, hpkgs: h.pkgs(),
		toXo: func(e string) string { return rt + "{V: " + e + "}" },
		toGo: func(e string) string { return e + ".V" }}, nil
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

// importable reports whether a program can import Go package path: not
// internal to another module or package, not vendored.
func importable(path string) bool {
	for _, e := range strings.Split(path, "/") {
		if e == "internal" || e == "vendor" {
			return false
		}
	}
	return path != "unsafe" && path != "C"
}

// depAlias is the declaration file's local name for the binding of
// package p (decision 0392): its Go name, numbered when another package
// or the bound one already has it.
func (g *gen) depAlias(p *types.Package) string {
	if a, ok := g.deps[p.Path()]; ok {
		return a
	}
	a := p.Name()
	taken := func(x string) bool {
		if x == g.pkg.Name() || x == "go" || x == "std" {
			return true
		}
		for _, v := range g.deps {
			if v == x {
				return true
			}
		}
		return false
	}
	for i := 2; taken(a); i++ {
		a = fmt.Sprintf("%s%d", p.Name(), i)
	}
	g.deps[p.Path()] = a
	return a
}

// homeHandleName is the Xo name the binding of n's own package gives
// n's handle, as its run names its types (in order, a name taken by the
// prelude or an earlier type prefixed with the package name, then Go),
// or false when that binding has none.
func homeHandleName(n *types.Named) (string, bool) {
	pkg := n.Obj().Pkg()
	scope := pkg.Scope()
	taken := map[string]bool{"go": true}
	for _, name := range scope.Names() {
		tn, ok := scope.Lookup(name).(*types.TypeName)
		if !ok || !tn.Exported() || tn.IsAlias() {
			continue
		}
		m, ok := tn.Type().(*types.Named)
		if !ok || m.TypeParams().Len() > 0 && !genericBindable(m) {
			continue
		}
		if _, basic := m.Underlying().(*types.Basic); basic {
			continue
		}
		if pkg.Path() == "time" && (name == "Duration" || name == "Time") {
			continue
		}
		xn := name
		if preludeTypes[xn] || taken[xn] {
			pn := pkg.Name()
			xn = strings.ToUpper(pn[:1]) + pn[1:] + name
			if preludeTypes[xn] || taken[xn] {
				xn = "Go" + xn
			}
		}
		if preludeTypes[xn] || taken[xn] {
			if tn == n.Obj() {
				return "", false
			}
			continue
		}
		taken[xn] = true
		if tn == n.Obj() {
			return xn, true
		}
	}
	return "", false
}

// genericBindable reports whether generic type n gets a handle in its
// own package's binding: each type parameter's constraint is any,
// comparable or an Ordered (typeParams without core types).
func genericBindable(n *types.Named) bool {
	for i := 0; i < n.TypeParams().Len(); i++ {
		c := n.TypeParams().At(i).Constraint()
		it, _ := c.Underlying().(*types.Interface)
		if !(it != nil && it.Empty() || types.Identical(c, types.Universe.Lookup("comparable").Type()) || isOrdered(c)) {
			return false
		}
	}
	return true
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
	if foreign {
		// one Xo type per Go type (decision 0392): the type is bound in
		// its own package's binding, and this one refers to it there
		path := obj.Pkg().Path()
		if !importable(path) {
			return nil, skipf(catForeign, "type %s is from package %s, which a program cannot import", g.display(n), path)
		}
		if n.TypeParams().Len() > 0 {
			return nil, skipf(catGeneric, "generic type %s of package %s", g.display(n), path)
		}
		hn, ok := homeHandleName(n)
		if !ok {
			return nil, skipf(catForeign, "type %s has no handle in package %s's binding", g.display(n), path)
		}
		_, byPtr := n.Underlying().(*types.Struct)
		if _, arr := n.Underlying().(*types.Array); arr {
			byPtr = true
		}
		h := &handle{obj: obj, named: n, xo: g.depAlias(obj.Pkg()) + "." + hn, goName: Prefix(path) + hn, gt: g.qual(n), byPtr: byPtr, ref: true, names: map[string]bool{}}
		h.xoT, h.rtT, h.xoDecl, h.rtDecl = h.xo, h.goName, h.xo, h.goName
		g.handles[obj] = h
		return h, nil
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
	h.xoT, h.rtT, h.xoDecl, h.rtDecl = h.xo, h.goName, h.xo, h.goName
	if n.TypeParams().Len() > 0 {
		// a generic type (decision 0415): Cache[K: Eq, V] over
		// lru.Cache[K, V], its methods bound with K and V in scope
		m, order, err := g.typeParams(n.TypeParams())
		if err != nil {
			return nil, err
		}
		var names, xds, rds []string
		for _, tp := range order {
			t := m[tp]
			if t.core != nil {
				return nil, skipf(catGeneric, "type parameter %s of %s has a core type constraint", t.name, g.display(n))
			}
			h.tps = append(h.tps, t)
			names = append(names, t.name)
			rds = append(rds, t.name+" "+t.goBound)
			if t.xoBound != "" {
				xds = append(xds, t.name+": "+t.xoBound)
			} else {
				xds = append(xds, t.name)
			}
		}
		args := "[" + strings.Join(names, ", ") + "]"
		h.xoT, h.rtT, h.gt = h.xo+args, h.goName+args, g.alias(obj.Pkg())+"."+obj.Name()+args
		h.xoDecl, h.rtDecl = h.xo+"["+strings.Join(xds, ", ")+"]", h.goName+"["+strings.Join(rds, ", ")+"]"
	}
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

// anyConv maps Go's any (interface{}) to go.Any (decision 0412): the
// adapter wraps and unwraps the Go value.
func anyConv() conv {
	return conv{xo: "go.Any", rt: "Go_Any", gt: "any",
		toXo: func(e string) string { return "Go_Any{V: " + e + "}" },
		toGo: func(e string) string { return e + ".V" }}
}

// variadicFunc reports whether parameter i of sig is a variadic of an
// unnamed function type, with the element type and its signature. A
// named one (zstd's ...EOption) stays a List of handles, which Go's
// option helpers return and Xo passes back (decision 0422).
func variadicFunc(sig *types.Signature, i int, pt types.Type) (types.Type, *types.Signature, bool) {
	if !sig.Variadic() || i != sig.Params().Len()-1 {
		return nil, nil, false
	}
	sl, ok := pt.(*types.Slice)
	if !ok {
		return nil, nil, false
	}
	fs, ok := types.Unalias(sl.Elem()).(*types.Signature)
	return sl.Elem(), fs, ok
}

// callbackConv maps a func parameter to an Xo function type with an effect
// parameter `fx`.
func (g *gen) callbackConv(s *types.Signature, gt string, depth int) (conv, *skipErr) {
	if s.Variadic() || s.TypeParams().Len() > 0 {
		return conv{}, skipf(catFunc, "callback %s", g.display(s))
	}
	// a final error result is the callback's `! go.Error` (decision 0416)
	nres := s.Results().Len()
	hasErr := nres > 0 && isError(s.Results().At(nres-1).Type())
	if hasErr {
		nres--
	}
	if nres > 1 {
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
	if nres == 1 {
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
	rerr := "Never"
	if hasErr {
		if rc == nil {
			xo += " -> ()"
		}
		xo += " ! go.Error"
		rerr = "Go_Error"
		switch {
		case rc != nil:
			gres = " (" + rc.gt + ", error)"
		default:
			gres = " error"
		}
	}
	xo += " uses fx"
	rt := "func(" + strings.Join(append([]string{"*Task"}, rps...), ", ") + ") Result[" + rres + ", " + rerr + "]"
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
		switch {
		case hasErr && rc != nil:
			// an Xo error is Go's error: the Go error it came from, or
			// one with its message (rt GoErrorOf)
			fn = "func(" + strings.Join(gps, ", ") + ")" + gres + " { " + begin + "r := " + call + "; if !r.Ok { var z " + rc.gt + "; return z, GoErrorOf(r.E) }; return " + rc.toGo("r.V") + ", nil }"
		case hasErr:
			fn = "func(" + strings.Join(gps, ", ") + ")" + gres + " { " + begin + "r := " + call + "; if !r.Ok { return GoErrorOf(r.E) }; return nil }"
		case rc != nil:
			fn = "func(" + strings.Join(gps, ", ") + ")" + gres + " { " + begin + "return " + rc.toGo(call+".V") + " }"
		}
		return "func(cb *GoCallback) " + gt + " { return " + fn + " }(NewGoCallback(tk))"
	}}, nil
}

// mapResult maps a value Go gives Xo (a result, a field or a variable
// read): mapType, and a Go function value that mapType skips is an Xo
// function that calls it (funcValueConv).
func (g *gen) mapResult(t types.Type, depth int) (conv, *skipErr) {
	fs, ok := t.Underlying().(*types.Signature)
	if !ok {
		return g.mapType(t, depth)
	}
	// a named function type stays mapType's handle: it goes back to Go
	// (a functional option, zstd.EOption, passed to NewWriter) and keeps
	// its methods (http.HandlerFunc); an instance (iter.Seq[T]) is a List
	if _, named := types.Unalias(t).(*types.Named); named {
		return g.mapType(t, depth)
	}
	return g.funcValueConv(fs, g.qual(t), depth)
}

// funcValueConv maps a Go function value given to Xo to an Xo function
// `fn(..) -> R ! go.Error uses go` whose call calls it, converting the
// arguments to Go and the results to Xo as a bound function's adapter
// does. A nil Go function faults when called (Go's nil call panic). It
// cannot go back to Go (toGo is nil); a Go function parameter is a
// callback (callbackConv).
func (g *gen) funcValueConv(s *types.Signature, gt string, depth int) (conv, *skipErr) {
	if s.Variadic() || s.TypeParams().Len() > 0 {
		return conv{}, skipf(catFunc, "function value %s", g.display(s))
	}
	nres := s.Results().Len()
	hasErr := nres > 0 && isError(s.Results().At(nres-1).Type())
	if hasErr {
		nres--
	}
	if nres > 1 {
		return conv{}, skipf(catFunc, "function value %s has several results", g.display(s))
	}
	hpkgs := []string{g.pkg.Path()}
	var xps, rps, args []string
	for i := 0; i < s.Params().Len(); i++ {
		pc, err := g.mapType(s.Params().At(i).Type(), depth)
		if err != nil {
			return conv{}, skipf(catFunc, "function value %s: %s", g.display(s), err.detail)
		}
		if pc.toGo == nil {
			return conv{}, skipf(catFunc, "function value %s: parameter %s cannot be passed to Go", g.display(s), pc.xo)
		}
		xps = append(xps, pc.xo)
		rps = append(rps, fmt.Sprintf("a%d %s", i, pc.rt))
		args = append(args, pc.toGo(fmt.Sprintf("a%d", i)))
		hpkgs = append(hpkgs, pc.hpkgs...)
	}
	var rc *conv
	if nres == 1 {
		c, err := g.mapType(s.Results().At(0).Type(), depth)
		if err != nil {
			return conv{}, skipf(catFunc, "function value %s: %s", g.display(s), err.detail)
		}
		if c.nilable {
			crt := c.rt
			c = conv{xo: c.xo + "?", rt: "Option[" + crt + "]", hpkgs: c.hpkgs, toXo: func(e string) string {
				return "Option[" + crt + "]{Some: " + e + " != nil, V: " + crt + "{P: " + e + "}}"
			}}
		}
		rc = &c
		hpkgs = append(hpkgs, c.hpkgs...)
	}
	xo := "fn(" + strings.Join(xps, ", ") + ")"
	rres, rerr := "Unit", "Never"
	if rc != nil {
		xo += " -> " + rc.xo
		rres = rc.rt
	}
	if hasErr {
		if rc == nil {
			xo += " -> ()"
		}
		xo += " ! go.Error"
		rerr = "Go_Error"
	}
	// parenthesized: in a result a trailing `uses` would be the
	// function's own (spec, "Function types in return position")
	xo = "(" + xo + " uses go)"
	res := "Result[" + rres + ", " + rerr + "]"
	var rtps []string
	for _, p := range rps {
		rtps = append(rtps, p[strings.IndexByte(p, ' ')+1:])
	}
	rt := "func(" + strings.Join(append([]string{"*Task"}, rtps...), ", ") + ") " + res
	display := g.display(s)
	return conv{xo: xo, rt: rt, gt: gt, hpkgs: hpkgs, toXo: func(e string) string {
		call := "f(" + strings.Join(args, ", ") + ")"
		var body string
		switch {
		case rc != nil && hasErr:
			body = "r0, err := " + call + "; if err != nil { return " + res + "{E: GoErr(err)} }; return " + res + "{Ok: true, V: " + rc.toXo("r0") + "}"
		case rc != nil:
			body = "r0 := " + call + "; return " + res + "{Ok: true, V: " + rc.toXo("r0") + "}"
		case hasErr:
			body = "if err := " + call + "; err != nil { return " + res + "{E: GoErr(err)} }; return " + res + "{Ok: true}"
		default:
			body = call + "; return " + res + "{Ok: true}"
		}
		fn := "func(" + strings.Join(append([]string{"tk *Task"}, rps...), ", ") + ") " + res + " { GoEnter(tk); defer GoExit(tk, " + strconv.Quote(display) + "); " + body + " }"
		return "func(f " + gt + ") " + rt + " { return " + fn + " }(" + e + ")"
	}}, nil
}

// bindFunc writes the declaration and adapter of one function or method.
func (g *gen) bindFunc(s fnSpec) *skipErr {
	sig := s.sig
	var tpOrder []*types.TypeParam
	if sig.TypeParams().Len() > 0 {
		if p := g.pkg.Path(); p == "slices" || p == "maps" {
			// owner, 2026-10-29 (decision 0415): the prelude's List and
			// Map methods cover them
			return skipf(catGeneric, "generic function of Go's %s: use the prelude's List and Map methods (decision 0415)", p)
		}
		tps, order, err := g.typeParams(sig.TypeParams())
		if err != nil {
			return err
		}
		g.tps, tpOrder = tps, order
		defer func() { g.tps = nil }()
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
		if writesThrough(s.display, p.Name(), pt, g.pkg.Path()) {
			pn := p.Name()
			if pn == "" {
				pn = fmt.Sprintf("#%d", i+1)
			}
			return skipf(catPointer, "parameter %s: Go writes through %s, which Xo passes as a copy (decision 0419)", pn, g.display(pt))
		}
		if outBuffer(s.display, p.Name(), pt, sig) {
			pn := p.Name()
			if pn == "" {
				pn = fmt.Sprintf("#%d", i+1)
			}
			return skipf(catOutBuf, "parameter %s: a []byte Go writes into, which an Xo Bytes (a value) cannot carry back", pn)
		}
		var c conv
		var err *skipErr
		if nt, ok := types.Unalias(pt).(*types.Named); ok && nt.Obj().Pkg() != nil && !nt.Obj().Exported() {
			// a named unexported callback type (bluemonday's urlRewriter):
			// the adapter would name it
			err = skipf(catUnexport, "unexported type %s", g.display(pt))
		} else if fs, ok := pt.Underlying().(*types.Signature); ok && !(sig.Variadic() && i == n-1) {
			c, err = g.callbackConv(fs, g.qual(pt), s.depth)
			fx = true
		} else if et, fs, ok := variadicFunc(sig, i, pt); ok {
			// Go's functional options, optFns ...func(*s3.Options):
			// a List of callbacks, each its own (decision 0422)
			var ec conv
			if ec, err = g.callbackConv(fs, g.qual(et), s.depth); err == nil {
				// an option sets the options through Go setters, so the
				// element allows go itself: a list's elements share one
				// effect variable, and a first element that performs
				// nothing would make it pure for the rest
				ec.xo = strings.TrimSuffix(ec.xo, " uses fx") + " uses (go, fx)"
				c = listConv(ec)
			}
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
		c, err := g.mapResult(res.At(i).Type(), s.depth)
		if err != nil {
			return skipf(err.cat, "result %d: %s", i+1, err.detail)
		}
		rcs = append(rcs, c)
		hpkgs = append(hpkgs, c.hpkgs...)
	}
	mode := g.replayMode(s, fx, hpkgs)
	if len(tpOrder) > 0 && mode == replayValue {
		// a type argument may be a handle the signature cannot show
		mode = replayHandle
	}
	// result types and the Xo value of the results r0..rn
	var xoRes, rtRes, val string
	var checks []string
	optional := func(c conv) conv {
		if !c.nilable {
			return c
		}
		return conv{xo: c.xo + "?", rt: "Option[" + c.rt + "]", hpkgs: c.hpkgs, toXo: func(e string) string {
			return "Option[" + c.rt + "]{Some: " + e + " != nil, V: " + c.rt + "{P: " + e + "}}"
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
		fmt.Fprintf(&d, "(%s: %s) ", recvName, s.recv.xoT)
	}
	d.WriteString(s.xoName)
	var dtps, gtps, targs []string
	for _, tp := range tpOrder {
		t := g.tps[tp]
		switch {
		case t.core != nil:
			targs = append(targs, g.coreGo(t.core))
		case t.xoBound != "":
			dtps = append(dtps, t.name+": "+t.xoBound)
		default:
			dtps = append(dtps, t.name)
		}
		if t.core == nil {
			gtps = append(gtps, t.name+" "+t.goBound)
			targs = append(targs, t.name)
		}
	}
	if fx {
		dtps = append(dtps, "fx: Effects")
	}
	if len(dtps) > 0 {
		d.WriteString("[" + strings.Join(dtps, ", ") + "]")
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
		fmt.Fprintf(&a, "func (h %s) M_%s(%s)%s {\n", s.recv.rtT, s.xoName, params, goRet)
	} else {
		tpl := ""
		if len(gtps) > 0 {
			tpl = "[" + strings.Join(gtps, ", ") + "]"
		}
		fmt.Fprintf(&a, "func %s%s%s(%s)%s {\n", g.b.Prefix, s.xoName, tpl, params, goRet)
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
	if len(targs) > 0 {
		// explicit, so a type parameter only a result names is known
		call = s.goCall + "[" + strings.Join(targs, ", ") + "](" + strings.Join(args, ", ") + ")"
	}
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
		if !ok {
			continue
		}
		if _, basic := n.Underlying().(*types.Basic); basic {
			continue
		}
		if tn.Pkg().Path() == "time" && (name == "Duration" || name == "Time") {
			continue
		}
		if _, err := g.handleFor(n, 0); err != nil && n.TypeParams().Len() > 0 {
			g.skipMethods(n, err.cat, err.detail)
		}
	}
	var basics []*types.Named
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
			if _, basic := n.Underlying().(*types.Basic); basic {
				// bound after every package function, which keeps its
				// name on a clash
				basics = append(basics, n)
			}
		}
	}
	for _, n := range basics {
		g.bindBasicMethods(n)
	}
	// an Xo implementation of each interface (decision 0416), after every
	// handle its methods name exists; a constructor's callbacks may add
	// handles, bound in turn
	impl := map[*handle]bool{}
	for {
		for len(g.queue) > 0 {
			h := g.queue[0]
			g.queue = g.queue[1:]
			g.bindHandle(h)
		}
		n := len(impl)
		for _, h := range append([]*handle(nil), g.order...) {
			if !impl[h] && !h.foreign && !h.ref && len(h.tps) == 0 && isInterfaceType(h.named) {
				impl[h] = true
				g.bindImpl(h)
			}
		}
		if len(impl) == n && len(g.queue) == 0 {
			break
		}
	}
	g.bindUpcasts()
}

// upcastTargets are the interfaces of other packages bindUpcasts offers.
var upcastTargets = map[string][]string{
	"io":  {"Reader", "Writer", "Closer", "ReadCloser", "WriteCloser", "ReadWriter", "ReaderAt", "WriterTo", "ReaderFrom", "StringWriter", "ByteReader"},
	"fmt": {"Stringer"},
}

// bindUpcasts declares, for each handle of the package whose Go type is
// assignable to another of its interfaces, a method that gives the
// value as that interface: `fn (x: ReadCloser) as_reader() -> Reader`
// (decision 0416 follow-up: Go's assignability does not cross the handle
// boundary, so `io.read_all(body)` with an io.ReadCloser body did not
// check).
func (g *gen) bindUpcasts() {
	var ifaces []*handle
	for _, h := range g.order {
		if !h.foreign && !h.ref && len(h.tps) == 0 && isInterfaceType(h.named) {
			ifaces = append(ifaces, h)
		}
	}
	// the interfaces Go composes I/O with, from io and fmt when the
	// package depends on them (bytes.Buffer as an io.Writer)
	for _, path := range sortedKeys(upcastTargets) {
		if g.imp == nil || path == g.pkg.Path() {
			continue
		}
		// the same *types.Package, completed: the importer shares its
		// packages, so the interfaces stay identical to the ones the
		// package's signatures name; a package that is not a
		// dependency has no export data and offers nothing
		full, err := g.imp.Import(path)
		if err != nil {
			continue
		}
		for _, name := range upcastTargets[path] {
			tn, ok := full.Scope().Lookup(name).(*types.TypeName)
			if !ok {
				continue
			}
			n, ok := tn.Type().(*types.Named)
			if !ok {
				continue
			}
			if i, err := g.handleFor(n, 0); err == nil {
				ifaces = append(ifaces, i)
			}
		}
	}
	for _, h := range append([]*handle(nil), g.order...) {
		if h.foreign || h.ref || len(h.tps) > 0 {
			continue
		}
		var from types.Type = h.named
		field := "V"
		if h.byPtr {
			from, field = types.NewPointer(h.named), "P"
		}
		for _, i := range ifaces {
			if i == h || !types.AssignableTo(from, i.named) {
				continue
			}
			xn := "as_" + snake(i.named.Obj().Name())
			if h.names[xn] {
				g.skip(h.xo+"."+xn, g.display(from)+" as "+g.display(i.named), skipf(catCollision, "method name `%s` is taken", xn), false)
				continue
			}
			h.names[xn] = true
			h.decls = append(h.decls, fmt.Sprintf("/// Go: the value as a %s\nfn (x: %s) %s() -> %s uses go", g.display(i.named), h.xoT, xn, i.xo))
			fmt.Fprintf(&g.adapt, "func (h %s) M_%s(tk *Task) %s {\n\treturn %s{V: h.%s}\n}\n\n", h.rtT, xn, i.goName, i.goName, field)
		}
	}
}

// unnamedSig is the Go function type func(ps...) (rs...) with no names.
func unnamedSig(ps, rs []types.Type) *types.Signature {
	var pv, rv []*types.Var
	for _, t := range ps {
		pv = append(pv, types.NewVar(token.NoPos, nil, "", t))
	}
	for _, t := range rs {
		rv = append(rv, types.NewVar(token.NoPos, nil, "", t))
	}
	return types.NewSignatureType(nil, nil, nil, types.NewTuple(pv...), types.NewTuple(rv...), false)
}

// bindImpl binds the constructor of an Xo implementation of interface
// handle h (decision 0416, as built: named function parameters, one per
// method, instead of a struct of functions): `fn writer(write: fn(Bytes)
// -> Int ! go.Error uses fx) -> Writer`. The adapter's type xoImpl_H
// holds the Go functions callbackConv makes of the Xo ones, and its
// methods call them. A Read method (a []byte it fills, (int, error)
// results; outBuffer) is `fn(Int) -> Bytes ! go.Error` given len(p): the
// bytes are copied into p, and none with no error is io.EOF.
func (g *gen) bindImpl(h *handle) {
	it := h.named.Underlying().(*types.Interface)
	if it.NumMethods() == 0 {
		return
	}
	xn := xoName(snake(h.xo))
	if g.scope[xn] {
		xn = "implement_" + snake(h.xo)
	}
	display := g.display(h.named) + " implementation"
	if g.scope[xn] {
		g.skip(xn, display, skipf(catCollision, "name `%s` is taken", xn), false)
		return
	}
	impl := "xoImpl_" + h.goName
	var params []*types.Var
	var fields, methods, fargs []string
	for i := 0; i < it.NumMethods(); i++ {
		m := it.Method(i)
		if !m.Exported() {
			g.skip(xn, display, skipf(catIface, "interface %s: method %s is unexported", g.display(h.named), m.Name()), false)
			return
		}
		sig := m.Type().(*types.Signature)
		if sig.Variadic() {
			g.skip(xn, display, skipf(catFunc, "interface %s: method %s is variadic", g.display(h.named), m.Name()), false)
			return
		}
		ps, rs := sig.Params(), sig.Results()
		var gps, args []string
		for j := 0; j < ps.Len(); j++ {
			gps = append(gps, fmt.Sprintf("p%d %s", j, g.qual(ps.At(j).Type())))
			args = append(args, fmt.Sprintf("p%d", j))
		}
		var grs []string
		for j := 0; j < rs.Len(); j++ {
			grs = append(grs, g.qual(rs.At(j).Type()))
		}
		gres := ""
		switch len(grs) {
		case 0:
		case 1:
			gres = " " + grs[0]
		default:
			gres = " (" + strings.Join(grs, ", ") + ")"
		}
		// the callback's Go type, with no names (Go rejects a mix of
		// named and unnamed parameters)
		var pts, rts []types.Type
		for j := 0; j < ps.Len(); j++ {
			pts = append(pts, ps.At(j).Type())
		}
		for j := 0; j < rs.Len(); j++ {
			rts = append(rts, rs.At(j).Type())
		}
		ft := unnamedSig(pts, rts)
		body := fmt.Sprintf("return x.f%d(%s)", i, strings.Join(args, ", "))
		if len(gres) == 0 {
			body = fmt.Sprintf("x.f%d(%s)", i, strings.Join(args, ", "))
		}
		readRule := ps.Len() > 0 && outBuffer(m.Name(), ps.At(0).Name(), ps.At(0).Type(), sig) && rs.Len() == 2 &&
			types.Identical(rs.At(0).Type(), types.Typ[types.Int]) && isError(rs.At(1).Type())
		if readRule {
			// Read(p []byte) (int, error): the Xo function is given len(p)
			// and returns the bytes
			ft = unnamedSig(append([]types.Type{types.Typ[types.Int]}, pts[1:]...), []types.Type{types.NewSlice(types.Typ[types.Byte]), rts[1]})
			rest := append([]string{"len(p0)"}, args[1:]...)
			body = fmt.Sprintf("b, err := x.f%d(%s)\n\tif len(b) > len(p0) {\n\t\tpanic(\"%s: the Xo function returned more bytes than the buffer holds\")\n\t}\n\tn := copy(p0, b)\n\tif err == nil && n == 0 && len(p0) > 0 {\n\t\terr = %s.EOF\n\t}\n\treturn n, err",
				i, strings.Join(rest, ", "), g.display(h.named)+"."+m.Name(), g.aliasPath("io"))
		}
		pn := xoName(snake(m.Name()))
		params = append(params, types.NewVar(token.NoPos, nil, pn, ft))
		fields = append(fields, fmt.Sprintf("f%d %s", i, g.qual(ft)))
		fargs = append(fargs, fmt.Sprintf("f%d", i))
		methods = append(methods, fmt.Sprintf("func (x *%s) %s(%s)%s {\n\t%s\n}\n", impl, m.Name(), strings.Join(gps, ", "), gres, body))
	}
	sig := types.NewSignatureType(nil, nil, nil, types.NewTuple(params...), types.NewTuple(types.NewVar(token.NoPos, nil, "", h.named)), false)
	mark := g.adapt.Len()
	if err := g.bindFunc(fnSpec{xoName: xn, display: display, sig: sig, goCall: "xoNew_" + h.goName}); err != nil {
		g.adapt.Truncate(mark)
		g.skip(xn, display, &skipErr{cat: err.cat, detail: "interface " + g.display(h.named) + " cannot be implemented in Xo: " + err.detail}, false)
		return
	}
	g.scope[xn] = true
	g.stats.Impls++
	var fps []string
	for i, p := range params {
		fps = append(fps, fmt.Sprintf("f%d %s", i, g.qual(p.Type())))
	}
	fmt.Fprintf(&g.adapt, "// %s is an Xo implementation of Go %s (decision 0416).\ntype %s struct{\n\t%s\n}\n\n", impl, g.display(h.named), impl, strings.Join(fields, "\n\t"))
	for _, m := range methods {
		g.adapt.WriteString(m + "\n")
	}
	fmt.Fprintf(&g.adapt, "func xoNew_%s(%s) %s {\n\treturn &%s{%s}\n}\n\n", h.goName, strings.Join(fps, ", "), h.gt, impl, strings.Join(fargs, ", "))
}

// skipMethods records each exported method of n as skipped.
// bindBasicMethods binds each value receiver method of a named basic type,
// which Xo sees as its basic type (time.Month is Int), as a package
// function `<type>_<method>` taking the value first (decision 0416):
// time.month_string(m: Int) -> Str calls time.Month(m).String() through
// the method expression. A pointer receiver method (it changes the value)
// and a name a package function already has are skipped.
func (g *gen) bindBasicMethods(n *types.Named) {
	ms := types.NewMethodSet(types.NewPointer(n))
	for i := 0; i < ms.Len(); i++ {
		f := ms.At(i).Obj().(*types.Func)
		if !f.Exported() {
			continue
		}
		g.stats.Methods++
		name := xoName(snake(n.Obj().Name()) + "_" + snake(f.Name()))
		display := "(" + g.display(n) + ")." + f.Name()
		sig := f.Type().(*types.Signature)
		if _, ptr := sig.Recv().Type().(*types.Pointer); ptr {
			g.skip(name, display, skipf(catNamedBase, "pointer receiver method of %s, which Xo passes as a value", g.display(n)), true)
			continue
		}
		if g.scope[name] {
			g.skip(name, display, skipf(catCollision, "name `%s` is taken", name), true)
			continue
		}
		rn := sig.Recv().Name()
		if rn == "" || rn == "_" {
			rn = "v"
		}
		ps := []*types.Var{types.NewParam(token.NoPos, nil, rn, n)}
		for j := 0; j < sig.Params().Len(); j++ {
			ps = append(ps, sig.Params().At(j))
		}
		fsig := types.NewSignatureType(nil, nil, nil, types.NewTuple(ps...), sig.Results(), sig.Variadic())
		goCall := g.alias(g.pkg) + "." + n.Obj().Name() + "." + f.Name()
		if err := g.bindFunc(fnSpec{xoName: name, display: display, sig: fsig, goCall: goCall}); err != nil {
			g.skip(name, display, err, true)
			continue
		}
		g.scope[name] = true
		g.stats.MethodsMapped++
	}
}

func (g *gen) skipMethods(n *types.Named, cat, detail string) {
	ms := types.NewMethodSet(types.NewPointer(n))
	for i := 0; i < ms.Len(); i++ {
		f := ms.At(i).Obj()
		if !f.Exported() {
			continue
		}
		g.stats.Methods++
		g.skip(n.Obj().Name()+"."+xoName(snake(f.Name())), "("+g.display(n)+")."+f.Name(), &skipErr{cat: cat, detail: detail}, true)
	}
}

func (g *gen) bindHandle(h *handle) {
	if len(h.tps) > 0 {
		// K and V of Cache[K, V] are in scope in its methods, under the
		// type's names (a method may rename them: RecvTypeParams)
		g.tps = map[*types.TypeParam]*tparam{}
		for i, t := range h.tps {
			g.tps[h.named.TypeParams().At(i)] = t
		}
		defer func() { g.tps = nil }()
	}
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
		if rtp := sig.RecvTypeParams(); rtp != nil && len(h.tps) == rtp.Len() {
			for j, t := range h.tps {
				g.tps[rtp.At(j)] = t
			}
		}
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
		if fs, ok := fv.Type().Underlying().(*types.Signature); ok {
			// a function field: a setter taking an Xo callback and a
			// getter giving Go's function as an Xo function
			g.bindFuncSetter(h, fv, fs, depth)
			c, err := g.mapResult(fv.Type(), depth)
			if err == nil && h.names[xn] {
				err = skipf(catCollision, "field accessor `%s` collides with a method", xn)
			}
			if err != nil {
				g.skip(qname, display, &skipErr{cat: err.cat, detail: "field: " + err.detail}, false)
				continue
			}
			h.names[xn] = true
			h.decls = append(h.decls, fmt.Sprintf("/// Go: field %s %s\nfn (recv: %s) %s() -> %s uses go", fv.Name(), g.display(fv.Type()), h.xoT, xn, c.xo))
			fmt.Fprintf(&g.adapt, "func (h %s) M_%s(tk *Task) %s {\n\tdefer GoBoundary(%q)\n\tv := h.P.%s\n\treturn %s\n}\n\n", h.rtT, xn, c.rt, display, fv.Name(), c.toXo("v"))
			continue
		}
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
			rc = conv{xo: c.xo + "?", rt: "Option[" + c.rt + "]", toXo: func(e string) string {
				return "Option[" + c.rt + "]{Some: " + e + " != nil, V: " + c.rt + "{P: " + e + "}}"
			}}
		}
		h.decls = append(h.decls, fmt.Sprintf("/// Go: field %s %s\nfn (recv: %s) %s() -> %s uses go", fv.Name(), g.display(fv.Type()), h.xoT, xn, rc.xo))
		fmt.Fprintf(&g.adapt, "func (h %s) M_%s(tk *Task) %s {\n\tdefer GoBoundary(%q)\n\tv := h.P.%s\n\treturn %s\n}\n\n", h.rtT, xn, rc.rt, display, fv.Name(), rc.toXo("v"))
		sn := "set_" + snake(fv.Name())
		h.names[sn] = true
		h.decls = append(h.decls, fmt.Sprintf("/// Go: assigns field %s\nfn (recv: %s) %s(v: %s) uses go", fv.Name(), h.xoT, sn, c.xo))
		fmt.Fprintf(&g.adapt, "func (h %s) M_%s(tk *Task, a0 %s) {\n\tdefer GoBoundary(%q)\n\th.P.%s = %s\n}\n\n", h.rtT, sn, c.rt, display, fv.Name(), c.toGo("a0"))
	}
	if !h.names["new"] && len(h.tps) == 0 {
		h.names["new"] = true
		h.decls = append(h.decls, fmt.Sprintf("/// Go: new(%s), the zero value\nfn (static: %s) new() -> %s uses go", g.display(h.named), h.xo, h.xo))
		fmt.Fprintf(&g.adapt, "func %s_new(tk *Task) %s {\n\treturn %s{P: new(%s)}\n}\n\n", h.goName, h.goName, h.goName, h.gt)
	}
}

// bindFuncSetter binds `set_f(v: fn(..) uses fx)` for a field or package
// variable of function type through bindFunc, as a call of a Go function
// literal that assigns its argument, so the callback is converted the way
// a callback parameter is. h is nil for a variable.
func (g *gen) bindFuncSetter(h *handle, v *types.Var, fs *types.Signature, depth int) bool {
	sn := "set_" + snake(v.Name())
	display := g.pkg.Name() + "." + v.Name()
	target := g.alias(g.pkg) + "." + v.Name()
	taken := g.scope[sn]
	if h != nil {
		display = g.display(h.named) + "." + v.Name()
		target = "h.P." + v.Name()
		taken = h.names[sn]
	}
	kind := "variable"
	if h != nil {
		kind = "field"
	}
	if taken {
		g.skip(sn, display, skipf(catCollision, "%s setter `%s` collides with a method", kind, sn), false)
		return false
	}
	if nt, ok := types.Unalias(v.Type()).(*types.Named); ok && nt.Obj().Pkg() != nil && !nt.Obj().Exported() {
		g.skip(sn, display, skipf(catUnexport, "%s: unexported type %s", kind, g.display(v.Type())), false)
		return false
	}
	p := types.NewParam(token.NoPos, nil, "v", v.Type())
	sig := types.NewSignatureType(nil, nil, nil, types.NewTuple(p), nil, false)
	goCall := "(func(v " + g.qual(v.Type()) + ") { " + target + " = v })"
	if err := g.bindFunc(fnSpec{xoName: sn, display: display + " =", sig: sig, recv: h, goCall: goCall, depth: depth}); err != nil {
		g.skip(sn, display, &skipErr{cat: err.cat, detail: kind + ": " + err.detail}, false)
		return false
	}
	if h != nil {
		h.names[sn] = true
		g.stats.FieldsMapped++
	} else {
		g.scope[sn] = true
		g.stats.VarsMapped++
	}
	return true
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
		// a sentinel (io.EOF) is a go.Error, the Go error itself, so
		// errors.is(e, io.eof()) holds (decision 0416); a nil one faults
		g.scope[name] = true
		g.stats.VarsMapped++
		g.vars = append(g.vars, fmt.Sprintf("/// Go: reads variable %s error, the sentinel\nfn %s() -> go.Error uses go", display, name))
		fmt.Fprintf(&g.adapt, "func %s%s(tk *Task) Go_Error {\n\treturn GoErr(%s.%s)\n}\n\n", g.b.Prefix, name, g.alias(g.pkg), o.Name())
		return
	}
	set := false
	if fs, ok := o.Type().Underlying().(*types.Signature); ok {
		set = g.bindFuncSetter(nil, o, fs, 0)
	}
	c, err := g.mapResult(o.Type(), 0)
	if err != nil {
		g.skip(name, display, &skipErr{cat: err.cat, detail: "variable: " + err.detail}, false)
		return
	}
	if c.nilable {
		crt := c.rt
		c = conv{xo: c.xo + "?", rt: "Option[" + crt + "]", toXo: func(e string) string {
			return "Option[" + crt + "]{Some: " + e + " != nil, V: " + crt + "{P: " + e + "}}"
		}}
	}
	g.scope[name] = true
	if !set {
		g.stats.VarsMapped++
	}
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
	for _, path := range sortedKeys(g.deps) {
		fmt.Fprintf(&b, "use go %q as %s\n", path, g.deps[path])
	}
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
			fmt.Fprintf(&b, "/// Go: %s\nstruct %s {}\n", kind, h.xoDecl)
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
		fmt.Fprintf(&body, "// %s is the handle of Go %s.\ntype %s struct{ %s }\n\n", h.goName, disp, h.rtDecl, field)
		fmt.Fprintf(&body, "func (h %s) XoDebug(b *strings.Builder) { GoDebugHandle(b, %q) }\n\n", h.rtT, disp)
		// go.any(h) and go.Any.as[H]() (decision 0412)
		switch {
		case h.byPtr && h.byValue:
			// a type Go hands out as a value (netip.Addr): its value, and a
			// *T read back too
			fmt.Fprintf(&body, "func (h %s) XoGoValue() any {\n\tif h.P == nil {\n\t\treturn nil\n\t}\n\treturn *h.P\n}\n\n", h.rtT)
			fmt.Fprintf(&body, "func (%s) XoFromGo(v any) (any, bool) {\n\tif p, ok := v.(*%s); ok {\n\t\treturn %s{P: p}, true\n\t}\n\tx, ok := v.(%s)\n\treturn %s{P: &x}, ok\n}\n\n", h.rtT, h.gt, h.rtT, h.gt, h.rtT)
		case h.byPtr:
			fmt.Fprintf(&body, "func (h %s) XoGoValue() any { return h.P }\n\n", h.rtT)
			fmt.Fprintf(&body, "func (%s) XoFromGo(v any) (any, bool) {\n\tp, ok := v.(*%s)\n\treturn %s{P: p}, ok\n}\n\n", h.rtT, h.gt, h.rtT)
		default:
			fmt.Fprintf(&body, "func (h %s) XoGoValue() any { return h.V }\n\n", h.rtT)
			fmt.Fprintf(&body, "func (%s) XoFromGo(v any) (any, bool) {\n\tx, ok := v.(%s)\n\treturn %s{V: x}, ok\n}\n\n", h.rtT, h.gt, h.rtT)
		}
		switch {
		case h.byPtr && len(h.tps) == 0 && types.Comparable(h.named) && (!isStructType(h.named) || h.byValue):
			// Go's equality (core 2.9): a comparable type Go hands out as a
			// value (netip.Addr, language.Tag, arrays such as uuid.UUID)
			// compares by value; a struct the API passes only as *T
			// (url.Parse's *url.URL) compares by pointer, as Go's == on
			// the pointers does (Intel b15, 0.33.0: two equal netip.Addr
			// were unequal)
			fmt.Fprintf(&body, "func (h %s) XoEq(o any) bool {\n\tx, ok := o.(%s)\n\treturn ok && (h.P == x.P || h.P != nil && x.P != nil && *h.P == *x.P)\n}\n\n", h.rtT, h.rtT)
			fmt.Fprintf(&body, "func (h %s) XoHash() uint64 {\n\tif h.P == nil {\n\t\treturn 0\n\t}\n\treturn GoHashComparable(*h.P)\n}\n\n", h.rtT)
		case h.byPtr:
			fmt.Fprintf(&body, "func (h %s) XoEq(o any) bool {\n\tx, ok := o.(%s)\n\treturn ok && h.P == x.P\n}\n\n", h.rtT, h.rtT)
			fmt.Fprintf(&body, "func (h %s) XoHash() uint64 { return GoHashComparable(h.P) }\n\n", h.rtT)
		case isInterfaceType(h.named):
			fmt.Fprintf(&body, "func (h %s) XoEq(o any) bool {\n\tx, ok := o.(%s)\n\treturn ok && GoEqIface(h.V, x.V)\n}\n\n", h.rtT, h.rtT)
			fmt.Fprintf(&body, "func (h %s) XoHash() uint64 { return GoHashIface(h.V) }\n\n", h.rtT)
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
