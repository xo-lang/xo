package gen

import (
	"fmt"

	"github.com/xo-lang/xo/internal/check"
)

// rtHandWritten lists methods of std types that the runtime writes by hand,
// so StdTypes does not generate them.
var rtHandWritten = map[string]bool{
	"Json_DecodeErr.M_display": true, // "at $.path: msg" (std.md 5)
	"Json_Patch.json":          true, // Absent, Null, Value (std.md 5)
}

// rtExtraFields are hidden fields of std structs that the runtime needs:
// the route parameters of a request, the body producer of a streamed
// response.
var rtExtraFields = map[string][]string{
	"Http_Request":  {"XParams Map[string, string]"},
	"Http_Response": {"XStream func(*Task, Http_BodyWriter) Result[Unit, IoErr]"},
}

// isPatch reports whether t is json.Patch[T].
func (g *gen) isPatch(t check.Type) bool {
	n := g.named(t)
	return n != nil && n.Obj.Module.IsStd && n.Obj.Module.Path == "std/json" && n.Obj.Name == "Patch"
}

// genJSONStruct emits XoJsonEnc and XoJsonDec for a struct with
// `derive Json` (std.md 5).
func (pg *pkgGen) genJSONStruct(t *check.TypeObj) {
	rt := pg.rt()
	self := selfType(t)
	pg.p("func (v %s) XoJsonEnc(tk *%sTask, w *%sJsonW) {", self, rt, rt)
	pg.p("\tw.ObjStart()")
	for _, f := range t.Fields {
		if pg.g.isPatch(f.Type) {
			pg.p("\tif !v.%s.PatchAbsent() {", goField(f.Name))
			pg.p("\t\tw.Field(%q)\n\t\t%sJsonEnc(tk, w, v.%s)\n\t}", f.Name, rt, goField(f.Name))
			continue
		}
		pg.p("\tw.Field(%q)", f.Name)
		pg.p("\t%sJsonEnc(tk, w, v.%s)", rt, goField(f.Name))
	}
	pg.p("\tw.ObjEnd()\n}\n")
	pg.p("func (v *%s) XoJsonDec(tk *%sTask, x any, path string) *%sJson_DecodeErr {", self, rt, rt)
	pg.p("\tobj, err := %sJsonObject(x, path)", rt)
	pg.p("\tif err != nil {\n\t\treturn err\n\t}")
	for _, f := range t.Fields {
		boxed := pg.g.isBoxedField(t, f)
		pg.jsonField(f.Name, goField(f.Name), f.Type, func() string {
			if f.Default != nil && !t.Module.IsStd {
				d := fmt.Sprintf("%s_D_%s%s(tk)", localTypeName(t), f.Name, tparamUse(t.TParams))
				if boxed {
					d = pg.rt() + "BoxOf(" + d + ")"
				}
				return fmt.Sprintf("v.%s = %s", goField(f.Name), d)
			}
			if boxed {
				return fmt.Sprintf("v.%s = %sBox[%s]{}", goField(f.Name), pg.rt(), pg.goType(f.Type))
			}
			return ""
		})
	}
	pg.p("\treturn nil\n}\n")
}

// jsonField emits the decoding of one object field into v.<goName>.
func (pg *pkgGen) jsonField(name, goName string, t check.Type, dflt func() string) {
	rt := pg.rt()
	pg.p("\tif e, ok := obj[%q]; ok {", name)
	pg.p("\t\tif err := %sJsonDec(tk, &v.%s, e, path+%q); err != nil {\n\t\t\treturn err\n\t\t}", rt, goName, "."+name)
	pg.p("\t} else {")
	switch {
	case dflt() != "":
		pg.p("\t\t%s", dflt())
	case pg.g.optionElem(t) != nil:
		pg.p("\t\tv.%s = %s{}", goName, pg.goType(t))
	case pg.g.isPatch(t):
		pg.p("\t\tv.%s = %s{Tag: 1}", goName, pg.goType(t))
	default:
		pg.p("\t\treturn %sJsonErr(path, %q)", rt, "missing field `"+name+"`")
	}
	pg.p("\t}")
}

