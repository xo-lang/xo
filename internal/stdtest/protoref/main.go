// Command protoref is the differential oracle for std/protobuf (decision 0530): google.golang.org/protobuf's protowire
// (and, as the module grows, its proto package) over the inputs scripts/stdcorpus sends. One line in per case, "OP hex
// [args]", one JSON line out.
package main

import (
	"bufio"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"math"

	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"
)

// msgDesc is the proto3 message the typed ops use: M { int64 id=1; string name=2; sint64 z=3; bool ok=4; double f=5;
// bytes raw=6 }, the struct of scripts/stdcorpus/protobuf.go's typed cases.
func msgDesc() protoreflect.MessageDescriptor {
	t := descriptorpb.FieldDescriptorProto_Type(0)
	_ = t
	f := func(name string, num int32, typ descriptorpb.FieldDescriptorProto_Type) *descriptorpb.FieldDescriptorProto {
		return &descriptorpb.FieldDescriptorProto{Name: proto.String(name), Number: proto.Int32(num), Type: typ.Enum(),
			Label: descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(), JsonName: proto.String(name)}
	}
	fd, err := protodesc.NewFile(&descriptorpb.FileDescriptorProto{
		Name: proto.String("m.proto"), Syntax: proto.String("proto3"), Package: proto.String("t"),
		MessageType: []*descriptorpb.DescriptorProto{{Name: proto.String("M"), Field: []*descriptorpb.FieldDescriptorProto{
			f("id", 1, descriptorpb.FieldDescriptorProto_TYPE_INT64), f("name", 2, descriptorpb.FieldDescriptorProto_TYPE_STRING),
			f("z", 3, descriptorpb.FieldDescriptorProto_TYPE_SINT64), f("ok", 4, descriptorpb.FieldDescriptorProto_TYPE_BOOL),
			f("f", 5, descriptorpb.FieldDescriptorProto_TYPE_DOUBLE), f("raw", 6, descriptorpb.FieldDescriptorProto_TYPE_BYTES),
		}}},
	}, nil)
	if err != nil {
		panic(err)
	}
	return fd.Messages().ByName("M")
}

// repDesc is R { int64 id=1; repeated int64 nums=2; repeated string tags=3; optional int64 maybe=4; In inner=5;
// repeated In inners=6; optional In mo=7; repeated double ds=8 } with In { int64 a=1 }, the struct of the typed cases for
// Option, List and nested fields.
func repDesc() protoreflect.MessageDescriptor {
	T := descriptorpb.FieldDescriptorProto_Type(0)
	_ = T
	f := func(name string, num int32, typ descriptorpb.FieldDescriptorProto_Type, rep bool, tn string) *descriptorpb.FieldDescriptorProto {
		l := descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL
		if rep {
			l = descriptorpb.FieldDescriptorProto_LABEL_REPEATED
		}
		d := &descriptorpb.FieldDescriptorProto{Name: proto.String(name), Number: proto.Int32(num), Type: typ.Enum(), Label: l.Enum(), JsonName: proto.String(name)}
		if tn != "" {
			d.TypeName = proto.String(tn)
		}
		return d
	}
	maybe := f("maybe", 4, descriptorpb.FieldDescriptorProto_TYPE_INT64, false, "")
	maybe.Proto3Optional = proto.Bool(true)
	maybe.OneofIndex = proto.Int32(0)
	fd, err := protodesc.NewFile(&descriptorpb.FileDescriptorProto{
		Name: proto.String("r.proto"), Syntax: proto.String("proto3"), Package: proto.String("t"),
		MessageType: []*descriptorpb.DescriptorProto{
			{Name: proto.String("In"), Field: []*descriptorpb.FieldDescriptorProto{f("a", 1, descriptorpb.FieldDescriptorProto_TYPE_INT64, false, "")}},
			{Name: proto.String("R"), Field: []*descriptorpb.FieldDescriptorProto{
				f("id", 1, descriptorpb.FieldDescriptorProto_TYPE_INT64, false, ""),
				f("nums", 2, descriptorpb.FieldDescriptorProto_TYPE_INT64, true, ""),
				f("tags", 3, descriptorpb.FieldDescriptorProto_TYPE_STRING, true, ""),
				maybe,
				f("inner", 5, descriptorpb.FieldDescriptorProto_TYPE_MESSAGE, false, ".t.In"),
				f("inners", 6, descriptorpb.FieldDescriptorProto_TYPE_MESSAGE, true, ".t.In"),
				f("mo", 7, descriptorpb.FieldDescriptorProto_TYPE_MESSAGE, false, ".t.In"),
				f("ds", 8, descriptorpb.FieldDescriptorProto_TYPE_DOUBLE, true, ""),
			}, OneofDecl: []*descriptorpb.OneofDescriptorProto{{Name: proto.String("_maybe")}}},
		},
	}, nil)
	if err != nil {
		panic(err)
	}
	return fd.Messages().ByName("R")
}

