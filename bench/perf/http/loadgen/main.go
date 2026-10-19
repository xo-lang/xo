// Command loadgen drives the task 01 users CRUD API on loopback for a fixed
// duration and prints one JSON line of results (bench/perf/http/run.sh).
//
// Each worker repeats a cycle on its own keep-alive connection: POST
// /users, GET /users/{id}, PATCH /users/{id} (nickname), GET /users/{id},
// DELETE /users/{id}, and every eighth cycle GET /users. The store is
// preloaded with -preload users so the list and the email check have work
// to do; it stays near preload + concurrency users. Latencies are recorded
// only after the warmup; requests and errors are counted for the whole run
// (the server's GC trace covers the whole run too).
//
// HTTPS (decision 0105): an https:// URL skips certificate verification
// (a loopback test certificate); -newconn makes every request open a new
// connection, so a TLS server does a full handshake per request;
// -gencert dir writes a self signed ECDSA P-256 certificate for
// 127.0.0.1 (cert.pem, key.pem) and exits.
//
// HTTP/2 (decision 0121): -h2 with an https:// URL speaks HTTP/2 only
// (ALPN h2): the workers' requests share one connection as streams, as
// net/http's client multiplexes them, and loadgen exits when the server
// answers with another protocol. -h2det also writes each request's frames
// at once (h2det.go), so a request costs the server the same from run to
// run.
package main

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"flag"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type result struct {
	Requests    int64   `json:"requests"`     // measured window
	AllRequests int64   `json:"all_requests"` // warmup and measured
	Errors      int64   `json:"errors"`
	Seconds     float64 `json:"seconds"`
	RPS         float64 `json:"rps"`
	P50us       float64 `json:"p50_us"`
	P99us       float64 `json:"p99_us"`
	P999us      float64 `json:"p999_us"`
	PeakRSSKB   int64   `json:"peak_rss_kb"`
	CPUusReq    float64 `json:"cpu_us_per_req"` // server user+sys CPU over the measured window, per request (needs -pid)
}

