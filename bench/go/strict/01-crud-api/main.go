// Command benchtask serves an in-memory users CRUD JSON API.
package main

import (
	"log"
	"net/http"
	"os"
	"time"
)

func listenAddr() string {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	return ":" + port
}

func main() {
	srv := &http.Server{
		Addr:              listenAddr(),
		Handler:           NewHandler(NewStore()),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	log.Printf("listening on %s", srv.Addr)
	if err := srv.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}
