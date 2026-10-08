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
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
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
}

func main() {
	base := flag.String("url", "http://127.0.0.1:18080", "server base URL (loopback only)")
	conc := flag.Int("c", 32, "concurrent workers")
	dur := flag.Duration("d", 10*time.Second, "measured duration")
	warm := flag.Duration("warmup", 2*time.Second, "warmup before measuring")
	preload := flag.Int("preload", 64, "users created before the run")
	pid := flag.Int("pid", 0, "server pid whose RSS is sampled")
	flag.Parse()
	if !strings.HasPrefix(*base, "http://127.0.0.1:") && !strings.HasPrefix(*base, "http://localhost:") {
		fmt.Fprintln(os.Stderr, "loadgen: loopback URLs only")
		os.Exit(2)
	}
	tr := &http.Transport{
		MaxIdleConns:        *conc * 2,
		MaxIdleConnsPerHost: *conc * 2,
		DialContext:         (&net.Dialer{Timeout: 5 * time.Second}).DialContext,
	}
	client := &http.Client{Transport: tr, Timeout: 10 * time.Second}
	waitReady(client, *base)
	for i := 0; i < *preload; i++ {
		if _, err := create(client, *base, fmt.Sprintf("seed%d@x.io", i)); err != nil {
			fmt.Fprintln(os.Stderr, "loadgen: preload:", err)
			os.Exit(1)
		}
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
	t0 := time.Now()
	time.Sleep(*dur)
	measuring.Store(false)
	el := time.Since(t0)
	after := all.Load()
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
	out, _ := json.Marshal(r)
	fmt.Println(string(out))
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
