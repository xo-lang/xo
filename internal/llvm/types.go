package llvm

import (
	"fmt"
	"strings"

	"github.com/xo-lang/xo/internal/ast"
	"github.com/xo-lang/xo/internal/check"
)

// kind classifies lowered types.
type kind int

const (
	kInt kind = iota
	kBool
	kFloat
	kStr
	kUnit // `()`, Never, and capability values: no data
	kStruct
	kTuple
	kEnum
	kList
	kFn
)

// ty is a lowered concrete Xo type.
type ty struct {
	t     check.Type
	key   string
	k     kind
	ll    string
	size  int64
	align int64
	// rc reports that values hold list buffers inline (not behind a box),
	// so copying one must retain them (mark them shared).
	rc     bool
	fields []*field   // struct fields, tuple elements
	vars   []*variant // enum variants
	words  int64      // enum payload size in 8 byte words
	elem   *ty        // list element
	fn     *check.Func
	obj    *check.TypeObj
}

// field is a struct field, tuple element, or variant field.
type field struct {
	name  string
	ct    check.Type
	t     *ty // nil while boxed and not yet needed (fieldTy)
	boxed bool
}

// variant is an enum variant and its payload layout.
type variant struct {
	name   string
	obj    *check.VariantObj
	fields []*field
	ll     string // payload struct type
}

var unitType check.Type = &check.Tuple{}

// fieldTy returns the lowered type of a field (boxed fields are lowered
// lazily, which breaks the cycle of a recursive type).
func (g *gen) fieldTy(f *field, at ast.Node) *ty {
	if f.t == nil {
		f.t = g.lower(f.ct, at)
	}
	return f.t
}

func (g *gen) isPrelude(t check.Type, name string) bool {
	n, ok := check.Resolve(t).(*check.Named)
	return ok && n.Obj != nil && n.Obj.Module != nil && n.Obj.Module.IsPrelude && n.Obj.Name == name
}

func (g *gen) named(t check.Type) *check.Named {
	n, _ := check.Resolve(t).(*check.Named)
	return n
}

// optionElem returns T of `T?`, or nil.
func (g *gen) optionElem(t check.Type) check.Type {
	if n := g.named(t); n != nil && g.isPrelude(n, "Option") && len(n.Args) == 1 {
		return n.Args[0]
	}
	return nil
}

// resultParts returns T and E of Result[T, E].
func (g *gen) resultParts(t check.Type) (check.Type, check.Type, bool) {
	if n := g.named(t); n != nil && g.isPrelude(n, "Result") && len(n.Args) == 2 {
		return n.Args[0], n.Args[1], true
	}
	return nil, nil, false
}

func (g *gen) preludeType(name string, args ...check.Type) check.Type {
	obj, _ := g.prelude.Scope[name].(*check.TypeObj)
	return &check.Named{Obj: obj, Args: args}
}

func (g *gen) resultOf(t, e check.Type) check.Type { return g.preludeType("Result", t, e) }

// key is a unique name of a concrete type.
func (g *gen) key(t check.Type) string {
	switch x := check.Resolve(t).(type) {
	case nil:
		return "()"
	case *check.Named:
		var b strings.Builder
		if x.Obj.Module != nil && !x.Obj.Module.IsPrelude {
			b.WriteString(x.Obj.Module.Path + ".")
		}
		b.WriteString(x.Obj.Name)
		if len(x.Args) > 0 {
			b.WriteByte('[')
			for i, a := range x.Args {
				if i > 0 {
					b.WriteByte(',')
				}
				b.WriteString(g.key(a))
			}
			b.WriteByte(']')
		}
		return b.String()
	case *check.Tuple:
		var ks []string
		for _, e := range x.Elems {
			ks = append(ks, g.key(e))
		}
		return "(" + strings.Join(ks, ",") + ")"
	case *check.Func:
		var ks []string
		for _, p := range x.Params {
			s := g.key(p.T)
			if p.Var {
				s = "var " + s
			}
			ks = append(ks, s)
		}
		s := "fn(" + strings.Join(ks, ",") + ")->" + g.key(x.Result)
		if x.Err != nil && !check.IsNever(x.Err) {
			s += "!" + g.key(x.Err)
		}
		return s
	case *check.Var:
		switch x.LitKind() {
		case "float":
			return "Float"
		case "int":
			return "Int"
		}
		return "?"
	case *check.ParamType:
		return "$" + x.P.Name
	case *check.EffArg:
		return "fx"
	}
	return fmt.Sprintf("%T", t)
}

