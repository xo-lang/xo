// Command server runs the notes HTTP service.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"benchtask/api"
	"benchtask/store"
)

const shutdownTimeout = 10 * time.Second

func main() {
	dataDir := flag.String("data-dir", "data", "directory holding one JSON file per note")
	addr := flag.String("addr", ":8080", "listen address")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	err := run(ctx, *dataDir, *addr, func(a string) { log.Printf("listening on %s", a) })
	if err != nil {
		log.Fatal(err)
	}
	log.Print("shut down cleanly")
}

// run serves until ctx is cancelled, then shuts down gracefully, letting
// in flight requests finish. ready, if non-nil, receives the bound address.
func run(ctx context.Context, dataDir, addr string, ready func(string)) error {
	st, err := store.Open(dataDir)
	if err != nil {
		return err
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	srv := &http.Server{
		Handler:           api.New(st, time.Now),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	errc := make(chan error, 1)
	go func() { errc <- srv.Serve(ln) }()
	if ready != nil {
		ready(ln.Addr().String())
	}

	select {
	case err := <-errc:
		return fmt.Errorf("serve: %w", err)
	case <-ctx.Done():
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown: %w", err)
	}
	if err := <-errc; err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("serve: %w", err)
	}
	return nil
}
