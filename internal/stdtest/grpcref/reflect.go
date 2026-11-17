package main

import (
	"context"
	"fmt"
	"net"
	"sort"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/reflection"
	rpb "google.golang.org/grpc/reflection/grpc_reflection_v1"
	rpbalpha "google.golang.org/grpc/reflection/grpc_reflection_v1alpha"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"
)

// The server reflection oracle for std/grpcreflect (task #236, TestGrpcReflectInterop): grpc-go's reflection server
// and client against Xo's, over the two files of internal/stdtest/testdata/grpcreflect (b.proto, and a.proto, which
// imports it, extends b.Resp and declares a.Svc).

func reflectFiles() *protoregistry.Files {
	opt := descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum()
	str := descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum()
	i32 := descriptorpb.FieldDescriptorProto_TYPE_INT32.Enum()
	b := &descriptorpb.FileDescriptorProto{
		Name:    proto.String("b.proto"),
		Package: proto.String("b"),
		MessageType: []*descriptorpb.DescriptorProto{{
			Name:           proto.String("Resp"),
			Field:          []*descriptorpb.FieldDescriptorProto{{Name: proto.String("r"), Number: proto.Int32(1), Label: opt, Type: str, JsonName: proto.String("r")}},
			ExtensionRange: []*descriptorpb.DescriptorProto_ExtensionRange{{Start: proto.Int32(100), End: proto.Int32(200)}},
		}},
	}
	a := &descriptorpb.FileDescriptorProto{
		Name:       proto.String("a.proto"),
		Package:    proto.String("a"),
		Dependency: []string{"b.proto"},
		MessageType: []*descriptorpb.DescriptorProto{{
			Name: proto.String("Req"),
			Field: []*descriptorpb.FieldDescriptorProto{
				{Name: proto.String("q"), Number: proto.Int32(1), Label: opt, Type: str, JsonName: proto.String("q")},
				{Name: proto.String("n"), Number: proto.Int32(2), Label: opt, Type: i32, JsonName: proto.String("n"), OneofIndex: proto.Int32(0)},
			},
			EnumType: []*descriptorpb.EnumDescriptorProto{{
				Name:  proto.String("Kind"),
				Value: []*descriptorpb.EnumValueDescriptorProto{{Name: proto.String("K0"), Number: proto.Int32(0)}, {Name: proto.String("K1"), Number: proto.Int32(1)}},
			}},
			OneofDecl: []*descriptorpb.OneofDescriptorProto{{Name: proto.String("pick")}},
		}},
		Extension: []*descriptorpb.FieldDescriptorProto{
			{Name: proto.String("tag"), Number: proto.Int32(150), Label: opt, Type: i32, Extendee: proto.String(".b.Resp"), JsonName: proto.String("tag")},
			{Name: proto.String("more"), Number: proto.Int32(120), Label: opt, Type: i32, Extendee: proto.String(".b.Resp"), JsonName: proto.String("more")},
		},
		Service: []*descriptorpb.ServiceDescriptorProto{{
			Name:   proto.String("Svc"),
			Method: []*descriptorpb.MethodDescriptorProto{{Name: proto.String("Get"), InputType: proto.String(".a.Req"), OutputType: proto.String(".b.Resp")}},
		}},
	}
	files, err := protodesc.NewFiles(&descriptorpb.FileDescriptorSet{File: []*descriptorpb.FileDescriptorProto{b, a}})
	if err != nil {
		panic(err)
	}
	return files
}

