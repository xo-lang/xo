package main

// The fuzz part of the corpus (decision 0616's fuzz kind, task #27): random
// FileDescriptorProtos drawn from every descriptor.proto field std/protodesc
// models (nested messages, fields of every type and label, oneofs, extension
// ranges, reserved ranges and names, enums, services with streaming flags,
// options and json_name present or not, dependencies, syntax and editions,
// source info), marshaled by protobuf-go deterministically, which std/protodesc
// must decode and encode back to the same bytes with the same counts; and
// those bytes changed (bits flipped, cut, bytes inserted or removed), which
// std/protodesc must decode whenever proto.Unmarshal into a
// FileDescriptorProto accepts them and every string in them is UTF-8 (an Xo
// Str is UTF-8 only; see fzUTF8).
//
// XO_PROTODESCFUZZ_SEED (default 0, the committed corpus) moves the seed and
// XO_PROTODESCFUZZ_N (default 120) sets how many descriptors are drawn;
// internal/stdtest's TestProtodescFuzz runs a fresh seed.

import (
	"fmt"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"unicode/utf8"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
)

type fz struct{ r *rand.Rand }

var fzWords = []string{"a", "b", "id", "name", "Value", "x_y", "HTTPRule", "big_data", "é", "日本", "v2", "foo_bar_baz", "Z"}

func (g *fz) word() string {
	w := fzWords[g.r.Intn(len(fzWords))]
	if g.r.Intn(4) == 0 {
		w += strconv.Itoa(g.r.Intn(100))
	}
	return w
}

func (g *fz) maybe(n int) bool { return g.r.Intn(n) == 0 }

func (g *fz) typeName() string {
	parts := []string{}
	for i, k := 0, 1+g.r.Intn(3); i < k; i++ {
		parts = append(parts, g.word())
	}
	n := strings.Join(parts, ".")
	if g.r.Intn(3) > 0 {
		n = "." + n
	}
	return n
}

func (g *fz) num() int32 {
	switch g.r.Intn(6) {
	case 0:
		return 536870911
	case 1:
		return 19000 + g.r.Int31n(1000)
	default:
		return 1 + g.r.Int31n(2000)
	}
}

func (g *fz) ranges() ([]*descriptorpb.DescriptorProto_ReservedRange, []string) {
	var rr []*descriptorpb.DescriptorProto_ReservedRange
	for i, k := 0, g.r.Intn(3); i < k; i++ {
		s := g.num()
		rr = append(rr, &descriptorpb.DescriptorProto_ReservedRange{Start: proto.Int32(s), End: proto.Int32(s + 1 + g.r.Int31n(10))})
	}
	var names []string
	for i, k := 0, g.r.Intn(3); i < k; i++ {
		names = append(names, g.word())
	}
	return rr, names
}

func (g *fz) field(oneofs int) *descriptorpb.FieldDescriptorProto {
	f := &descriptorpb.FieldDescriptorProto{Name: proto.String(g.word()), Number: proto.Int32(g.num())}
	t := descriptorpb.FieldDescriptorProto_Type(1 + g.r.Intn(18))
	if g.r.Intn(8) > 0 {
		f.Type = t.Enum()
	}
	if g.r.Intn(8) > 0 {
		f.Label = descriptorpb.FieldDescriptorProto_Label(1 + g.r.Intn(3)).Enum()
	}
	if t == descriptorpb.FieldDescriptorProto_TYPE_MESSAGE || t == descriptorpb.FieldDescriptorProto_TYPE_ENUM || t == descriptorpb.FieldDescriptorProto_TYPE_GROUP || g.maybe(10) {
		f.TypeName = proto.String(g.typeName())
	}
	if g.maybe(5) {
		f.DefaultValue = proto.String([]string{"0", "-1", "1.5", "inf", "-nan", "true", "abc", "\\001\\377", "A"}[g.r.Intn(9)])
	}
	if oneofs > 0 && g.maybe(3) {
		f.OneofIndex = proto.Int32(g.r.Int31n(int32(oneofs)))
	}
	if g.maybe(2) {
		f.JsonName = proto.String([]string{"", g.word(), "fooBar"}[g.r.Intn(3)])
	}
	if g.maybe(4) {
		o := &descriptorpb.FieldOptions{}
		if g.maybe(2) {
			o.Packed = proto.Bool(g.maybe(2))
		}
		if g.maybe(2) {
			o.Deprecated = proto.Bool(true)
		}
		if g.maybe(3) {
			o.Lazy = proto.Bool(false)
		}
		f.Options = o
	}
	if g.maybe(8) {
		f.Proto3Optional = proto.Bool(g.maybe(3) == false)
	}
	return f
}

