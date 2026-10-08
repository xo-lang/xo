// Command benchtask serves an in-memory users CRUD API.
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
	return ":" + port
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
