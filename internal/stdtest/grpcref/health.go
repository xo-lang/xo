package main

import (
	"context"
	"fmt"
	"net"
	"sort"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
)

// The health checking oracle for std/grpchealth (decision 0546 item 5, TestGrpcHealthInterop): grpc-go's health
// server and client against Xo's. Besides the health service, a server takes /t.Ctl/Set, whose request is a
// HealthCheckRequest naming "SERVICE=STATUS" (a status number), "!shutdown" or "!resume", so the client can drive
// the status changes Watch reports.

// healthServe serves grpc-go's health server with "a.A" NOT_SERVING, printing its address first.
func healthServe() {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		panic(err)
	}
	fmt.Println(l.Addr().String())
	hs := health.NewServer()
	hs.SetServingStatus("a.A", healthpb.HealthCheckResponse_NOT_SERVING)
	ctl := func(_ any, s grpc.ServerStream) error {
		var req healthpb.HealthCheckRequest
		if err := s.RecvMsg(&req); err != nil {
			return err
		}
		switch req.Service {
		case "!shutdown":
			hs.Shutdown()
		case "!resume":
			hs.Resume()
		default:
			name, st, _ := strings.Cut(req.Service, "=")
			var n int32
			fmt.Sscan(st, &n)
			hs.SetServingStatus(name, healthpb.HealthCheckResponse_ServingStatus(n))
		}
		return s.SendMsg(&healthpb.HealthCheckResponse{})
	}
	srv := grpc.NewServer(grpc.UnknownServiceHandler(ctl))
	healthpb.RegisterHealthServer(srv, hs)
	_ = srv.Serve(l)
}

// healthClient makes the calls of testdata/grpchealth against addr and prints one line per call.
func healthClient(addr string) {
	cc := dial(addr)
	defer cc.Close()
	hc := healthpb.NewHealthClient(cc)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	set := func(v string) {
		var out healthpb.HealthCheckResponse
		if err := cc.Invoke(ctx, "/t.Ctl/Set", &healthpb.HealthCheckRequest{Service: v}, &out); err != nil {
			panic(err)
		}
	}
	check := func(service string) string {
		r, err := hc.Check(ctx, &healthpb.HealthCheckRequest{Service: service})
		if err != nil {
			return "err " + err.Error()
		}
		return r.Status.String()
	}
	fmt.Println("check: " + check(""))
	fmt.Println("check a.A: " + check("a.A"))
	fmt.Println("check nope: " + check("nope"))
	lr, err := hc.List(ctx, &healthpb.HealthListRequest{})
	if err != nil {
		fmt.Println("list: err " + err.Error())
	} else {
		var names []string
		for k := range lr.Statuses {
			names = append(names, k)
		}
		sort.Strings(names)
		var parts []string
		for _, k := range names {
			parts = append(parts, fmt.Sprintf("%q=%s", k, lr.Statuses[k].Status))
		}
		fmt.Println("list: " + strings.Join(parts, " "))
	}
	wctx, wcancel := context.WithCancel(ctx)
	ws, err := hc.Watch(wctx, &healthpb.HealthCheckRequest{Service: "b.B"})
	if err != nil {
		panic(err)
	}
	var seen []string
	for len(seen) < 3 {
		r, err := ws.Recv()
		if err != nil {
			seen = append(seen, "err "+err.Error())
			break
		}
		seen = append(seen, r.Status.String())
		switch len(seen) {
		case 1:
			set("b.B=1")
			set("b.B=1")
		case 2:
			set("b.B=2")
		}
	}
	wcancel()
	fmt.Println("watch b.B: " + quoteList(seen))
	set("!shutdown")
	set("c.C=1")
	fmt.Println("shutdown: " + check("") + " " + check("a.A") + " " + check("c.C"))
	set("!resume")
	fmt.Println("resume: " + check("") + " " + check("a.A") + " " + check("b.B"))
}
