// The server of bench/perf/httpstream: GET /big?n=N writes N bytes in
// 64 KiB writes with a Content-Length, so the client's reading is the
// measured work. Prints "ready" once listening.
package main

import (
	"flag"
	"fmt"
	"net"
	"net/http"
	"strconv"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:18097", "listen address")
	flag.Parse()
	buf := make([]byte, 65536)
	for i := range buf {
		buf[i] = byte('a' + i%26)
	}
	http.HandleFunc("/big", func(w http.ResponseWriter, r *http.Request) {
		n, _ := strconv.Atoi(r.URL.Query().Get("n"))
		w.Header().Set("Content-Length", strconv.Itoa(n))
		for n > 0 {
			k := min(n, len(buf))
			if _, err := w.Write(buf[:k]); err != nil {
				return
			}
			n -= k
		}
	})
	l, err := net.Listen("tcp", *addr)
	if err != nil {
		panic(err)
	}
	fmt.Println("ready")
	http.Serve(l, nil)
}
