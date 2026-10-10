package check

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/xo-lang/xo/internal/diag"
	"github.com/xo-lang/xo/internal/token"
)

// Sections selects the parts of a Dump (decision 0141).
type Sections struct {
	Names bool // what each identifier resolves to by name lookup
	Types bool // the type of every checked expression
	Sigs  bool // the signature and effects of every function and method
	Diags bool // the diagnostics: code, span, message
}

// AllSections selects every section.
var AllSections = Sections{Names: true, Types: true, Sigs: true, Diags: true}

// ParseSections reads a comma separated list of section names (names,
// types, sigs, diags); "" or "all" selects every section.
func ParseSections(s string) (Sections, error) {
	if s == "" || s == "all" {
		return AllSections, nil
	}
	var out Sections
	for _, p := range strings.Split(s, ",") {
		switch p {
		case "names":
			out.Names = true
		case "types":
			out.Types = true
		case "sigs":
			out.Sigs = true
		case "diags":
			out.Diags = true
		default:
			return out, fmt.Errorf("unknown section %q (names, types, sigs, diags)", p)
		}
	}
	return out, nil
}

// Dump prints what the checker resolved for the root modules of a run in
// the canonical text form of `xo typedump` (decision 0141), the form the
// self hosted checker prints, so the two can be compared line by line, a
// section at a time:
//
//	== module <path> <dir>
//	-- names <file>
//	<line>:<col> <name> <object>
//	-- types <file>
//	<line>:<col>-<line>:<col> <node kind> <type>
//	-- sigs
//	<line>:<col> <Type.name> <signature>[ uses <effects>]
//	== diags
//	{"code":...,"file":...,"line":...,"col":...,"end_line":...,"end_col":...,"message":...}
//
// An object is `local <let|var|param|var-param|recv|var-recv|predeclared>
// <pos>`, `fn|const|type <module> <name> <pos>`, `tparam <name> <pos>`, or
// `module <path>`; a position is `file:line:col`, or `-` when there is none.
// Lines are sorted by position. Diagnostics are those of every module, in
// position order, then by code and message.
func (r *Result) Dump(sec Sections) string {
	var b strings.Builder
	c := r.c
	for _, m := range r.Modules {
		fmt.Fprintf(&b, "== module %s %s\n", m.Path, m.Dir)
		for _, f := range m.Files {
			if sec.Names {
				fmt.Fprintf(&b, "-- names %s\n", f.Path)
				c.dumpNames(&b, f)
			}
			if sec.Types {
				fmt.Fprintf(&b, "-- types %s\n", f.Path)
				c.dumpTypes(&b, f)
			}
		}
		if sec.Sigs {
			b.WriteString("-- sigs\n")
			c.dumpSigs(&b, m)
		}
	}
	if sec.Diags {
		b.WriteString("== diags\n")
		for _, l := range DiagLines(r.Diagnostics) {
			b.WriteString(l)
			b.WriteByte('\n')
		}
	}
	return b.String()
}

// DiagLines are the diagnostic lines of a dump, sorted.
func DiagLines(ds []diag.Diagnostic) []string {
	type brief struct {
		Code    string `json:"code"`
		File    string `json:"file"`
		Line    int    `json:"line"`
		Col     int    `json:"col"`
		EndLine int    `json:"end_line"`
		EndCol  int    `json:"end_col"`
		Message string `json:"message"`
	}
	sorted := append([]diag.Diagnostic(nil), ds...)
	sort.SliceStable(sorted, func(i, j int) bool {
		a, b := sorted[i], sorted[j]
		if a.Span.File != b.Span.File {
			return a.Span.File < b.Span.File
		}
		if a.Span.Line != b.Span.Line {
			return a.Span.Line < b.Span.Line
		}
		if a.Span.Col != b.Span.Col {
			return a.Span.Col < b.Span.Col
		}
		if a.Code != b.Code {
			return a.Code < b.Code
		}
		return a.Message < b.Message
	})
	var out []string
	for _, d := range sorted {
		var buf bytes.Buffer
		enc := json.NewEncoder(&buf)
		enc.SetEscapeHTML(false)
		enc.Encode(brief{Code: d.Code, File: d.Span.File, Line: d.Span.Line, Col: d.Span.Col, EndLine: d.Span.EndLine, EndCol: d.Span.EndCol, Message: d.Message})
		out = append(out, strings.TrimSuffix(buf.String(), "\n"))
	}
	return out
}

func posText(sp token.Span) string {
	if sp.File == "" {
		return "-"
	}
	return fmt.Sprintf("%s:%d:%d", sp.File, sp.Start.Line, sp.Start.Col)
}

