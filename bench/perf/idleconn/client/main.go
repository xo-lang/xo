// client opens N connections to a server, makes one request on each, holds
// them idle, and reports the server process's memory from /proc.
//
//	client -gencert DIR                     writes DIR/cert.pem and key.pem
//	client -addr HOST:PORT -n N -mode h1|h2 -pid PID [-settle 5s] [-think 10ms]
//
// With -think D (h1), every connection sends a request D after each
// response for the whole settle window instead of idling, and the server is
// sampled at the window's end while that traffic runs: a connection that is
// never idle longer than D never parks.
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
		for _, p := range []string{"VmHWM", "VmRSS", "RssAnon", "Threads"} {
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

// maps counts the process's mappings and sums the Rss of the anonymous
// ones by size class (the mapping sizes tell fiber and goroutine stacks
// from heap arenas).
func maps(pid int) string {
	b, err := os.ReadFile(fmt.Sprintf("/proc/%d/smaps", pid))
	if err != nil {
		return ""
	}
	type cls struct {
		n   int
		rss int64
	}
	names := []string{"<=64K", "<=256K", "<=1M", "<=16M", ">16M"}
	cs := make([]cls, len(names))
	var size, rss int64
	var anon, have bool
	flush := func() {
		if !have || !anon {
			return
		}
		i := 4
		switch {
		case size <= 64:
			i = 0
		case size <= 256:
			i = 1
		case size <= 1024:
			i = 2
		case size <= 16384:
			i = 3
		}
		cs[i].n++
		cs[i].rss += rss
	}
	total := 0
	for _, l := range strings.Split(string(b), "\n") {
		f := strings.Fields(l)
		if len(f) >= 5 && strings.Contains(f[0], "-") && !strings.HasSuffix(f[0], ":") {
			flush()
			total++
			size, rss, have = 0, 0, true
			anon = len(f) < 6
			continue
		}
		if len(f) >= 2 && f[0] == "Size:" {
			fmt.Sscan(f[1], &size)
		}
		if len(f) >= 2 && f[0] == "Rss:" {
			fmt.Sscan(f[1], &rss)
		}
	}
	flush()
	out := fmt.Sprintf("mappings=%d anon:", total)
	for i, c := range cs {
		out += fmt.Sprintf(" %s n=%d rss=%dK", names[i], c.n, c.rss)
	}
	return out
}

// readResp reads one response's head and Content-Length body.
func readResp(r *bufio.Reader) error {
	var cl int
	for {
		l, e := r.ReadString('\n')
		if e != nil {
			return e
		}
		if l == "\r\n" {
			break
		}
		fmt.Sscanf(strings.ToLower(l), "content-length: %d", &cl)
	}
	_, err := io.CopyN(io.Discard, r, int64(cl))
	return err
}

func main() {
	gen := flag.String("gencert", "", "write a self-signed cert and key to this directory and exit")
	addr := flag.String("addr", "127.0.0.1:8080", "server address")
	n := flag.Int("n", 1000, "connections")
	mode := flag.String("mode", "h1", "h1 (plain keep-alive) or h2 (TLS, one transport per connection)")
	pid := flag.Int("pid", 0, "server process id")
	settle := flag.Duration("settle", 5*time.Second, "wait after the last response before measuring")
	maxmaps := flag.Int("maxmaps", 0, "fail when the server has more than this many mappings (lines of /proc/PID/maps); 0 checks nothing")
	srcs := flag.Int("srcs", 1, "h1: spread the connections over this many loopback source addresses (127.0.0.1, 127.0.0.2, ...): one source address reaches only about 28,000 ephemeral ports to one server address")
	useTLS := flag.Bool("tls", false, "h1: speak HTTP/1.1 over TLS (ALPN http/1.1; the server needs TLS_CERT and TLS_KEY)")
	again := flag.Bool("again", false, "h1: after the idle wait, send a second request on every held connection and fail if any fails")
	think := flag.Duration("think", 0, "h1: during the settle window, send a request on every connection this long after each response, and sample while that runs (0: the connections idle)")
	flag.Parse()
	if *gen != "" {
		gencert(*gen)
		return
	}
	fmt.Printf("before: %s\n        %s\n", status(*pid), maps(*pid))
	var wg sync.WaitGroup
	sem := make(chan struct{}, 64)
	var mu sync.Mutex
	var conns []net.Conn
	var clients []*http.Client
	var ok, failed int
	for i := 0; i < *n; i++ {
		wg.Add(1)
		sem <- struct{}{}
		i := i
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			var err error
			if *mode == "h2" {
				tr := &http.Transport{ForceAttemptHTTP2: true, TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
					MaxIdleConnsPerHost: 1}
				if *srcs > 1 {
					la := &net.TCPAddr{IP: net.IPv4(127, 0, 0, byte(1+i%*srcs))}
					tr.DialContext = (&net.Dialer{LocalAddr: la}).DialContext
				}
				var resp *http.Response
				cl := &http.Client{Transport: tr}
				resp, err = cl.Get("https://" + *addr + "/")
				if err == nil {
					io.Copy(io.Discard, resp.Body)
					resp.Body.Close()
					if resp.ProtoMajor != 2 {
						err = fmt.Errorf("proto %d", resp.ProtoMajor)
					}
					mu.Lock()
					clients = append(clients, cl)
					mu.Unlock()
				}
			} else {
				var c net.Conn
				d := net.Dialer{}
				if *srcs > 1 {
					d.LocalAddr = &net.TCPAddr{IP: net.IPv4(127, 0, 0, byte(1+i%*srcs))}
				}
				c, err = d.Dial("tcp", *addr)
				if err == nil && *useTLS {
					tc := tls.Client(c, &tls.Config{InsecureSkipVerify: true, NextProtos: []string{"http/1.1"}})
					if err = tc.Handshake(); err == nil {
						c = tc
					} else {
						c.Close()
					}
				}
				if err == nil {
					_, err = c.Write([]byte("GET / HTTP/1.1\r\nHost: x\r\n\r\n"))
					if err == nil {
						err = readResp(bufio.NewReader(c))
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
	var reqs, tfail int64
	var twg sync.WaitGroup
	if *think > 0 {
		stop := time.Now().Add(*settle)
		for _, c := range conns {
			twg.Add(1)
			go func(c net.Conn) {
				defer twg.Done()
				r := bufio.NewReader(c)
				var k, f int64
				for {
					time.Sleep(*think)
					if time.Now().After(stop) {
						break
					}
					c.SetDeadline(time.Now().Add(10 * time.Second))
					if _, err := c.Write([]byte("GET / HTTP/1.1\r\nHost: x\r\n\r\n")); err != nil {
						f++
						break
					}
					if err := readResp(r); err != nil {
						f++
						break
					}
					k++
				}
				mu.Lock()
				reqs += k
				tfail += f
				mu.Unlock()
			}(c)
		}
		// sample just before the window ends, while the requests still run
		time.Sleep(*settle - *settle/20)
	} else {
		time.Sleep(*settle)
	}
	fmt.Printf("mode=%s n=%d ok=%d failed=%d\n", *mode, *n, ok, failed)
	label := "idle:  "
	if *think > 0 {
		label = "think: "
	}
	fmt.Printf("%s %s  %s\n        %s\n", label, status(*pid), smaps(*pid), maps(*pid))
	bad := false
	if *think > 0 {
		twg.Wait()
		fmt.Printf("think %v: %d requests over %v (%.0f/s), %d connections failed\n", *think, reqs, *settle,
			float64(reqs)/settle.Seconds(), tfail)
		if tfail != 0 {
			bad = true
		}
	}
	if failed != 0 {
		fmt.Printf("FAIL: %d of %d connections failed\n", failed, *n)
		bad = true
	}
	if *maxmaps > 0 {
		b, _ := os.ReadFile(fmt.Sprintf("/proc/%d/maps", *pid))
		n := strings.Count(string(b), "\n")
		fmt.Printf("server mappings: %d (limit %d)\n", n, *maxmaps)
		if n > *maxmaps || n == 0 {
			fmt.Println("FAIL: mappings over the limit (or the server is gone)")
			bad = true
		}
	}
	if *again {
		nbad := 0
		for _, c := range conns {
			c.SetDeadline(time.Now().Add(10 * time.Second))
			if _, err := c.Write([]byte("GET / HTTP/1.1\r\nHost: x\r\n\r\n")); err != nil {
				nbad++
				continue
			}
			l, err := bufio.NewReader(c).ReadString('\n')
			if err != nil || !strings.Contains(l, " 200 ") {
				nbad++
			}
		}
		for _, cl := range clients {
			resp, err := cl.Get("https://" + *addr + "/")
			if err != nil {
				nbad++
				continue
			}
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			if resp.ProtoMajor != 2 || resp.StatusCode != 200 {
				nbad++
			}
		}
		fmt.Printf("second request: %d of %d held connections failed\n", nbad, len(conns)+len(clients))
		fmt.Printf("after:  %s\n", status(*pid))
		if nbad != 0 {
			bad = true
		}
	}
	if bad {
		os.Exit(1)
	}
}
