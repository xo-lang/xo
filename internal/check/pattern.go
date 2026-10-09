package check

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/xo-lang/xo/internal/ast"
	"github.com/xo-lang/xo/internal/diag"
	"github.com/xo-lang/xo/internal/token"
)

type ipKind int

const (
	ipWild ipKind = iota
	ipCtor
	ipLit
	ipOr
)

// ipat is a typed pattern in the form used by the exhaustiveness check.
type ipat struct {
	kind ipKind
	key  int // constructor: variant index, 0/1 for false/true, 0 for tuples and structs
	args []*ipat
	lit  string
	alts []*ipat
	// lists: an exact length (rest false), or a prefix and suffix around `..`
	list   bool
	rest   bool
	prefix int
}

var wildPat = &ipat{kind: ipWild}

func wilds(n int) []*ipat {
	out := make([]*ipat, n)
	for i := range out {
		out[i] = wildPat
	}
	return out
}

// pattern types p against t, declares its bindings in the current scope,
// and returns its exhaustiveness form.
func (c *checker) pattern(p ast.Pattern, t Type, refutable bool) *ipat {
	if p != nil {
		c.rec.patType[p] = t
	}
	switch x := p.(type) {
	case nil, *ast.BadPat:
		return wildPat
	case *ast.WildcardPat:
		return wildPat
	case *ast.BindPat:
		v := &VarObj{Name: x.Name.Name, Type: t, Kind: VLocal, Span: x.Name.Sp, decl: p}
		c.declareVar(v)
		c.identObj[x.Name] = v
		return wildPat
	case *ast.LitPat:
		return c.litPattern(x.Value, t, x.Sp)
	case *ast.RangePat:
		lt := c.infer(x.Lo, t)
		c.check(x.Hi, lt)
		if !isBad(t) && !isBad(lt) {
			if !c.isNumeric(t) && !c.isPrelude(t, "Rune") {
				c.errorf(diag.PatternTypeMismatch, x.Sp, "a range pattern needs a number or Rune, but the value is `%s`", deep(t))
			} else if !c.tryUnify(lt, t) {
				c.errorf(diag.PatternTypeMismatch, x.Sp, "range of `%s` does not fit a value of type `%s`", deep(lt), deep(t))
			}
		}
		return &ipat{kind: ipLit, lit: "range"}
	case *ast.OrPat:
		return c.orPattern(x, t, refutable)
	case *ast.TuplePat:
		r := resolve(t)
		tt, ok := r.(*Tuple)
		if !ok || len(tt.Elems) != len(x.Elems) {
			if !isBad(r) {
				c.errorf(diag.PatternTypeMismatch, x.Sp, "tuple pattern with %d element(s) does not fit a value of type `%s`", len(x.Elems), deep(t))
			}
			for _, e := range x.Elems {
				c.pattern(e, badType, refutable)
			}
			return wildPat
		}
		ip := &ipat{kind: ipCtor}
		for i, e := range x.Elems {
			ip.args = append(ip.args, c.pattern(e, tt.Elems[i], refutable))
		}
		return ip
	case *ast.VariantPat:
		return c.variantPattern(x, t, refutable)
	case *ast.StructPat:
		return c.structPattern(x, t, refutable)
	case *ast.ListPat:
		r := resolve(t)
		var et Type = badType
		if n, ok := r.(*Named); ok && n.Obj.Module == c.prelude && n.Obj.Name == "List" {
			et = n.Args[0]
		} else if !isBad(r) {
			c.errorf(diag.PatternTypeMismatch, x.Sp, "list pattern does not fit a value of type `%s`", deep(t))
		}
		ip := &ipat{kind: ipCtor, list: true}
		for _, e := range x.Elems {
			ip.args = append(ip.args, c.pattern(e, et, refutable))
		}
		if x.Rest >= 0 {
			ip.rest = true
			ip.prefix = x.Rest
		}
		ip.key = len(x.Elems)
		if isBad(et) {
			return wildPat
		}
		return ip
	}
	return wildPat
}