// reflectServe serves grpc-go's reflection (v1 and v1alpha) over reflectFiles with a.Svc registered, printing its
// address first.
func reflectServe() {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		panic(err)
	}
	fmt.Println(l.Addr().String())
	s := grpc.NewServer()
	s.RegisterService(&grpc.ServiceDesc{
		ServiceName: "a.Svc",
		HandlerType: (*any)(nil),
		// listed by reflection, never called
		Methods: []grpc.MethodDesc{{MethodName: "Get", Handler: func(_ any, _ context.Context, _ func(any) error, _ grpc.UnaryServerInterceptor) (any, error) {
			return nil, nil
		}}},
	}, struct{}{})
	files := reflectFiles()
	types := new(protoregistry.Types)
	files.RangeFiles(func(fd protoreflect.FileDescriptor) bool {
		for i := 0; i < fd.Extensions().Len(); i++ {
			if err := types.RegisterExtension(dynamicpb.NewExtensionType(fd.Extensions().Get(i))); err != nil {
				panic(err)
			}
		}
		return true
	})
	opts := reflection.ServerOptions{Services: s, DescriptorResolver: files, ExtensionResolver: types}
	rpb.RegisterServerReflectionServer(s, reflection.NewServerV1(opts))
	rpbalpha.RegisterServerReflectionServer(s, reflection.NewServer(opts))
	s.Serve(l)
}

func fileNames(r *rpb.ServerReflectionResponse) string {
	if e := r.GetErrorResponse(); e != nil {
		// protobuf-go writes the space after "proto:" as U+00A0 in some builds (internal/detrand, chosen from the
		// binary's hash) so that callers do not match on its texts: printed as a plain space
		return fmt.Sprintf("err code %d: %s", e.ErrorCode, strings.ReplaceAll(e.ErrorMessage, "\u00a0", " "))
	}
	var names []string
	for _, b := range r.GetFileDescriptorResponse().GetFileDescriptorProto() {
		var f descriptorpb.FileDescriptorProto
		if err := proto.Unmarshal(b, &f); err != nil {
			return "bad file: " + err.Error()
		}
		names = append(names, f.GetName())
	}
	return "[" + strings.Join(names, " ") + "]"
}

// reflectClient asks a server at addr what testdata/grpcreflect's client asks, each on one stream, as Xo's ask does.
func reflectClient(addr string) {
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		panic(err)
	}
	defer conn.Close()
	c := rpb.NewServerReflectionClient(conn)
	ask := func(req *rpb.ServerReflectionRequest) *rpb.ServerReflectionResponse {
		st, err := c.ServerReflectionInfo(context.Background())
		if err != nil {
			panic(err)
		}
		if err := st.Send(req); err != nil {
			panic(err)
		}
		st.CloseSend()
		r, err := st.Recv()
		if err != nil {
			return &rpb.ServerReflectionResponse{MessageResponse: &rpb.ServerReflectionResponse_ErrorResponse{ErrorResponse: &rpb.ErrorResponse{ErrorCode: -1, ErrorMessage: err.Error()}}}
		}
		return r
	}
	var svcs []string
	for _, s := range ask(&rpb.ServerReflectionRequest{MessageRequest: &rpb.ServerReflectionRequest_ListServices{}}).GetListServicesResponse().GetService() {
		svcs = append(svcs, s.GetName())
	}
	sort.Strings(svcs)
	fmt.Printf("list: [%s]\n", strings.Join(svcs, " "))
	for _, sym := range []string{"a.Svc.Get", "a.Req.K1", "b.Resp", "a.Nope"} {
		fmt.Printf("symbol %s: %s\n", sym, fileNames(ask(&rpb.ServerReflectionRequest{MessageRequest: &rpb.ServerReflectionRequest_FileContainingSymbol{FileContainingSymbol: sym}})))
	}
	for _, f := range []string{"b.proto", "c.proto"} {
		fmt.Printf("file %s: %s\n", f, fileNames(ask(&rpb.ServerReflectionRequest{MessageRequest: &rpb.ServerReflectionRequest_FileByFilename{FileByFilename: f}})))
	}
	fmt.Printf("extension b.Resp 150: %s\n", fileNames(ask(&rpb.ServerReflectionRequest{MessageRequest: &rpb.ServerReflectionRequest_FileContainingExtension{FileContainingExtension: &rpb.ExtensionRequest{ContainingType: "b.Resp", ExtensionNumber: 150}}})))
	ext := ask(&rpb.ServerReflectionRequest{MessageRequest: &rpb.ServerReflectionRequest_AllExtensionNumbersOfType{AllExtensionNumbersOfType: "b.Resp"}}).GetAllExtensionNumbersResponse()
	fmt.Printf("extensions %s: %v\n", ext.GetBaseTypeName(), ext.GetExtensionNumber())
}
