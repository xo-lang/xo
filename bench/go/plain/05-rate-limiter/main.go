package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// NewHandler returns the application routes wrapped in the limiter.
func NewHandler(l *Limiter) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /hello", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"msg": "hi"})
	})
	return l.Middleware(mux)
}

// Run serves on addr until ctx is cancelled, then stops the evictor and
// shuts the server down.
func Run(ctx context.Context, addr string, l *Limiter) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	evictorDone := l.RunEvictor(ctx, DefaultEvictEvery)
	srv := &http.Server{Addr: addr, Handler: NewHandler(l), ReadHeaderTimeout: 5 * time.Second}

	errc := make(chan error, 1)
	go func() { errc <- srv.ListenAndServe() }()

	var err error
	select {
	case <-ctx.Done():
	case err = <-errc:
	}
	cancel()
	shutCtx, shutCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutCancel()
	if serr := srv.Shutdown(shutCtx); serr != nil && err == nil {
		err = serr
	}
	<-evictorDone
	if errors.Is(err, http.ErrServerClosed) {
		err = nil
	}
	return err
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	l := NewLimiter(map[string]Limit{
		"premium": {Capacity: 100, RefillPerSec: 50},
	})
	if err := Run(ctx, ":8080", l); err != nil {
		log.Fatal(err)
	}
}
