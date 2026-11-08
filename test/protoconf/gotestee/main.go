// Go's protobuf as the conformance testee: the same envelope handling as test/protoconf/main.xo, over
// protobuf-go's protojson with dynamicpb, so the two pass counts compare the JSON mapping libraries.
package main

import (
	"encoding/binary"
	"io"
	"os"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"
)

type req struct {
	proto, json    []byte
	hasProto       bool
	fmt            int
	typ            string
	other, ignoreU bool
}

func resp(num protowire.Number, data []byte) []byte {
	return protowire.AppendBytes(protowire.AppendTag(nil, num, protowire.BytesType), data)
}

func parse(m []byte) (r req, err error) {
	for len(m) > 0 {
		num, typ, n := protowire.ConsumeTag(m)
		if n < 0 {
			return r, protowire.ParseError(n)
		}
		m = m[n:]
		switch {
		case num == 1 && typ == protowire.BytesType:
			v, n := protowire.ConsumeBytes(m)
			r.proto, r.hasProto, m = v, true, m[n:]
		case num == 2 && typ == protowire.BytesType:
			v, n := protowire.ConsumeBytes(m)
			r.json, m = v, m[n:]
		case num == 3 && typ == protowire.VarintType:
			v, n := protowire.ConsumeVarint(m)
			r.fmt, m = int(v), m[n:]
		case num == 4 && typ == protowire.BytesType:
			v, n := protowire.ConsumeBytes(m)
			r.typ, m = string(v), m[n:]
		case num == 5 && typ == protowire.VarintType:
			v, n := protowire.ConsumeVarint(m)
			r.ignoreU, m = v == 3, m[n:]
		default:
			if num == 7 || num == 8 {
				r.other = true
			}
			n := protowire.ConsumeFieldValue(num, typ, m)
			if n < 0 {
				return r, protowire.ParseError(n)
			}
			m = m[n:]
		}
	}
	return r, nil
}

func respond(files *protoregistry.Files, types *dynamicpb.Types, q req) []byte {
	name := q.typ
	if q.other || (name != "protobuf_test_messages.proto3.TestAllTypesProto3" && name != "protobuf_test_messages.proto2.TestAllTypesProto2") {
		return resp(5, []byte("unsupported"))
	}
	d, err := files.FindDescriptorByName(protoreflect.FullName(name))
	if err != nil {
		return resp(2, []byte(err.Error()))
	}
	m := dynamicpb.NewMessage(d.(protoreflect.MessageDescriptor))
	if q.hasProto {
		if err := proto.Unmarshal(q.proto, m); err != nil {
			return resp(1, []byte(err.Error()))
		}
	} else if err := (protojson.UnmarshalOptions{DiscardUnknown: q.ignoreU, Resolver: types}).Unmarshal(q.json, m); err != nil {
		return resp(1, []byte(err.Error()))
	}
	switch q.fmt {
	case 1:
		b, err := proto.Marshal(m)
		if err != nil {
			return resp(6, []byte(err.Error()))
		}
		return resp(3, b)
	case 2:
		b, err := protojson.MarshalOptions{Resolver: types}.Marshal(m)
		if err != nil {
			return resp(6, []byte(err.Error()))
		}
		return resp(4, b)
	}
	return resp(5, []byte("unsupported output format"))
}

func main() {
	setb, err := os.ReadFile(os.Args[1])
	if err != nil {
		panic(err)
	}
	var set descriptorpb.FileDescriptorSet
	if err := proto.Unmarshal(setb, &set); err != nil {
		panic(err)
	}
	// protobuf-go refuses a MessageSet (a legacy proto1 feature) when it builds descriptors: the proto2
	// conformance file's two, and the extensions of them, are dropped so the other messages load
	gone := map[string]bool{}
	for _, f := range set.File {
		for _, m := range f.MessageType {
			dropMessageSets(f.GetPackage(), m, gone)
		}
	}
	for _, f := range set.File {
		for _, m := range f.MessageType {
			dropExtensions(m, gone)
		}
		f.Extension = keepExtensions(f.Extension, gone)
	}
	files, err := protodesc.NewFiles(&set)
	if err != nil {
		panic(err)
	}
	types := dynamicpb.NewTypes(files)
	for {
		var head [4]byte
		if _, err := io.ReadFull(os.Stdin, head[:]); err != nil {
			return
		}
		body := make([]byte, binary.LittleEndian.Uint32(head[:]))
		if _, err := io.ReadFull(os.Stdin, body); err != nil {
			return
		}
		var out []byte
		if q, err := parse(body); err != nil {
			out = resp(2, []byte("parse: "+err.Error()))
		} else {
			out = respond(files, types, q)
		}
		binary.LittleEndian.PutUint32(head[:], uint32(len(out)))
		os.Stdout.Write(head[:])
		os.Stdout.Write(out)
	}
}

func dropMessageSets(scope string, m *descriptorpb.DescriptorProto, gone map[string]bool) {
	full := scope + "." + m.GetName()
	var keep []*descriptorpb.DescriptorProto
	for _, n := range m.NestedType {
		if n.GetOptions().GetMessageSetWireFormat() {
			gone["."+full+"."+n.GetName()] = true
			continue
		}
		keep = append(keep, n)
	}
	m.NestedType = keep
	for _, n := range m.NestedType {
		dropMessageSets(full, n, gone)
	}
}

func dropExtensions(m *descriptorpb.DescriptorProto, gone map[string]bool) {
	m.Extension = keepExtensions(m.Extension, gone)
	for _, n := range m.NestedType {
		dropExtensions(n, gone)
	}
}

func keepExtensions(xs []*descriptorpb.FieldDescriptorProto, gone map[string]bool) []*descriptorpb.FieldDescriptorProto {
	var keep []*descriptorpb.FieldDescriptorProto
	for _, x := range xs {
		if gone[x.GetExtendee()] {
			continue
		}
		keep = append(keep, x)
	}
	return keep
}
