// Command protojsonoracle prints the std/protojson corpus: protobuf-go v1.36.12's
// protojson (marshal and unmarshal) over a test schema that has every field
// kind (the scalars singular and repeated, enums, maps of every key kind,
// oneofs, proto3 optional, nested messages, the well known types and Any), with
// seeded random messages built through dynamicpb. The schema travels as a
// FileDescriptorSet, so std/protojson reads the descriptors the way
// reflection does. It is its own module so the repository's go.mod has no
// third-party requirement.
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"os"
	"strings"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/durationpb"
	_ "google.golang.org/protobuf/types/known/emptypb"
	_ "google.golang.org/protobuf/types/known/fieldmaskpb"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

type T = descriptorpb.FieldDescriptorProto_Type

func camel(s string) string {
	var b strings.Builder
	up := false
	for _, c := range s {
		if c == '_' {
			up = true
			continue
		}
		if up && c >= 'a' && c <= 'z' {
			c -= 32
		}
		up = false
		b.WriteRune(c)
	}
	return b.String()
}

func field(name string, num int32, t T, label descriptorpb.FieldDescriptorProto_Label, typeName string) *descriptorpb.FieldDescriptorProto {
	f := &descriptorpb.FieldDescriptorProto{Name: proto.String(name), Number: proto.Int32(num), Type: t.Enum(), Label: label.Enum(), JsonName: proto.String(camel(name))}
	if typeName != "" {
		f.TypeName = proto.String(typeName)
	}
	return f
}

var opt = descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL
var rep = descriptorpb.FieldDescriptorProto_LABEL_REPEATED

var scalarKinds = []struct {
	n string
	t T
}{
	{"double", descriptorpb.FieldDescriptorProto_TYPE_DOUBLE}, {"float", descriptorpb.FieldDescriptorProto_TYPE_FLOAT},
	{"int64", descriptorpb.FieldDescriptorProto_TYPE_INT64}, {"uint64", descriptorpb.FieldDescriptorProto_TYPE_UINT64},
	{"int32", descriptorpb.FieldDescriptorProto_TYPE_INT32}, {"fixed64", descriptorpb.FieldDescriptorProto_TYPE_FIXED64},
	{"fixed32", descriptorpb.FieldDescriptorProto_TYPE_FIXED32}, {"bool", descriptorpb.FieldDescriptorProto_TYPE_BOOL},
	{"string", descriptorpb.FieldDescriptorProto_TYPE_STRING}, {"bytes", descriptorpb.FieldDescriptorProto_TYPE_BYTES},
	{"uint32", descriptorpb.FieldDescriptorProto_TYPE_UINT32}, {"sfixed32", descriptorpb.FieldDescriptorProto_TYPE_SFIXED32},
	{"sfixed64", descriptorpb.FieldDescriptorProto_TYPE_SFIXED64}, {"sint32", descriptorpb.FieldDescriptorProto_TYPE_SINT32},
	{"sint64", descriptorpb.FieldDescriptorProto_TYPE_SINT64},
}

func mapEntry(name string, kt, vt T, vtn string) *descriptorpb.DescriptorProto {
	return &descriptorpb.DescriptorProto{
		Name:    proto.String(name),
		Field:   []*descriptorpb.FieldDescriptorProto{field("key", 1, kt, opt, ""), field("value", 2, vt, opt, vtn)},
		Options: &descriptorpb.MessageOptions{MapEntry: proto.Bool(true)},
	}
}

