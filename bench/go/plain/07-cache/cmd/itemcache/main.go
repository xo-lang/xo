// Command itemcache serves GET /items/{id} from a loading cache in front of
// an upstream item service.
package main

import (
	"flag"
	"log/slog"
	"net/http"
	"os"
	"time"

	"benchtask/cache"
	"benchtask/service"
)

func main() {
	addr := flag.String("addr", ":8080", "listen address")
	upstream := flag.String("upstream", "http://localhost:9090", "upstream base URL")
	flag.Parse()

	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	up := &service.HTTPUpstream{BaseURL: *upstream, Client: &http.Client{Timeout: 5 * time.Second}}
	srv, err := service.New(up, cache.Options{
		MaxEntries:  10000,
		TTL:         30 * time.Second,
		StaleTTL:    5 * time.Minute,
		NegativeTTL: 10 * time.Second,
		LoadTimeout: 2 * time.Second,
	}, log)
	if err != nil {
		log.Error("init", "err", err)
		os.Exit(1)
	}
	hs := &http.Server{Addr: *addr, Handler: srv, ReadHeaderTimeout: 5 * time.Second}
	log.Info("listening", "addr", *addr, "upstream", *upstream)
	if err := hs.ListenAndServe(); err != nil {
		log.Error("serve", "err", err)
		os.Exit(1)
	}
}
