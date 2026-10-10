package check

import (
	"fmt"
	"sort"

	"github.com/xo-lang/xo/internal/ast"
	"github.com/xo-lang/xo/internal/diag"
	"github.com/xo-lang/xo/internal/token"
)

// Comparability (decision 0112). A type is comparable (`==`, `!=`, Map
// keys, Set elements, `Eq`, `Hash`, and `derive Ord`) only if all its
// fields, payloads, tuple elements, and type arguments are; function
// values and handles are not. Interface values compare through their
// dynamic value and count as comparable.

// handleTypes are the std types that are handles to a runtime object,
// by module path ("" for the prelude) and name.
var handleTypes = map[string]map[string]bool{
	"":            {"Chan": true, "Mutex": true, "Atomic": true, "Scope": true, "Task": true, "Listener": true, "Conn": true},
	"std/task":    {"Group": true, "Semaphore": true, "Once": true},
	"std/http":    {"Router": true, "Server": true, "Client": true, "BodyWriter": true, "TlsCert": true},
	"std/sql":     {"Db": true, "Tx": true},
	"std/testing": {"FakeNet": true, "FakeClock": true, "MemFs": true, "FakeEnv": true, "FakeStdio": true, "FakeProc": true},
}

// isHandleObj reports a handle type: a std handle of handleTypes, or an
// opaque object of a native implementation module. The opaque Go values
// of `use go` bindings are not handles here: they exist only on the Go
// backend and keep Go's equality (`uuid.UUID`, a [16]byte, compares by
// value; decision 0041).
func isHandleObj(o *TypeObj) bool {
	m := o.Module
	if m == nil || o.Kind != KStruct || m.Go != nil {
		return false
	}
	switch {
	case m.IsPrelude:
		return handleTypes[""][o.Name]
	case m.C != nil:
		// an opaque C pointer of `use c` (decision 0113)
		return o.Opaque
	case m.Native:
		return o.Opaque && o.Name != "Loc"
	case m.IsStd:
		return handleTypes[m.Path][o.Name]
	}
	return false
}

// incomparable returns why values of t cannot be compared ("" when they
// can): the path to the first function value or handle inside t.
func (c *checker) incomparable(t Type) string {
	return c.incomparableIn(t, "", map[*TypeObj]bool{})
}

func (c *checker) incomparableIn(t Type, at string, seen map[*TypeObj]bool) string {
	where := func(what string) string {
		if at == "" {
			return what
		}
		return at + " holds " + what
	}
	switch x := resolve(t).(type) {
	case *Func:
		return where(fmt.Sprintf("a function value (`%s`)", deep(x)))
	case *Tuple:
		for i, e := range x.Elems {
			if r := c.incomparableIn(e, fieldPath(at, fmt.Sprint(i)), seen); r != "" {
				return r
			}
		}
	case *Named:
		o := x.Obj
		if isHandleObj(o) {
			return where(fmt.Sprintf("the handle `%s`", deep(x)))
		}
		if o.Kind == KInterface {
			return ""
		}
		for _, a := range x.Args {
			if _, eff := a.(*EffArg); eff {
				continue
			}
			if r := c.incomparableIn(a, at, seen); r != "" {
				return r
			}
		}
		if seen[o] {
			return ""
		}
		seen[o] = true
		defer delete(seen, o)
		sm := typeArgsSubst(o.TParams, x.Args)
		switch o.Kind {
		case KStruct:
			for _, f := range o.Fields {
				if r := c.incomparableIn(subst(f.Type, sm), fieldPath(at, f.Name), seen); r != "" {
					return r
				}
			}
		case KEnum:
			for _, v := range o.Variants {
				for i, f := range v.Fields {
					n := f.Name
					if n == "" {
						n = fmt.Sprint(i)
					}
					if r := c.incomparableIn(subst(f.Type, sm), fieldPath(at, v.Name+"."+n), seen); r != "" {
						return r
					}
				}
			}
		case KNewtype:
			if o.Target != nil {
				return c.incomparableIn(subst(o.Target, sm), fieldPath(at, "0"), seen)
			}
		}
	}
	return ""
}

func fieldPath(at, f string) string {
	if at == "" {
		return "field `" + f + "`"
	}
	return at[:len(at)-1] + "." + f + "`"
}