// objText describes the object of a name record.
func objText(o any) string {
	switch x := o.(type) {
	case *VarObj:
		kind := "let"
		switch x.Kind {
		case VParam:
			kind = "param"
		case VRecv:
			kind = "recv"
		case VPredeclared:
			kind = "predeclared"
		}
		if x.Mutable {
			if kind == "let" {
				kind = "var"
			} else if kind != "predeclared" {
				kind = "var-" + kind
			}
		}
		return "local " + kind + " " + posText(x.Span)
	case *FuncObj:
		return fmt.Sprintf("fn %s %s %s", x.Module.Path, x.DisplayName(), posText(x.Span))
	case *VariantObj:
		return fmt.Sprintf("variant %s %s.%s %s", x.Enum.Module.Path, x.Enum.Name, x.Name, posText(x.Span))
	case fieldRef:
		return fmt.Sprintf("field %s %s.%s %s", x.mod.Path, x.owner, x.name, posText(x.span))
	case paramRef:
		return fmt.Sprintf("param %s %s.%s %s", x.fn.Module.Path, x.fn.DisplayName(), x.name, posText(x.span))
	case *ConstObj:
		sp := token.Span{}
		if x.Decl != nil {
			sp = x.Decl.Name.Sp
		}
		return fmt.Sprintf("const %s %s %s", x.Module.Path, x.Name, posText(sp))
	case *TypeObj:
		return fmt.Sprintf("type %s %s %s", x.Module.Path, x.Name, posText(x.Span))
	case *TParam:
		return fmt.Sprintf("tparam %s %s", x.Name, posText(x.Span))
	case *Module:
		return "module " + x.Path
	}
	return fmt.Sprintf("? %T", o)
}

// dumpNames prints the name records of the identifiers of file f.
func (c *checker) dumpNames(b *strings.Builder, f *File) {
	type line struct {
		off  int
		text string
	}
	var lines []line
	for id, r := range c.rec.names {
		if id.Sp.File != f.Path {
			continue
		}
		lines = append(lines, line{id.Sp.Start.Offset, fmt.Sprintf("%d:%d %s %s", id.Sp.Start.Line, id.Sp.Start.Col, id.Name, objText(r.obj))})
	}
	sort.Slice(lines, func(i, j int) bool {
		if lines[i].off != lines[j].off {
			return lines[i].off < lines[j].off
		}
		return lines[i].text < lines[j].text
	})
	for _, l := range lines {
		b.WriteString(l.text)
		b.WriteByte('\n')
	}
}

// dumpTypes prints the type of every checked expression of file f.
func (c *checker) dumpTypes(b *strings.Builder, f *File) {
	type line struct {
		start, end int
		text       string
	}
	var lines []line
	for e, t := range c.exprType {
		sp := e.Span()
		if sp.File != f.Path || t == nil {
			continue
		}
		kind := reflect.TypeOf(e).Elem().Name()
		lines = append(lines, line{sp.Start.Offset, sp.End.Offset, fmt.Sprintf("%d:%d-%d:%d %s %s", sp.Start.Line, sp.Start.Col, sp.End.Line, sp.End.Col, kind, Deep(t))})
	}
	sort.Slice(lines, func(i, j int) bool {
		a, b := lines[i], lines[j]
		if a.start != b.start {
			return a.start < b.start
		}
		if a.end != b.end {
			return a.end > b.end
		}
		return a.text < b.text
	})
	for _, l := range lines {
		b.WriteString(l.text)
		b.WriteByte('\n')
	}
}

// dumpSigs prints the signature and effects of every function and method
// declared in m.
func (c *checker) dumpSigs(b *strings.Builder, m *Module) {
	var fns []*FuncObj
	for _, o := range m.Scope {
		switch x := o.(type) {
		case *FuncObj:
			fns = append(fns, x)
		case *TypeObj:
			for _, f := range x.Methods {
				fns = append(fns, f)
			}
			for _, f := range x.Statics {
				fns = append(fns, f)
			}
			fns = append(fns, x.IfaceMeths...)
		}
	}
	sort.Slice(fns, func(i, j int) bool {
		a, b := fns[i].Span, fns[j].Span
		if a.File != b.File {
			return a.File < b.File
		}
		return a.Start.Offset < b.Start.Offset
	})
	for _, f := range fns {
		if !f.sigDone {
			continue
		}
		fmt.Fprintf(b, "%s %s %s", posText(f.Span), f.DisplayName(), sigText(f))
		eff := ""
		if f.Declared != nil {
			eff = f.Declared.String()
		} else if fl := c.inferred[f]; fl != nil {
			eff = fl.String()
		}
		if eff != "" {
			b.WriteString(" uses " + eff)
		}
		b.WriteByte('\n')
	}
}

// sigText is a signature without its effects: type parameters, receiver,
// parameters, result, and error type.
func sigText(f *FuncObj) string {
	var b strings.Builder
	var tps []string
	for _, p := range f.TParams {
		s := p.Name
		if p.Implicit {
			s = "implicit " + s
		}
		if p.IsEffect {
			s += ": Effects"
		} else if p.Constraint != nil {
			s += ": " + Deep(p.Constraint).String()
		}
		tps = append(tps, s)
	}
	if len(tps) > 0 {
		b.WriteString("[" + strings.Join(tps, ", ") + "]")
	}
	b.WriteByte('(')
	if f.RecvType != nil {
		if f.RecvVar {
			b.WriteString("var ")
		}
		b.WriteString("self: " + Deep(f.RecvType).String())
		if len(f.Params) > 0 {
			b.WriteString(", ")
		}
	}
	for i, p := range f.Params {
		if i > 0 {
			b.WriteString(", ")
		}
		if p.Var {
			b.WriteString("var ")
		}
		b.WriteString(p.Name + ": " + Deep(p.Type).String())
	}
	b.WriteByte(')')
	if f.Result != nil && !isUnit(f.Result) {
		b.WriteString(" -> " + Deep(f.Result).String())
	}
	if f.Err != nil {
		b.WriteString(" ! " + Deep(f.Err).String())
	}
	return b.String()
}
