package ssa

import (
	"fmt"
	"strconv"
	"strings"
)

// String prints the module: its named types, then its functions, in a
// stable textual form used by golden tests.
func (m *Module) String() string {
	var b strings.Builder
	for _, t := range m.order {
		switch t.Kind {
		case KStruct:
			var fs []string
			for _, f := range t.Fields {
				fs = append(fs, f.Name+": "+f.Type.Name)
			}
			fmt.Fprintf(&b, "type %s = struct { %s }\n", t.Name, strings.Join(fs, ", "))
		case KEnum:
			var vs []string
			for _, v := range t.Variants {
				if len(v.Fields) == 0 {
					vs = append(vs, v.Name)
					continue
				}
				var fs []string
				for _, f := range v.Fields {
					fs = append(fs, f.Name+": "+f.Type.Name)
				}
				vs = append(vs, v.Name+"("+strings.Join(fs, ", ")+")")
			}
			fmt.Fprintf(&b, "type %s = enum { %s }\n", t.Name, strings.Join(vs, " | "))
		}
	}
	if len(m.order) > 0 {
		b.WriteByte('\n')
	}
	for i, f := range m.Funcs {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(f.String())
	}
	return b.String()
}

// String prints one function.
func (f *Func) String() string {
	var b strings.Builder
	var ps []string
	for i, p := range f.Params() {
		b := ""
		if i < len(f.Borrowed) && f.Borrowed[i] {
			b = "borrow "
		}
		ps = append(ps, b+p.Name()+": "+p.Type.Name)
	}
	var rs []string
	for _, r := range f.Results {
		rs = append(rs, r.Name)
	}
	res := strings.Join(rs, ", ")
	if len(rs) != 1 {
		res = "(" + res + ")"
	}
	if f.Indirect {
		var cs []string
		for _, c := range f.Captures {
			cs = append(cs, c.Name)
		}
		res += " captures (" + strings.Join(cs, ", ") + ")"
	}
	fmt.Fprintf(&b, "func %s(%s) -> %s {\n", f.Name, strings.Join(ps, ", "), res)
	for _, bl := range f.Blocks {
		b.WriteString(bl.Name())
		if len(bl.Params) > 0 {
			var ps []string
			for _, p := range bl.Params {
				ps = append(ps, p.Name()+": "+p.Type.Name)
			}
			b.WriteString("(" + strings.Join(ps, ", ") + ")")
		}
		b.WriteString(":")
		if len(bl.Preds) > 0 {
			var ps []string
			for _, e := range bl.Preds {
				ps = append(ps, e.From.Name())
			}
			b.WriteString("  ; preds " + strings.Join(ps, ", "))
		}
		b.WriteByte('\n')
		for _, v := range bl.Instrs {
			b.WriteString("  " + v.LongString() + "\n")
		}
	}
	b.WriteString("}\n")
	return b.String()
}

// LongString prints an instruction: `v3: Int = add v1, v2 @ 4:9`.
func (v *Value) LongString() string {
	var b strings.Builder
	if v.Type != nil && !v.Op.IsTerminator() {
		b.WriteString(v.Name() + ": " + v.Type.Name + " = ")
	}
	b.WriteString(v.Op.String())
	var ops []string
	switch a := v.Aux.(type) {
	case nil:
	case string:
		if v.Op == OpConst {
			ops = append(ops, strconv.Quote(a))
		} else {
			ops = append(ops, a)
		}
	case float64:
		f := strconv.FormatFloat(a, 'g', -1, 64)
		if !strings.ContainsAny(f, ".eIN") {
			f += ".0"
		}
		ops = append(ops, f)
	case PayloadRef:
		ops = append(ops, fmt.Sprintf("%d.%d", a.Tag, a.Field))
	default:
		ops = append(ops, fmt.Sprint(a))
	}
	for _, x := range v.Args {
		ops = append(ops, x.Name())
	}
	for _, e := range v.Succs {
		ops = append(ops, e.String())
	}
	if len(ops) > 0 {
		b.WriteString(" " + strings.Join(ops, ", "))
	}
	if v.Pos != "" {
		b.WriteString(" @ " + v.Pos)
	}
	return b.String()
}

// String prints an edge: `b3(v1, v2)`.
func (e *Edge) String() string {
	var as []string
	for _, a := range e.Args {
		as = append(as, a.Name())
	}
	return e.To.Name() + "(" + strings.Join(as, ", ") + ")"
}