// noteIncomparable adds decision 0112's note to d: why the type is not
// comparable and what to compare instead.
func noteIncomparable(d *diag.Diagnostic, why string) {
	if d == nil {
		return
	}
	note(d, token.Span{}, "it is not comparable: %s; handles and function values have no equality (decision 0112)", why)
	note(d, token.Span{}, "compare or key by an identifying field instead, such as an `id: Int` or `name: Str` kept next to the handle")
}

// checkDerivesComparable rejects `derive Eq` or `Hash` on a type
// of m that holds a handle or a function value (decision 0112); `derive
// Ord` is checked field by field elsewhere, and neither is `Ord`.
func (c *checker) checkDerivesComparable(m *Module) {
	names := sortedKeys(m.Scope)
	for _, n := range names {
		t, ok := m.Scope[n].(*TypeObj)
		if !ok || t.Module != m {
			continue
		}
		var ids []*ast.Ident
		switch {
		case t.structDecl != nil:
			ids = t.structDecl.Derive
		case t.enumDecl != nil:
			ids = t.enumDecl.Derive
		}
		var args []Type
		for _, p := range t.TParams {
			args = append(args, &ParamType{P: p})
		}
		why := ""
		for _, id := range ids {
			if id.Name != "Eq" && id.Name != "Hash" {
				continue
			}
			if why == "" {
				why = c.incomparable(&Named{Obj: t, Args: args})
				if why == "" {
					break
				}
			}
			d := c.errorf(diag.NotImplemented, id.Sp, "`%s` cannot derive `%s`", t.Name, id.Name)
			noteIncomparable(d, why)
		}
	}
}

// checkKeyType reports a Map whose key type, or a Set whose element
// type, is not comparable, once per type and file.
func (c *checker) checkKeyType(t Type, sp token.Span) {
	n, ok := resolve(t).(*Named)
	if !ok || n.Obj.Module != c.prelude || len(n.Args) == 0 {
		return
	}
	what := ""
	switch n.Obj.Name {
	case "Map":
		what = "a Map key"
	case "Set":
		what = "a Set element"
	default:
		return
	}
	k := deep(n.Args[0])
	if hasOpenVars(k) {
		return
	}
	why := c.incomparable(k)
	if why == "" {
		return
	}
	key := sp.File + "\x00" + deep(n).String()
	if c.keyReported[key] {
		return
	}
	c.keyReported[key] = true
	d := c.errorf(diag.NotImplemented, sp, "`%s` cannot be %s", k, what)
	noteIncomparable(d, why)
}

// checkKeyTypes looks at the type of every expression of the checked
// modules for Map and Set types whose keys are not comparable, and
// reports each at its first expression in a file.
func (c *checker) checkKeyTypes() {
	files := map[string]bool{}
	for _, m := range c.order {
		if m.IsStd {
			continue
		}
		for _, f := range m.Files {
			files[f.Path] = true
		}
	}
	type cand struct {
		t  Type
		sp token.Span
	}
	var cs []cand
	for e, t := range c.exprType {
		sp := e.Span()
		if !files[sp.File] {
			continue
		}
		walkNamed(deep(t), func(n *Named) {
			if n.Obj.Module == c.prelude && (n.Obj.Name == "Map" || n.Obj.Name == "Set") {
				cs = append(cs, cand{n, sp})
			}
		})
	}
	sort.Slice(cs, func(i, j int) bool {
		a, b := cs[i].sp, cs[j].sp
		if a.File != b.File {
			return a.File < b.File
		}
		if a.Start.Offset != b.Start.Offset {
			return a.Start.Offset < b.Start.Offset
		}
		return a.End.Offset < b.End.Offset
	})
	for _, x := range cs {
		c.checkKeyType(x.t, x.sp)
	}
}

// walkNamed calls f on every named type inside t, outermost first.
func walkNamed(t Type, f func(*Named)) {
	switch x := t.(type) {
	case *Named:
		f(x)
		for _, a := range x.Args {
			walkNamed(a, f)
		}
	case *Tuple:
		for _, e := range x.Elems {
			walkNamed(e, f)
		}
	case *Func:
		for _, p := range x.Params {
			walkNamed(p.T, f)
		}
		if x.Result != nil {
			walkNamed(x.Result, f)
		}
		if x.Err != nil {
			walkNamed(x.Err, f)
		}
	}
}
