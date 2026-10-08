// Command benchtask serves an in-memory users CRUD API. Copy of
// bench/go/plain/01-crud-api for bench/perf/http, listening on loopback only.
package main

import (
	"log"
	"net/http"
	"os"
	"time"
)

func addr() string {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	return "127.0.0.1:" + port
}

func main() {
	srv := &http.Server{
		Addr:              addr(),
		Handler:           NewHandler(NewStore()),
		ReadHeaderTimeout: 10 * time.Second,
	}
	log.Printf("listening on %s", srv.Addr)
	log.Fatal(srv.ListenAndServe())
}
