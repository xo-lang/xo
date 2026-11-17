// protoparseoracle writes the std/protoparse fuzz corpus (decision 0616's
// fuzz kind, task #152): seeded random proto2 and proto3 files (packages,
// imports, file options, messages nested three deep with fields of every
// scalar type, message types, maps, oneofs, groups, reserved ranges and
// names, extension ranges, extend blocks, enums with aliases and reserved
// values, services with streaming rpcs), and half of them changed at the
// token level (a token dropped, doubled or replaced, the file cut short).
// The reference is bufbuild/protocompile v0.14.1's parser with its
// validation (parser.Parse, then parser.ResultFromAST(validate=true)): a
// file it accepts must parse in std/protoparse to the same outline (its
// messages, fields, enums, services and rpcs), and a file it rejects must be
// rejected (rejectChecked, off until task #230). XO_PROTOPARSEFUZZ_WHY=1
// prints protocompile's reason for each file it rejects.
//
// XO_PROTOPARSEFUZZ_SEED (default 0, the committed corpus) moves the seed
// and XO_PROTOPARSEFUZZ_N (default 150) sets how many files are drawn;
// internal/stdtest's TestProtoparseFuzz runs a fresh seed.
package main

import (
	"bytes"
	"fmt"
	"math/rand"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/bufbuild/protocompile/parser"
	"github.com/bufbuild/protocompile/reporter"
	"google.golang.org/protobuf/types/descriptorpb"
)