func (g *fz) enum() *descriptorpb.EnumDescriptorProto {
	e := &descriptorpb.EnumDescriptorProto{Name: proto.String(g.word())}
	for i, k := 0, g.r.Intn(5); i < k; i++ {
		v := &descriptorpb.EnumValueDescriptorProto{Name: proto.String(strings.ToUpper(g.word())), Number: proto.Int32(int32(g.r.Intn(20)) - 5)}
		if g.maybe(5) {
			v.Options = &descriptorpb.EnumValueOptions{Deprecated: proto.Bool(true)}
		}
		e.Value = append(e.Value, v)
	}
	if g.maybe(4) {
		e.Options = &descriptorpb.EnumOptions{AllowAlias: proto.Bool(g.maybe(2))}
	}
	for i, k := 0, g.r.Intn(3); i < k; i++ {
		s := int32(g.r.Intn(100)) - 10
		e.ReservedRange = append(e.ReservedRange, &descriptorpb.EnumDescriptorProto_EnumReservedRange{Start: proto.Int32(s), End: proto.Int32(s + g.r.Int31n(5))})
	}
	if g.maybe(3) {
		e.ReservedName = append(e.ReservedName, strings.ToUpper(g.word()))
	}
	return e
}

func (g *fz) message(depth int) *descriptorpb.DescriptorProto {
	m := &descriptorpb.DescriptorProto{Name: proto.String(g.word())}
	oneofs := 0
	if g.maybe(3) {
		oneofs = 1 + g.r.Intn(2)
		for i := 0; i < oneofs; i++ {
			o := &descriptorpb.OneofDescriptorProto{Name: proto.String(g.word())}
			if g.maybe(5) {
				o.Options = &descriptorpb.OneofOptions{}
			}
			m.OneofDecl = append(m.OneofDecl, o)
		}
	}
	for i, k := 0, g.r.Intn(6); i < k; i++ {
		m.Field = append(m.Field, g.field(oneofs))
	}
	if g.maybe(5) {
		x := g.field(0)
		x.Extendee = proto.String(g.typeName())
		m.Extension = append(m.Extension, x)
	}
	if depth < 3 {
		for i, k := 0, g.r.Intn(3); i < k; i++ {
			m.NestedType = append(m.NestedType, g.message(depth+1))
		}
	}
	for i, k := 0, g.r.Intn(2); i < k; i++ {
		m.EnumType = append(m.EnumType, g.enum())
	}
	if g.maybe(4) {
		s := 100 + g.r.Int31n(1000)
		er := &descriptorpb.DescriptorProto_ExtensionRange{Start: proto.Int32(s), End: proto.Int32(s + 1 + g.r.Int31n(1000))}
		if g.maybe(3) {
			er.Options = &descriptorpb.ExtensionRangeOptions{}
		}
		m.ExtensionRange = append(m.ExtensionRange, er)
	}
	if g.maybe(4) {
		o := &descriptorpb.MessageOptions{}
		if g.maybe(2) {
			o.MapEntry = proto.Bool(true)
		}
		if g.maybe(2) {
			o.Deprecated = proto.Bool(g.maybe(2))
		}
		m.Options = o
	}
	m.ReservedRange, m.ReservedName = g.ranges()
	return m
}