// genJSONEnum emits the codec of an enum with `derive Json`: a unit
// variant is its name, a variant with fields is {"Variant": {fields}}, a
// `from` variant is {"Variant": value}.
func (pg *pkgGen) genJSONEnum(t *check.TypeObj) {
	rt := pg.rt()
	self := selfType(t)
	pg.p("func (v %s) XoJsonEnc(tk *%sTask, w *%sJsonW) {", self, rt, rt)
	pg.p("\tswitch v.Tag {")
	for _, v := range t.Variants {
		pg.p("\tcase %d:", v.Index+1)
		if len(v.Fields) == 0 {
			pg.p("\t\tw.Str(%q)", v.Name)
			continue
		}
		pg.p("\t\ta := v.P.(%s%s)", localVariantType(v), tparamUse(t.TParams))
		pg.p("\t\tw.ObjStart()\n\t\tw.Field(%q)", v.Name)
		if len(v.Fields) == 1 && (v.Fields[0].From || v.Fields[0].Name == "") {
			pg.p("\t\t%sJsonEnc(tk, w, a.%s)", rt, vfieldName(v.Fields[0], 0))
		} else {
			pg.p("\t\tw.ObjStart()")
			for i, f := range v.Fields {
				pg.p("\t\tw.Field(%q)\n\t\t%sJsonEnc(tk, w, a.%s)", f.Name, rt, vfieldName(f, i))
			}
			pg.p("\t\tw.ObjEnd()")
		}
		pg.p("\t\tw.ObjEnd()")
	}
	pg.p("\t}\n}\n")
	pg.p("func (v *%s) XoJsonDec(tk *%sTask, x any, path string) *%sJson_DecodeErr {", self, rt, rt)
	pg.p("\tname, payload, hasPayload, err := %sJsonVariant(x, path)", rt)
	pg.p("\tif err != nil {\n\t\treturn err\n\t}")
	pg.p("\t_, _ = payload, hasPayload")
	pg.p("\tswitch name {")
	for _, v := range t.Variants {
		pg.p("\tcase %q:", v.Name)
		if len(v.Fields) == 0 {
			pg.p("\t\t*v = %s{Tag: %d}\n\t\treturn nil", self, v.Index+1)
			continue
		}
		pt := localVariantType(v) + tparamUse(t.TParams)
		pg.p("\t\tif !hasPayload {\n\t\t\treturn %sJsonErr(path, %q)\n\t\t}", rt, "variant `"+v.Name+"` needs fields")
		pg.p("\t\tvar p %s", pt)
		if len(v.Fields) == 1 && (v.Fields[0].From || v.Fields[0].Name == "") {
			pg.p("\t\tif err := %sJsonDec(tk, &p.%s, payload, path+%q); err != nil {\n\t\t\treturn err\n\t\t}", rt, vfieldName(v.Fields[0], 0), "."+v.Name)
		} else {
			pg.p("\t\tobj, err := %sJsonObject(payload, path+%q)", rt, "."+v.Name)
			pg.p("\t\tif err != nil {\n\t\t\treturn err\n\t\t}")
			pg.p("\t\tif err := func(v *%s, path string) *%sJson_DecodeErr {", pt, rt)
			for i, f := range v.Fields {
				pg.jsonField(f.Name, vfieldName(f, i), f.Type, func() string { return "" })
			}
			pg.p("\t\t\treturn nil\n\t\t}(&p, path+%q); err != nil {\n\t\t\treturn err\n\t\t}", "."+v.Name)
		}
		pg.p("\t\t*v = %s{Tag: %d, P: p}\n\t\treturn nil", self, v.Index+1)
	}
	pg.p("\t}")
	pg.p("\treturn %sJsonErr(path, \"unknown variant `\"+name+\"`\")", rt)
	pg.p("}\n")
}