func (c *checker) litPattern(v ast.Expr, t Type, sp token.Span) *ipat {
	if id, ok := v.(*ast.Ident); ok && (id.Name == "true" || id.Name == "false") {
		if !isBad(t) && !c.isPrelude(t, "Bool") {
			c.errorf(diag.PatternTypeMismatch, sp, "`%s` does not fit a value of type `%s`", id.Name, deep(t))
			return wildPat
		}
		k := 0
		if id.Name == "true" {
			k = 1
		}
		c.exprType[v] = c.named("Bool")
		return &ipat{kind: ipCtor, key: k}
	}
	lt := c.infer(v, t)
	if !isBad(t) && !isBad(lt) && !c.tryUnify(lt, t) {
		d := c.errorf(diag.PatternTypeMismatch, sp, "literal of type `%s` does not fit a value of type `%s`", deep(lt), deep(t))
		if c.optionElem(t) != nil {
			note(d, token.Span{}, "match an optional with `Some(...)` or `None`")
		}
		return wildPat
	}
	text := ""
	switch x := v.(type) {
	case *ast.IntLit:
		text = x.Value
	case *ast.StringLit:
		for _, p := range x.Parts {
			text += p.Text
		}
		text = strconv.Quote(text)
	case *ast.RuneLit:
		text = x.Raw
		if c.isInt(lt) {
			// the same arm as the integer literal of its code point
			text = strconv.Itoa(int(x.Value))
		}
	case *ast.FloatLit:
		text = x.Raw
	case *ast.UnaryExpr:
		if il, ok := x.X.(*ast.IntLit); ok {
			text = "-" + il.Value
		}
	}
	return &ipat{kind: ipLit, lit: text}
}

func (c *checker) orPattern(x *ast.OrPat, t Type, refutable bool) *ipat {
	ip := &ipat{kind: ipOr}
	before := len(c.sc.order)
	ip.alts = append(ip.alts, c.pattern(x.Alts[0], t, refutable))
	first := map[string]*VarObj{}
	for _, v := range c.sc.order[before:] {
		first[v.Name] = v
	}
	for _, alt := range x.Alts[1:] {
		c.pushScope()
		ip.alts = append(ip.alts, c.pattern(alt, t, refutable))
		got := map[string]*VarObj{}
		for _, v := range c.sc.order {
			got[v.Name] = v
			v.Used = true
		}
		c.popScope()
		var diff []string
		for n, v := range got {
			fv, ok := first[n]
			if !ok || !identical(fv.Type, v.Type) {
				diff = append(diff, n)
			}
		}
		for n := range first {
			if _, ok := got[n]; !ok {
				diff = append(diff, n)
			}
		}
		if len(diff) > 0 {
			sort.Strings(diff)
			c.errorf(diag.OrPatternBindings, alt.Span(), "every alternative of an or pattern must bind the same names with the same types; `%s` differs", strings.Join(diff, "`, `"))
		}
	}
	return ip
}

// resolveVariantPath finds the variant named by a pattern path.
func (c *checker) resolveVariantPath(x *ast.VariantPat, t Type) (*VariantObj, bool) {
	path := x.Path
	name := path[len(path)-1].Name
	r := resolve(t)
	n, _ := r.(*Named)
	var enum *TypeObj
	switch len(path) {
	case 1:
		if n != nil && n.Obj.Kind == KEnum {
			if v := n.Obj.variant(name); v != nil {
				return v, true
			}
		}
		if v := c.preludeVariant(name); v != nil {
			return v, true
		}
		if isBad(r) {
			return nil, false
		}
		if n != nil && n.Obj.Kind == KEnum {
			var cands []string
			for _, v := range n.Obj.Variants {
				cands = append(cands, v.Name)
			}
			c.unresolved(path[0].Sp, "variant of `"+n.Obj.QualName()+"`", name, cands)
			return nil, false
		}
		c.errorf(diag.PatternTypeMismatch, x.Sp, "variant pattern `%s` does not fit a value of type `%s`", name, deep(t))
		return nil, false
	case 2:
		o := c.lookupModuleObj(c.mod, path[0].Name)
		to, ok := o.(*TypeObj)
		if !ok {
			c.unresolved(path[0].Sp, "enum", path[0].Name, c.visibleTypeNames(c.localEnv()))
			return nil, false
		}
		enum = to
	case 3:
		imp := c.file.Imports[path[0].Name]
		if imp == nil {
			c.unresolved(path[0].Sp, "module", path[0].Name, importNames(c.file))
			return nil, false
		}
		imp.Used = true
		if imp.Module == nil {
			return nil, false
		}
		to, ok := imp.Module.Scope[path[1].Name].(*TypeObj)
		if !ok {
			c.unresolved(path[1].Sp, "enum of module `"+path[0].Name+"`", path[1].Name, nil)
			return nil, false
		}
		enum = to
	default:
		c.errorf(diag.PatternTypeMismatch, x.Sp, "pattern path has too many segments")
		return nil, false
	}
	if enum.Kind == KAlias {
		o2, _ := c.aliasNamed(enum, nil, x.Sp)
		if o2 != nil {
			enum = o2
		}
	}
	if enum.Kind != KEnum {
		c.errorf(diag.PatternTypeMismatch, x.Sp, "`%s` is not an enum", enum.Name)
		return nil, false
	}
	v := enum.variant(name)
	if v == nil {
		var cands []string
		for _, vv := range enum.Variants {
			cands = append(cands, vv.Name)
		}
		c.unresolved(path[len(path)-1].Sp, "variant of `"+enum.QualName()+"`", name, cands)
		return nil, false
	}
	return v, true
}