func schema() *descriptorpb.FileDescriptorSet {
	deps := []string{"google/protobuf/timestamp.proto", "google/protobuf/duration.proto", "google/protobuf/field_mask.proto", "google/protobuf/struct.proto", "google/protobuf/any.proto", "google/protobuf/wrappers.proto", "google/protobuf/empty.proto"}
	inner := &descriptorpb.DescriptorProto{Name: proto.String("Inner"), Field: []*descriptorpb.FieldDescriptorProto{
		field("id", 1, descriptorpb.FieldDescriptorProto_TYPE_INT32, opt, ""),
		field("label", 2, descriptorpb.FieldDescriptorProto_TYPE_STRING, opt, ""),
		field("next", 3, descriptorpb.FieldDescriptorProto_TYPE_MESSAGE, opt, ".pjt.Inner"),
	}}
	sc := &descriptorpb.DescriptorProto{Name: proto.String("Scalars")}
	n := int32(1)
	for _, k := range scalarKinds {
		sc.Field = append(sc.Field, field("s_"+k.n, n, k.t, opt, ""))
		n++
		sc.Field = append(sc.Field, field("r_"+k.n, n, k.t, rep, ""))
		n++
	}
	sc.Field = append(sc.Field,
		field("color", n, descriptorpb.FieldDescriptorProto_TYPE_ENUM, opt, ".pjt.Color"),
		field("colors", n+1, descriptorpb.FieldDescriptorProto_TYPE_ENUM, rep, ".pjt.Color"),
		field("inner", n+2, descriptorpb.FieldDescriptorProto_TYPE_MESSAGE, opt, ".pjt.Inner"),
		field("inners", n+3, descriptorpb.FieldDescriptorProto_TYPE_MESSAGE, rep, ".pjt.Inner"),
	)
	mf := func(name string, num int32, entry string) {
		sc.Field = append(sc.Field, field(name, num, descriptorpb.FieldDescriptorProto_TYPE_MESSAGE, rep, ".pjt.Scalars."+entry))
	}
	sc.NestedType = []*descriptorpb.DescriptorProto{
		mapEntry("MStrI32Entry", descriptorpb.FieldDescriptorProto_TYPE_STRING, descriptorpb.FieldDescriptorProto_TYPE_INT32, ""),
		mapEntry("MI32StrEntry", descriptorpb.FieldDescriptorProto_TYPE_INT32, descriptorpb.FieldDescriptorProto_TYPE_STRING, ""),
		mapEntry("MBoolInnerEntry", descriptorpb.FieldDescriptorProto_TYPE_BOOL, descriptorpb.FieldDescriptorProto_TYPE_MESSAGE, ".pjt.Inner"),
		mapEntry("MU64StrEntry", descriptorpb.FieldDescriptorProto_TYPE_UINT64, descriptorpb.FieldDescriptorProto_TYPE_STRING, ""),
		mapEntry("MI64ColorEntry", descriptorpb.FieldDescriptorProto_TYPE_INT64, descriptorpb.FieldDescriptorProto_TYPE_ENUM, ".pjt.Color"),
		mapEntry("MStrBytesEntry", descriptorpb.FieldDescriptorProto_TYPE_STRING, descriptorpb.FieldDescriptorProto_TYPE_BYTES, ""),
		mapEntry("MS32DoubleEntry", descriptorpb.FieldDescriptorProto_TYPE_SINT32, descriptorpb.FieldDescriptorProto_TYPE_DOUBLE, ""),
	}
	mf("m_str_i32", n+4, "MStrI32Entry")
	mf("m_i32_str", n+5, "MI32StrEntry")
	mf("m_bool_inner", n+6, "MBoolInnerEntry")
	mf("m_u64_str", n+7, "MU64StrEntry")
	mf("m_i64_color", n+8, "MI64ColorEntry")
	mf("m_str_bytes", n+9, "MStrBytesEntry")
	mf("m_s32_double", n+10, "MS32DoubleEntry")
	oneofStart := n + 11
	sc.Field = append(sc.Field, field("o_int", oneofStart, descriptorpb.FieldDescriptorProto_TYPE_INT32, opt, ""), field("o_str", oneofStart+1, descriptorpb.FieldDescriptorProto_TYPE_STRING, opt, ""), field("o_msg", oneofStart+2, descriptorpb.FieldDescriptorProto_TYPE_MESSAGE, opt, ".pjt.Inner"))
	for i := len(sc.Field) - 3; i < len(sc.Field); i++ {
		sc.Field[i].OneofIndex = proto.Int32(0)
	}
	sc.OneofDecl = []*descriptorpb.OneofDescriptorProto{{Name: proto.String("choice")}}
	of := field("opt_i32", oneofStart+3, descriptorpb.FieldDescriptorProto_TYPE_INT32, opt, "")
	of.Proto3Optional = proto.Bool(true)
	of.OneofIndex = proto.Int32(1)
	sc.Field = append(sc.Field, of)
	sc.OneofDecl = append(sc.OneofDecl, &descriptorpb.OneofDescriptorProto{Name: proto.String("_opt_i32")})

	wk := &descriptorpb.DescriptorProto{Name: proto.String("Wkt")}
	for i, w := range []struct{ n, t string }{
		{"ts", "google.protobuf.Timestamp"}, {"dur", "google.protobuf.Duration"}, {"mask", "google.protobuf.FieldMask"},
		{"st", "google.protobuf.Struct"}, {"val", "google.protobuf.Value"}, {"list", "google.protobuf.ListValue"},
		{"any", "google.protobuf.Any"}, {"empty", "google.protobuf.Empty"},
		{"w_double", "google.protobuf.DoubleValue"}, {"w_float", "google.protobuf.FloatValue"}, {"w_int64", "google.protobuf.Int64Value"},
		{"w_uint64", "google.protobuf.UInt64Value"}, {"w_int32", "google.protobuf.Int32Value"}, {"w_uint32", "google.protobuf.UInt32Value"},
		{"w_bool", "google.protobuf.BoolValue"}, {"w_string", "google.protobuf.StringValue"}, {"w_bytes", "google.protobuf.BytesValue"},
	} {
		wk.Field = append(wk.Field, field(w.n, int32(i+1), descriptorpb.FieldDescriptorProto_TYPE_MESSAGE, opt, "."+w.t))
	}
	wk.Field = append(wk.Field, field("tss", 30, descriptorpb.FieldDescriptorProto_TYPE_MESSAGE, rep, ".google.protobuf.Timestamp"))
	fd := &descriptorpb.FileDescriptorProto{
		Name: proto.String("pjtest.proto"), Package: proto.String("pjt"), Syntax: proto.String("proto3"), Dependency: deps,
		EnumType: []*descriptorpb.EnumDescriptorProto{{Name: proto.String("Color"), Value: []*descriptorpb.EnumValueDescriptorProto{
			{Name: proto.String("RED"), Number: proto.Int32(0)}, {Name: proto.String("GREEN"), Number: proto.Int32(1)}, {Name: proto.String("BLUE"), Number: proto.Int32(2)}}}},
		MessageType: []*descriptorpb.DescriptorProto{inner, sc, wk},
	}
	set := &descriptorpb.FileDescriptorSet{}
	for _, d := range deps {
		f, err := protoregistry.GlobalFiles.FindFileByPath(d)
		if err != nil {
			panic(err)
		}
		set.File = append(set.File, protodesc.ToFileDescriptorProto(f))
	}
	set.File = append(set.File, fd)
	// a proto2 file with required fields, plain and nested (Go's protojson reports a missing one)
	req := descriptorpb.FieldDescriptorProto_LABEL_REQUIRED
	leaf := &descriptorpb.DescriptorProto{Name: proto.String("Leaf"), Field: []*descriptorpb.FieldDescriptorProto{
		field("a", 1, descriptorpb.FieldDescriptorProto_TYPE_INT32, req, ""),
		field("b", 2, descriptorpb.FieldDescriptorProto_TYPE_STRING, opt, ""),
	}}
	rq := &descriptorpb.DescriptorProto{Name: proto.String("Req"), Field: []*descriptorpb.FieldDescriptorProto{
		field("id", 1, descriptorpb.FieldDescriptorProto_TYPE_INT32, req, ""),
		field("name", 2, descriptorpb.FieldDescriptorProto_TYPE_STRING, opt, ""),
		field("leaf", 3, descriptorpb.FieldDescriptorProto_TYPE_MESSAGE, req, ".pjt2.Leaf"),
		field("leaves", 4, descriptorpb.FieldDescriptorProto_TYPE_MESSAGE, rep, ".pjt2.Leaf"),
		field("opt_leaf", 5, descriptorpb.FieldDescriptorProto_TYPE_MESSAGE, opt, ".pjt2.Leaf"),
	}}
	set.File = append(set.File, &descriptorpb.FileDescriptorProto{
		Name: proto.String("pjtest2.proto"), Package: proto.String("pjt2"), Syntax: proto.String("proto2"),
		MessageType: []*descriptorpb.DescriptorProto{leaf, rq},
	})
	return set
}