func main() {
	base := flag.String("url", "http://127.0.0.1:18080", "server base URL (loopback only)")
	conc := flag.Int("c", 32, "concurrent workers")
	dur := flag.Duration("d", 10*time.Second, "measured duration")
	warm := flag.Duration("warmup", 2*time.Second, "warmup before measuring")
	preload := flag.Int("preload", 64, "users created before the run")
	pid := flag.Int("pid", 0, "server pid whose RSS is sampled")
	route := flag.String("route", "", "allocation mode: send -n requests of one route on one connection and exit (get, list, patch, createdel, cycle)")
	n := flag.Int("n", 1000, "requests (create and delete pairs, cycles) in -route mode")
	newConn := flag.Bool("newconn", false, "a new connection (and TLS handshake) per request")
	resume := flag.Bool("resume", false, "with https: keep TLS sessions (a client session cache), so new connections resume")
	gencert := flag.String("gencert", "", "write cert.pem and key.pem for 127.0.0.1 into this directory and exit")
	h2 := flag.Bool("h2", false, "with https: HTTP/2 only (the workers' requests are streams of one connection)")
	gap := flag.Duration("gap", 0, "-route mode: a pause before each request, so a server is idle (waiting to read) when it comes (callgrind.sh's h2)")
	burst := flag.Int("burst", 0, "burst mode: dial this many connections at once (raw TCP, one GET /users/1 each, Connection: close), -reps times, and print the failures and the time until all are answered (burst.go)")
	reps := flag.Int("reps", 10, "-burst mode: rounds")
	h2det := flag.Bool("h2det", false, "with -h2: write each request's frames in one TLS write (h2det.go; callgrind.sh's deterministic counts)")
	flag.Parse()
	if *gencert != "" {
		if err := genCert(*gencert); err != nil {
			fmt.Fprintln(os.Stderr, "loadgen:", err)
			os.Exit(1)
		}
		return
	}
	loop := false
	for _, p := range []string{"http://127.0.0.1:", "http://localhost:", "https://127.0.0.1:", "https://localhost:"} {
		loop = loop || strings.HasPrefix(*base, p)
	}
	if !loop {
		fmt.Fprintln(os.Stderr, "loadgen: loopback URLs only")
		os.Exit(2)
	}
	tr := &http.Transport{
		MaxIdleConns:        *conc * 2,
		MaxIdleConnsPerHost: *conc * 2,
		DialContext:         (&net.Dialer{Timeout: 5 * time.Second}).DialContext,
		// a loopback test certificate; HTTP/1.1 as the Xo servers speak
		TLSClientConfig:   &tls.Config{InsecureSkipVerify: true, ClientSessionCache: sessionCache(*resume, *conc)},
		DisableKeepAlives: *newConn,
	}
	if *h2 {
		tr.Protocols = new(http.Protocols)
		tr.Protocols.SetHTTP2(true)
	}
	if *h2det {
		if !*h2 || !strings.HasPrefix(*base, "https://") {
			fmt.Fprintln(os.Stderr, "loadgen: -h2det needs -h2 and an https:// URL")
			os.Exit(2)
		}
		*base = h2Det(tr, *base)
	}
	client := &http.Client{Transport: tr, Timeout: 10 * time.Second}
	waitReady(client, *base)
	if *h2 {
		resp, err := client.Get(*base + "/users/1")
		if err != nil || resp.ProtoMajor != 2 {
			fmt.Fprintln(os.Stderr, "loadgen: -h2: the server does not speak HTTP/2:", err)
			os.Exit(1)
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}
	for i := 0; i < *preload; i++ {
		if _, err := create(client, *base, fmt.Sprintf("seed%d@x.io", i)); err != nil {
			fmt.Fprintln(os.Stderr, "loadgen: preload:", err)
			os.Exit(1)
		}
	}
	if *route != "" {
		os.Exit(runRoute(client, *base, *route, *n, *gap))
	}
	if *burst > 0 {
		os.Exit(runBurst(*base, *burst, *reps))
	}

	var peak atomic.Int64
	stopRSS := make(chan struct{})
	if *pid > 0 {
		go sampleRSS(*pid, &peak, stopRSS)
	}

	var all, errs atomic.Int64
	var measuring atomic.Bool
	stop := make(chan struct{})
	lats := make([][]time.Duration, *conc)
	var wg sync.WaitGroup
	for w := 0; w < *conc; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			mine := make([]time.Duration, 0, 1<<16)
			n := 0
			do := func(method, path, body string, want int) []byte {
				t0 := time.Now()
				data, code, err := call(client, method, *base+path, body)
				el := time.Since(t0)
				all.Add(1)
				if err != nil || code != want {
					errs.Add(1)
					return nil
				}
				if measuring.Load() {
					mine = append(mine, el)
				}
				return data
			}
			for {
				select {
				case <-stop:
					lats[w] = mine
					return
				default:
				}
				n++
				email := fmt.Sprintf("w%d-%d@x.io", w, n)
				data := do("POST", "/users", `{"name": "Worker `+strconv.Itoa(w)+`", "email": "`+email+`"}`, 201)
				id := idOf(data)
				if id == 0 {
					continue
				}
				p := "/users/" + strconv.Itoa(id)
				do("GET", p, "", 200)
				do("PATCH", p, `{"nickname": "nick`+strconv.Itoa(n)+`"}`, 200)
				do("GET", p, "", 200)
				do("DELETE", p, "", 204)
				if n%8 == 0 {
					do("GET", "/users", "", 200)
				}
			}
		}(w)
	}
	time.Sleep(*warm)
	measuring.Store(true)
	before := all.Load()
	cpu0 := cpuTime(*pid)
	t0 := time.Now()
	time.Sleep(*dur)
	measuring.Store(false)
	el := time.Since(t0)
	after := all.Load()
	cpu1 := cpuTime(*pid)
	close(stop)
	wg.Wait()
	close(stopRSS)

	var merged []time.Duration
	for _, l := range lats {
		merged = append(merged, l...)
	}
	slices.Sort(merged)
	pct := func(p float64) float64 {
		if len(merged) == 0 {
			return 0
		}
		i := int(p * float64(len(merged)-1))
		return float64(merged[i].Nanoseconds()) / 1e3
	}
	r := result{
		Requests:    after - before,
		AllRequests: all.Load(),
		Errors:      errs.Load(),
		Seconds:     el.Seconds(),
		RPS:         float64(after-before) / el.Seconds(),
		P50us:       pct(0.50),
		P99us:       pct(0.99),
		P999us:      pct(0.999),
		PeakRSSKB:   peak.Load(),
	}
	if *pid > 0 && after > before {
		r.CPUusReq = (cpu1 - cpu0).Seconds() * 1e6 / float64(after-before)
	}
	out, _ := json.Marshal(r)
	fmt.Println(string(out))
}