// newtypeOfPattern returns the newtype a pattern path `Email` or
// `mod.Email` names, unless the value is an enum with a variant of that
// name (variants win, as before newtype patterns existed).
func (c *checker) newtypeOfPattern(x *ast.VariantPat, t Type) *TypeObj {
	var o Object
	switch len(x.Path) {
	case 1:
		name := x.Path[0].Name
		if n, ok := resolve(t).(*Named); ok && n.Obj.Kind == KEnum && n.Obj.variant(name) != nil {
			return nil
		}
		if c.preludeVariant(name) != nil {
			return nil
		}
		o = c.lookupModuleObj(c.mod, name)
	case 2:
		imp := c.file.Imports[x.Path[0].Name]
		if imp == nil || imp.Module == nil {
			return nil
		}
		o = imp.Module.Scope[x.Path[1].Name]
		if o != nil {
			imp.Used = true
		}
	}
	to, ok := o.(*TypeObj)
	if !ok {
		return nil
	}
	if to.Kind == KAlias {
		if o2, _ := c.aliasNamed(to, nil, x.Sp); o2 != nil {
			to = o2
		}
	}
	if to.Kind != KNewtype {
		return nil
	}
	return to
}

// newtypePattern types `Email(p)`: p matches the newtype's underlying
// value (core 2.8, decision 0097). It is irrefutable when p is.
func (c *checker) newtypePattern(x *ast.VariantPat, obj *TypeObj, t Type, refutable bool) *ipat {
	c.rec.patNewtype[x] = obj
	r := resolve(t)
	n, isNamed := r.(*Named)
	var inner Type = badType
	if isNamed && n.Obj == obj {
		inner = subst(obj.Target, typeArgsSubst(obj.TParams, n.Args))
	} else if !isBad(r) {
		c.errorf(diag.PatternTypeMismatch, x.Sp, "newtype pattern `%s(..)` does not fit a value of type `%s`", obj.Name, deep(t))
	}
	if !x.Parens || len(x.Fields) > 1 || (len(x.Fields) == 1 && x.Fields[0].Name != nil) || (len(x.Fields) == 0 && !x.Rest) {
		d := c.errorf(diag.PatternTypeMismatch, x.Sp, "a newtype pattern has one positional pattern: `%s(p)`", obj.Name)
		if !x.Parens {
			fix(d, "match any value", insertEdit(x.Sp.File, x.Sp.End, "(_)"))
		}
		before := len(c.sc.order)
		for _, f := range x.Fields {
			c.pattern(f.Pattern, badType, refutable)
		}
		for _, v := range c.sc.order[before:] {
			v.Used = true // no unused binding warning on top of the error
		}
		return wildPat
	}
	sub := wildPat
	before := len(c.sc.order)
	if len(x.Fields) == 1 {
		sub = c.pattern(x.Fields[0].Pattern, inner, refutable)
	}
	if isBad(inner) {
		for _, v := range c.sc.order[before:] {
			v.Used = true
		}
		return wildPat
	}
	return &ipat{kind: ipCtor, key: 0, args: []*ipat{sub}}
}