type gen struct {
	r   *rand.Rand
	reg *protoregistry.Files
}

func (g *gen) scalar(fd protoreflect.FieldDescriptor) protoreflect.Value {
	r := g.r
	big := []int64{0, 1, -1, 127, -128, 32767, math.MaxInt32, math.MinInt32, math.MaxInt64, math.MinInt64, r.Int63(), -r.Int63()}
	switch fd.Kind() {
	case protoreflect.BoolKind:
		return protoreflect.ValueOfBool(r.Intn(2) == 0)
	case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Sfixed32Kind:
		return protoreflect.ValueOfInt32(int32(big[r.Intn(len(big))]))
	case protoreflect.Uint32Kind, protoreflect.Fixed32Kind:
		return protoreflect.ValueOfUint32(uint32(big[r.Intn(len(big))]))
	case protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Sfixed64Kind:
		return protoreflect.ValueOfInt64(big[r.Intn(len(big))])
	case protoreflect.Uint64Kind, protoreflect.Fixed64Kind:
		return protoreflect.ValueOfUint64(uint64(big[r.Intn(len(big))]))
	case protoreflect.FloatKind:
		fs := []float32{0, 1, -1, 0.5, 3.4028235e38, 1e-10, float32(math.Inf(1)), float32(math.Inf(-1)), float32(math.NaN()), 0.1, 16777216}
		return protoreflect.ValueOfFloat32(fs[r.Intn(len(fs))])
	case protoreflect.DoubleKind:
		fs := []float64{0, 1, -1, 0.5, 1e300, 1e-300, 1e21, 1e20, 1e-6, 1e-7, math.Inf(1), math.Inf(-1), math.NaN(), 0.1, 123456789.125, math.MaxFloat64, 5e-324}
		return protoreflect.ValueOfFloat64(fs[r.Intn(len(fs))])
	case protoreflect.StringKind:
		ss := []string{"", "a", "héllo", "日本語", "with \"quotes\" and \\ and \n newline", "<tag>&amp;", "  ", "tab\there", "\x01\x1f", "😀"}
		return protoreflect.ValueOfString(ss[r.Intn(len(ss))])
	case protoreflect.BytesKind:
		b := make([]byte, r.Intn(10))
		r.Read(b)
		return protoreflect.ValueOfBytes(b)
	case protoreflect.EnumKind:
		return protoreflect.ValueOfEnum(protoreflect.EnumNumber([]int{0, 1, 2, 2, 7, -1}[r.Intn(6)]))
	}
	panic("kind")
}

func (g *gen) value(fd protoreflect.FieldDescriptor, depth int) protoreflect.Value {
	if fd.Kind() == protoreflect.MessageKind || fd.Kind() == protoreflect.GroupKind {
		return protoreflect.ValueOfMessage(g.msg(fd.Message(), depth+1))
	}
	return g.scalar(fd)
}

func (g *gen) msg(md protoreflect.MessageDescriptor, depth int) protoreflect.Message {
	m := dynamicpb.NewMessage(md)
	switch md.FullName() {
	case "google.protobuf.Timestamp":
		secs := []int64{0, 1, -1, 1700000000, -62135596800, 253402300799, 951782400, 1}
		nanos := []int32{0, 1, 1000, 1000000, 999999999, 120000000, 500000000}
		m.Set(md.Fields().ByName("seconds"), protoreflect.ValueOfInt64(secs[g.r.Intn(len(secs))]))
		m.Set(md.Fields().ByName("nanos"), protoreflect.ValueOfInt32(nanos[g.r.Intn(len(nanos))]))
		return m
	case "google.protobuf.Duration":
		secs := []int64{0, 1, -1, 3600, 315576000000, -315576000000, 59}
		nanos := []int32{0, 1, 1000, 1000000, 999999999, 500000000}
		s := secs[g.r.Intn(len(secs))]
		n := nanos[g.r.Intn(len(nanos))]
		if s < 0 {
			n = -n
		}
		m.Set(md.Fields().ByName("seconds"), protoreflect.ValueOfInt64(s))
		m.Set(md.Fields().ByName("nanos"), protoreflect.ValueOfInt32(n))
		return m
	case "google.protobuf.FieldMask":
		paths := []string{"a", "foo_bar", "a.b_c.d", "x"}
		l := m.Mutable(md.Fields().ByName("paths")).List()
		for i, k := 0, g.r.Intn(4); i < k; i++ {
			l.Append(protoreflect.ValueOfString(paths[g.r.Intn(len(paths))]))
		}
		return m
	case "google.protobuf.Struct", "google.protobuf.Value", "google.protobuf.ListValue":
		v, _ := structpb.NewValue(randJSON(g.r, 0))
		switch md.FullName() {
		case "google.protobuf.Value":
			return v.ProtoReflect()
		case "google.protobuf.Struct":
			s, _ := structpb.NewStruct(map[string]any{"a": randJSON(g.r, 1), "b_c": randJSON(g.r, 1)})
			return s.ProtoReflect()
		default:
			l, _ := structpb.NewList([]any{randJSON(g.r, 1), randJSON(g.r, 1)})
			return l.ProtoReflect()
		}
	case "google.protobuf.Any":
		var inner proto.Message
		switch g.r.Intn(4) {
		case 0:
			inner = timestamppb.New(timestamppb.Now().AsTime().Truncate(1e9))
			inner = &timestamppb.Timestamp{Seconds: 1700000000, Nanos: 120000000}
		case 1:
			inner = &durationpb.Duration{Seconds: 3, Nanos: 500000000}
		case 2:
			md2, _ := g.reg.FindDescriptorByName("pjt.Inner")
			im := g.msg(md2.(protoreflect.MessageDescriptor), depth+1)
			inner = im.Interface()
		default:
			inner = wrapperspb.Int64(1 << 40)
		}
		// Deterministic: the field order of a dynamic message otherwise changes from run to run.
		a := &anypb.Any{}
		if err := anypb.MarshalFrom(a, inner, proto.MarshalOptions{Deterministic: true}); err != nil {
			panic(err)
		}
		b, _ := proto.MarshalOptions{Deterministic: true}.Marshal(a)
		proto.Unmarshal(b, m.Interface())
		return m
	case "google.protobuf.Empty":
		return m
	}
	if strings.HasPrefix(string(md.FullName()), "google.protobuf.") {
		// wrappers
		m.Set(md.Fields().ByName("value"), g.scalar(md.Fields().ByName("value")))
		return m
	}
	fields := md.Fields()
	for i := 0; i < fields.Len(); i++ {
		fd := fields.Get(i)
		if depth > 2 && fd.Kind() == protoreflect.MessageKind {
			continue
		}
		if g.r.Intn(3) == 0 {
			continue
		}
		if od := fd.ContainingOneof(); od != nil && !od.IsSynthetic() && m.WhichOneof(od) != nil && g.r.Intn(2) == 0 {
			continue
		}
		switch {
		case fd.IsMap():
			mp := m.Mutable(fd).Map()
			for k, n := 0, g.r.Intn(4); k < n; k++ {
				var key protoreflect.MapKey
				switch fd.MapKey().Kind() {
				case protoreflect.StringKind:
					key = protoreflect.ValueOfString([]string{"a", "b", "", "k y", "日"}[g.r.Intn(5)]).MapKey()
				case protoreflect.BoolKind:
					key = protoreflect.ValueOfBool(g.r.Intn(2) == 0).MapKey()
				default:
					key = g.scalar(fd.MapKey()).MapKey()
				}
				mp.Set(key, g.value(fd.MapValue(), depth))
			}
		case fd.IsList():
			l := m.Mutable(fd).List()
			for k, n := 0, g.r.Intn(4); k < n; k++ {
				l.Append(g.value(fd, depth))
			}
		default:
			m.Set(fd, g.value(fd, depth))
		}
	}
	return m
}