// fieldsOf returns the fields of a struct type with its type arguments
// applied.
func fieldsOf(n *check.Named) []*field {
	sub := check.NewSubst(n.Obj.TParams, n.Args)
	var out []*field
	for _, f := range n.Obj.Fields {
		out = append(out, &field{name: f.Name, ct: check.Deep(sub.Apply(f.Type))})
	}
	return out
}

// variantFields returns the fields of a variant with type arguments
// applied.
func variantFields(n *check.Named, v *check.VariantObj) []*field {
	sub := check.NewSubst(n.Obj.TParams, n.Args)
	var out []*field
	for i, f := range v.Fields {
		name := f.Name
		if name == "" {
			name = fmt.Sprint(i)
		}
		out = append(out, &field{name: name, ct: check.Deep(sub.Apply(f.Type))})
	}
	return out
}

// contains reports whether a value of type t holds a value of the type
// with key target inline (through structs, tuples, and enums, not through
// lists or functions). Boxing is ignored, so the answer is conservative.
func (g *gen) contains(t check.Type, target string, seen map[string]bool) bool {
	switch x := check.Resolve(t).(type) {
	case *check.Tuple:
		for _, e := range x.Elems {
			if g.contains(e, target, seen) {
				return true
			}
		}
	case *check.Named:
		k := g.key(x)
		if k == target {
			return true
		}
		if seen[k] {
			return false
		}
		seen[k] = true
		switch x.Obj.Kind {
		case check.KStruct:
			if x.Obj.Module.IsPrelude {
				return false
			}
			for _, f := range fieldsOf(x) {
				if g.contains(f.ct, target, seen) {
					return true
				}
			}
		case check.KEnum:
			for _, v := range x.Obj.Variants {
				for _, f := range variantFields(x, v) {
					if g.contains(f.ct, target, seen) {
						return true
					}
				}
			}
		}
	}
	return false
}

func alignUp(n, a int64) int64 {
	if a <= 1 {
		return n
	}
	return (n + a - 1) / a * a
}

// layout computes the size and alignment of a struct of the given field
// types, as LLVM's default data layout does on 64 bit targets.
func layout(ts []*ty, boxed []bool) (size, align int64) {
	align = 1
	for i, t := range ts {
		s, a := int64(8), int64(8)
		if !boxed[i] {
			s, a = t.size, t.align
		}
		size = alignUp(size, a) + s
		if a > align {
			align = a
		}
	}
	return alignUp(size, align), align
}