func (c *checker) variantPattern(x *ast.VariantPat, t Type, refutable bool) *ipat {
	if nt := c.newtypeOfPattern(x, t); nt != nil {
		return c.newtypePattern(x, nt, t, refutable)
	}
	v, ok := c.resolveVariantPath(x, t)
	if ok {
		c.rec.patVariant[x] = v
	}
	typeFields := func(ft []Type) {
		for _, f := range x.Fields {
			var pt Type = badType
			_ = ft
			c.pattern(f.Pattern, pt, refutable)
		}
	}
	if !ok {
		typeFields(nil)
		return wildPat
	}
	r := resolve(t)
	n, isNamed := r.(*Named)
	if !isBad(r) && (!isNamed || n.Obj != v.Enum) {
		c.errorf(diag.PatternTypeMismatch, x.Sp, "`%s` is a variant of `%s`, but the value is `%s`", v.Name, v.Enum.QualName(), deep(t))
		typeFields(nil)
		return wildPat
	}
	var s *substMap
	typed := isNamed && n.Obj == v.Enum
	if typed {
		s = typeArgsSubst(v.Enum.TParams, n.Args)
	}
	fieldType := func(i int) Type {
		if !typed {
			return badType
		}
		return subst(v.Fields[i].Type, s)
	}
	ip := &ipat{kind: ipCtor, key: v.Index, args: wilds(len(v.Fields))}
	if !x.Parens {
		if len(v.Fields) > 0 {
			d := c.errorf(diag.PatternTypeMismatch, x.Sp, "variant `%s` has fields; write `%s(..)` to match any of them", v.Name, v.Name)
			fix(d, "match any fields", insertEdit(x.Sp.File, x.Sp.End, "(..)"))
		}
		return ip
	}
	if len(v.Fields) == 0 {
		if len(x.Fields) > 0 || !x.Rest {
			c.errorf(diag.PatternTypeMismatch, x.Sp, "variant `%s` has no fields; write it without parentheses", v.Name)
		}
		typeFields(nil)
		return ip
	}
	used := make([]bool, len(v.Fields))
	pos := 0
	for _, f := range x.Fields {
		idx := -1
		if f.Name == nil {
			for pos < len(v.Fields) && used[pos] {
				pos++
			}
			if pos >= len(v.Fields) {
				c.errorf(diag.PatternTypeMismatch, f.Sp, "too many fields in pattern: `%s` has %d", v.Name, len(v.Fields))
				c.pattern(f.Pattern, badType, refutable)
				continue
			}
			idx = pos
			pos++
		} else {
			for i, vf := range v.Fields {
				if vf.Name == f.Name.Name {
					idx = i
				}
			}
			if idx < 0 {
				var cands []string
				for _, vf := range v.Fields {
					cands = append(cands, vf.Name)
				}
				c.unresolved(f.Name.Sp, "field of `"+v.Name+"`", f.Name.Name, cands)
				c.pattern(f.Pattern, badType, refutable)
				continue
			}
			if used[idx] {
				c.errorf(diag.PatternTypeMismatch, f.Name.Sp, "field `%s` appears twice", f.Name.Name)
			}
		}
		used[idx] = true
		ip.args[idx] = c.pattern(f.Pattern, fieldType(idx), refutable)
	}
	if !x.Rest {
		var missing []string
		for i, u := range used {
			if !u {
				missing = append(missing, v.Fields[i].Name)
			}
		}
		if len(missing) > 0 {
			d := c.errorf(diag.PatternTypeMismatch, x.Sp, "pattern for `%s` does not mention field(s) `%s`; add them or `..`", v.Name, strings.Join(missing, "`, `"))
			end := x.Sp.End
			end.Col--
			end.Offset--
			fix(d, "ignore the other fields with ..", insertEdit(x.Sp.File, end, ", .."))
		}
	}
	return ip
}

