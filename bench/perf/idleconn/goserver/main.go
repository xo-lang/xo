// goserver serves "ok" on every path for measuring idle connections:
// plain HTTP/1.1 on PORT, or HTTPS (h2 by ALPN) with CERT and KEY files.
package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "ok") })
	addr := "127.0.0.1:" + port
	if cert := os.Getenv("CERT"); cert != "" {
		log.Fatal(http.ListenAndServeTLS(addr, cert, os.Getenv("KEY"), h))
	}
	log.Fatal(http.ListenAndServe(addr, h))
}
