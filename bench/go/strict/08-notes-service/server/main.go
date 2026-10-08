// Command server runs the notes HTTP service.
package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"benchtask/api"
	"benchtask/store"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	err := run(ctx, os.Args[1:])
	stop()
	if err != nil {
		log.Fatal(err)
	}
}

// run serves until ctx is cancelled, then shuts down gracefully, letting
// in flight requests finish for up to 15 seconds.
func run(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("server", flag.ContinueOnError)
	dataDir := flags.String("data-dir", "./data", "directory holding one JSON file per note")
	addr := flags.String("addr", ":8080", "listen address")
	if err := flags.Parse(args); err != nil {
		return err
	}

	st, err := store.Open(*dataDir)
	if err != nil {
		return err
	}
	logger := slog.Default()
	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		return err
	}
	srv := &http.Server{
		Handler:           api.New(st, api.Options{Logger: logger}).Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       time.Minute,
		IdleTimeout:       2 * time.Minute,
	}
	logger.Info("listening", "addr", ln.Addr().String(), "data_dir", *dataDir)

	errCh := make(chan error, 1)
	go func() { errCh <- srv.Serve(ln) }()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
	}

	logger.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return err
	}
	if err := <-errCh; err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