func list(parts []string) string {
	if len(parts) == 0 {
		return "-"
	}
	return strings.Join(parts, ",")
}

// showRep is a repDesc message as "id nums tags maybe inner inners mo ds" (lists comma separated or -, tags in hex,
// maybe and mo n when unset, doubles as bits).
func showRep(m *dynamicpb.Message) string {
	d := m.Descriptor().Fields()
	var nums, tags, inners, ds []string
	l := m.Get(d.ByNumber(2)).List()
	for i := 0; i < l.Len(); i++ {
		nums = append(nums, strconv.FormatInt(l.Get(i).Int(), 10))
	}
	l = m.Get(d.ByNumber(3)).List()
	for i := 0; i < l.Len(); i++ {
		t := hex.EncodeToString([]byte(l.Get(i).String()))
		if t == "" {
			t = "~"
		}
		tags = append(tags, t)
	}
	maybe := "n"
	if m.Has(d.ByNumber(4)) {
		maybe = strconv.FormatInt(m.Get(d.ByNumber(4)).Int(), 10)
	}
	a := func(mm protoreflect.Message) string {
		return strconv.FormatInt(mm.Get(mm.Descriptor().Fields().ByNumber(1)).Int(), 10)
	}
	l = m.Get(d.ByNumber(6)).List()
	for i := 0; i < l.Len(); i++ {
		inners = append(inners, a(l.Get(i).Message()))
	}
	mo := "n"
	if m.Has(d.ByNumber(7)) {
		mo = a(m.Get(d.ByNumber(7)).Message())
	}
	l = m.Get(d.ByNumber(8)).List()
	for i := 0; i < l.Len(); i++ {
		ds = append(ds, strconv.FormatUint(math.Float64bits(l.Get(i).Float()), 10))
	}
	return fmt.Sprintf("%d %s %s %s %s %s %s %s", m.Get(d.ByNumber(1)).Int(), list(nums), list(tags), maybe, a(m.Get(d.ByNumber(5)).Message()), list(inners), mo, list(ds))
}

func showMsg(m *dynamicpb.Message) string {
	d := m.Descriptor().Fields()
	return fmt.Sprintf("%d %s %d %t %d %s", m.Get(d.ByNumber(1)).Int(), hex.EncodeToString([]byte(m.Get(d.ByNumber(2)).String())),
		m.Get(d.ByNumber(3)).Int(), m.Get(d.ByNumber(4)).Bool(), math.Float64bits(m.Get(d.ByNumber(5)).Float()),
		hex.EncodeToString(m.Get(d.ByNumber(6)).Bytes()))
}

type ans struct {
	Err string `json:"err,omitempty"`
	V   string `json:"v,omitempty"`
	N   int    `json:"n"`
}

func kind(n int) string {
	switch {
	case n == -1:
		return "Truncated"
	case n == -2:
		return "BadTag"
	case n == -3:
		return "Overflow"
	case n == -4:
		return "BadWire"
	case n == -5:
		return "EndGroup"
	case n == -6:
		return "Recursion"
	}
	return fmt.Sprintf("code%d", n)
}