func randJSON(r *rand.Rand, depth int) any {
	n := 6
	if depth >= 2 {
		n = 4
	}
	switch r.Intn(n) {
	case 0:
		return nil
	case 1:
		return []float64{0, 1, -1, 1.5, 1e21, 1e-7, 123456789012}[r.Intn(7)]
	case 2:
		return []string{"", "x", "héllo", "a\"b"}[r.Intn(4)]
	case 3:
		return r.Intn(2) == 0
	case 4:
		return map[string]any{"k": randJSON(r, depth+1), "": randJSON(r, depth+1)}
	}
	return []any{randJSON(r, depth+1), randJSON(r, depth+1)}
}

func compact(b []byte) string {
	var o bytes.Buffer
	if err := json.Compact(&o, b); err != nil {
		panic(err)
	}
	return o.String()
}

func q(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '\\':
			b.WriteString(`\\`)
		case c == '"':
			b.WriteString(`\"`)
		case c == '$':
			b.WriteString(`\$`)
		case c == '\n':
			b.WriteString(`\n`)
		case c == '\r':
			b.WriteString(`\r`)
		case c == '\t':
			b.WriteString(`\t`)
		case c < 0x20 || c == 0x7f:
			fmt.Fprintf(&b, `\u{%x}`, c)
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}

// resolver finds the schema's messages for Any, as Xo's registry does.
var resolver *dynamicpb.Types

func opts(flags int) protojson.MarshalOptions {
	return protojson.MarshalOptions{Resolver: resolver, UseProtoNames: flags&1 != 0, EmitDefaultValues: flags&2 != 0, UseEnumNumbers: flags&4 != 0}
}

