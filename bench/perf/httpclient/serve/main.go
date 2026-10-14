// The server of bench/perf/httpclient: answers each request with a
// fixed response written raw in one piece, then closes the connection.
// Paths: /len (a 200 with nine header fields and a 256 byte body by
// Content-Length), /chunked (the same fields, 4 KiB in 32 chunks),
// /big (64 KiB by Content-Length), /redirect (a 302 to /len). Prints
// "ready" once it listens on -addr.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"net"
	"strings"
)

func head(extra string) string {
	return "HTTP/1.1 200 OK\r\nContent-Type: text/plain; charset=utf-8\r\nDate: Wed, 14 Oct 2026 06:00:00 GMT\r\n" +
		"Server: bench\r\nCache-Control: no-cache\r\nX-Request-Id: 0123456789abcdef\r\nVary: Accept-Encoding\r\n" +
		"Set-Cookie: a=1\r\nSet-Cookie: b=2\r\n" + extra
}

func responses() map[string]string {
	body := strings.Repeat("x", 256)
	var ch strings.Builder
	for i := 0; i < 32; i++ {
		fmt.Fprintf(&ch, "80\r\n%s\r\n", strings.Repeat("c", 128))
	}
	ch.WriteString("0\r\n\r\n")
	return map[string]string{
		"/len":      head(fmt.Sprintf("Content-Length: %d\r\n\r\n", len(body))) + body,
		"/chunked":  head("Transfer-Encoding: chunked\r\n\r\n") + ch.String(),
		"/big":      head(fmt.Sprintf("Content-Length: %d\r\n\r\n", 64<<10)) + strings.Repeat("b", 64<<10),
		"/redirect": "HTTP/1.1 302 Found\r\nLocation: /len\r\nContent-Length: 0\r\n\r\n",
	}
}

func main() {
	addr := flag.String("addr", "127.0.0.1:18095", "listen address")
	flag.Parse()
	rs := responses()
	l, err := net.Listen("tcp", *addr)
	if err != nil {
		panic(err)
	}
	fmt.Println("ready")
	for {
		c, err := l.Accept()
		if err != nil {
			return
		}
		go func() {
			defer c.Close()
			br := bufio.NewReader(c)
			line, err := br.ReadString('\n')
			if err != nil {
				return
			}
			for {
				h, err := br.ReadString('\n')
				if err != nil || h == "\r\n" {
					break
				}
			}
			resp := "HTTP/1.1 404 Not Found\r\nContent-Length: 0\r\n\r\n"
			if f := strings.Fields(line); len(f) >= 2 {
				if r, ok := rs[f[1]]; ok {
					resp = r
				}
			}
			io.WriteString(c, resp)
		}()
	}
}
