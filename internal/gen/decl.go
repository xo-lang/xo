package gen

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/xo-lang/xo/internal/ast"
	"github.com/xo-lang/xo/internal/check"
)

func (pg *pkgGen) p(format string, args ...any) {
	fmt.Fprintf(&pg.out, format, args...)
	pg.out.WriteByte('\n')
}

// lineDirective returns a //line directive for a position.
func (g *gen) lineDirective(n ast.Node) string {
	sp := n.Span()
	return fmt.Sprintf("//line %s:%d:%d", g.linePath(sp.File), sp.Start.Line, sp.Start.Col)
}

// loc is the `file:line:col` text of a node, used in fault messages.
func (g *gen) loc(n ast.Node) string {
	sp := n.Span()
	return fmt.Sprintf("%s:%d:%d", g.linePath(sp.File), sp.Start.Line, sp.Start.Col)
}

func (g *gen) includeFile(f *check.File) bool {
	if f.IsTest {
		return g.opts.Mode == Test && f.Module.Root
	}
	return true
}

// genModule generates every declaration of the module.
func (pg *pkgGen) genModule() {
	m := pg.m
	for _, f := range m.Files {
		if !pg.g.includeFile(f) {
			continue
		}
		pg.cur = f
		for _, d := range f.AST.Decls {
			switch x := d.(type) {
			case *ast.StructDecl:
				if t, ok := m.Scope[x.Name.Name].(*check.TypeObj); ok && t.Decl() == ast.Node(x) {
					pg.genStruct(t, x)
				}
			case *ast.EnumDecl:
				if t, ok := m.Scope[x.Name.Name].(*check.TypeObj); ok && t.Decl() == ast.Node(x) {
					pg.genEnum(t, x)
				}
			case *ast.InterfaceDecl:
				if t, ok := m.Scope[x.Name.Name].(*check.TypeObj); ok && t.Decl() == ast.Node(x) {
					pg.genInterface(t)
				}
			case *ast.TypeDecl:
				if t, ok := m.Scope[x.Name.Name].(*check.TypeObj); ok && t.Decl() == ast.Node(x) && t.Kind == check.KNewtype {
					pg.genNewtype(t, x)
				}
			case *ast.FnDecl:
				fo := pg.g.info.FuncOf(m, x)
				if fo != nil && x.Body != nil {
					pg.genFunc(fo)
					if pg.g.opts.Mode == Test && m.Root && !f.IsTest {
						pg.genFuzz(fo)
					}
				}
			case *ast.ConstDecl:
				if co, ok := m.Scope[x.Name.Name].(*check.ConstObj); ok && co.Decl == x {
					pg.genConst(co)
				}
			case *ast.TestDecl:
				if pg.g.opts.Mode == Test && m.Root {
					pg.genTest(x)
				}
			}
		}
	}
	if pg.g.opts.Mode == Test && m.Root {
		pg.genTestTable()
	}
}