func xq(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch {
		case r == '"':
			b.WriteString(`\"`)
		case r == '\\':
			b.WriteString(`\\`)
		case r == '$':
			b.WriteString(`\$`)
		case r == '\n':
			b.WriteString(`\n`)
		case r == '\r':
			b.WriteString(`\r`)
		case r == '\t':
			b.WriteString(`\t`)
		case r < 0x20 || r == 0x7f:
			fmt.Fprintf(&b, `\u{%x}`, r)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

func envInt(name string, def int) int {
	if v, err := strconv.Atoi(os.Getenv(name)); err == nil {
		return v
	}
	return def
}

var scalars = []string{"double", "float", "int32", "int64", "uint32", "uint64", "sint32", "sint64", "fixed32", "fixed64", "sfixed32", "sfixed64", "bool", "string", "bytes"}

var mapKeys = []string{"int32", "int64", "uint32", "uint64", "sint32", "sint64", "fixed32", "fixed64", "sfixed32", "sfixed64", "bool", "string"}

type gen struct {
	r      *rand.Rand
	proto2 bool
	b      strings.Builder
	names  int
	types  []string // message and enum names declared so far, for field types
}

func (g *gen) name(prefix string) string {
	g.names++
	return fmt.Sprintf("%s%d", prefix, g.names)
}

func (g *gen) ind(d int) string { return strings.Repeat("  ", d) }

func (g *gen) fieldType() string {
	if len(g.types) > 0 && g.r.Intn(4) == 0 {
		return g.types[g.r.Intn(len(g.types))]
	}
	return scalars[g.r.Intn(len(scalars))]
}

func (g *gen) fieldOpts() string {
	var o []string
	if g.r.Intn(6) == 0 {
		o = append(o, "deprecated = true")
	}
	if g.r.Intn(8) == 0 {
		o = append(o, fmt.Sprintf("json_name = %q", g.name("j")))
	}
	if len(o) == 0 {
		return ""
	}
	return " [" + strings.Join(o, ", ") + "]"
}

// message writes a message at depth d whose field numbers start at 1.
func (g *gen) message(d int) {
	name := g.name("M")
	fmt.Fprintf(&g.b, "%smessage %s {\n", g.ind(d), name)
	num := 1
	nf := g.r.Intn(7)
	for i := 0; i < nf; i++ {
		switch k := g.r.Intn(10); {
		case k == 0 && d < 3:
			g.message(d + 1)
		case k == 1:
			g.enum(d + 1)
		case k == 2:
			fmt.Fprintf(&g.b, "%smap<%s, %s> %s = %d%s;\n", g.ind(d+1), mapKeys[g.r.Intn(len(mapKeys))], g.fieldType(), g.name("f"), num, g.fieldOpts())
			num++
		case k == 3:
			fmt.Fprintf(&g.b, "%soneof %s {\n", g.ind(d+1), g.name("o"))
			for j := 0; j <= g.r.Intn(3); j++ {
				fmt.Fprintf(&g.b, "%s%s %s = %d%s;\n", g.ind(d+2), g.fieldType(), g.name("f"), num, g.fieldOpts())
				num++
			}
			fmt.Fprintf(&g.b, "%s}\n", g.ind(d+1))
		case k == 4 && g.proto2 && d < 3:
			gn := g.name("G")
			fmt.Fprintf(&g.b, "%soptional group %s = %d {\n", g.ind(d+1), gn, num)
			num++
			fmt.Fprintf(&g.b, "%soptional int32 %s = 1;\n", g.ind(d+2), g.name("f"))
			fmt.Fprintf(&g.b, "%s}\n", g.ind(d+1))
		case k == 5:
			a := num + 100 + g.r.Intn(50)
			fmt.Fprintf(&g.b, "%sreserved %d, %d to %d;\n", g.ind(d+1), a, a+60, a+60+g.r.Intn(10))
			fmt.Fprintf(&g.b, "%sreserved %q;\n", g.ind(d+1), g.name("gone"))
		case k == 6 && g.proto2:
			fmt.Fprintf(&g.b, "%sextensions %d to %d;\n", g.ind(d+1), 1000+num*10, 1000+num*10+5)
		default:
			label := ""
			switch g.r.Intn(4) {
			case 0:
				label = "repeated "
			case 1:
				label = "optional "
			case 2:
				if g.proto2 {
					label = "required "
				}
			}
			if g.proto2 && label == "" {
				label = "optional "
			}
			def := ""
			if g.proto2 && label == "optional " && g.r.Intn(4) == 0 {
				t := "int32"
				fmt.Fprintf(&g.b, "%s%s%s %s = %d [default = %d];\n", g.ind(d+1), label, t, g.name("f"), num, g.r.Intn(200)-100)
				num++
				continue
			}
			_ = def
			fmt.Fprintf(&g.b, "%s%s%s %s = %d%s;\n", g.ind(d+1), label, g.fieldType(), g.name("f"), num, g.fieldOpts())
			num++
		}
	}
	fmt.Fprintf(&g.b, "%s}\n", g.ind(d))
	g.types = append(g.types, name)
}

func (g *gen) enum(d int) {
	name := g.name("E")
	fmt.Fprintf(&g.b, "%senum %s {\n", g.ind(d), name)
	alias := g.r.Intn(4) == 0
	if alias {
		fmt.Fprintf(&g.b, "%soption allow_alias = true;\n", g.ind(d+1))
	}
	fmt.Fprintf(&g.b, "%s%s = 0;\n", g.ind(d+1), g.name("V"))
	n := 1 + g.r.Intn(4)
	for i := 1; i <= n; i++ {
		v := i
		if g.r.Intn(5) == 0 {
			v = -i
		}
		fmt.Fprintf(&g.b, "%s%s = %d;\n", g.ind(d+1), g.name("V"), v)
	}
	if alias {
		fmt.Fprintf(&g.b, "%s%s = 1;\n", g.ind(d+1), g.name("V"))
	}
	if g.r.Intn(4) == 0 {
		fmt.Fprintf(&g.b, "%sreserved %d to %d;\n", g.ind(d+1), 50, 50+g.r.Intn(9))
	}
	fmt.Fprintf(&g.b, "%s}\n", g.ind(d))
	g.types = append(g.types, name)
}

func (g *gen) file() string {
	g.b.Reset()
	g.types = nil
	if g.proto2 {
		g.b.WriteString("syntax = \"proto2\";\n\n")
	} else {
		g.b.WriteString("syntax = \"proto3\";\n\n")
	}
	if g.r.Intn(3) > 0 {
		fmt.Fprintf(&g.b, "package %s.%s;\n\n", g.name("p"), g.name("q"))
	}
	for i := 0; i < g.r.Intn(3); i++ {
		mod := []string{"", "public ", "weak "}[g.r.Intn(3)]
		fmt.Fprintf(&g.b, "import %s%q;\n", mod, g.name("dep/f")+".proto")
	}
	if g.r.Intn(2) == 0 {
		fmt.Fprintf(&g.b, "option go_package = %q;\n", "example.com/"+g.name("g"))
	}
	if g.r.Intn(3) == 0 {
		g.b.WriteString("option java_multiple_files = true;\n")
	}
	g.b.WriteString("\n")
	for i := 0; i < 1+g.r.Intn(4); i++ {
		if g.r.Intn(3) == 0 {
			g.enum(0)
		} else {
			g.message(0)
		}
	}
	if g.proto2 && len(g.types) > 0 && g.r.Intn(3) == 0 {
		fmt.Fprintf(&g.b, "extend %s {\n  optional int32 %s = 1001;\n}\n", g.types[0], g.name("x"))
	}
	if g.r.Intn(2) == 0 && len(g.types) > 0 {
		fmt.Fprintf(&g.b, "service %s {\n", g.name("S"))
		for i := 0; i < 1+g.r.Intn(3); i++ {
			cs, ss := "", ""
			if g.r.Intn(3) == 0 {
				cs = "stream "
			}
			if g.r.Intn(3) == 0 {
				ss = "stream "
			}
			fmt.Fprintf(&g.b, "  rpc %s(%s%s) returns (%s%s);\n", g.name("R"), cs, g.types[g.r.Intn(len(g.types))], ss, g.types[g.r.Intn(len(g.types))])
		}
		g.b.WriteString("}\n")
	}
	return g.b.String()
}

var junk = []string{";", "{", "}", "=", "1", "-1", "0x10", "\"s\"", "message", "enum", "repeated", "required", "optional", "group", "map<", ">", ",", "oneof", "reserved", "extensions", "to", "max", "int32", "string", "[", "]", "(", ")", "stream", "rpc", "returns", "service", "option", "syntax", "package", "import", ".", "true", "inf", "nan", "1e3", "'q'", "/*", "//"}

// tokens splits src at spaces and punctuation, keeping the separators, so a
// change at the token level keeps the rest of the file as it was.
func tokens(src string) []string {
	var ts []string
	cur := ""
	flush := func() {
		if cur != "" {
			ts = append(ts, cur)
			cur = ""
		}
	}
	for _, c := range src {
		switch {
		case strings.ContainsRune(" \n\t;{}=<>,[]()", c):
			flush()
			ts = append(ts, string(c))
		default:
			cur += string(c)
		}
	}
	flush()
	return ts
}

func mutate(r *rand.Rand, src string) string {
	ts := tokens(src)
	var idx []int
	for i, t := range ts {
		if strings.TrimSpace(t) != "" {
			idx = append(idx, i)
		}
	}
	if len(idx) == 0 {
		return src
	}
	i := idx[r.Intn(len(idx))]
	switch r.Intn(5) {
	case 0:
		ts = append(ts[:i:i], ts[i+1:]...)
	case 1:
		ts = append(ts[:i+1:i+1], append([]string{" ", ts[i]}, ts[i+1:]...)...)
	case 2:
		ts[i] = junk[r.Intn(len(junk))]
	case 3:
		ts = append(ts[:i:i], append([]string{junk[r.Intn(len(junk))], " "}, ts[i:]...)...)
	default:
		ts = ts[:i]
	}
	return strings.Join(ts, "")
}

// outline is the file as std/protoparse's test renders it, from protocompile's
// unlinked FileDescriptorProto.
func outline(fd *descriptorpb.FileDescriptorProto) string {
	var b strings.Builder
	fmt.Fprintf(&b, "syntax %s package %s imports %s\n", syntaxOf(fd), fd.GetPackage(), strings.Join(fd.GetDependency(), ","))
	for _, m := range fd.GetMessageType() {
		msgOutline(&b, fd, m, 0)
	}
	for _, e := range fd.GetEnumType() {
		enumOutline(&b, e, 0)
	}
	for _, s := range fd.GetService() {
		fmt.Fprintf(&b, "service %s\n", s.GetName())
		for _, m := range s.GetMethod() {
			fmt.Fprintf(&b, " rpc %s %s %s %v %v\n", m.GetName(), m.GetInputType(), m.GetOutputType(), m.GetClientStreaming(), m.GetServerStreaming())
		}
	}
	return b.String()
}

func syntaxOf(fd *descriptorpb.FileDescriptorProto) string {
	if fd.GetSyntax() == "" {
		return "proto2"
	}
	return fd.GetSyntax()
}

func typeName(f *descriptorpb.FieldDescriptorProto) string {
	if f.GetTypeName() != "" {
		return f.GetTypeName()
	}
	return strings.ToLower(strings.TrimPrefix(f.GetType().String(), "TYPE_"))
}

func msgOutline(b *strings.Builder, fd *descriptorpb.FileDescriptorProto, m *descriptorpb.DescriptorProto, d int) {
	pad := strings.Repeat(" ", d)
	fmt.Fprintf(b, "%smessage %s\n", pad, m.GetName())
	entries := map[string]*descriptorpb.DescriptorProto{}
	for _, n := range m.GetNestedType() {
		if n.GetOptions().GetMapEntry() {
			entries[n.GetName()] = n
		}
	}
	for _, f := range m.GetField() {
		label := ""
		switch f.GetLabel() {
		case descriptorpb.FieldDescriptorProto_LABEL_REPEATED:
			label = "repeated"
		case descriptorpb.FieldDescriptorProto_LABEL_REQUIRED:
			label = "required"
		}
		t := typeName(f)
		if e, ok := entries[f.GetTypeName()]; ok && label == "repeated" {
			label = "map"
			t = typeName(e.GetField()[0]) + "," + typeName(e.GetField()[1])
		}
		oneof := ""
		if f.OneofIndex != nil && !f.GetProto3Optional() {
			oneof = m.GetOneofDecl()[f.GetOneofIndex()].GetName()
		}
		fmt.Fprintf(b, "%s field %s %d %s %s %s\n", pad, f.GetName(), f.GetNumber(), label, t, oneof)
	}
	for _, n := range m.GetNestedType() {
		if !n.GetOptions().GetMapEntry() {
			msgOutline(b, fd, n, d+1)
		}
	}
	for _, e := range m.GetEnumType() {
		enumOutline(b, e, d+1)
	}
}

func enumOutline(b *strings.Builder, e *descriptorpb.EnumDescriptorProto, d int) {
	pad := strings.Repeat(" ", d)
	var vs []string
	for _, v := range e.GetValue() {
		vs = append(vs, fmt.Sprintf("%s=%d", v.GetName(), v.GetNumber()))
	}
	fmt.Fprintf(b, "%senum %s %s\n", pad, e.GetName(), strings.Join(vs, ","))
}

// reference parses src with protocompile: the outline, or "" when it rejects it.
func reference(src string) (string, bool) {
	h := reporter.NewHandler(nil)
	f, err := parser.Parse("f.proto", bytes.NewReader([]byte(src)), h)
	if err != nil {
		if os.Getenv("XO_PROTOPARSEFUZZ_WHY") == "1" {
			fmt.Fprintf(os.Stderr, "why: %v\n", err)
		}
		return "", false
	}
	res, err := parser.ResultFromAST(f, true, h)
	if err != nil {
		if os.Getenv("XO_PROTOPARSEFUZZ_WHY") == "1" {
			fmt.Fprintf(os.Stderr, "why: %v\n", err)
		}
		return "", false
	}
	return outline(res.FileDescriptorProto()), true
}

func main() {
	seed := int64(envInt("XO_PROTOPARSEFUZZ_SEED", 0))
	n := envInt("XO_PROTOPARSEFUZZ_N", 150)
	r := rand.New(rand.NewSource(seed*7919 + 1))
	type row struct {
		src, want string
		ok        bool
	}
	var rows []row
	acc := 0
	for i := 0; i < n; i++ {
		g := &gen{r: r, proto2: r.Intn(2) == 0}
		src := g.file()
		if i%2 == 1 {
			src = mutate(r, src)
		}
		want, ok := reference(src)
		if ok {
			acc++
		}
		rows = append(rows, row{src, want, ok})
	}
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].ok && !rows[j].ok })
	var o strings.Builder
	o.WriteString("// Generated by scripts/stdcorpus from bufbuild/protocompile v0.14.1's parser (" + os.Args[1] + ").\n")
	o.WriteString("// Do not edit: regenerate with go run ./scripts/stdcorpus protoparsefuzz.\n")
	o.WriteString("use std/protoparse\n\n")
	fmt.Fprintf(&o, "/// seed %d: (source, accepted by protocompile, its outline)\nconst CASES: List[(Str, Bool, Str)] = [\n", seed)
	for _, x := range rows {
		fmt.Fprintf(&o, "  (%s, %v, %s),\n", xq(x.src), x.ok, xq(x.want))
	}
	o.WriteString("]\n")
	o.WriteString(xoTests)
	if rejectChecked {
		o.WriteString(xoReject)
	}
	fmt.Fprintf(os.Stderr, "protoparseoracle: seed %d, %d files, %d accepted\n", seed, n, acc)
	os.Stdout.WriteString(o.String())
}