func main() {
	sc := bufio.NewScanner(os.Stdin)
	sc.Buffer(make([]byte, 1<<20), 1<<24)
	enc := json.NewEncoder(os.Stdout)
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		op := f[0]
		var b []byte
		if len(f) > 1 && f[1] != "-" {
			b, _ = hex.DecodeString(f[1])
		}
		var a ans
		switch op {
		case "varint":
			v, n := protowire.ConsumeVarint(b)
			if n < 0 {
				a = ans{Err: kind(n)}
			} else {
				a = ans{V: strconv.FormatUint(v, 10), N: n}
			}
		case "tag":
			num, typ, n := protowire.ConsumeTag(b)
			if n < 0 {
				a = ans{Err: kind(n)}
			} else {
				a = ans{V: fmt.Sprintf("%d %d", num, typ), N: n}
			}
		case "fixed32":
			v, n := protowire.ConsumeFixed32(b)
			if n < 0 {
				a = ans{Err: kind(n)}
			} else {
				a = ans{V: strconv.FormatUint(uint64(v), 10), N: n}
			}
		case "fixed64":
			v, n := protowire.ConsumeFixed64(b)
			if n < 0 {
				a = ans{Err: kind(n)}
			} else {
				a = ans{V: strconv.FormatUint(v, 10), N: n}
			}
		case "bytes":
			v, n := protowire.ConsumeBytes(b)
			if n < 0 {
				a = ans{Err: kind(n)}
			} else {
				a = ans{V: hex.EncodeToString(v), N: n}
			}
		case "field":
			// the tag, then the whole field value
			num, typ, n := protowire.ConsumeTag(b)
			if n < 0 {
				a = ans{Err: kind(n)}
				break
			}
			m := protowire.ConsumeFieldValue(num, typ, b[n:])
			if m < 0 {
				a = ans{Err: kind(m)}
			} else {
				a = ans{V: fmt.Sprintf("%d %d", num, typ), N: n + m}
			}
		case "encvarint":
			u, _ := strconv.ParseUint(f[1], 10, 64)
			a = ans{V: hex.EncodeToString(protowire.AppendVarint(nil, u)), N: protowire.SizeVarint(u)}
		case "zigzag":
			i, _ := strconv.ParseInt(f[1], 10, 64)
			z := protowire.EncodeZigZag(i)
			a = ans{V: strconv.FormatUint(z, 10), N: int(protowire.DecodeZigZag(z))}
		case "msg":
			// msg ID NAMEHEX Z OK FBITS RAWHEX: marshal, answer the bytes
			md := msgDesc()
			m := dynamicpb.NewMessage(md)
			d := md.Fields()
			id, _ := strconv.ParseInt(f[1], 10, 64)
			nm, _ := hex.DecodeString(strings.TrimPrefix(f[2], "-"))
			z, _ := strconv.ParseInt(f[3], 10, 64)
			fb, _ := strconv.ParseUint(f[5], 10, 64)
			raw, _ := hex.DecodeString(strings.TrimPrefix(f[6], "-"))
			m.Set(d.ByNumber(1), protoreflect.ValueOfInt64(id))
			m.Set(d.ByNumber(2), protoreflect.ValueOfString(string(nm)))
			m.Set(d.ByNumber(3), protoreflect.ValueOfInt64(z))
			m.Set(d.ByNumber(4), protoreflect.ValueOfBool(f[4] == "1"))
			m.Set(d.ByNumber(5), protoreflect.ValueOfFloat64(math.Float64frombits(fb)))
			m.Set(d.ByNumber(6), protoreflect.ValueOfBytes(raw))
			out, err := proto.MarshalOptions{Deterministic: true}.Marshal(m)
			if err != nil {
				a = ans{Err: err.Error()}
			} else {
				a = ans{V: hex.EncodeToString(out)}
			}
		case "unmsg":
			// unmsg HEX: unmarshal, answer the fields (the Xo side compares only ok against error)
			m := dynamicpb.NewMessage(msgDesc())
			if err := proto.Unmarshal(b, m); err != nil {
				a = ans{Err: "err"}
			} else {
				a = ans{V: showMsg(m)}
			}
		case "rmsg":
			// rmsg ID NUMS TAGS MAYBE INNER INNERS MO DS (the showRep form): marshal
			md := repDesc()
			m := dynamicpb.NewMessage(md)
			d := md.Fields()
			id, _ := strconv.ParseInt(f[1], 10, 64)
			m.Set(d.ByNumber(1), protoreflect.ValueOfInt64(id))
			if f[2] != "-" {
				l := m.Mutable(d.ByNumber(2)).List()
				for _, x := range strings.Split(f[2], ",") {
					v, _ := strconv.ParseInt(x, 10, 64)
					l.Append(protoreflect.ValueOfInt64(v))
				}
			}
			if f[3] != "-" {
				l := m.Mutable(d.ByNumber(3)).List()
				for _, x := range strings.Split(f[3], ",") {
					if x == "~" {
						x = ""
					}
					bs, _ := hex.DecodeString(x)
					l.Append(protoreflect.ValueOfString(string(bs)))
				}
			}
			if f[4] != "n" {
				v, _ := strconv.ParseInt(f[4], 10, 64)
				m.Set(d.ByNumber(4), protoreflect.ValueOfInt64(v))
			}
			mk := func(x string) protoreflect.Value {
				in := dynamicpb.NewMessage(d.ByNumber(5).Message())
				v, _ := strconv.ParseInt(x, 10, 64)
				in.Set(in.Descriptor().Fields().ByNumber(1), protoreflect.ValueOfInt64(v))
				return protoreflect.ValueOfMessage(in)
			}
			m.Set(d.ByNumber(5), mk(f[5]))
			if f[6] != "-" {
				l := m.Mutable(d.ByNumber(6)).List()
				for _, x := range strings.Split(f[6], ",") {
					l.Append(mk(x))
				}
			}
			if f[7] != "n" {
				m.Set(d.ByNumber(7), mk(f[7]))
			}
			if f[8] != "-" {
				l := m.Mutable(d.ByNumber(8)).List()
				for _, x := range strings.Split(f[8], ",") {
					v, _ := strconv.ParseUint(x, 10, 64)
					l.Append(protoreflect.ValueOfFloat64(math.Float64frombits(v)))
				}
			}
			out, err := proto.MarshalOptions{Deterministic: true}.Marshal(m)
			if err != nil {
				a = ans{Err: err.Error()}
			} else {
				a = ans{V: hex.EncodeToString(out)}
			}
		case "runmsg":
			m := dynamicpb.NewMessage(repDesc())
			if err := proto.Unmarshal(b, m); err != nil {
				a = ans{Err: "err"}
			} else {
				a = ans{V: showRep(m)}
			}
		default:
			a = ans{Err: "unknown op " + op}
		}
		enc.Encode(a)
	}
}