func (c *checker) structPattern(x *ast.StructPat, t Type, refutable bool) *ipat {
	var obj *TypeObj
	switch len(x.Path) {
	case 1:
		obj, _ = c.lookupModuleObj(c.mod, x.Path[0].Name).(*TypeObj)
	case 2:
		if imp := c.file.Imports[x.Path[0].Name]; imp != nil && imp.Module != nil {
			imp.Used = true
			obj, _ = imp.Module.Scope[x.Path[1].Name].(*TypeObj)
		}
	}
	if obj != nil && obj.Kind == KAlias {
		obj, _ = c.aliasNamed(obj, nil, x.Sp)
	}
	r := resolve(t)
	n, isNamed := r.(*Named)
	if obj == nil || obj.Kind != KStruct {
		c.errorf(diag.PatternTypeMismatch, x.Sp, "`%s` is not a struct type", x.Path[len(x.Path)-1].Name)
		for _, f := range x.Fields {
			c.pattern(f.Pattern, badType, refutable)
		}
		return wildPat
	}
	if !isBad(r) && (!isNamed || n.Obj != obj) {
		c.errorf(diag.PatternTypeMismatch, x.Sp, "struct pattern `%s` does not fit a value of type `%s`", obj.Name, deep(t))
		for _, f := range x.Fields {
			c.pattern(f.Pattern, badType, refutable)
		}
		return wildPat
	}
	var s *substMap
	if isNamed {
		s = typeArgsSubst(obj.TParams, n.Args)
	}
	ip := &ipat{kind: ipCtor, args: wilds(len(obj.Fields))}
	used := map[string]bool{}
	for _, f := range x.Fields {
		name := ""
		if f.Name != nil {
			name = f.Name.Name
		} else if bp, ok := f.Pattern.(*ast.BindPat); ok {
			name = bp.Name.Name
		}
		idx := -1
		for i, fo := range obj.Fields {
			if fo.Name == name {
				idx = i
			}
		}
		if idx < 0 {
			var cands []string
			for _, fo := range obj.Fields {
				cands = append(cands, fo.Name)
			}
			sp := f.Sp
			if f.Name != nil {
				sp = f.Name.Sp
			}
			c.unresolved(sp, "field of `"+obj.Name+"`", name, cands)
			c.pattern(f.Pattern, badType, refutable)
			continue
		}
		used[name] = true
		if !obj.Fields[idx].Pub && obj.Module != c.mod {
			c.errorf(diag.NotVisible, f.Sp, "field `%s` of `%s` is not `pub`", name, obj.Name)
		}
		var ft Type = badType
		if s != nil || len(obj.TParams) == 0 {
			ft = subst(obj.Fields[idx].Type, s)
		}
		ip.args[idx] = c.pattern(f.Pattern, ft, refutable)
	}
	if !x.Rest {
		var missing []string
		for _, fo := range obj.Fields {
			if !used[fo.Name] {
				missing = append(missing, fo.Name)
			}
		}
		if len(missing) > 0 {
			c.errorf(diag.PatternTypeMismatch, x.Sp, "pattern for `%s` does not mention field(s) `%s`; add them or `..`", obj.Name, strings.Join(missing, "`, `"))
		}
	}
	return ip
}

// ---------------------------------------------------------------------------
// Exhaustiveness (core 2.3, 3.5): the usefulness algorithm with witnesses.

type ctorInfo struct {
	key      int
	arity    int
	argTypes []Type
	list     bool
}