// tparamDecl is `[tp_T any, tp_U any]` for the non effect parameters.
func tparamDecl(ps []*check.TParam) string {
	ps = typeParams(ps)
	if len(ps) == 0 {
		return ""
	}
	parts := make([]string, len(ps))
	for i, p := range ps {
		parts[i] = goTParam(p) + " any"
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

// tparamUse is `[tp_T, tp_U]`.
func tparamUse(ps []*check.TParam) string {
	ps = typeParams(ps)
	if len(ps) == 0 {
		return ""
	}
	parts := make([]string, len(ps))
	for i, p := range ps {
		parts[i] = goTParam(p)
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

// selfType is the Go type of a declared type inside its own package:
// `Name[tp_T]`.
func selfType(t *check.TypeObj) string { return localTypeName(t) + tparamUse(t.TParams) }

// localTypeName is the Go name of a type in its own package: std types
// in package rt carry their module prefix (`Http_Request`).
func localTypeName(t *check.TypeObj) string {
	if t.Module.IsStd {
		return stdPrefix(t.Module) + t.Name
	}
	return t.Name
}

func (pg *pkgGen) genStruct(t *check.TypeObj, d *ast.StructDecl) {
	pg.p("%s", pg.g.lineDirective(d))
	pg.p("type %s%s struct {", localTypeName(t), tparamDecl(t.TParams))
	for _, f := range t.Fields {
		if pg.g.isBoxedField(t, f) {
			pg.p("\t%s %sBox[%s]", goField(f.Name), pg.rt(), pg.goType(f.Type))
			continue
		}
		pg.p("\t%s %s", goField(f.Name), pg.goType(f.Type))
	}
	if pg.inRT {
		for _, x := range rtExtraFields[localTypeName(t)] {
			pg.p("\t%s", x)
		}
	}
	pg.p("}\n")
	self := selfType(t)
	// field defaults
	for _, f := range t.Fields {
		if f.Default == nil {
			continue
		}
		fg := pg.newFn(nil)
		fg.fr = &frame{name: t.Name, resT: f.Type}
		code := fg.val(f.Default)
		pg.p("func %s_D_%s%s(tk *%sTask) %s {", localTypeName(t), f.Name, tparamDecl(t.TParams), pg.rt(), pg.goType(f.Type))
		pg.out.WriteString(fg.out.String())
		pg.p("\treturn %s\n}\n", code)
	}
	pg.genDerived(t, self, func(recv string) []fieldRef {
		var fs []fieldRef
		for _, f := range t.Fields {
			fs = append(fs, fieldRef{name: f.Name, code: recv + "." + goField(f.Name)})
		}
		return fs
	})
	if t.Invariant != nil {
		pg.genInvariant(t)
	}
	if t.Derives["Json"] && !rtHandWritten[localTypeName(t)+".json"] {
		pg.genJSONStruct(t)
	}
	if pg.g.opts.Mode == Test && !pg.inRT {
		pg.genGenStruct(t)
	}
}

type fieldRef struct {
	name string
	code string
}

// genDerived emits XoEq, XoHash, XoCmp, XoDebug, M_debug, and M_display
// for a struct or newtype.
func (pg *pkgGen) genDerived(t *check.TypeObj, self string, fields func(recv string) []fieldRef) {
	rt := pg.rt()
	pg.imports["strings"] = "strings"
	fs := fields("v")
	os := fields("o")
	pg.p("func (v %s) XoEq(x any) bool {", self)
	pg.p("\to, ok := x.(%s)", self)
	pg.p("\tif !ok {\n\t\treturn false\n\t}")
	for i := range fs {
		pg.p("\tif !%sEq(%s, %s) {\n\t\treturn false\n\t}", rt, fs[i].code, os[i].code)
	}
	pg.p("\t_ = o\n\treturn true\n}\n")
	pg.p("func (v %s) XoHash() uint64 {", self)
	pg.p("\th := uint64(%d)", hashSeedOf(t.Name))
	for _, f := range fs {
		pg.p("\th = %sMixHash(h, %sHash(%s))", rt, rt, f.code)
	}
	pg.p("\treturn h\n}\n")
	if t.Derives["Ord"] {
		pg.p("func (v %s) XoCmp(x any) int {", self)
		pg.p("\to := x.(%s)", self)
		for i := range fs {
			pg.p("\tif c := %sCmp(%s, %s); c != 0 {\n\t\treturn c\n\t}", rt, fs[i].code, os[i].code)
		}
		pg.p("\t_ = o\n\treturn 0\n}\n")
	}
	pg.p("func (v %s) XoDebug(b *strings.Builder) {", self)
	if t.Kind == check.KNewtype {
		pg.p("\tb.WriteString(%q)", t.Name+"(")
		pg.p("\t%sDebugTo(b, v.F0)", rt)
		pg.p("\tb.WriteString(\")\")")
	} else {
		pg.p("\tb.WriteString(%q)", t.Name+"{")
		for i, f := range fs {
			sep := ""
			if i > 0 {
				sep = ", "
			}
			pg.p("\tb.WriteString(%q)", sep+f.name+": ")
			pg.p("\t%sDebugTo(b, %s)", rt, f.code)
		}
		pg.p("\tb.WriteString(\"}\")")
	}
	pg.p("}\n")
	pg.genIfaceMethods(t, self)
	pg.genTypedEq(t, self, fields)
}

// genIfaceMethods emits M_debug unless declared, and M_display for
// `derive Display`.
func (pg *pkgGen) genIfaceMethods(t *check.TypeObj, self string) {
	rt := pg.rt()
	if t.Methods["debug"] == nil {
		pg.p("func (v %s) M_debug(*%sTask) string { return %sDebug(v) }\n", self, rt, rt)
	}
	if t.Derives["Display"] && t.Methods["display"] == nil && !rtHandWritten[localTypeName(t)+".M_display"] {
		pg.p("func (v %s) M_display(*%sTask) string { return %sDebug(v) }\n", self, rt, rt)
	}
}

func hashSeedOf(name string) uint64 {
	h := uint64(1469598103934665603)
	for i := 0; i < len(name); i++ {
		h ^= uint64(name[i])
		h *= 1099511628211
	}
	return h >> 1
}

func (pg *pkgGen) genNewtype(t *check.TypeObj, d *ast.TypeDecl) {
	pg.p("%s", pg.g.lineDirective(d))
	pg.p("type %s%s struct {\n\tF0 %s\n}\n", localTypeName(t), tparamDecl(t.TParams), pg.goType(t.Target))
	pg.genDerived(t, selfType(t), func(recv string) []fieldRef {
		return []fieldRef{{name: "0", code: recv + ".F0"}}
	})
	if t.Derives["Json"] {
		pg.genJSONNewtype(t)
	}
	if pg.g.opts.Mode == Test && !pg.inRT {
		pg.genGenNewtype(t)
	}
}

// variantType is the Go payload type name of a variant, unqualified.
func variantType(v *check.VariantObj) string { return v.Enum.Name + "_" + v.Name }

// localVariantType is the payload type name in the enum's own package.
func localVariantType(v *check.VariantObj) string { return localTypeName(v.Enum) + "_" + v.Name }

// vfieldName is the Go field of a variant field: F_<name>, or F0 for a
// `from` field.
func vfieldName(f *check.VFieldObj, i int) string {
	if f.From || f.Name == "" {
		return "F" + strconv.Itoa(i)
	}
	return goField(f.Name)
}

func (pg *pkgGen) genEnum(t *check.TypeObj, d *ast.EnumDecl) {
	rt := pg.rt()
	pg.imports["strings"] = "strings"
	pg.p("%s", pg.g.lineDirective(d))
	pg.p("type %s%s struct {\n\tTag int\n\tP any\n}\n", localTypeName(t), tparamDecl(t.TParams))
	self := selfType(t)
	for _, v := range t.Variants {
		if len(v.Fields) == 0 {
			continue
		}
		pg.p("type %s%s struct {", localVariantType(v), tparamDecl(t.TParams))
		for i, f := range v.Fields {
			pg.p("\t%s %s", vfieldName(f, i), pg.goType(f.Type))
		}
		pg.p("}\n")
	}
	// Eq
	pg.p("func (v %s) XoEq(x any) bool {", self)
	pg.p("\to, ok := x.(%s)", self)
	pg.p("\tif !ok || o.Tag != v.Tag {\n\t\treturn false\n\t}")
	pg.p("\tswitch v.Tag {")
	for _, v := range t.Variants {
		if len(v.Fields) == 0 {
			continue
		}
		pt := localVariantType(v) + tparamUse(t.TParams)
		pg.p("\tcase %d:", v.Index+1)
		pg.p("\t\ta, b := v.P.(%s), o.P.(%s)", pt, pt)
		var conds []string
		for i, f := range v.Fields {
			fn := vfieldName(f, i)
			conds = append(conds, fmt.Sprintf("%sEq(a.%s, b.%s)", rt, fn, fn))
		}
		pg.p("\t\treturn %s", strings.Join(conds, " && "))
	}
	pg.p("\t}\n\treturn true\n}\n")
	// Hash
	pg.p("func (v %s) XoHash() uint64 {", self)
	pg.p("\th := %sMixHash(uint64(%d), uint64(v.Tag))", rt, hashSeedOf(t.Name))
	pg.p("\tswitch v.Tag {")
	for _, v := range t.Variants {
		if len(v.Fields) == 0 {
			continue
		}
		pg.p("\tcase %d:", v.Index+1)
		pg.p("\t\ta := v.P.(%s%s)", localVariantType(v), tparamUse(t.TParams))
		for i, f := range v.Fields {
			pg.p("\t\th = %sMixHash(h, %sHash(a.%s))", rt, rt, vfieldName(f, i))
		}
	}
	pg.p("\t}\n\treturn h\n}\n")
	// Ord: variant order, then fields
	if t.Derives["Ord"] {
		pg.p("func (v %s) XoCmp(x any) int {", self)
		pg.p("\to := x.(%s)", self)
		pg.p("\tif v.Tag != o.Tag {\n\t\tif v.Tag < o.Tag {\n\t\t\treturn -1\n\t\t}\n\t\treturn 1\n\t}")
		pg.p("\tswitch v.Tag {")
		for _, v := range t.Variants {
			if len(v.Fields) == 0 {
				continue
			}
			pt := localVariantType(v) + tparamUse(t.TParams)
			pg.p("\tcase %d:", v.Index+1)
			pg.p("\t\ta, b := v.P.(%s), o.P.(%s)", pt, pt)
			for i, f := range v.Fields {
				fn := vfieldName(f, i)
				pg.p("\t\tif c := %sCmp(a.%s, b.%s); c != 0 {\n\t\t\treturn c\n\t\t}", rt, fn, fn)
			}
		}
		pg.p("\t}\n\treturn 0\n}\n")
	}
	// Debug
	pg.p("func (v %s) XoDebug(b *strings.Builder) {", self)
	pg.p("\tswitch v.Tag {")
	for _, v := range t.Variants {
		pg.p("\tcase %d:", v.Index+1)
		if len(v.Fields) == 0 {
			pg.p("\t\tb.WriteString(%q)", v.Name)
			continue
		}
		pg.p("\t\ta := v.P.(%s%s)", localVariantType(v), tparamUse(t.TParams))
		pg.p("\t\tb.WriteString(%q)", v.Name+"(")
		for i, f := range v.Fields {
			sep := ""
			if i > 0 {
				sep = ", "
			}
			if !f.From && f.Name != "" {
				sep += f.Name + ": "
			}
			if sep != "" {
				pg.p("\t\tb.WriteString(%q)", sep)
			}
			pg.p("\t\t%sDebugTo(b, a.%s)", rt, vfieldName(f, i))
		}
		pg.p("\t\tb.WriteString(\")\")")
	}
	pg.p("\t}\n}\n")
	pg.genIfaceMethods(t, self)
	if t.Derives["Json"] && !rtHandWritten[localTypeName(t)+".json"] {
		pg.genJSONEnum(t)
	}
	if pg.g.opts.Mode == Test && !pg.inRT {
		pg.genGenEnum(t)
	}
}

func (pg *pkgGen) genInterface(t *check.TypeObj) {
	if d := t.Decl(); d != nil {
		pg.p("%s", pg.g.lineDirective(d))
	}
	pg.p("type %s%s interface {", localTypeName(t), tparamDecl(t.TParams))
	for _, m := range t.IfaceMeths {
		if len(typeParams(ownTParams(m))) > 0 {
			continue // generic interface methods are not Go interface methods
		}
		pg.p("\t%s%s", goMethod(m.Name), pg.sigTail(m))
	}
	pg.p("}\n")
}

// ownTParams returns the declared (not implicit receiver) type parameters
// of a function.
func ownTParams(fo *check.FuncObj) []*check.TParam {
	var out []*check.TParam
	for _, p := range fo.TParams {
		if !p.Implicit {
			out = append(out, p)
		}
	}
	return out
}

// returnsResult reports whether the Go form of a declared function returns
// rt.Result: it declares an error type other than Never.
func returnsResult(fo *check.FuncObj) bool {
	return fo.Err != nil && !check.IsNever(fo.Err)
}

// returnsVoid reports whether the Go form returns nothing: `-> ()` that
// cannot fail.
func returnsVoid(fo *check.FuncObj) bool {
	return !returnsResult(fo) && check.IsUnit(fo.Result)
}

// goResult is the Go result type of a declared function ("" for void).
func (pg *pkgGen) goResult(fo *check.FuncObj) string {
	switch {
	case returnsResult(fo):
		return pg.resultOf(fo.Result, fo.Err)
	case returnsVoid(fo):
		return ""
	}
	return pg.goType(fo.Result)
}

// sigTail is `(tk *rt.Task, p0 T0, ...) R` for a declared function,
// parameters named p0, p1, ...
func (pg *pkgGen) sigTail(fo *check.FuncObj) string {
	var b strings.Builder
	b.WriteString("(tk *" + pg.rt() + "Task")
	for i, p := range fo.Params {
		fmt.Fprintf(&b, ", p%d ", i)
		if p.Var {
			b.WriteByte('*')
		}
		b.WriteString(pg.goType(p.Type))
	}
	b.WriteString(")")
	if r := pg.goResult(fo); r != "" {
		b.WriteString(" " + r)
	}
	return b.String()
}

// methodStyle tells how a method is lowered: as a Go method on the
// receiver type, or as a top level function `<Type>_M_<name>` (user) /
// `<Type>_<name>` (std) taking the receiver first.
func methodIsGoMethod(fo *check.FuncObj) bool {
	if fo.Iface {
		return len(typeParams(ownTParams(fo))) == 0
	}
	if fo.Static || fo.Recv == nil {
		return false
	}
	if len(typeParams(ownTParams(fo))) > 0 {
		return false
	}
	if fo.Module.IsStd && fo.Recv.Module.IsPrelude {
		if _, scalar := scalarGo[fo.Recv.Name]; scalar {
			return false
		}
		switch fo.Recv.Name {
		case "Rune", "Bytes", "Duration", "Time":
			return false
		}
	}
	if growsSelf(fo) {
		return false
	}
	// the receiver's type arguments must be exactly its parameters
	rn, ok := fo.RecvType.(*check.Named)
	if !ok {
		return true
	}
	seen := map[*check.TParam]bool{}
	for _, a := range typeArgs(rn) {
		pt, ok := a.(*check.ParamType)
		if !ok || seen[pt.P] {
			return false
		}
		seen[pt.P] = true
	}
	return true
}

// growsSelf reports whether a method's signature instantiates its own
// receiver type (or List, whose methods could) with arguments other than
// the receiver's parameters: as a Go method that is an instantiation
// cycle (`List[T].indexed() -> List[(Int, T)]`).
func growsSelf(fo *check.FuncObj) bool {
	rn, ok := fo.RecvType.(*check.Named)
	if !ok {
		return false
	}
	recvArgs := typeArgs(rn)
	var grows func(t check.Type) bool
	grows = func(t check.Type) bool {
		switch x := check.Resolve(t).(type) {
		case *check.Named:
			if x.Obj == fo.Recv {
				args := typeArgs(x)
				if len(args) != len(recvArgs) {
					return true
				}
				for i := range args {
					a, ok1 := check.Resolve(args[i]).(*check.ParamType)
					b, ok2 := check.Resolve(recvArgs[i]).(*check.ParamType)
					if !ok1 || !ok2 || a.P != b.P {
						return true
					}
				}
			}
			for _, a := range x.Args {
				if grows(a) {
					return true
				}
			}
		case *check.Tuple:
			for _, e := range x.Elems {
				if grows(e) {
					return true
				}
			}
		case *check.Func:
			for _, p := range x.Params {
				if grows(p.T) {
					return true
				}
			}
			return grows(x.Result) || x.Err != nil && grows(x.Err)
		}
		return false
	}
	for _, p := range fo.Params {
		if grows(p.Type) {
			return true
		}
	}
	if grows(fo.Result) {
		return true
	}
	if fo.Module.IsStd && fo.Recv.Module.IsPrelude && (fo.Recv.Name == "Map" && fo.Name == "entries") {
		return true
	}
	return fo.Err != nil && grows(fo.Err)
}

// recvTParams returns the type parameters bound by a method receiver, in
// the order of the receiver type's arguments.
func recvTParams(fo *check.FuncObj) []*check.TParam {
	rn, ok := fo.RecvType.(*check.Named)
	if !ok {
		return nil
	}
	var out []*check.TParam
	for _, a := range typeArgs(rn) {
		if pt, ok := a.(*check.ParamType); ok {
			out = append(out, pt.P)
		}
	}
	return out
}

// funcGoName is the Go name of a declared user function or method, as
// declared (unqualified).
func funcGoName(fo *check.FuncObj) string {
	if fo.Recv == nil {
		return "F_" + fo.Name
	}
	if methodIsGoMethod(fo) {
		return goMethod(fo.Name)
	}
	return fo.Recv.Name + "_M_" + fo.Name
}

// genFunc generates a declared function or method.
func (pg *pkgGen) genFunc(fo *check.FuncObj) { pg.genFuncWith(fo, nil) }

// genFuncWith generates a declared function or method, or with sp a
// specialized copy of a generic one (mono.go): a top level function named
// sp.name with the receiver, if any, as its first parameter and the type
// parameters substituted.
func (pg *pkgGen) genFuncWith(fo *check.FuncObj, sp *specReq) {
	if sp != nil {
		savedSub, savedDepth := pg.ms.sub, pg.ms.depth
		pg.ms.sub, pg.ms.depth = sp.sub, sp.depth
		defer func() { pg.ms.sub, pg.ms.depth = savedSub, savedDepth }()
	}
	fd := fo.Decl
	fg := pg.newFn(fo)
	fr := &frame{name: pg.frameName(fo), resT: fo.Result, errT: fo.Err, fo: fo}
	if fo.Err != nil && check.IsNever(fo.Err) {
		fr.errT = nil
	}
	fr.goRes = pg.goResult(fo)
	fr.isResult = returnsResult(fo)
	fg.fr = fr
	var params []string
	// receiver
	recv := ""
	info := pg.g.info
	if rv := info.RecvVar(fo); rv != nil {
		name := fg.declName(rv)
		rtype := pg.goType(fo.RecvType)
		if fo.RecvVar {
			fg.names[rv] = "(*" + name + ")"
			rtype = "*" + rtype
		}
		fr.params = append(fr.params, paramRef{xo: rv.Name, code: fg.names[rv], t: fo.RecvType})
		if methodIsGoMethod(fo) && sp == nil {
			recv = fmt.Sprintf("(%s %s) ", name, rtype)
		} else {
			params = append(params, name+" "+rtype)
		}
	}
	for i, pv := range info.ParamVars(fo) {
		if sp != nil && sp.lits[i] != nil {
			if fg.litParams == nil {
				fg.litParams = map[*check.VarObj]*ast.FnLit{}
			}
			fg.litParams[pv] = sp.lits[i]
		}
		name := fg.declName(pv)
		pt := pg.goType(fo.Params[i].Type)
		if fo.Params[i].Var {
			fg.names[pv] = "(*" + name + ")"
			pt = "*" + pt
		}
		params = append(params, name+" "+pt)
		fr.params = append(fr.params, paramRef{xo: pv.Name, code: fg.names[pv], t: fo.Params[i].Type})
	}
	tps := fo.TParams
	if recv != "" || sp != nil {
		// a Go method gets its type parameters from the receiver; a copy
		// has none
		tps = nil
	}
	name := funcGoName(fo)
	if sp != nil {
		name = sp.name
	}
	// receiver type parameter names come from the receiver type pattern
	recvText := recv
	if recv != "" {
		rtps := recvTParams(fo)
		recvText = strings.Replace(recv, pg.goType(fo.RecvType), fo.Recv.Name+tparamUse(rtps), 1)
	}
	var fixed, byRef []*check.VarObj
	if rv := info.RecvVar(fo); rv != nil {
		if fo.RecvVar {
			byRef = append(byRef, rv)
		} else {
			fixed = append(fixed, rv)
		}
	}
	for i, pv := range info.ParamVars(fo) {
		if fo.Params[i].Var {
			byRef = append(byRef, pv)
		} else {
			fixed = append(fixed, pv)
		}
	}
	fg.analyzeRanges(fd.Body, fixed, byRef)
	body := fg.funcBody(fd)
	sig := fmt.Sprintf("func %s%s%s(tk *%sTask", recvText, name, tparamDecl(tps), pg.rt())
	for _, p := range params {
		sig += ", " + p
	}
	sig += ")"
	if fr.goRes != "" {
		sig += " " + fr.goRes
	}
	pg.p("%s", pg.g.lineDirective(fd))
	pg.p("%s {", sig)
	pg.out.WriteString(body)
	pg.p("}\n")
	if sp != nil {
		return // a copy calls the generic default functions
	}
	// parameter defaults
	for i, p := range fo.Params {
		if p.Default == nil {
			continue
		}
		dg := pg.newFn(nil)
		dg.fr = &frame{name: fo.Name, resT: p.Type}
		code := dg.val(p.Default)
		pg.p("func %s_D_%d%s(tk *%sTask) %s {", name, i, tparamDecl(fo.TParams), pg.rt(), pg.goType(p.Type))
		pg.out.WriteString(dg.out.String())
		pg.p("\treturn %s\n}\n", code)
	}
}

// frameName is the function name in error frames: `module.fn` or
// `module.Type.fn` (core 11.1).
func (pg *pkgGen) frameName(fo *check.FuncObj) string {
	mod := fo.Module.Name
	if fo.Recv != nil {
		return mod + "." + fo.Recv.Name + "." + fo.Name
	}
	return mod + "." + fo.Name
}

func (pg *pkgGen) genConst(co *check.ConstObj) {
	fg := pg.newFn(nil)
	fg.fr = &frame{name: co.Name, resT: co.Type}
	code := fg.val(co.Decl.Value)
	pg.p("%s", pg.g.lineDirective(co.Decl))
	pg.p("var C_%s = func() %s {", co.Name, pg.goType(co.Type))
	pg.p("\ttk := %sInitTask", pg.rt())
	pg.p("\t_ = tk")
	pg.out.WriteString(fg.out.String())
	pg.p("\treturn %s\n}()\n", code)
}

// genInvariant emits `xoInvariant` for a struct with an invariant (core
// 9.1), checked after construction and after var methods.
func (pg *pkgGen) genInvariant(t *check.TypeObj) {
	fg := pg.newFn(nil)
	fg.fr = &frame{name: t.Name}
	vars := pg.g.info.InvariantVars(t)
	for i, v := range vars {
		fg.names[v] = "v." + goField(t.Fields[i].Name)
		if pg.g.isBoxedField(t, t.Fields[i]) {
			fg.names[v] += ".Get()"
		}
	}
	code := fg.val(t.Invariant)
	pg.p("func (v %s) XoInvariant(tk *%sTask, at string) {", selfType(t), pg.rt())
	pg.out.WriteString(fg.out.String())
	pg.p("\t%sInvariant(%s, %q, %q, at)", pg.rt(), code, t.Name, pg.g.source(t.Invariant))
	pg.p("}\n")
	pg.p("func (v %s) xoInvariantHolds(tk *%sTask) bool {", selfType(t), pg.rt())
	pg.out.WriteString(fg.out.String())
	pg.p("\treturn %s\n}\n", code)
}