// lower returns the lowered form of a concrete type, reporting types
// outside the subset at `at`.
func (g *gen) lower(t check.Type, at ast.Node) *ty {
	if t == nil {
		t = unitType
	}
	t = check.Deep(t)
	k := g.key(t)
	if x, ok := g.types[k]; ok {
		return x
	}
	x := &ty{t: t, key: k}
	g.types[k] = x
	bad := func(what string) *ty {
		g.unsupported(at, what)
		x.k, x.ll, x.size, x.align = kInt, "i64", 8, 8
		return x
	}
	switch r := check.Resolve(t).(type) {
	case *check.Tuple:
		if len(r.Elems) == 0 {
			x.k, x.ll, x.size, x.align = kUnit, "{}", 0, 1
			return x
		}
		x.k = kTuple
		var lls []string
		var ts []*ty
		var boxed []bool
		for i, e := range r.Elems {
			et := g.lower(e, at)
			x.fields = append(x.fields, &field{name: fmt.Sprint(i), ct: e, t: et})
			lls = append(lls, et.ll)
			ts = append(ts, et)
			boxed = append(boxed, false)
			x.rc = x.rc || et.rc
		}
		x.ll = "{ " + strings.Join(lls, ", ") + " }"
		x.size, x.align = layout(ts, boxed)
		return x
	case *check.Func:
		if r.Err != nil && !check.IsNever(r.Err) {
			g.retType(r.Result, r.Err, at)
		}
		x.k, x.ll, x.size, x.align, x.fn = kFn, "%xo.fn", 16, 8, r
		return x
	case *check.Var:
		switch r.LitKind() {
		case "float":
			return g.lower(g.preludeType("Float"), at)
		case "int":
			return g.lower(g.preludeType("Int"), at)
		}
		return bad("a value whose type was not inferred")
	case *check.Named:
		obj := r.Obj
		x.obj = obj
		if obj.Module != nil && obj.Module.IsPrelude {
			switch obj.Name {
			case "Int", "Int64":
				x.k, x.ll, x.size, x.align = kInt, "i64", 8, 8
				return x
			case "Bool":
				x.k, x.ll, x.size, x.align = kBool, "i1", 1, 1
				return x
			case "Float":
				x.k, x.ll, x.size, x.align = kFloat, "double", 8, 8
				return x
			case "Str":
				x.k, x.ll, x.size, x.align = kStr, "%xo.str", 16, 8
				return x
			case "Never", "Os", "Stdio":
				x.k, x.ll, x.size, x.align = kUnit, "{}", 0, 1
				return x
			case "List":
				x.k, x.ll, x.size, x.align, x.rc = kList, "ptr", 8, 8, true
				x.elem = g.lower(r.Args[0], at)
				return x
			case "Option", "Result":
				return g.lowerEnum(x, r, at)
			}
			return bad("the type `" + t.String() + "`")
		}
		switch obj.Kind {
		case check.KStruct:
			return g.lowerStruct(x, r, at)
		case check.KEnum:
			return g.lowerEnum(x, r, at)
		case check.KAlias:
			sub := check.NewSubst(obj.TParams, r.Args)
			y := g.lower(sub.Apply(obj.Target), at)
			g.types[k] = y
			return y
		case check.KInterface:
			return bad("interface types (`" + t.String() + "`)")
		case check.KNewtype:
			return bad("newtypes (`" + t.String() + "`)")
		}
	}
	return bad("the type `" + t.String() + "`")
}

func (g *gen) lowerStruct(x *ty, n *check.Named, at ast.Node) *ty {
	x.k = kStruct
	x.ll = "%" + quoteName("T:"+x.key)
	x.fields = fieldsOf(n)
	var lls []string
	var ts []*ty
	var boxed []bool
	for _, f := range x.fields {
		f.boxed = g.contains(f.ct, x.key, map[string]bool{})
		var ft *ty
		if !f.boxed {
			ft = g.lower(f.ct, at)
			f.t = ft
			x.rc = x.rc || ft.rc
			lls = append(lls, ft.ll)
		} else {
			lls = append(lls, "ptr")
		}
		ts = append(ts, ft)
		boxed = append(boxed, f.boxed)
	}
	x.size, x.align = layout(ts, boxed)
	fmt.Fprintf(&g.typeDefs, "%s = type { %s }\n", x.ll, strings.Join(lls, ", "))
	return x
}

func (g *gen) lowerEnum(x *ty, n *check.Named, at ast.Node) *ty {
	x.k = kEnum
	x.ll = "%" + quoteName("T:"+x.key)
	var maxSize int64
	for _, v := range n.Obj.Variants {
		vr := &variant{name: v.Name, obj: v, fields: variantFields(n, v)}
		var lls []string
		var ts []*ty
		var boxed []bool
		for _, f := range vr.fields {
			f.boxed = g.contains(f.ct, x.key, map[string]bool{})
			var ft *ty
			if !f.boxed {
				ft = g.lower(f.ct, at)
				f.t = ft
				x.rc = x.rc || ft.rc
				lls = append(lls, ft.ll)
			} else {
				lls = append(lls, "ptr")
			}
			ts = append(ts, ft)
			boxed = append(boxed, f.boxed)
		}
		vr.ll = "{ " + strings.Join(lls, ", ") + " }"
		if len(lls) == 0 {
			vr.ll = "{}"
		}
		s, _ := layout(ts, boxed)
		if s > maxSize {
			maxSize = s
		}
		x.vars = append(x.vars, vr)
	}
	x.words = (maxSize + 7) / 8
	if x.words == 0 {
		fmt.Fprintf(&g.typeDefs, "%s = type { i64 }\n", x.ll)
	} else {
		fmt.Fprintf(&g.typeDefs, "%s = type { i64, [%d x i64] }\n", x.ll, x.words)
	}
	x.size, x.align = 8+8*x.words, 8
	return x
}