func (g *fz) file(i int) *descriptorpb.FileDescriptorProto {
	f := &descriptorpb.FileDescriptorProto{Name: proto.String(fmt.Sprintf("fz/%s_%d.proto", g.word(), i))}
	if g.r.Intn(4) > 0 {
		f.Package = proto.String(strings.TrimPrefix(g.typeName(), "."))
	}
	for i, k := 0, g.r.Intn(3); i < k; i++ {
		f.Dependency = append(f.Dependency, g.word()+".proto")
	}
	if len(f.Dependency) > 0 && g.maybe(3) {
		f.PublicDependency = []int32{0}
	}
	if len(f.Dependency) > 1 && g.maybe(4) {
		f.WeakDependency = []int32{1}
	}
	for i, k := 0, g.r.Intn(4); i < k; i++ {
		f.MessageType = append(f.MessageType, g.message(0))
	}
	for i, k := 0, g.r.Intn(2); i < k; i++ {
		f.EnumType = append(f.EnumType, g.enum())
	}
	for i, k := 0, g.r.Intn(2); i < k; i++ {
		s := &descriptorpb.ServiceDescriptorProto{Name: proto.String(g.word())}
		for j, n := 0, g.r.Intn(4); j < n; j++ {
			me := &descriptorpb.MethodDescriptorProto{Name: proto.String(g.word()), InputType: proto.String(g.typeName()), OutputType: proto.String(g.typeName())}
			if g.maybe(3) {
				me.ClientStreaming = proto.Bool(g.maybe(2))
			}
			if g.maybe(3) {
				me.ServerStreaming = proto.Bool(g.maybe(2))
			}
			if g.maybe(5) {
				me.Options = &descriptorpb.MethodOptions{Deprecated: proto.Bool(true)}
			}
			s.Method = append(s.Method, me)
		}
		if g.maybe(5) {
			s.Options = &descriptorpb.ServiceOptions{Deprecated: proto.Bool(false)}
		}
		f.Service = append(f.Service, s)
	}
	if g.maybe(5) {
		x := g.field(0)
		x.Extendee = proto.String(g.typeName())
		f.Extension = append(f.Extension, x)
	}
	if g.maybe(3) {
		o := &descriptorpb.FileOptions{}
		if g.maybe(2) {
			o.GoPackage = proto.String("example.com/" + g.word())
		}
		if g.maybe(2) {
			o.JavaPackage = proto.String("com." + g.word())
		}
		if g.maybe(3) {
			o.OptimizeFor = descriptorpb.FileOptions_LITE_RUNTIME.Enum()
		}
		f.Options = o
	}
	switch g.r.Intn(4) {
	case 0:
		f.Syntax = proto.String("proto2")
	case 1:
		f.Syntax = proto.String("proto3")
	case 2:
		f.Syntax = proto.String("editions")
		f.Edition = []descriptorpb.Edition{descriptorpb.Edition_EDITION_2023, descriptorpb.Edition_EDITION_2024}[g.r.Intn(2)].Enum()
	}
	if g.maybe(6) {
		f.SourceCodeInfo = &descriptorpb.SourceCodeInfo{Location: []*descriptorpb.SourceCodeInfo_Location{{Path: []int32{4, 0}, Span: []int32{1, 0, 3, 1}, LeadingComments: proto.String(" " + g.word() + "\n")}}}
	}
	return f
}

func fzCount(f *descriptorpb.FileDescriptorProto) (msgs, fields, enums, services, methods int) {
	var walk func(ms []*descriptorpb.DescriptorProto)
	walk = func(ms []*descriptorpb.DescriptorProto) {
		for _, m := range ms {
			msgs++
			fields += len(m.Field)
			enums += len(m.EnumType)
			walk(m.NestedType)
		}
	}
	walk(f.MessageType)
	enums += len(f.EnumType)
	services = len(f.Service)
	for _, s := range f.Service {
		methods += len(s.Method)
	}
	return
}

