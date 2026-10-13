// The client of bench/perf/httpparse (decision 0193 step 6): a fixed
// amount of requests per workload, so the server's instruction count is
// comparable between builds.
//
//	load -gencert DIR                 writes cert.pem and key.pem (127.0.0.1)
//	load -addr A -w WORKLOAD -n N     sends N requests and exits
//
// Workloads: h1_get (GETs with 14 browser like header fields, pipelined
// 50 at a time on one connection), h1_chunked (POSTs of a chunked body of
// 8 chunks and a trailer), h1_url (GETs of escaped targets with 20 query
// parameters), h2_get (HTTP/2 over TLS, the same fields every request,
// one request at a time on one connection, so the server does the same
// work every run: HPACK indexes them), h2_varied (fields
// whose values change per request: literals, Huffman, table churn).
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
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const fields = "Host: 127.0.0.1\r\n" +
	"User-Agent: Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/128.0.0.0 Safari/537.36\r\n" +
	"Accept: text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8\r\n" +
	"Accept-Language: en-US,en;q=0.9\r\n" +
	"Accept-Encoding: gzip, deflate, br\r\n" +
	"Cookie: session=4f3c2a1b9e8d7c6b; theme=dark; id=42\r\n" +
	"Cache-Control: no-cache\r\n" +
	"Pragma: no-cache\r\n" +
	"Sec-Fetch-Dest: document\r\n" +
	"Sec-Fetch-Mode: navigate\r\n" +
	"Sec-Fetch-Site: none\r\n" +
	"Upgrade-Insecure-Requests: 1\r\n" +
	"X-Request-Id: 7d1c5e0a-3b2f-4e8a-9c6d-1a2b3c4d5e6f\r\n" +
	"Connection: keep-alive\r\n"

func h1Request(w string, i int) string {
	switch w {
	case "h1_get":
		return fmt.Sprintf("GET /p/%d?a=1&b=two HTTP/1.1\r\n%s\r\n", i, fields)
	case "h1_chunked":
		var b strings.Builder
		fmt.Fprintf(&b, "POST /u HTTP/1.1\r\nHost: 127.0.0.1\r\nContent-Type: application/octet-stream\r\nTransfer-Encoding: chunked\r\n\r\n")
		for k := 0; k < 8; k++ {
			fmt.Fprintf(&b, "40;k=%d\r\n%s\r\n", k, strings.Repeat(string(rune('a'+k)), 64))
		}
		b.WriteString("0\r\nX-Checksum: 1234\r\n\r\n")
		return b.String()
	case "h1_url":
		var q strings.Builder
		for k := 0; k < 20; k++ {
			if k > 0 {
				q.WriteByte('&')
			}
			fmt.Fprintf(&q, "key%%20%d=value+%d%%2Fx%%E2%%82%%AC", k, i)
		}
		return fmt.Sprintf("GET http://127.0.0.1/p/caf%%C3%%A9%%20%d?%s HTTP/1.1\r\nHost: 127.0.0.1\r\n\r\n", i, q.String())
	}
	panic("workload " + w)
}

// h1 sends n requests on one connection, 50 at a time, and reads the
// answers.
func h1(addr, w string, n int) error {
	c, err := net.Dial("tcp", addr)
	if err != nil {
		return err
	}
	defer c.Close()
	br := bufio.NewReaderSize(c, 1<<16)
	for done := 0; done < n; {
		k := min(50, n-done)
		var b strings.Builder
		for j := 0; j < k; j++ {
			b.WriteString(h1Request(w, done+j))
		}
		errc := make(chan error, 1)
		go func() {
			_, err := io.WriteString(c, b.String())
			errc <- err
		}()
		for j := 0; j < k; j++ {
			if err := readAnswer(br); err != nil {
				return fmt.Errorf("answer %d: %v", done+j, err)
			}
		}
		if err := <-errc; err != nil {
			return err
		}
		done += k
	}
	return nil
}

func readAnswer(br *bufio.Reader) error {
	line, err := br.ReadString('\n')
	if err != nil {
		return err
	}
	if !strings.HasPrefix(line, "HTTP/1.1 200 ") {
		return fmt.Errorf("status %q", line)
	}
	cl := -1
	for {
		h, err := br.ReadString('\n')
		if err != nil {
			return err
		}
		if h == "\r\n" {
			break
		}
		if v, ok := strings.CutPrefix(h, "Content-Length: "); ok {
			cl, _ = strconv.Atoi(strings.TrimSpace(v))
		}
	}
	if cl < 0 {
		return fmt.Errorf("no Content-Length")
	}
	_, err = io.CopyN(io.Discard, br, int64(cl))
	return err
}

// h2 sends n requests as streams of one HTTP/2 connection, one at a
// time.
func h2(addr, w string, n int) error {
	tr := &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, MaxConnsPerHost: 1}
	tr.Protocols = new(http.Protocols)
	tr.Protocols.SetHTTP2(true)
	cl := &http.Client{Transport: tr, Timeout: 30 * time.Second}
	var next atomic.Int64
	var wg sync.WaitGroup
	var first error
	var mu sync.Mutex
	for range 1 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				i := int(next.Add(1)) - 1
				if i >= n {
					return
				}
				req, _ := http.NewRequest("GET", fmt.Sprintf("https://%s/p/%d?a=1&b=two", addr, i), nil)
				for _, l := range strings.Split(strings.TrimSpace(fields), "\r\n") {
					k, v, _ := strings.Cut(l, ": ")
					if k == "Host" || k == "Connection" {
						continue
					}
					if w == "h2_varied" && (k == "X-Request-Id" || k == "Cookie") {
						v = fmt.Sprintf("%s-%d-%x", v, i, i*2654435761)
					}
					req.Header.Set(k, v)
				}
				resp, err := cl.Do(req)
				if err == nil {
					io.Copy(io.Discard, resp.Body)
					resp.Body.Close()
					if resp.StatusCode != 200 || resp.ProtoMajor != 2 {
						err = fmt.Errorf("%s %d", resp.Proto, resp.StatusCode)
					}
				}
				if err != nil {
					mu.Lock()
					if first == nil {
						first = err
					}
					mu.Unlock()
					return
				}
			}
		}()
	}
	wg.Wait()
	return first
}

func genCert(dir string) error {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return err
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "127.0.0.1"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		IPAddresses:  []net.IP{net.IPv4(127, 0, 0, 1)},
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return err
	}
	kd, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "cert.pem"), pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o644); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "key.pem"), pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: kd}), 0o600)
}

func main() {
	gencert := flag.String("gencert", "", "write cert.pem and key.pem into this directory")
	addr := flag.String("addr", "127.0.0.1:18090", "the server (loopback)")
	w := flag.String("w", "h1_get", "the workload")
	n := flag.Int("n", 10000, "requests")
	flag.Parse()
	if *gencert != "" {
		if err := genCert(*gencert); err != nil {
			fmt.Fprintln(os.Stderr, "load:", err)
			os.Exit(1)
		}
		return
	}
	if !strings.HasPrefix(*addr, "127.0.0.1:") {
		fmt.Fprintln(os.Stderr, "load: loopback only")
		os.Exit(2)
	}
	var err error
	if strings.HasPrefix(*w, "h2") {
		err = h2(*addr, *w, *n)
	} else {
		err = h1(*addr, *w, *n)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "load:", err)
		os.Exit(1)
	}
}
