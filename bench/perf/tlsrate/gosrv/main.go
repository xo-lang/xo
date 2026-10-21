// The Go side of bench/perf/tlsrate: net/http over TLS (HTTP/1.1 and,
// by ALPN, HTTP/2) answering GET /hs with 16 bytes and GET /bulk?n=N
// with N zero bytes written in 64 KiB pieces (no Content-Length, so a
// chunked body on HTTP/1.1, DATA frames on HTTP/2, as the Xo server's
// http.stream). CERT and KEY name PEM files, PORT the port.
package main

import (
	"log"
	"net/http"
	"os"
	"strconv"
)

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /hs", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Write([]byte("0123456789abcdef"))
	})
	piece := make([]byte, 65536)
	mux.HandleFunc("GET /bulk", func(w http.ResponseWriter, r *http.Request) {
		n, _ := strconv.Atoi(r.URL.Query().Get("n"))
		w.Header().Set("Content-Type", "application/octet-stream")
		for n > 0 {
			k := min(n, len(piece))
			if _, err := w.Write(piece[:k]); err != nil {
				return
			}
			n -= k
		}
	})
	srv := &http.Server{Addr: "127.0.0.1:" + os.Getenv("PORT"), Handler: mux}
	log.Fatal(srv.ListenAndServeTLS(os.Getenv("CERT"), os.Getenv("KEY")))
}