func fzMutate(r *rand.Rand, b []byte) []byte {
	b = append([]byte(nil), b...)
	for k := 1 + r.Intn(3); k > 0 && len(b) > 0; k-- {
		i := r.Intn(len(b))
		switch r.Intn(5) {
		case 0:
			b[i] ^= 1 << r.Intn(8)
		case 1:
			b = b[:i]
		case 2:
			b = append(b[:i], append([]byte{byte(r.Intn(256))}, b[i:]...)...)
		case 3:
			b = append(b[:i], b[i+1:]...)
		default:
			b[i] = byte(r.Intn(256))
		}
	}
	return b
}

// fzUTF8 reports whether every string field of m (nested ones too) is UTF-8:
// descriptor.proto is proto2, so protobuf-go accepts any bytes there, but an
// Xo Str is UTF-8 and std/protodesc refuses such a descriptor
// ("proto: string field contains invalid UTF-8"), on purpose.
func fzUTF8(m protoreflect.Message) bool {
	ok := true
	m.Range(func(fd protoreflect.FieldDescriptor, v protoreflect.Value) bool {
		one := func(v protoreflect.Value) {
			switch fd.Kind() {
			case protoreflect.StringKind:
				ok = ok && utf8.ValidString(v.String())
			case protoreflect.MessageKind, protoreflect.GroupKind:
				ok = ok && fzUTF8(v.Message())
			}
		}
		if fd.IsList() {
			for i := 0; i < v.List().Len(); i++ {
				one(v.List().Get(i))
			}
		} else {
			one(v)
		}
		return ok
	})
	return ok
}

// fuzzRows writes FUZZ (random descriptors with their counts) and MUTATED
// (changed bytes and whether protobuf-go accepts them).
func fuzzRows(o *strings.Builder) {
	seed, _ := strconv.ParseInt(os.Getenv("XO_PROTODESCFUZZ_SEED"), 10, 64)
	n := 120
	if v, err := strconv.Atoi(os.Getenv("XO_PROTODESCFUZZ_N")); err == nil && v > 0 {
		n = v
	}
	g := &fz{r: rand.New(rand.NewSource(9161 + seed))}
	var good [][]byte
	o.WriteString("\n/// random FileDescriptorProtos marshaled by protobuf-go (messages, fields, enums, services, methods, hex)\nconst FUZZ: List[(Int, Int, Int, Int, Int, Str)] = [\n")
	for i := 0; i < n; i++ {
		f := g.file(i)
		b, err := proto.MarshalOptions{Deterministic: true}.Marshal(f)
		if err != nil {
			panic(err)
		}
		good = append(good, b)
		m, fl, e, s, me := fzCount(f)
		fmt.Fprintf(o, "  (%d, %d, %d, %d, %d, \"%x\"),\n", m, fl, e, s, me, b)
	}
	o.WriteString("]\n\n/// those bytes changed, and whether proto.Unmarshal into a FileDescriptorProto accepts them with UTF-8 strings\nconst MUTATED: List[(Bool, Str)] = [\n")
	for _, b := range good {
		mb := fzMutate(g.r, b)
		var d descriptorpb.FileDescriptorProto
		ok := proto.Unmarshal(mb, &d) == nil && fzUTF8(d.ProtoReflect())
		fmt.Fprintf(o, "  (%v, \"%x\"),\n", ok, mb)
	}
	o.WriteString("]\n")
}

const fuzzTests = `
test "random descriptors decode and encode back to protobuf-go's bytes" {
  for (msgs, fields, enums, services, methods, h) in FUZZ {
    let d = protodesc.decode(unhex(h)).or_fault("decode " + h)
    expect(hex(protodesc.encode(d))) == h
    var m = 0
    var f = 0
    var e = d.enum_type.len()
    for x in d.message_type {
      let (a, b, c) = tally(x)
      m += a
      f += b
      e += c
    }
    var me = 0
    for s in d.service {
      me += s.method.len()
    }
    expect((m, f, e, d.service.len(), me)) == (msgs, fields, enums, services, methods)
  }
}

test "changed descriptor bytes protobuf-go accepts decode" {
  for (go_ok, h) in MUTATED {
    let ok = protodesc.decode(unhex(h)).is_ok()
    if go_ok {
      expect((h, ok)) == (h, true)
    }
  }
}
`
