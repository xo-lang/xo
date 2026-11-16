// Command prototextoracle prints the std/prototext corpus: protobuf-go
// v1.36.12's prototext (Marshal and Format) over std/protojson's test schema
// (every field kind, proto2, proto3 and editions, maps of every key kind,
// oneofs, groups, closed enums, the well known types and Any), with seeded
// random messages built through dynamicpb, their wire bytes parsed back the
// way Go's Unmarshal does (so a closed enum's unknown value is an unknown
// field) and changed with unknown and repeated fields. The schema travels
// as a FileDescriptorSet. It is its own module so the repository's go.mod
// has no third-party requirement.
package main

import (
	"fmt"
	"math"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"unicode/utf8"

	"google.golang.org/protobuf/encoding/prototext"
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

func packedField(f *descriptorpb.FieldDescriptorProto) *descriptorpb.FieldDescriptorProto {
	f.Options = &descriptorpb.FieldOptions{Packed: proto.Bool(true)}
	return f
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
		// a closed enum (a value it does not name is an unknown field once parsed), a group, and
		// proto2 strings, which need not be UTF-8
		field("mood", 6, descriptorpb.FieldDescriptorProto_TYPE_ENUM, opt, ".pjt2.Mood"),
		field("moods", 7, descriptorpb.FieldDescriptorProto_TYPE_ENUM, rep, ".pjt2.Mood"),
		packedField(field("pmoods", 8, descriptorpb.FieldDescriptorProto_TYPE_ENUM, rep, ".pjt2.Mood")),
		field("mood_map", 9, descriptorpb.FieldDescriptorProto_TYPE_MESSAGE, rep, ".pjt2.Req.MoodMapEntry"),
		field("grp", 10, descriptorpb.FieldDescriptorProto_TYPE_GROUP, opt, ".pjt2.Req.Grp"),
		field("raw", 11, descriptorpb.FieldDescriptorProto_TYPE_STRING, opt, ""),
	}}
	rq.NestedType = []*descriptorpb.DescriptorProto{
		mapEntry("MoodMapEntry", descriptorpb.FieldDescriptorProto_TYPE_INT32, descriptorpb.FieldDescriptorProto_TYPE_ENUM, ".pjt2.Mood"),
		{Name: proto.String("Grp"), Field: []*descriptorpb.FieldDescriptorProto{field("g", 1, descriptorpb.FieldDescriptorProto_TYPE_INT32, opt, "")}},
	}
	set.File = append(set.File, &descriptorpb.FileDescriptorProto{
		Name: proto.String("pjtest2.proto"), Package: proto.String("pjt2"), Syntax: proto.String("proto2"),
		MessageType: []*descriptorpb.DescriptorProto{leaf, rq},
		EnumType: []*descriptorpb.EnumDescriptorProto{{Name: proto.String("Mood"), Value: []*descriptorpb.EnumValueDescriptorProto{
			{Name: proto.String("CALM"), Number: proto.Int32(0)}, {Name: proto.String("ANGRY"), Number: proto.Int32(1)}, {Name: proto.String("SAD"), Number: proto.Int32(3)},
		}}},
	})
	// an editions file (2023): per-field presence, packed encoding and delimited message encoding, written as protoc
	// writes them (the unresolved features of each field)
	fs := func(f func(*descriptorpb.FeatureSet)) *descriptorpb.FieldOptions {
		x := &descriptorpb.FeatureSet{}
		f(x)
		return &descriptorpb.FieldOptions{Features: x}
	}
	withOpts := func(fd *descriptorpb.FieldDescriptorProto, o *descriptorpb.FieldOptions) *descriptorpb.FieldDescriptorProto {
		fd.Options = o
		return fd
	}
	i32 := descriptorpb.FieldDescriptorProto_TYPE_INT32
	msgT := descriptorpb.FieldDescriptorProto_TYPE_MESSAGE
	sub := &descriptorpb.DescriptorProto{Name: proto.String("Sub"), Field: []*descriptorpb.FieldDescriptorProto{field("x", 1, i32, opt, "")}}
	edm := &descriptorpb.DescriptorProto{Name: proto.String("EdMsg"), Field: []*descriptorpb.FieldDescriptorProto{
		withOpts(field("a", 1, i32, opt, ""), fs(func(x *descriptorpb.FeatureSet) { x.FieldPresence = descriptorpb.FeatureSet_IMPLICIT.Enum() })),
		field("b", 2, i32, opt, ""),
		field("p", 3, i32, rep, ""),
		withOpts(field("e", 4, i32, rep, ""), fs(func(x *descriptorpb.FeatureSet) { x.RepeatedFieldEncoding = descriptorpb.FeatureSet_EXPANDED.Enum() })),
		withOpts(field("d", 5, msgT, opt, ".pjed.Sub"), fs(func(x *descriptorpb.FeatureSet) { x.MessageEncoding = descriptorpb.FeatureSet_DELIMITED.Enum() })),
		withOpts(field("r", 6, i32, opt, ""), fs(func(x *descriptorpb.FeatureSet) { x.FieldPresence = descriptorpb.FeatureSet_LEGACY_REQUIRED.Enum() })),
		field("s", 7, descriptorpb.FieldDescriptorProto_TYPE_STRING, opt, ""),
		field("i", 8, msgT, opt, ".pjed.Sub"),
		withOpts(field("imp", 9, descriptorpb.FieldDescriptorProto_TYPE_STRING, opt, ""), fs(func(x *descriptorpb.FeatureSet) { x.FieldPresence = descriptorpb.FeatureSet_IMPLICIT.Enum() })),
		withOpts(field("ds", 10, msgT, rep, ".pjed.Sub"), fs(func(x *descriptorpb.FeatureSet) { x.MessageEncoding = descriptorpb.FeatureSet_DELIMITED.Enum() })),
		withOpts(field("pd", 11, descriptorpb.FieldDescriptorProto_TYPE_DOUBLE, rep, ""), fs(func(x *descriptorpb.FeatureSet) { x.RepeatedFieldEncoding = descriptorpb.FeatureSet_PACKED.Enum() })),
	}}
	// an implicit file: no presence for scalars, packed by default, a field made explicit by its feature
	imp := &descriptorpb.DescriptorProto{Name: proto.String("ImpMsg"), Field: []*descriptorpb.FieldDescriptorProto{
		field("n", 1, i32, opt, ""),
		withOpts(field("o", 2, i32, opt, ""), fs(func(x *descriptorpb.FeatureSet) { x.FieldPresence = descriptorpb.FeatureSet_EXPLICIT.Enum() })),
		field("r", 3, i32, rep, ""),
		field("t", 4, descriptorpb.FieldDescriptorProto_TYPE_STRING, opt, ""),
	}}
	set.File = append(set.File, &descriptorpb.FileDescriptorProto{
		Name: proto.String("pjted.proto"), Package: proto.String("pjed"), Syntax: proto.String("editions"),
		Edition:     descriptorpb.Edition_EDITION_2023.Enum(),
		MessageType: []*descriptorpb.DescriptorProto{sub, edm},
	}, &descriptorpb.FileDescriptorProto{
		Name: proto.String("pjtedimp.proto"), Package: proto.String("pjedi"), Syntax: proto.String("editions"),
		Edition:     descriptorpb.Edition_EDITION_2023.Enum(),
		Options:     &descriptorpb.FileOptions{Features: &descriptorpb.FeatureSet{FieldPresence: descriptorpb.FeatureSet_IMPLICIT.Enum()}},
		MessageType: []*descriptorpb.DescriptorProto{imp},
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
		ss := []string{"", "a", "héllo", "日本語", "with \"quotes\" and \\ and \n newline", "<tag>&amp;", "  ", "tab\there", "\x01\x1f", "😀", "a\xffb", "\u0085 \u00a0", "'single'\x7f"}
		s := ss[r.Intn(len(ss))]
		if !utf8.ValidString(s) && fd.ParentFile().Syntax() != protoreflect.Proto2 {
			// only a proto2 string may hold other bytes
			s = "a"
		}
		return protoreflect.ValueOfString(s)
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

// undetrand drops the second space protobuf-go's text encoder puts at
// random (internal/detrand) between fields and after a name: outside
// strings, a space after a space that follows neither a newline nor a
// space (an indent) is one.
func undetrand(s string) string {
	var b strings.Builder
	in := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if in {
			b.WriteByte(c)
			if c == '\\' {
				i++
				b.WriteByte(s[i])
			} else if c == '"' {
				in = false
			}
			continue
		}
		if c == '"' {
			in = true
		}
		if c == ' ' && i > 0 && s[i-1] == ' ' && i > 1 && s[i-2] != ' ' && s[i-2] != '\n' {
			continue
		}
		b.WriteByte(c)
	}
	return b.String()
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

var resolver *dynamicpb.Types

// mopts: 1 multiline, 2 emit ASCII, 4 emit unknown, 8 allow partial.
func mopts(flags int) prototext.MarshalOptions {
	return prototext.MarshalOptions{Resolver: resolver, Multiline: flags&1 != 0, EmitASCII: flags&2 != 0, EmitUnknown: flags&4 != 0, AllowPartial: flags&8 != 0}
}

// unknownTail is wire for fields the schema does not have: every wire
// type, a group with a field inside, numbers past the schema's.
func unknownTail(r *rand.Rand) []byte {
	var b []byte
	for i, n := 0, 1+r.Intn(3); i < n; i++ {
		num := protowire.Number(1000 + r.Intn(50))
		switch r.Intn(5) {
		case 0:
			b = protowire.AppendTag(b, num, protowire.VarintType)
			b = protowire.AppendVarint(b, []uint64{0, 1, 300, 1 << 63, math.MaxUint64}[r.Intn(5)])
		case 1:
			b = protowire.AppendTag(b, num, protowire.Fixed32Type)
			b = protowire.AppendFixed32(b, []uint32{0, 1, 0xdeadbeef}[r.Intn(3)])
		case 2:
			b = protowire.AppendTag(b, num, protowire.Fixed64Type)
			b = protowire.AppendFixed64(b, []uint64{0, 255, math.MaxUint64}[r.Intn(3)])
		case 3:
			b = protowire.AppendTag(b, num, protowire.BytesType)
			b = protowire.AppendBytes(b, [][]byte{{}, []byte("hi"), {0xff, 0x00, '"'}}[r.Intn(3)])
		case 4:
			b = protowire.AppendTag(b, num, protowire.StartGroupType)
			b = protowire.AppendTag(b, 1, protowire.VarintType)
			b = protowire.AppendVarint(b, 5)
			b = protowire.AppendTag(b, num, protowire.EndGroupType)
		}
	}
	return b
}

func main() {
	set := schema()
	files, err := protodesc.NewFiles(set)
	if err != nil {
		panic(err)
	}
	resolver = dynamicpb.NewTypes(files)
	// XO_PROTOTEXTFUZZ_SEED (default 0, the committed corpus) moves the seed;
	// XO_PROTOTEXTFUZZ_N is the count of random messages (default 300)
	fuzzSeed, _ := strconv.ParseInt(os.Getenv("XO_PROTOTEXTFUZZ_SEED"), 10, 64)
	nMsgs := 300
	if v, err := strconv.Atoi(os.Getenv("XO_PROTOTEXTFUZZ_N")); err == nil && v > 0 {
		nMsgs = v
	}
	g := &gen{r: rand.New(rand.NewSource(8521 + fuzzSeed)), reg: files}
	var o strings.Builder
	o.WriteString("// Generated by scripts/stdcorpus from protobuf-go v1.36.12's prototext over std/protojson's test schema, with seeded random messages from dynamicpb (" + os.Args[1] + ").\n")
	o.WriteString("// Do not edit: regenerate with go run ./scripts/stdcorpus.\n")
	o.WriteString("use std/protojson\nuse std/prototext\n\n")
	sb, err := proto.MarshalOptions{Deterministic: true}.Marshal(set)
	if err != nil {
		panic(err)
	}
	fmt.Fprintf(&o, "/// the schema: a FileDescriptorSet, files in dependency order, as hex\nconst SCHEMA: Str = \"%x\"\n\n", sb)
	names := []string{"pjt.Scalars", "pjt.Wkt", "pjt.Inner", "pjed.EdMsg", "pjedi.ImpMsg", "pjt2.Req", "google.protobuf.Any"}
	var rows, fmtRows []string
	for i := 0; i < nMsgs; i++ {
		name := names[g.r.Intn(len(names))]
		d, err := files.FindDescriptorByName(protoreflect.FullName(name))
		if err != nil {
			dd, err2 := protoregistry.GlobalFiles.FindDescriptorByName(protoreflect.FullName(name))
			if err2 != nil {
				panic(name)
			}
			d = dd
		}
		md := d.(protoreflect.MessageDescriptor)
		var wire []byte
		if name == "google.protobuf.Any" {
			// an Any of a schema message, or of a name nobody knows
			in := names[g.r.Intn(3)]
			id, _ := files.FindDescriptorByName(protoreflect.FullName(in))
			inner, _ := proto.MarshalOptions{Deterministic: true, AllowPartial: true}.Marshal(g.msg(id.(protoreflect.MessageDescriptor), 1).Interface())
			url := []string{"type.googleapis.com/", "example.com/x/", "", "type.googleapis.com/nope."}[g.r.Intn(4)] + in
			a := &anypb.Any{TypeUrl: url, Value: inner}
			wire, _ = proto.MarshalOptions{Deterministic: true}.Marshal(a)
		} else {
			wire, err = proto.MarshalOptions{Deterministic: true, AllowPartial: true}.Marshal(g.msg(md, 0).Interface())
			if err != nil {
				continue
			}
		}
		switch g.r.Intn(6) {
		case 0:
			wire = append(wire, unknownTail(g.r)...)
		case 1:
			// a second message of the type: singular fields replace, messages merge, lists append
			more, err := proto.MarshalOptions{Deterministic: true, AllowPartial: true}.Marshal(g.msg(md, 0).Interface())
			if err == nil && name != "google.protobuf.Any" {
				wire = append(wire, more...)
			}
		}
		// the message Go's Unmarshal makes of the wire, as std/prototext reads it
		m := dynamicpb.NewMessage(md)
		if err := (proto.UnmarshalOptions{AllowPartial: true, Resolver: resolver}).Unmarshal(wire, m); err != nil {
			continue
		}
		for _, fl := range []int{0, 1 + g.r.Intn(15)} {
			var want string
			txt, err := mopts(fl).Marshal(m)
			if err != nil {
				// protobuf-go's errors put a no-break space after "proto:" at random (internal/detrand)
				want = "ERR:" + strings.Replace(err.Error(), "proto:\u00a0", "proto: ", 1)
			} else {
				want = undetrand(string(txt))
			}
			rows = append(rows, fmt.Sprintf("  (\"%s\", %d, \"%x\", \"%s\"),\n", name, fl, wire, q(want)))
		}
		if i%5 == 0 {
			fmtRows = append(fmtRows, fmt.Sprintf("  (\"%s\", \"%x\", \"%s\"),\n", name, wire, q(undetrand(prototext.MarshalOptions{Multiline: true, Resolver: resolver}.Format(m)))))
		}
	}
	o.WriteString("/// (message full name, flags: 1 multiline, 2 emit ASCII, 4 emit unknown, 8 allow partial; wire hex, Go's prototext output with one space where it put two at random, or ERR: and its error)\nconst MARSHAL: List[(Str, Int, Str, Str)] = [\n")
	for _, r := range rows {
		o.WriteString(r)
	}
	o.WriteString("]\n\n/// (message full name, wire hex, Go's prototext.Format, with one space where it put two)\nconst FORMAT: List[(Str, Str, Str)] = [\n")
	for _, r := range fmtRows {
		o.WriteString(r)
	}
	o.WriteString("]\n")
	o.WriteString(xoTests)
	fmt.Print(o.String())
}

const xoTests = `
fn unhex(s: Str) -> Bytes { Bytes.from_hex(s).or_fault("hex") }

/// Where got and want first differ, with some text around it from each.
fn first_diff(got: Str, want: Str) -> Str {
  let g = got.bytes()
  let w = want.bytes()
  var i = 0
  while i < g.len() && i < w.len() && g[i] == w[i] {
    i += 1
  }
  let lo = (i - 30).max(0)
  "at ${i}: got ${g[lo..(i + 40).min(g.len())].to_str_lossy().quote()} want ${w[lo..(i + 40).min(w.len())].to_str_lossy().quote()}"
}

fn opts(fl: Int) -> prototext.Opts {
  prototext.Opts{multiline: (fl & 1) != 0, emit_ascii: (fl & 2) != 0, emit_unknown: (fl & 4) != 0, allow_partial: (fl & 8) != 0}
}

test "marshal: Go's prototext.Marshal of the message the wire bytes hold" {
  let r = protojson.registry_from_set(unhex(SCHEMA)).or_fault("schema")
  var bad: List[Str] = []
  for (name, fl, wire, want) in MARSHAL {
    let got = match prototext.marshal(r, name, unhex(wire), opts(fl)) {
      Ok(t) => t
      Err(e) => "ERR:" + e.display()
    }
    if got != want {
      bad.push("${name} ${fl} ${wire[0..wire.len().min(40)]}: " + first_diff(got, want))
    }
  }
  expect(bad[0..bad.len().min(4)]) == []
}

test "format: Go's prototext.Format" {
  let r = protojson.registry_from_set(unhex(SCHEMA)).or_fault("schema")
  var bad: List[Str] = []
  for (name, wire, want) in FORMAT {
    let got = prototext.format(r, name, unhex(wire))
    if got != want {
      bad.push("${name} ${wire[0..wire.len().min(40)]}: " + first_diff(got, want))
    }
  }
  expect(bad[0..bad.len().min(4)]) == []
}
`