func main() {
	set := schema()
	files, err := protodesc.NewFiles(set)
	if err != nil {
		panic(err)
	}
	resolver = dynamicpb.NewTypes(files)
	g := &gen{r: rand.New(rand.NewSource(5460)), reg: files}
	var o strings.Builder
	o.WriteString("// Generated by scripts/stdcorpus from protobuf-go v1.36.12's protojson over a schema with every field kind and the well known types, with seeded random messages from dynamicpb (" + os.Args[1] + ").\n")
	o.WriteString("// Do not edit: regenerate with go run ./scripts/stdcorpus.\n")
	o.WriteString("use std/protojson\n\n")
	sb, err := proto.MarshalOptions{Deterministic: true}.Marshal(set)
	if err != nil {
		panic(err)
	}
	fmt.Fprintf(&o, "/// the schema: a FileDescriptorSet, files in dependency order, as hex\nconst SCHEMA: Str = \"%x\"\n\n", sb)
	o.WriteString("/// (message full name, flags: 1 proto names, 2 emit defaults, 4 enum numbers; wire hex, Go's protojson output compacted)\nconst MARSHAL: List[(Str, Int, Str, Str)] = [\n")
	names := []string{"pjt.Scalars", "pjt.Wkt", "pjt.Inner"}
	var rows []string
	type unm struct{ name, in, wire string }
	var umRows []unm
	for i := 0; i < 400; i++ {
		name := names[g.r.Intn(len(names))]
		d, _ := files.FindDescriptorByName(protoreflect.FullName(name))
		m := g.msg(d.(protoreflect.MessageDescriptor), 0)
		wire, err := proto.MarshalOptions{Deterministic: true}.Marshal(m.Interface())
		if err != nil {
			continue
		}
		for _, fl := range []int{0, g.r.Intn(8)} {
			js, err := opts(fl).Marshal(m.Interface())
			if err != nil {
				continue
			}
			rows = append(rows, fmt.Sprintf("  (\"%s\", %d, \"%x\", \"%s\"),\n", name, fl, wire, q(compact(js))))
		}
		// unmarshal inputs: Go's own output, and the same with a proto name or a number form swapped
		js, err := opts(0).Marshal(m.Interface())
		if err != nil {
			continue
		}
		umRows = append(umRows, unm{name, compact(js), fmt.Sprintf("%x", wire)})
		if js2, err := opts(1).Marshal(m.Interface()); err == nil {
			umRows = append(umRows, unm{name, compact(js2), fmt.Sprintf("%x", wire)})
		}
	}
	for _, r := range rows {
		o.WriteString(r)
	}
	o.WriteString("]\n\n")
	// curated unmarshal inputs, results from Go
	cur := []struct{ name, in string }{
		{"pjt.Scalars", `{}`}, {"pjt.Scalars", `null`}, {"pjt.Scalars", `{"sInt32":null}`}, {"pjt.Scalars", `{"sInt32":"12"}`}, {"pjt.Scalars", `{"sInt32":12.0}`}, {"pjt.Scalars", `{"sInt32":1e2}`},
		{"pjt.Scalars", `{"sInt32":1.5}`}, {"pjt.Scalars", `{"sInt32":2147483648}`}, {"pjt.Scalars", `{"sInt64":"9223372036854775807"}`}, {"pjt.Scalars", `{"sInt64":"9223372036854775808"}`},
		{"pjt.Scalars", `{"sInt64":9007199254740993}`}, {"pjt.Scalars", `{"sUint64":"18446744073709551615"}`}, {"pjt.Scalars", `{"sUint64":-1}`}, {"pjt.Scalars", `{"sDouble":"NaN"}`},
		{"pjt.Scalars", `{"sDouble":"Infinity"}`}, {"pjt.Scalars", `{"sDouble":"-Infinity"}`}, {"pjt.Scalars", `{"sDouble":"1.5"}`}, {"pjt.Scalars", `{"sDouble":1e400}`}, {"pjt.Scalars", `{"sFloat":3.5e38}`},
		{"pjt.Scalars", `{"sFloat":1e-50}`}, {"pjt.Scalars", `{"sBool":true}`}, {"pjt.Scalars", `{"sBool":"true"}`}, {"pjt.Scalars", `{"sBool":1}`}, {"pjt.Scalars", `{"sString":"x"}`},
		{"pjt.Scalars", `{"sString":1}`}, {"pjt.Scalars", `{"sString":"😀"}`}, {"pjt.Scalars", `{"sString":"\ud83d"}`}, {"pjt.Scalars", `{"sBytes":"aGk="}`}, {"pjt.Scalars", `{"sBytes":"aGk"}`},
		{"pjt.Scalars", `{"sBytes":"a-_b"}`}, {"pjt.Scalars", `{"sBytes":"a+/b"}`}, {"pjt.Scalars", `{"sBytes":"!!"}`}, {"pjt.Scalars", `{"color":"GREEN"}`}, {"pjt.Scalars", `{"color":1}`}, {"pjt.Scalars", `{"color":"NOPE"}`},
		{"pjt.Scalars", `{"color":7}`}, {"pjt.Scalars", `{"color":"1"}`}, {"pjt.Scalars", `{"colors":["RED",2,"BLUE"]}`}, {"pjt.Scalars", `{"colors":null}`}, {"pjt.Scalars", `{"colors":[null]}`},
		{"pjt.Scalars", `{"s_int32":5}`}, {"pjt.Scalars", `{"sInt32":5,"s_int32":6}`}, {"pjt.Scalars", `{"sInt32":5,"sInt32":6}`}, {"pjt.Scalars", `{"unknown":1}`}, {"pjt.Scalars", `{"inner":{"id":1,"next":{"id":2}}}`},
		{"pjt.Scalars", `{"inner":null}`}, {"pjt.Scalars", `{"inner":[]}`}, {"pjt.Scalars", `{"mStrI32":{"a":1,"b":"2"}}`}, {"pjt.Scalars", `{"mStrI32":{"a":1,"a":2}}`}, {"pjt.Scalars", `{"mI32Str":{"1":"a","-2":"b"}}`},
		{"pjt.Scalars", `{"mI32Str":{"x":"a"}}`}, {"pjt.Scalars", `{"mBoolInner":{"true":{"id":1},"false":{}}}`}, {"pjt.Scalars", `{"mBoolInner":{"yes":{}}}`}, {"pjt.Scalars", `{"mU64Str":{"18446744073709551615":"x"}}`},
		{"pjt.Scalars", `{"mI64Color":{"1":"BLUE","2":1}}`}, {"pjt.Scalars", `{"mStrBytes":{"k":"AAEC"}}`}, {"pjt.Scalars", `{"mS32Double":{"-1":"NaN","2":1.5}}`}, {"pjt.Scalars", `{"oInt":1,"oStr":"x"}`},
		{"pjt.Scalars", `{"oInt":1,"oStr":null}`}, {"pjt.Scalars", `{"oMsg":{"id":3}}`}, {"pjt.Scalars", `{"optI32":0}`}, {"pjt.Scalars", `{"optI32":null}`}, {"pjt.Scalars", ` { "sInt32" : 1 } `},
		{"pjt.Scalars", `{"sInt32":1,}`}, {"pjt.Scalars", `{"sInt32":01}`}, {"pjt.Scalars", `{"sInt32":+1}`}, {"pjt.Scalars", `{"sInt32":-0}`}, {"pjt.Scalars", `[]`}, {"pjt.Scalars", `1`}, {"pjt.Scalars", `{`}, {"pjt.Scalars", ``},
		{"pjt.Scalars", `{"sInt32":1}x`}, {"pjt.Scalars", `{"sString":"a\u0000b"}`}, {"pjt.Scalars", `{"sString":"\x"}`}, {"pjt.Scalars", `{"sString":"tab	here"}`}, {"pjt.Scalars", `{"sInt32":-2147483648}`}, {"pjt.Scalars", `{"sSint64":"-9223372036854775808"}`}, {"pjt.Scalars", `{"sSint64":"9223372036854775807"}`}, {"pjt.Scalars", `{"sSint64":"-1"}`},
		{"pjt.Wkt", `{"ts":"2023-11-14T22:13:20Z"}`}, {"pjt.Wkt", `{"ts":"2023-11-14T22:13:20.120Z"}`}, {"pjt.Wkt", `{"ts":"2023-11-14T22:13:20.123456789Z"}`}, {"pjt.Wkt", `{"ts":"2023-11-14T22:13:20+01:00"}`},
		{"pjt.Wkt", `{"ts":"2023-11-14t22:13:20z"}`}, {"pjt.Wkt", `{"ts":"2023-11-14 22:13:20Z"}`}, {"pjt.Wkt", `{"ts":"0001-01-01T00:00:00Z"}`}, {"pjt.Wkt", `{"ts":"9999-12-31T23:59:59.999999999Z"}`},
		{"pjt.Wkt", `{"ts":"0000-12-31T23:59:59Z"}`}, {"pjt.Wkt", `{"ts":"2023-02-30T00:00:00Z"}`}, {"pjt.Wkt", `{"ts":1}`}, {"pjt.Wkt", `{"ts":null}`}, {"pjt.Wkt", `{"dur":"1.5s"}`}, {"pjt.Wkt", `{"dur":"-1.5s"}`},
		{"pjt.Wkt", `{"dur":"3600s"}`}, {"pjt.Wkt", `{"dur":"0.000000001s"}`}, {"pjt.Wkt", `{"dur":"1s "}`}, {"pjt.Wkt", `{"dur":"1"}`}, {"pjt.Wkt", `{"dur":"315576000001s"}`}, {"pjt.Wkt", `{"dur":"1.0000000001s"}`},
		{"pjt.Wkt", `{"mask":"fooBar,a.bC"}`}, {"pjt.Wkt", `{"mask":""}`}, {"pjt.Wkt", `{"mask":"foo_bar"}`}, {"pjt.Wkt", `{"st":{"a":1,"b":[1,"x",null,true],"c":{}}}`}, {"pjt.Wkt", `{"st":null}`}, {"pjt.Wkt", `{"st":[]}`},
		{"pjt.Wkt", `{"val":null}`}, {"pjt.Wkt", `{"val":1.5}`}, {"pjt.Wkt", `{"val":"s"}`}, {"pjt.Wkt", `{"val":[1,[2]]}`}, {"pjt.Wkt", `{"list":[1,2]}`}, {"pjt.Wkt", `{"list":{}}`}, {"pjt.Wkt", `{"empty":{}}`}, {"pjt.Wkt", `{"empty":{"a":1}}`},
		{"pjt.Wkt", `{"wDouble":1.5}`}, {"pjt.Wkt", `{"wDouble":"NaN"}`}, {"pjt.Wkt", `{"wInt64":"5"}`}, {"pjt.Wkt", `{"wInt64":5}`}, {"pjt.Wkt", `{"wUint32":4294967295}`}, {"pjt.Wkt", `{"wBool":false}`}, {"pjt.Wkt", `{"wString":""}`},
		{"pjt.Wkt", `{"wBytes":"AQI="}`}, {"pjt.Wkt", `{"wInt32":null}`}, {"pjt.Wkt", `{"wInt32":{"value":1}}`}, {"pjt.Wkt", `{"any":{"@type":"type.googleapis.com/google.protobuf.Duration","value":"1s"}}`},
		{"pjt.Wkt", `{"any":{"@type":"type.googleapis.com/pjt.Inner","id":5,"label":"x"}}`}, {"pjt.Wkt", `{"any":{"id":5,"@type":"type.googleapis.com/pjt.Inner"}}`}, {"pjt.Wkt", `{"any":{"@type":"type.googleapis.com/nope.X"}}`},
		{"pjt.Wkt", `{"any":{"@type":"pjt.Inner","id":1}}`}, {"pjt.Wkt", `{"any":{"@type":"type.googleapis.com/google.protobuf.Timestamp","value":"2023-11-14T22:13:20Z"}}`}, {"pjt.Wkt", `{"any":{"@type":"type.googleapis.com/google.protobuf.Empty"}}`},
		{"pjt.Wkt", `{"any":{"@type":"type.googleapis.com/google.protobuf.Value","value":[1,2]}}`}, {"pjt.Wkt", `{"any":{}}`}, {"pjt.Wkt", `{"tss":["2023-11-14T22:13:20Z",null]}`}, {"pjt.Wkt", `{"tss":null}`},
	}
	o.WriteString("/// (message full name, JSON input, wire hex of Go's result, or the text `error`)\nconst UNMARSHAL: List[(Str, Str, Str)] = [\n")
	emitU := func(name, in string) {
		d, _ := files.FindDescriptorByName(protoreflect.FullName(name))
		m := dynamicpb.NewMessage(d.(protoreflect.MessageDescriptor))
		res := "error"
		if err := (protojson.UnmarshalOptions{Resolver: resolver}).Unmarshal([]byte(in), m); err == nil {
			b, err := proto.MarshalOptions{Deterministic: true}.Marshal(m)
			if err != nil {
				panic(err)
			}
			res = fmt.Sprintf("%x", b)
		}
		fmt.Fprintf(&o, "  (\"%s\", \"%s\", \"%s\"),\n", name, q(in), res)
	}
	cur = append(cur, []struct{ name, in string }{
		{"pjt2.Req", `{"id":1,"leaf":{"a":2}}`}, {"pjt2.Req", `{"id":1}`}, {"pjt2.Req", `{"leaf":{"a":2}}`}, {"pjt2.Req", `{"id":1,"leaf":{}}`},
		{"pjt2.Req", `{"id":1,"leaf":{"a":2},"leaves":[{"a":3},{"b":"x"}]}`}, {"pjt2.Req", `{"id":1,"leaf":{"a":2},"optLeaf":{"b":"x"}}`},
		{"pjt2.Req", `{"id":null,"leaf":{"a":2}}`}, {"pjt2.Req", `{"id":1,"leaf":null}`}, {"pjt2.Req", `{}`}, {"pjt2.Leaf", `{"a":1,"b":"x"}`},
		{"pjt2.Leaf", `{"b":"x"}`}, {"pjt2.Req", `{"id":1,"leaf":{"a":2},"name":"n"}`},
	}...)
	for _, c := range cur {
		emitU(c.name, c.in)
	}
	for _, u := range umRows {
		emitU(u.name, u.in)
	}
	o.WriteString("]\n\n")
	// normalize: Go's proto.Unmarshal then proto.Marshal on a dynamic message, with unknown fields before, after
	// and between the known ones, and a known field sent with the wrong wire type
	o.WriteString("/// (message full name, wire hex in, wire hex of Go's proto.Unmarshal then Marshal, or the text `error`)\nconst NORMALIZE: List[(Str, Str, Str)] = [\n")
	unk := func(k int) []byte {
		var b []byte
		switch k % 4 {
		case 0:
			b = protowire.AppendTag(b, protowire.Number(1000+k), protowire.VarintType)
			b = protowire.AppendVarint(b, uint64(300+k))
		case 1:
			b = protowire.AppendTag(b, protowire.Number(1000+k), protowire.BytesType)
			b = protowire.AppendBytes(b, []byte("unk"))
		case 2:
			b = protowire.AppendTag(b, protowire.Number(1000+k), protowire.Fixed32Type)
			b = protowire.AppendFixed32(b, 0xdeadbeef)
		default:
			b = protowire.AppendTag(b, protowire.Number(1000+k), protowire.Fixed64Type)
			b = protowire.AppendFixed64(b, 0x0123456789abcdef)
		}
		return b
	}
	jsonCarries := func(name string, in []byte) bool {
		d, _ := files.FindDescriptorByName(protoreflect.FullName(name))
		md := d.(protoreflect.MessageDescriptor)
		m := dynamicpb.NewMessage(md)
		if proto.Unmarshal(in, m) != nil {
			return false
		}
		want, err := proto.MarshalOptions{Deterministic: true}.Marshal(m)
		if err != nil {
			return false
		}
		js, err := (protojson.MarshalOptions{Resolver: resolver}).Marshal(m)
		if err != nil {
			return false
		}
		m2 := dynamicpb.NewMessage(md)
		if (protojson.UnmarshalOptions{Resolver: resolver}).Unmarshal(js, m2) != nil {
			return false
		}
		got, err := proto.MarshalOptions{Deterministic: true}.Marshal(m2)
		return err == nil && bytes.Equal(got, want)
	}
	emitN := func(name string, in []byte) {
		d, _ := files.FindDescriptorByName(protoreflect.FullName(name))
		m := dynamicpb.NewMessage(d.(protoreflect.MessageDescriptor))
		res := "error"
		if err := (proto.UnmarshalOptions{Resolver: resolver}).Unmarshal(in, m); err == nil {
			b, err := proto.MarshalOptions{Deterministic: true}.Marshal(m)
			if err != nil {
				panic(err)
			}
			res = fmt.Sprintf("%x", b)
		}
		fmt.Fprintf(&o, "  (\"%s\", \"%x\", \"%s\"),\n", name, in, res)
	}
	for i := 0; i < 300; i++ {
		name := names[g.r.Intn(len(names))]
		d, _ := files.FindDescriptorByName(protoreflect.FullName(name))
		md := d.(protoreflect.MessageDescriptor)
		m := g.msg(md, 0)
		wire, err := proto.MarshalOptions{Deterministic: true}.Marshal(m.Interface())
		if err != nil {
			continue
		}
		// normalize goes through the JSON mapping for the known fields, so it is exact only for a message that
		// mapping carries without loss (an Any with no type URL, for one, is not): leave the others out
		if !jsonCarries(name, wire) {
			continue
		}
		// normalize goes through the JSON mapping for the known fields, so it is exact only for a message that
		// mapping carries without loss (an Any with no type URL, for one, is not): leave the others out
		if !jsonCarries(name, wire) {
			continue
		}
		u := unk(i)
		emitN(name, wire)
		emitN(name, append(append([]byte{}, wire...), u...))
		emitN(name, append(append([]byte{}, u...), wire...))
		emitN(name, append(append(append([]byte{}, u...), wire...), unk(i+1)...))
		// a known field with the wrong wire type is unknown
		if md.Fields().Len() > 0 {
			f0 := md.Fields().Get(g.r.Intn(md.Fields().Len()))
			var w []byte
			if f0.Kind() == protoreflect.StringKind || f0.Kind() == protoreflect.BytesKind || f0.Kind() == protoreflect.MessageKind {
				w = protowire.AppendTag(w, protowire.Number(f0.Number()), protowire.VarintType)
				w = protowire.AppendVarint(w, 7)
			} else {
				w = protowire.AppendTag(w, protowire.Number(f0.Number()), protowire.BytesType)
				w = protowire.AppendBytes(w, []byte("xy"))
			}
			emitN(name, append(append([]byte{}, wire...), w...))
		}
		if len(wire) > 1 && i%5 == 0 {
			emitN(name, wire[:len(wire)-1])
		}
	}
	emitN("pjt.Scalars", nil)
	emitN("pjt.Scalars", unk(0))
	emitN("pjt.Scalars", []byte{0x08})
	o.WriteString("]\n\n")
	// proto2 messages with required fields: the wire is made with AllowPartial, so a missing required field is
	// on the wire as such; Go's protojson.Marshal either writes the JSON or errors
	o.WriteString("/// (message full name, wire hex, Go's protojson output compacted, or the text `error` for a missing required field)\nconst MARSHAL2: List[(Str, Str, Str)] = [\n")
	rdd, _ := files.FindDescriptorByName("pjt2.Req")
	ldd, _ := files.FindDescriptorByName("pjt2.Leaf")
	rd, ld := rdd.(protoreflect.MessageDescriptor), ldd.(protoreflect.MessageDescriptor)
	mk := func(md protoreflect.MessageDescriptor, set func(m *dynamicpb.Message)) {
		m := dynamicpb.NewMessage(md)
		set(m)
		wire, err := proto.MarshalOptions{Deterministic: true, AllowPartial: true}.Marshal(m)
		if err != nil {
			panic(err)
		}
		res := "error"
		if js, err := (protojson.MarshalOptions{Resolver: resolver}).Marshal(m); err == nil {
			res = q(compact(js))
		}
		fmt.Fprintf(&o, "  (\"%s\", \"%x\", \"%s\"),\n", md.FullName(), wire, res)
	}
	field_ := func(md protoreflect.MessageDescriptor, n string) protoreflect.FieldDescriptor {
		return md.Fields().ByName(protoreflect.Name(n))
	}
	leafOf := func(a int32, hasA bool, b string) *dynamicpb.Message {
		l := dynamicpb.NewMessage(ld)
		if hasA {
			l.Set(field_(ld, "a"), protoreflect.ValueOfInt32(a))
		}
		if b != "" {
			l.Set(field_(ld, "b"), protoreflect.ValueOfString(b))
		}
		return l
	}
	mk(rd, func(m *dynamicpb.Message) {
		m.Set(field_(rd, "id"), protoreflect.ValueOfInt32(1))
		m.Set(field_(rd, "leaf"), protoreflect.ValueOfMessage(leafOf(2, true, "")))
	})
	mk(rd, func(m *dynamicpb.Message) { m.Set(field_(rd, "id"), protoreflect.ValueOfInt32(1)) })
	mk(rd, func(m *dynamicpb.Message) {
		m.Set(field_(rd, "leaf"), protoreflect.ValueOfMessage(leafOf(2, true, "")))
	})
	mk(rd, func(m *dynamicpb.Message) {
		m.Set(field_(rd, "id"), protoreflect.ValueOfInt32(1))
		m.Set(field_(rd, "leaf"), protoreflect.ValueOfMessage(leafOf(0, false, "x")))
	})
	mk(rd, func(m *dynamicpb.Message) {
		m.Set(field_(rd, "id"), protoreflect.ValueOfInt32(1))
		m.Set(field_(rd, "leaf"), protoreflect.ValueOfMessage(leafOf(2, true, "")))
		ls := m.Mutable(field_(rd, "leaves")).List()
		ls.Append(protoreflect.ValueOfMessage(leafOf(3, true, "y")))
		ls.Append(protoreflect.ValueOfMessage(leafOf(0, false, "z")))
	})
	mk(rd, func(m *dynamicpb.Message) {
		m.Set(field_(rd, "id"), protoreflect.ValueOfInt32(1))
		m.Set(field_(rd, "leaf"), protoreflect.ValueOfMessage(leafOf(2, true, "")))
		m.Set(field_(rd, "opt_leaf"), protoreflect.ValueOfMessage(leafOf(0, false, "w")))
	})
	mk(rd, func(m *dynamicpb.Message) {})
	mk(ld, func(m *dynamicpb.Message) { m.Set(field_(ld, "a"), protoreflect.ValueOfInt32(7)) })
	mk(ld, func(m *dynamicpb.Message) { m.Set(field_(ld, "b"), protoreflect.ValueOfString("q")) })
	o.WriteString("]\n")
	o.WriteString(xoTests)
	os.Stdout.WriteString(o.String())
}