// genJSONNewtype encodes a newtype as its underlying value.
func (pg *pkgGen) genJSONNewtype(t *check.TypeObj) {
	rt := pg.rt()
	self := selfType(t)
	pg.p("func (v %s) XoJsonEnc(tk *%sTask, w *%sJsonW) { %sJsonEnc(tk, w, v.F0) }\n", self, rt, rt, rt)
	pg.p("func (v *%s) XoJsonDec(tk *%sTask, x any, path string) *%sJson_DecodeErr { return %sJsonDec(tk, &v.F0, x, path) }\n", self, rt, rt, rt)
}

// ---------------------------------------------------------------------------
// Value generators for property tests (core 9.2, std.md 8.5)

// genGenStruct emits XoGenAny for a struct: every field generated; a struct
// with an invariant retries until it holds.
func (pg *pkgGen) genGenStruct(t *check.TypeObj) {
	rt := pg.rt()
	self := selfType(t)
	pg.p("func (%s) XoGenAny(g *%sGenRand) any {", self, rt)
	pg.p("\tg.Enter()\n\tdefer g.Leave()")
	pg.p("\tfor try := 0; ; try++ {")
	var fs []string
	for _, f := range t.Fields {
		v := fmt.Sprintf("%sGen[%s](g)", rt, pg.goType(f.Type))
		if pg.g.isBoxedField(t, f) {
			v = rt + "BoxOf(" + v + ")"
		}
		fs = append(fs, fmt.Sprintf("%s: %s", goField(f.Name), v))
	}
	pg.p("\t\tv := %s{%s}", self, joinComma(fs))
	if t.Invariant != nil {
		pg.p("\t\tif !v.xoInvariantHolds(%sInitTask) && try < 1000 {\n\t\t\tcontinue\n\t\t}", rt)
	}
	pg.p("\t\treturn v\n\t}\n}\n")
}

// genGenEnum emits XoGenAny for an enum: a random variant, unit variants
// once the value is deep.
func (pg *pkgGen) genGenEnum(t *check.TypeObj) {
	rt := pg.rt()
	self := selfType(t)
	var unit []int
	for _, v := range t.Variants {
		if len(v.Fields) == 0 {
			unit = append(unit, v.Index)
		}
	}
	pg.p("func (%s) XoGenAny(g *%sGenRand) any {", self, rt)
	pg.p("\tg.Enter()\n\tdefer g.Leave()")
	pg.p("\ti := g.Intn(%d)", len(t.Variants))
	if len(unit) > 0 {
		pg.p("\tif g.Deep() {\n\t\ti = []int{%s}[g.Intn(%d)]\n\t}", intList(unit), len(unit))
	}
	pg.p("\tswitch i {")
	for _, v := range t.Variants {
		pg.p("\tcase %d:", v.Index)
		if len(v.Fields) == 0 {
			pg.p("\t\treturn %s{Tag: %d}", self, v.Index+1)
			continue
		}
		var fs []string
		for i, f := range v.Fields {
			fs = append(fs, fmt.Sprintf("%s: %sGen[%s](g)", vfieldName(f, i), rt, pg.goType(f.Type)))
		}
		pg.p("\t\treturn %s{Tag: %d, P: %s%s{%s}}", self, v.Index+1, localVariantType(v), tparamUse(t.TParams), joinComma(fs))
	}
	pg.p("\t}\n\treturn %s{Tag: 1}\n}\n", self)
}

// genGenNewtype emits XoGenAny for a newtype.
func (pg *pkgGen) genGenNewtype(t *check.TypeObj) {
	rt := pg.rt()
	self := selfType(t)
	pg.p("func (%s) XoGenAny(g *%sGenRand) any { return %s{F0: %sGen[%s](g)} }\n", self, rt, self, rt, pg.goType(t.Target))
}

func joinComma(xs []string) string {
	out := ""
	for i, x := range xs {
		if i > 0 {
			out += ", "
		}
		out += x
	}
	return out
}

func intList(xs []int) string {
	out := ""
	for i, x := range xs {
		if i > 0 {
			out += ", "
		}
		out += fmt.Sprint(x)
	}
	return out
}
