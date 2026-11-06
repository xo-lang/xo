package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/mem"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// bytesCodec passes message bytes through (the interop methods carry plain text, not protobuf).
type bytesCodec struct{}

func (bytesCodec) Marshal(v any) (mem.BufferSlice, error) {
	return mem.BufferSlice{mem.SliceBuffer(*v.(*[]byte))}, nil
}
func (bytesCodec) Unmarshal(d mem.BufferSlice, v any) error {
	*(v.(*[]byte)) = d.Materialize()
	return nil
}
func (bytesCodec) Name() string { return "proto" }

func recvMsg(s grpc.ServerStream) ([]byte, error) {
	var b []byte
	err := s.RecvMsg(&b)
	return b, err
}

func sendMsg(s grpc.ServerStream, b []byte) error { return s.SendMsg(&b) }

// interopHandler serves the methods of testdata/grpcinterop (the same behavior as its Xo handlers).
func interopHandler(_ any, s grpc.ServerStream) error {
	method, _ := grpc.MethodFromServerStream(s)
	md, _ := metadata.FromIncomingContext(s.Context())
	first := func(k string) string {
		if v := md.Get(k); len(v) > 0 {
			return v[0]
		}
		return ""
	}
	switch method {
	case "/t.S/Echo":
		req, err := recvMsg(s)
		if err != nil {
			return err
		}
		if first("x-fail") == "1" {
			return status.Error(codes.NotFound, "no such user é%")
		}
		return sendMsg(s, append(req, first("x-who")...))
	case "/t.S/Count":
		m, err := recvMsg(s)
		if err != nil {
			return err
		}
		n, _ := strconv.Atoi(string(m))
		for i := 0; i < n; i++ {
			if err := sendMsg(s, []byte(strconv.Itoa(i))); err != nil {
				return err
			}
		}
		return nil
	case "/t.S/Sum":
		total := 0
		for {
			m, err := recvMsg(s)
			if err == io.EOF {
				break
			}
			if err != nil {
				return err
			}
			n, _ := strconv.Atoi(string(m))
			total += n
		}
		return sendMsg(s, []byte(strconv.Itoa(total)))
	case "/t.S/Echo2":
		for {
			m, err := recvMsg(s)
			if err == io.EOF {
				break
			}
			if err != nil {
				return err
			}
			if err := sendMsg(s, m); err != nil {
				return err
			}
		}
		return status.Error(codes.Aborted, "done")
	}
	parts := strings.Split(method, "/")
	return status.Errorf(codes.Unimplemented, "unknown method %s for service %s", parts[2], parts[1])
}

// interopServe listens on a free port, prints its address and serves until killed.
func interopServe() {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		panic(err)
	}
	fmt.Println(l.Addr().String())
	srv := grpc.NewServer(grpc.ForceServerCodecV2(bytesCodec{}), grpc.UnknownServiceHandler(interopHandler))
	_ = srv.Serve(l)
}

func quoteList(l []string) string {
	q := make([]string, len(l))
	for i, s := range l {
		q[i] = strconv.Quote(s)
	}
	return "[" + strings.Join(q, ", ") + "]"
}

// interopClient makes the calls of testdata/grpcinterop against addr and prints one line per call.
func interopClient(addr string) {
	cc := dial(addr)
	defer cc.Close()
	unary := func(method, req string, kv ...string) string {
		ctx := metadata.AppendToOutgoingContext(context.Background(), kv...)
		in := []byte(req)
		var out []byte
		if err := cc.Invoke(ctx, method, &in, &out, grpc.ForceCodecV2(bytesCodec{})); err != nil {
			return "err " + err.Error()
		}
		return "ok " + string(out)
	}
	fmt.Println("echo: " + unary("/t.S/Echo", "hi ", "x-who", "bob"))
	fmt.Println("fail: " + unary("/t.S/Echo", "x", "x-fail", "1"))
	fmt.Println("missing: " + unary("/t.S/Missing", "x"))
	desc := &grpc.StreamDesc{ServerStreams: true, ClientStreams: true}
	open := func(method string) grpc.ClientStream {
		st, err := cc.NewStream(context.Background(), desc, method, grpc.ForceCodecV2(bytesCodec{}))
		if err != nil {
			panic(err)
		}
		return st
	}
	send := func(st grpc.ClientStream, s string) {
		b := []byte(s)
		if err := st.SendMsg(&b); err != nil {
			panic(err)
		}
	}
	recv := func(st grpc.ClientStream) (string, error) {
		var b []byte
		err := st.RecvMsg(&b)
		return string(b), err
	}
	st := open("/t.S/Count")
	send(st, "3")
	st.CloseSend()
	var got []string
	for {
		m, err := recv(st)
		if err != nil {
			break
		}
		got = append(got, m)
	}
	fmt.Println("count: " + quoteList(got))
	st = open("/t.S/Sum")
	for i := 1; i < 5; i++ {
		send(st, strconv.Itoa(i))
	}
	st.CloseSend()
	m, _ := recv(st)
	fmt.Println("sum: " + m)
	st = open("/t.S/Echo2")
	got = nil
	for i := 0; i < 3; i++ {
		send(st, "m"+strconv.Itoa(i))
		m, _ := recv(st)
		got = append(got, m)
	}
	st.CloseSend()
	if _, err := recv(st); err != nil {
		got = append(got, err.Error())
	} else {
		got = append(got, "extra")
	}
	fmt.Println("bidi: " + quoteList(got))
}

func interopMain() bool {
	if len(os.Args) < 2 {
		return false
	}
	switch os.Args[1] {
	case "serve":
		interopServe()
	case "client":
		interopClient(os.Args[2])
	default:
		return false
	}
	return true
}