const xoTests = `
fn hex(b: Bytes) -> Str {
  let digits = "0123456789abcdef".bytes()
  var o = BytesBuilder.new()
  for c in b {
    o.write_byte(digits[Int(c) / 16])
    o.write_byte(digits[Int(c) % 16])
  }
  o.freeze().to_str_lossy()
}

fn unhex(s: Str) -> Bytes {
  var o = BytesBuilder.new()
  let b = s.bytes()
  var i = 0
  while i + 1 < b.len() {
    o.write_byte(UInt8.wrapping_from(nib(Int(b[i])) * 16 + nib(Int(b[i + 1]))))
    i += 2
  }
  o.freeze()
}

fn nib(c: Int) -> Int { if c >= 97 { c - 87 } else { c - 48 } }

fn registry() -> protojson.Registry {
  protojson.registry_from_set(unhex(SCHEMA)).or_fault("schema")
}

fn opts(flags: Int) -> protojson.Opts {
  protojson.Opts{proto_names: flags & 1 != 0, emit_defaults: flags & 2 != 0, enum_numbers: flags & 4 != 0, discard_unknown: false}
}

test "marshal equals Go's protojson" {
  let r = registry()
  for (name, flags, wire, want) in MARSHAL {
    let got = protojson.marshal(r, name, unhex(wire), opts(flags)).or_fault("marshal " + name)
    if got != want {
      expect("${name} ${flags} ${wire}: ${got}") == "${name} ${flags} ${wire}: ${want}"
    }
  }
}

test "marshal of proto2 required fields equals Go's protojson, a missing one an error" {
  let r = registry()
  for (name, wire, want) in MARSHAL2 {
    let got = match protojson.marshal(r, name, unhex(wire), opts(0)) {
      Ok(j) => j
      Err(_) => "error"
    }
    if got != want {
      expect("${name} ${wire}: ${got}") == "${name} ${wire}: ${want}"
    }
  }
}

test "normalize equals Go's proto.Unmarshal then Marshal, unknown fields kept" {
  let r = registry()
  for (name, input, want) in NORMALIZE {
    let got = match protojson.normalize(r, name, unhex(input)) {
      Ok(b) => hex(b)
      Err(_) => "error"
    }
    if got != want {
      expect("${name} ${input}: ${got}") == "${name} ${input}: ${want}"
    }
  }
}

test "unmarshal equals Go's protojson, errors included" {
  let r = registry()
  for (name, input, want) in UNMARSHAL {
    let got = match protojson.unmarshal(r, name, input, opts(0)) {
      Ok(b) => hex(b)
      Err(_) => "error"
    }
    if got != want {
      expect("${name} ${input}: ${got}") == "${name} ${input}: ${want}"
    }
  }
}
`