// ctorsOf returns the constructors of t, or infinite for types whose values
// can only be covered by a wildcard (numbers, strings, ...). heads are the
// patterns in the column, used for list lengths.
func (c *checker) ctorsOf(t Type, heads []*ipat) ([]ctorInfo, bool) {
	switch x := resolve(t).(type) {
	case *Tuple:
		return []ctorInfo{{key: 0, arity: len(x.Elems), argTypes: x.Elems}}, false
	case *Named:
		if x.Obj.Module == c.prelude && x.Obj.Name == "Bool" {
			return []ctorInfo{{key: 0}, {key: 1}}, false
		}
		if x.Obj.Module == c.prelude && x.Obj.Name == "List" {
			maxLen := 0
			for _, h := range heads {
				if h.kind == ipCtor && h.list {
					maxLen = max(maxLen, len(h.args))
				}
			}
			var out []ctorInfo
			for n := 0; n <= maxLen+1; n++ {
				out = append(out, ctorInfo{key: n, arity: n, argTypes: repeatType(x.Args[0], n), list: true})
			}
			return out, false
		}
		s := typeArgsSubst(x.Obj.TParams, x.Args)
		switch x.Obj.Kind {
		case KEnum:
			var out []ctorInfo
			for _, v := range x.Obj.Variants {
				ci := ctorInfo{key: v.Index, arity: len(v.Fields)}
				uninhabited := false
				for _, f := range v.Fields {
					ft := subst(f.Type, s)
					if c.isNever(ft) {
						uninhabited = true
					}
					ci.argTypes = append(ci.argTypes, ft)
				}
				if !uninhabited {
					out = append(out, ci)
				}
			}
			return out, false
		case KNewtype:
			return []ctorInfo{{key: 0, arity: 1, argTypes: []Type{subst(x.Obj.Target, s)}}}, false
		case KStruct:
			if x.Obj.Opaque {
				return nil, true
			}
			ci := ctorInfo{key: 0, arity: len(x.Obj.Fields)}
			for _, f := range x.Obj.Fields {
				ci.argTypes = append(ci.argTypes, subst(f.Type, s))
			}
			return []ctorInfo{ci}, false
		}
	}
	return nil, true
}

func repeatType(t Type, n int) []Type {
	out := make([]Type, n)
	for i := range out {
		out[i] = t
	}
	return out
}

// expandOr replaces rows whose head is an or pattern by one row per
// alternative.
func expandOr(rows [][]*ipat) [][]*ipat {
	var out [][]*ipat
	for _, r := range rows {
		if len(r) > 0 && r[0].kind == ipOr {
			for _, a := range r[0].alts {
				nr := append([]*ipat{a}, r[1:]...)
				out = append(out, expandOr([][]*ipat{nr})...)
			}
			continue
		}
		out = append(out, r)
	}
	return out
}

// specialize keeps the rows whose head matches constructor ci and replaces
// the head by its arguments.
func specialize(rows [][]*ipat, ci ctorInfo) [][]*ipat {
	var out [][]*ipat
	for _, r := range rows {
		h := r[0]
		var args []*ipat
		switch {
		case h.kind == ipWild:
			args = wilds(ci.arity)
		case h.kind == ipCtor && h.list && ci.list:
			if !h.rest {
				if len(h.args) != ci.key {
					continue
				}
				args = h.args
			} else {
				if ci.key < len(h.args) {
					continue
				}
				args = append(args, h.args[:h.prefix]...)
				args = append(args, wilds(ci.key-len(h.args))...)
				args = append(args, h.args[h.prefix:]...)
			}
		case h.kind == ipCtor && !h.list && h.key == ci.key:
			args = h.args
		default:
			continue
		}
		nr := append(append([]*ipat{}, args...), r[1:]...)
		out = append(out, nr)
	}
	return out
}

func defaultRows(rows [][]*ipat) [][]*ipat {
	var out [][]*ipat
	for _, r := range rows {
		if r[0].kind == ipWild {
			out = append(out, r[1:])
		}
	}
	return out
}

// witness is a missing value pattern, printable.
type witness struct {
	ctor *ctorInfo
	t    Type
	args []*witness
}