// runRoute sends n units of one route, one at a time (allocs.sh): get
// (GET /users/1), list (GET /users), patch (PATCH /users/1), createdel (a
// POST and a DELETE of the new user, so the store keeps its size), or
// cycle (one worker cycle of the load: POST, GET, PATCH, GET, DELETE, and
// a list every eighth). Prints {"requests": count} and returns the exit
// status.
func runRoute(c *http.Client, base, route string, n int, gap time.Duration) int {
	reqs := 0
	do := func(method, path, body string, want int) []byte {
		reqs++
		if gap > 0 {
			time.Sleep(gap)
		}
		data, code, err := call(c, method, base+path, body)
		if err != nil || code != want {
			fmt.Fprintf(os.Stderr, "loadgen: %s %s: status %d, %v\n", method, path, code, err)
			os.Exit(1)
		}
		return data
	}
	for i := 1; i <= n; i++ {
		switch route {
		case "get":
			do("GET", "/users/1", "", 200)
		case "list":
			do("GET", "/users", "", 200)
		case "patch":
			do("PATCH", "/users/1", `{"nickname": "nick`+strconv.Itoa(i)+`"}`, 200)
		case "createdel", "cycle":
			email := fmt.Sprintf("r-%d@x.io", i)
			id := idOf(do("POST", "/users", `{"name": "Worker 0", "email": "`+email+`"}`, 201))
			p := "/users/" + strconv.Itoa(id)
			if route == "cycle" {
				do("GET", p, "", 200)
				do("PATCH", p, `{"nickname": "nick`+strconv.Itoa(i)+`"}`, 200)
				do("GET", p, "", 200)
			}
			do("DELETE", p, "", 204)
			if route == "cycle" && i%8 == 0 {
				do("GET", "/users", "", 200)
			}
		default:
			fmt.Fprintln(os.Stderr, "loadgen: unknown route", route)
			return 2
		}
	}
	fmt.Printf("{\"requests\":%d}\n", reqs)
	return 0
}

func call(c *http.Client, method, url, body string) ([]byte, int, error) {
	var rd io.Reader
	if body != "" {
		rd = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, url, rd)
	if err != nil {
		return nil, 0, err
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.Do(req)
	if err != nil {
		return nil, 0, err
	}
	data, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	return data, resp.StatusCode, err
}

func create(c *http.Client, base, email string) (int, error) {
	data, code, err := call(c, "POST", base+"/users", `{"name": "Seed", "email": "`+email+`"}`)
	if err != nil {
		return 0, err
	}
	if code != 201 {
		return 0, fmt.Errorf("status %d: %s", code, bytes.TrimSpace(data))
	}
	return idOf(data), nil
}

func idOf(data []byte) int {
	var u struct {
		ID int `json:"id"`
	}
	if json.Unmarshal(data, &u) != nil {
		return 0
	}
	return u.ID
}

func waitReady(c *http.Client, base string) {
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if _, code, err := call(c, "GET", base+"/users/1", ""); err == nil && code > 0 {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	fmt.Fprintln(os.Stderr, "loadgen: server did not come up")
	os.Exit(1)
}

// sampleRSS records the largest resident set size of pid (ps, in KB).
func sampleRSS(pid int, peak *atomic.Int64, stop <-chan struct{}) {
	t := time.NewTicker(100 * time.Millisecond)
	defer t.Stop()
	for {
		out, err := exec.Command("ps", "-o", "rss=", "-p", strconv.Itoa(pid)).Output()
		if err == nil {
			if kb, err := strconv.ParseInt(strings.TrimSpace(string(out)), 10, 64); err == nil && kb > peak.Load() {
				peak.Store(kb)
			}
		}
		select {
		case <-stop:
			return
		case <-t.C:
		}
	}
}

// cpuTime is the user plus system CPU time of pid so far (ps -o time=,
// [[dd-]hh:]mm:ss.cc), or 0 when pid is 0 or ps fails.
func cpuTime(pid int) time.Duration {
	if pid <= 0 {
		return 0
	}
	out, err := exec.Command("ps", "-o", "time=", "-p", strconv.Itoa(pid)).Output()
	if err != nil {
		return 0
	}
	f := strings.TrimSpace(string(out))
	var days int64
	if d, rest, ok := strings.Cut(f, "-"); ok {
		days, _ = strconv.ParseInt(d, 10, 64)
		f = rest
	}
	var secs float64
	for _, part := range strings.Split(f, ":") {
		v, _ := strconv.ParseFloat(part, 64)
		secs = secs*60 + v
	}
	secs += float64(days) * 86400
	return time.Duration(secs * float64(time.Second))
}

// genCert writes a self signed ECDSA P-256 certificate for 127.0.0.1
// and localhost, valid for a week, and its key into dir.
func genCert(dir string) error {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return err
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject:      pkix.Name{CommonName: "loadgen"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(7 * 24 * time.Hour),
		DNSNames:     []string{"localhost"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return err
	}
	kd, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "cert.pem"), pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o644); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "key.pem"), pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: kd}), 0o600)
}

// sessionCache is the client's TLS session cache with -resume, else none
// (net/http's default: every handshake is a full one).
func sessionCache(resume bool, conc int) tls.ClientSessionCache {
	if !resume {
		return nil
	}
	return tls.NewLRUClientSessionCache(conc * 2)
}
