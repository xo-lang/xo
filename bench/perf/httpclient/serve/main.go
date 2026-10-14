// The server of bench/perf/httpclient: answers each request with a
// fixed response written raw in one piece, then closes the connection.
// Paths: /len (a 200 with nine header fields and a 256 byte body by
// Content-Length), /chunked (the same fields, 4 KiB in 32 chunks),
// /big (64 KiB by Content-Length), /redirect (a 302 to /len). With
// -tlsaddr it also serves HTTP/2 over TLS there (audit item B11, the
// HTTP/2 client's HPACK): /h2len (the same nine fields and a 256 byte
// body), /h2big (64 KiB), with a certificate made at start whose PEM it
// writes to -ca. Prints "ready" once it listens.
package main

import (
	"bufio"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"flag"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"os"
	"strings"
	"time"
)

// serveH2 serves /h2len and /h2big over HTTP/2 with TLS on addr, its
// self-signed certificate written to caFile.
func serveH2(addr, caFile string) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		panic(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "bench"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		panic(err)
	}
	if err := os.WriteFile(caFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o644); err != nil {
		panic(err)
	}
	body := strings.Repeat("x", 256)
	bigBody := strings.Repeat("b", 64<<10)
	fields := func(w http.ResponseWriter) {
		h := w.Header()
		h.Set("Content-Type", "text/plain; charset=utf-8")
		h.Set("Date", "Wed, 14 Oct 2026 06:00:00 GMT")
		h.Set("Server", "bench")
		h.Set("Cache-Control", "no-cache")
		h.Set("X-Request-Id", "0123456789abcdef")
		h.Set("Vary", "Accept-Encoding")
		h["Set-Cookie"] = []string{"a=1", "b=2"}
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/h2len", func(w http.ResponseWriter, r *http.Request) {
		fields(w)
		io.WriteString(w, body)
	})
	mux.HandleFunc("/h2big", func(w http.ResponseWriter, r *http.Request) {
		fields(w)
		io.WriteString(w, bigBody)
	})
	cfg := &tls.Config{
		Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}},
		NextProtos:   []string{"h2"},
	}
	l, err := tls.Listen("tcp", addr, cfg)
	if err != nil {
		panic(err)
	}
	srv := &http.Server{Handler: mux, TLSConfig: cfg}
	go srv.Serve(l)
}

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
	tlsAddr := flag.String("tlsaddr", "", "HTTP/2 over TLS listen address")
	caFile := flag.String("ca", "", "the file for the HTTP/2 server's CA (PEM)")
	flag.Parse()
	if *tlsAddr != "" {
		serveH2(*tlsAddr, *caFile)
	}
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
