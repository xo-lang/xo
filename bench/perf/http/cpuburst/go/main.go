// The cpuburst server in Go (bench/perf/http/cpuburst/README.md), the
// reference for the Xo one: the same routes, /cpu calibrated to the same
// time (Go's loop iteration costs twice Xo's here).
package main

import (
	"log"
	"net/http"
	"os"
	"strconv"
)

func spin(n int) int {
	x := 12345
	for i := 0; i < n; i++ {
		x = (x*1103515245 + 12345) % 2147483648
	}
	return x
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "18095"
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /users/{id}", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"id":` + r.PathValue("id") + `,"name":"a"}`))
	})
	mux.HandleFunc("POST /users", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(201)
		w.Write([]byte(`{"id":1}`))
	})
	mux.HandleFunc("GET /users", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("[]")) })
	mux.HandleFunc("GET /cpu", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(strconv.Itoa(spin(30000000))))
	})
	log.Fatal(http.ListenAndServe("127.0.0.1:"+port, mux))
}
