// client opens N connections to a server, makes one request on each, holds
// them idle, and reports the server process's memory from /proc.
//
//	client -gencert DIR                     writes DIR/cert.pem and key.pem
//	client -addr HOST:PORT -n N -mode h1|h2 -pid PID [-settle 5s]
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
	"strings"
	"sync"
	"time"
)

func gencert(dir string) {
	k, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	t := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "localhost"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(24 * time.Hour),
		KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames: []string{"localhost"}, IPAddresses: []net.IP{net.ParseIP("127.0.0.1")}}
	der, _ := x509.CreateCertificate(rand.Reader, t, t, &k.PublicKey, k)
	kb, _ := x509.MarshalECPrivateKey(k)
	os.WriteFile(filepath.Join(dir, "cert.pem"), pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o644)
	os.WriteFile(filepath.Join(dir, "key.pem"), pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: kb}), 0o600)
}

func status(pid int) string {
	b, _ := os.ReadFile(fmt.Sprintf("/proc/%d/status", pid))
	var keep []string
	for _, l := range strings.Split(string(b), "\n") {
		for _, p := range []string{"VmRSS", "RssAnon", "Threads"} {
			if strings.HasPrefix(l, p+":") {
				keep = append(keep, strings.Join(strings.Fields(l), " "))
			}
		}
	}
	return strings.Join(keep, "  ")
}

// stacks sums the sizes of anonymous mappings of 8 KiB to 1 MiB (fiber and
// goroutine-sized regions are not separable from /proc, so this reports the
// mapping count and the Rss of all mappings under 1 MiB).
func smaps(pid int) string {
	b, _ := os.ReadFile(fmt.Sprintf("/proc/%d/smaps_rollup", pid))
	for _, l := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(l, "Pss:") || strings.HasPrefix(l, "Private_Dirty:") {
			return strings.Join(strings.Fields(l), " ")
		}
	}
	return ""
}

func main() {
	gen := flag.String("gencert", "", "write a self-signed cert and key to this directory and exit")
	addr := flag.String("addr", "127.0.0.1:8080", "server address")
	n := flag.Int("n", 1000, "connections")
	mode := flag.String("mode", "h1", "h1 (plain keep-alive) or h2 (TLS, one transport per connection)")
	pid := flag.Int("pid", 0, "server process id")
	settle := flag.Duration("settle", 5*time.Second, "wait after the last response before measuring")
	flag.Parse()
	if *gen != "" {
		gencert(*gen)
		return
	}
	fmt.Printf("before: %s\n", status(*pid))
	var wg sync.WaitGroup
	sem := make(chan struct{}, 64)
	var mu sync.Mutex
	var conns []net.Conn
	var ok, failed int
	for i := 0; i < *n; i++ {
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			var err error
			if *mode == "h2" {
				tr := &http.Transport{ForceAttemptHTTP2: true, TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
					MaxIdleConnsPerHost: 1}
				var resp *http.Response
				resp, err = (&http.Client{Transport: tr}).Get("https://" + *addr + "/")
				if err == nil {
					io.Copy(io.Discard, resp.Body)
					resp.Body.Close()
					if resp.ProtoMajor != 2 {
						err = fmt.Errorf("proto %d", resp.ProtoMajor)
					}
				}
			} else {
				var c net.Conn
				c, err = net.Dial("tcp", *addr)
				if err == nil {
					_, err = c.Write([]byte("GET / HTTP/1.1\r\nHost: x\r\n\r\n"))
					if err == nil {
						r := bufio.NewReader(c)
						var cl int
						for {
							l, e := r.ReadString('\n')
							if e != nil {
								err = e
								break
							}
							if l == "\r\n" {
								break
							}
							fmt.Sscanf(strings.ToLower(l), "content-length: %d", &cl)
						}
						if err == nil {
							_, err = io.CopyN(io.Discard, r, int64(cl))
						}
					}
					mu.Lock()
					conns = append(conns, c)
					mu.Unlock()
				}
			}
			mu.Lock()
			if err != nil {
				failed++
			} else {
				ok++
			}
			mu.Unlock()
		}()
	}
	wg.Wait()
	time.Sleep(*settle)
	fmt.Printf("mode=%s n=%d ok=%d failed=%d\n", *mode, *n, ok, failed)
	fmt.Printf("idle:   %s  %s\n", status(*pid), smaps(*pid))
	_ = conns
}