// variantIndex returns the variant of t named name.
func (t *ty) variant(name string) (int, *variant) {
	for i, v := range t.vars {
		if v.name == name {
			return i, v
		}
	}
	return -1, nil
}

// fieldIndex returns the index of a struct field.
func (t *ty) fieldIndex(name string) int {
	for i, f := range t.fields {
		if f.name == name {
			return i
		}
	}
	return -1
}

// ---------------------------------------------------------------------------
// Helper functions: retain and equality

// retainFn returns the function that retains a value of t in memory, or
// "null" when t holds no list buffers inline.
func (g *gen) retainFn(t *ty, at ast.Node) string {
	if !t.rc {
		return "null"
	}
	name := "retain:" + t.key
	q := "@" + quoteName(name)
	if g.helpers[name] {
		return q
	}
	g.helpers[name] = true
	f := g.newFn(nil, nil)
	switch t.k {
	case kList:
		l := f.def("load ptr, ptr %%p")
		s := f.def("getelementptr %%xo.list, ptr %s, i32 0, i32 2", l)
		f.emit("store i64 1, ptr %s", s)
	case kStruct, kTuple:
		for i, fl := range t.fields {
			if fl.boxed || !g.fieldTy(fl, at).rc {
				continue
			}
			p := f.def("getelementptr %s, ptr %%p, i32 0, i32 %d", t.ll, i)
			f.emit("call void %s(ptr %s)", g.retainFn(fl.t, at), p)
		}
	case kEnum:
		tagp := f.def("getelementptr %s, ptr %%p, i32 0, i32 0", t.ll)
		tag := f.def("load i64, ptr %s", tagp)
		end := f.label("end")
		var cases []string
		var bodies []func()
		for i, v := range t.vars {
			need := false
			for _, fl := range v.fields {
				if !fl.boxed && g.fieldTy(fl, at).rc {
					need = true
				}
			}
			if !need {
				continue
			}
			l := f.label("v")
			cases = append(cases, fmt.Sprintf("i64 %d, label %%%s", i, l))
			v := v
			bodies = append(bodies, func() {
				f.startBlock(l)
				pay := f.def("getelementptr %s, ptr %%p, i32 0, i32 1", t.ll)
				for j, fl := range v.fields {
					if fl.boxed || !fl.t.rc {
						continue
					}
					p := f.def("getelementptr %s, ptr %s, i32 0, i32 %d", v.ll, pay, j)
					f.emit("call void %s(ptr %s)", g.retainFn(fl.t, at), p)
				}
				f.br(end)
			})
		}
		f.emit("switch i64 %s, label %%%s [ %s ]", tag, end, strings.Join(cases, " "))
		f.term = true
		for _, b := range bodies {
			b()
		}
		f.startBlock(end)
	}
	f.emit("ret void")
	f.term = true
	g.writeFunc(name, "void", "ptr %p", f)
	return q
}

