package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	os.Exit(realMain())
}

func realMain() int {
	addr := flag.String("addr", ":8080", "listen address")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		log.Printf("listen: %v", err)
		return 1
	}
	ticker := time.NewTicker(SweepInterval)
	defer ticker.Stop()

	svc := NewService(time.Now)
	log.Printf("inventory listening on %s", ln.Addr())
	if err := run(ctx, ln, svc, ticker.C); err != nil {
		log.Printf("server: %v", err)
		return 1
	}
	return 0
}

// run serves HTTP on ln and runs the sweeper on ticks until ctx is done, then
// shuts down gracefully: in flight requests finish and the sweeper stops
// before run returns.
func run(ctx context.Context, ln net.Listener, svc *Service, ticks <-chan time.Time) error {
	srv := &http.Server{Handler: NewHandler(svc), ReadHeaderTimeout: 5 * time.Second}

	sweepCtx, cancelSweep := context.WithCancel(context.Background())
	defer cancelSweep()
	sweepDone := make(chan struct{})
	go func() {
		defer close(sweepDone)
		svc.RunSweeper(sweepCtx, ticks)
	}()

	serveErr := make(chan error, 1)
	go func() {
		serveErr <- srv.Serve(ln)
	}()

	select {
	case <-ctx.Done():
	case err := <-serveErr:
		cancelSweep()
		<-sweepDone
		return err
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	err := srv.Shutdown(shutdownCtx)
	cancelSweep()
	<-sweepDone
	if sErr := <-serveErr; err == nil && !errors.Is(sErr, http.ErrServerClosed) {
		err = sErr
	}
	return err
}