// missing returns a vector of patterns not covered by rows, or nil when rows
// are exhaustive.
func (c *checker) missing(rows [][]*ipat, tys []Type, depth int) []*witness {
	if len(tys) == 0 {
		if len(rows) == 0 {
			return []*witness{}
		}
		return nil
	}
	if depth > 40 {
		return nil
	}
	rows = expandOr(rows)
	var heads []*ipat
	for _, r := range rows {
		heads = append(heads, r[0])
	}
	ctors, infinite := c.ctorsOf(tys[0], heads)
	if !infinite {
		used := map[int]bool{}
		listHasRest := false
		for _, h := range heads {
			if h.kind == ipCtor {
				if h.list && h.rest {
					listHasRest = true
				}
				used[h.key] = true
			}
		}
		complete := true
		for _, ci := range ctors {
			if !used[ci.key] && !(ci.list && listHasRest) {
				complete = false
			}
		}
		if complete {
			for i := range ctors {
				ci := ctors[i]
				sub := specialize(rows, ci)
				w := c.missing(sub, append(append([]Type{}, ci.argTypes...), tys[1:]...), depth+1)
				if w != nil {
					head := &witness{ctor: &ctors[i], t: tys[0], args: w[:ci.arity]}
					return append([]*witness{head}, w[ci.arity:]...)
				}
			}
			return nil
		}
	}
	w := c.missing(defaultRows(rows), tys[1:], depth+1)
	if w == nil {
		return nil
	}
	head := &witness{t: tys[0]}
	if !infinite {
		used := map[int]bool{}
		for _, h := range heads {
			if h.kind == ipCtor {
				used[h.key] = true
			}
		}
		for i := range ctors {
			if !used[ctors[i].key] {
				ci := ctors[i]
				head = &witness{ctor: &ctors[i], t: tys[0]}
				for j := 0; j < ci.arity; j++ {
					head.args = append(head.args, &witness{t: ci.argTypes[j]})
				}
				break
			}
		}
	}
	return append([]*witness{head}, w...)
}

func (c *checker) witnessString(w *witness) string {
	if w.ctor == nil {
		return "_"
	}
	allWild := true
	for _, a := range w.args {
		if a.ctor != nil {
			allWild = false
		}
	}
	switch x := resolve(w.t).(type) {
	case *Tuple:
		var parts []string
		for _, a := range w.args {
			parts = append(parts, c.witnessString(a))
		}
		return "(" + strings.Join(parts, ", ") + ")"
	case *Named:
		if x.Obj.Module == c.prelude && x.Obj.Name == "Bool" {
			if w.ctor.key == 1 {
				return "true"
			}
			return "false"
		}
		if w.ctor.list {
			var parts []string
			for _, a := range w.args {
				parts = append(parts, c.witnessString(a))
			}
			if w.ctor.key > 0 {
				// the last constructor stands for every longer list
				return "[" + strings.Join(parts, ", ") + "]"
			}
			return "[]"
		}
		if x.Obj.Kind == KEnum {
			var v *VariantObj
			for _, vv := range x.Obj.Variants {
				if vv.Index == w.ctor.key {
					v = vv
				}
			}
			if v == nil {
				return "_"
			}
			if len(v.Fields) == 0 {
				return v.Name
			}
			if allWild {
				if len(v.Fields) == 1 {
					return v.Name + "(_)"
				}
				return v.Name + "(..)"
			}
			var parts []string
			for _, a := range w.args {
				parts = append(parts, c.witnessString(a))
			}
			return v.Name + "(" + strings.Join(parts, ", ") + ")"
		}
		if x.Obj.Kind == KNewtype && len(w.args) == 1 {
			return x.Obj.Name + "(" + c.witnessString(w.args[0]) + ")"
		}
		if x.Obj.Kind == KStruct {
			if allWild {
				return x.Obj.Name + "{..}"
			}
			var parts []string
			for i, a := range w.args {
				parts = append(parts, x.Obj.Fields[i].Name+": "+c.witnessString(a))
			}
			return x.Obj.Name + "{" + strings.Join(parts, ", ") + "}"
		}
	}
	return "_"
}

// checkExhaustive reports XO0301 with the missing patterns.
func (c *checker) checkExhaustive(m *ast.MatchExpr, rows []*ipat, st Type) {
	if containsBad(st) || hasOpenVars(st) {
		return
	}
	var matrix [][]*ipat
	for _, r := range rows {
		matrix = append(matrix, []*ipat{r})
	}
	var missingPats []string
	for i := 0; i < 5; i++ {
		w := c.missing(matrix, []Type{st}, 0)
		if w == nil {
			break
		}
		s := c.witnessString(w[0])
		missingPats = append(missingPats, s)
		if s == "_" || strings.HasPrefix(s, "[") {
			break
		}
		matrix = append(matrix, []*ipat{c.witnessPat(w[0])})
	}
	if len(missingPats) == 0 {
		return
	}
	d := c.errorf(diag.NonExhaustiveMatch, m.Subject.Span(), "match on `%s` is not exhaustive: missing `%s`", deep(st), strings.Join(missingPats, "`, `"))
	if d == nil {
		return
	}
	note(d, token.Span{}, "add an arm for each missing case; matches must be exhaustive (core 2.3)")
	end := m.Sp.End
	end.Col--
	end.Offset--
	indent := strings.Repeat(" ", max(0, end.Col-1))
	var b strings.Builder
	for _, p := range missingPats {
		fmt.Fprintf(&b, "  %s => ???\n%s", p, indent)
	}
	fix(d, "add the missing arms as holes", insertEdit(m.Sp.File, end, b.String()))
}