// eqFn returns the function comparing two values of t in memory.
func (g *gen) eqFn(t *ty, at ast.Node) string {
	name := "eq:" + t.key
	q := "@" + quoteName(name)
	if g.helpers[name] {
		return q
	}
	g.helpers[name] = true
	f := g.newFn(nil, nil)
	no := f.label("no")
	// cmp compares the values of type ft at a and b, branching to no
	cmp := func(ft *ty, a, b string) {
		ok := f.label("ok")
		var c string
		switch ft.k {
		case kUnit:
			return
		case kInt, kBool, kFloat, kStr, kFn:
			x := f.def("load %s, ptr %s", ft.ll, a)
			y := f.def("load %s, ptr %s", ft.ll, b)
			c = f.eqScalar(ft, x, y, at)
		default:
			c = f.def("call i1 %s(ptr %s, ptr %s)", g.eqFn(ft, at), a, b)
		}
		f.condBr(c, ok, no)
		f.startBlock(ok)
	}
	switch t.k {
	case kStruct, kTuple:
		for i, fl := range t.fields {
			a := f.def("getelementptr %s, ptr %%a, i32 0, i32 %d", t.ll, i)
			b := f.def("getelementptr %s, ptr %%b, i32 0, i32 %d", t.ll, i)
			if fl.boxed {
				a = f.def("load ptr, ptr %s", a)
				b = f.def("load ptr, ptr %s", b)
			}
			cmp(g.fieldTy(fl, at), a, b)
		}
	case kEnum:
		ta := f.def("load i64, ptr %%a")
		tb := f.def("load i64, ptr %%b")
		c := f.def("icmp eq i64 %s, %s", ta, tb)
		sw := f.label("sw")
		f.condBr(c, sw, no)
		f.startBlock(sw)
		yes := f.label("yes")
		var cases []string
		var bodies []func()
		for i, v := range t.vars {
			if len(v.fields) == 0 {
				continue
			}
			l := f.label("v")
			cases = append(cases, fmt.Sprintf("i64 %d, label %%%s", i, l))
			v := v
			bodies = append(bodies, func() {
				f.startBlock(l)
				pa := f.def("getelementptr %s, ptr %%a, i32 0, i32 1", t.ll)
				pb := f.def("getelementptr %s, ptr %%b, i32 0, i32 1", t.ll)
				for j, fl := range v.fields {
					a := f.def("getelementptr %s, ptr %s, i32 0, i32 %d", v.ll, pa, j)
					b := f.def("getelementptr %s, ptr %s, i32 0, i32 %d", v.ll, pb, j)
					if fl.boxed {
						a = f.def("load ptr, ptr %s", a)
						b = f.def("load ptr, ptr %s", b)
					}
					cmp(g.fieldTy(fl, at), a, b)
				}
				f.br(yes)
			})
		}
		f.emit("switch i64 %s, label %%%s [ %s ]", ta, yes, strings.Join(cases, " "))
		f.term = true
		for _, b := range bodies {
			b()
		}
		f.startBlock(yes)
	case kList:
		la := f.def("load ptr, ptr %%a")
		lb := f.def("load ptr, ptr %%b")
		na := f.def("load i64, ptr %s", la)
		nb := f.def("load i64, ptr %s", lb)
		c := f.def("icmp eq i64 %s, %s", na, nb)
		loop := f.label("loop")
		f.condBr(c, loop, no)
		f.startBlock(loop)
		da := f.def("getelementptr %%xo.list, ptr %s, i64 1", la)
		db := f.def("getelementptr %%xo.list, ptr %s, i64 1", lb)
		ip := f.alloca(g.lower(g.preludeType("Int"), at))
		f.emit("store i64 0, ptr %s", ip)
		head := f.label("head")
		body := f.label("body")
		done := f.label("done")
		f.br(head)
		f.startBlock(head)
		i := f.def("load i64, ptr %s", ip)
		more := f.def("icmp slt i64 %s, %s", i, na)
		f.condBr(more, body, done)
		f.startBlock(body)
		ea := f.def("getelementptr %s, ptr %s, i64 %s", t.elem.ll, da, i)
		eb := f.def("getelementptr %s, ptr %s, i64 %s", t.elem.ll, db, i)
		cmp(t.elem, ea, eb)
		i2 := f.def("add i64 %s, 1", i)
		f.emit("store i64 %s, ptr %s", i2, ip)
		f.br(head)
		f.startBlock(done)
	case kInt, kBool, kFloat, kStr, kFn, kUnit:
		cmp(t, "%a", "%b")
	}
	f.emit("ret i1 true")
	f.term = true
	f.startBlock(no)
	f.emit("ret i1 false")
	f.term = true
	g.writeFunc(name, "i1", "ptr %a, ptr %b", f)
	return q
}

// name is the Xo spelling of the type, for messages.
func (t *ty) name() string { return t.t.String() }
