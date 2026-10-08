package main

import (
	"context"
	"flag"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	addr := flag.String("addr", ":8080", "listen address")
	flag.Parse()

	lim, err := NewLimiter(Limit{Capacity: DefaultCapacity, RefillPerSec: DefaultRefillPerSec}, nil, nil)
	if err != nil {
		log.Fatal(err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("listening on %s", ln.Addr())
	if err := Serve(ctx, ln, lim); err != nil {
		log.Printf("server: %v", err)
		stop()
		os.Exit(1)
	}
}