// witnessPat converts a witness back into a pattern row (to find further
// missing cases).
func (c *checker) witnessPat(w *witness) *ipat {
	if w.ctor == nil {
		return wildPat
	}
	ip := &ipat{kind: ipCtor, key: w.ctor.key, list: w.ctor.list}
	for _, a := range w.args {
		ip.args = append(ip.args, c.witnessPat(a))
	}
	return ip
}

// requireIrrefutable reports XO0303 when ip does not cover every value of t.
func (c *checker) requireIrrefutable(ip *ipat, t Type, p ast.Pattern, what string) {
	if containsBad(t) || hasOpenVars(t) || p == nil {
		return
	}
	w := c.missing([][]*ipat{{ip}}, []Type{t}, 0)
	if w == nil {
		return
	}
	d := c.errorf(diag.RefutablePattern, p.Span(), "%s must match every value of `%s`, but `%s` is not covered", what, deep(t), c.witnessString(w[0]))
	note(d, token.Span{}, "use `if let` or `match` for patterns that can fail (core 3.5)")
}

// warnWildcardOwnEnum warns (XO0305) when an unguarded `_` or catch all
// binding arm hides variants of an enum declared in the current module:
// adding a variant later would silently take that arm (core 3.5). Enums from
// other modules may use `_`, since their owners can add variants.
func (c *checker) warnWildcardOwnEnum(x *ast.MatchExpr, st Type) {
	n, ok := resolve(st).(*Named)
	if !ok || c.mod == nil || n.Obj.Module != c.mod || len(n.Obj.Variants) == 0 {
		return
	}
	named := map[string]bool{}
	var mention func(p ast.Pattern)
	mention = func(p ast.Pattern) {
		switch q := p.(type) {
		case *ast.VariantPat:
			if len(q.Path) > 0 {
				named[q.Path[len(q.Path)-1].Name] = true
			}
		case *ast.OrPat:
			for _, a := range q.Alts {
				mention(a)
			}
		}
	}
	for _, arm := range x.Arms {
		catchAll := false
		switch arm.Pattern.(type) {
		case *ast.WildcardPat, *ast.BindPat:
			catchAll = arm.Guard == nil
		}
		if !catchAll {
			mention(arm.Pattern)
			continue
		}
		var hidden []string
		for _, v := range n.Obj.Variants {
			if !named[v.Name] {
				hidden = append(hidden, v.Name)
			}
		}
		if len(hidden) == 0 || defaultArm(len(hidden), len(n.Obj.Variants)) {
			return
		}
		shown := hidden
		more := ""
		if len(shown) > wildcardListMax {
			shown = shown[:wildcardListMax]
			more = fmt.Sprintf(" and %d more", len(hidden)-wildcardListMax)
		}
		d := c.warnf(diag.WildcardOwnEnum, arm.Pattern.Span(), "this arm hides `%s`%s of `%s`, declared in this module", strings.Join(shown, "`, `"), more, n.Obj.Name)
		if d != nil {
			note(d, token.Span{}, "list the variants so that adding one later is a compile error (core 3.5)")
		}
		return
	}
}

// wildcardDefaultMin is the number of hidden variants from which a `_` arm
// that hides most of an enum is a default rather than a shortcut, and
// wildcardListMax the most variant names XO0305 prints (decision 0080).
const (
	wildcardDefaultMin = 8
	wildcardListMax    = 8
)

// defaultArm reports whether a `_` arm hiding hidden of total variants is
// a deliberate default ("these few are special, the rest are not"), which
// XO0305 does not report: it hides at least wildcardDefaultMin variants and
// more than half of them, so listing them would be the noise (decision
// 0080).
func defaultArm(hidden, total int) bool {
	return hidden >= wildcardDefaultMin && 2*hidden > total
}
