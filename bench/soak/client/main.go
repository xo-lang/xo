// soak: a steady, open loop mixed load against examples/notes-service
// for hours, with a sampler of the server process and a drift check
// (bench/soak/README.md). The request rate is fixed (a ticker feeds a
// worker pool; a request that finds all workers busy is counted as
// dropped, so a slow server shows as latency and drops, not as a lower
// offered rate).
//
//	soak -url http://127.0.0.1:8080 -token T -pid PID -rate 500 -d 2h -out DIR
//
// DIR/samples.csv gets one row per interval: elapsed seconds, requests,
// errors, drops, p50/p99/p99.9 microseconds of the interval, the server's
// RSS (KiB), open fds, and CPU microseconds per request in the interval.
// At the end DIR/drift.txt has the verdict and the exit status is 1 when
// a series drifts.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math"
	"math/rand"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type sample struct {
	t                 float64
	reqs, errs, drops int64
	p50, p99, p999    float64
	rss, fds          float64
	cpuPerReqUS       float64
}

// proc reads the server process: resident KiB, open fds, CPU ticks in
// microseconds. Linux reads /proc; macOS asks ps and lsof.
func proc(pid int) (rss, fds float64, cpuUS float64, err error) {
	if runtime.GOOS == "linux" {
		b, e := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
		if e != nil {
			return 0, 0, 0, e
		}
		s := string(b)
		f := strings.Fields(s[strings.LastIndexByte(s, ')')+2:])
		u, _ := strconv.ParseFloat(f[11], 64)
		k, _ := strconv.ParseFloat(f[12], 64)
		r, _ := strconv.ParseFloat(f[21], 64)
		ents, _ := os.ReadDir(fmt.Sprintf("/proc/%d/fd", pid))
		return r * 4, float64(len(ents)), (u + k) * 10000, nil
	}
	out, e := exec.Command("ps", "-o", "rss=,time=", "-p", strconv.Itoa(pid)).Output()
	if e != nil {
		return 0, 0, 0, e
	}
	f := strings.Fields(string(out))
	if len(f) < 2 {
		return 0, 0, 0, fmt.Errorf("ps: %q", out)
	}
	rss, _ = strconv.ParseFloat(f[0], 64)
	// [dd-]hh:mm:ss.cc or mm:ss.cc
	t := f[1]
	secs := 0.0
	parts := strings.Split(t, ":")
	for _, p := range parts {
		v, _ := strconv.ParseFloat(p, 64)
		secs = secs*60 + v
	}
	lo, e := exec.Command("lsof", "-p", strconv.Itoa(pid)).Output()
	if e == nil {
		fds = float64(bytes.Count(lo, []byte("\n")) - 1)
	}
	return rss, fds, secs * 1e6, nil
}

func pct(v []float64, p float64) float64 {
	if len(v) == 0 {
		return 0
	}
	return v[int(math.Min(float64(len(v)-1), p*float64(len(v))))]
}

// slope is the least squares slope of y over x.
func slope(x, y []float64) float64 {
	n := float64(len(x))
	if n < 3 {
		return 0
	}
	var sx, sy, sxx, sxy float64
	for i := range x {
		sx += x[i]
		sy += y[i]
		sxx += x[i] * x[i]
		sxy += x[i] * y[i]
	}
	d := n*sxx - sx*sx
	if d == 0 {
		return 0
	}
	return (n*sxy - sx*sy) / d
}

func main() {
	base := flag.String("url", "http://127.0.0.1:8080", "server base URL")
	token := flag.String("token", "", "bearer token of a user")
	pid := flag.Int("pid", 0, "server pid (RSS, fds, CPU)")
	rate := flag.Int("rate", 500, "requests per second offered")
	dur := flag.Duration("d", time.Hour, "total duration")
	every := flag.Duration("every", 10*time.Second, "sampling interval")
	warm := flag.Float64("warmup", 0.1, "fraction of the run before drift is judged")
	out := flag.String("out", "soak-out", "output directory")
	maxRSSGrowth := flag.Float64("max-rss-growth", 0.10, "alert when RSS grows by more than this fraction per hour (after warmup)")
	maxLat := flag.Float64("max-lat-growth", 0.20, "alert when the p99 of the last quarter exceeds the first quarter's by this fraction")
	flag.Parse()
	if *token == "" {
		fmt.Fprintln(os.Stderr, "soak: -token is required")
		os.Exit(2)
	}
	os.MkdirAll(*out, 0o755)
	cl := &http.Client{Transport: &http.Transport{MaxIdleConnsPerHost: 256, MaxConnsPerHost: 256}, Timeout: 10 * time.Second}
	hdr := "Bearer " + *token

	var mu sync.Mutex
	var lat []float64
	var reqs, errs, drops int64
	ids := make([]int64, 0, 1024)
	var idMu sync.Mutex
	do := func(method, path string, body []byte) (int, []byte) {
		var rd io.Reader
		if body != nil {
			rd = bytes.NewReader(body)
		}
		req, _ := http.NewRequest(method, *base+path, rd)
		req.Header.Set("Authorization", hdr)
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		t0 := time.Now()
		resp, err := cl.Do(req)
		us := float64(time.Since(t0).Microseconds())
		atomic.AddInt64(&reqs, 1)
		if err != nil {
			atomic.AddInt64(&errs, 1)
			return 0, nil
		}
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		mu.Lock()
		lat = append(lat, us)
		mu.Unlock()
		if resp.StatusCode >= 500 {
			atomic.AddInt64(&errs, 1)
		}
		return resp.StatusCode, b
	}
	op := func(r *rand.Rand) {
		x := r.Intn(100)
		switch {
		case x < 22: // create (ttl 60..180 s, so the purge job removes some)
			b, _ := json.Marshal(map[string]any{"body": fmt.Sprintf("note %d %s", r.Int63(), strings.Repeat("x", r.Intn(200))), "ttl_secs": 60 + r.Intn(120)})
			if st, rb := do("POST", "/notes", b); st == 201 {
				var n struct {
					ID int64 `json:"id"`
				}
				if json.Unmarshal(rb, &n) == nil {
					idMu.Lock()
					if len(ids) < 1000 {
						ids = append(ids, n.ID)
					} else {
						ids[r.Intn(len(ids))] = n.ID
					}
					idMu.Unlock()
				}
			}
		case x < 50: // get one
			idMu.Lock()
			var id int64 = -1
			if len(ids) > 0 {
				id = ids[r.Intn(len(ids))]
			}
			idMu.Unlock()
			if id >= 0 {
				do("GET", fmt.Sprintf("/notes/%d", id), nil)
			}
		case x < 60: // list (the user's notes: the heavy one)
			do("GET", "/notes", nil)
		case x < 82: // delete one
			idMu.Lock()
			var id int64 = -1
			if n := len(ids); n > 0 {
				i := r.Intn(n)
				id = ids[i]
				ids[i] = ids[n-1]
				ids = ids[:n-1]
			}
			idMu.Unlock()
			if id >= 0 {
				do("DELETE", fmt.Sprintf("/notes/%d", id), nil)
			}
		default:
			do("GET", "/health", nil)
		}
	}

	work := make(chan struct{}, 4096)
	var wg sync.WaitGroup
	for i := 0; i < 128; i++ {
		wg.Add(1)
		go func(seed int64) {
			defer wg.Done()
			r := rand.New(rand.NewSource(seed))
			for range work {
				op(r)
			}
		}(int64(i + 1))
	}
	stop := make(chan struct{})
	go func() {
		tk := time.NewTicker(time.Second / time.Duration(*rate))
		defer tk.Stop()
		for {
			select {
			case <-tk.C:
				select {
				case work <- struct{}{}:
				default:
					atomic.AddInt64(&drops, 1)
				}
			case <-stop:
				return
			}
		}
	}()

	f, _ := os.Create(filepath.Join(*out, "samples.csv"))
	fmt.Fprintln(f, "t_s,reqs,errs,drops,p50_us,p99_us,p999_us,rss_kib,fds,cpu_us_per_req")
	start := time.Now()
	var samples []sample
	_, _, cpu0, _ := proc(*pid)
	var lastReqs int64
	tk := time.NewTicker(*every)
	for time.Since(start) < *dur {
		<-tk.C
		mu.Lock()
		l := lat
		lat = nil
		mu.Unlock()
		sort.Float64s(l)
		rss, fds, cpu, _ := proc(*pid)
		n := atomic.LoadInt64(&reqs)
		s := sample{t: time.Since(start).Seconds(), reqs: n, errs: atomic.LoadInt64(&errs), drops: atomic.LoadInt64(&drops),
			p50: pct(l, 0.5), p99: pct(l, 0.99), p999: pct(l, 0.999), rss: rss, fds: fds}
		if n > lastReqs {
			s.cpuPerReqUS = (cpu - cpu0) / float64(n-lastReqs)
		}
		cpu0, lastReqs = cpu, n
		samples = append(samples, s)
		fmt.Fprintf(f, "%.0f,%d,%d,%d,%.0f,%.0f,%.0f,%.0f,%.0f,%.2f\n", s.t, s.reqs, s.errs, s.drops, s.p50, s.p99, s.p999, s.rss, s.fds, s.cpuPerReqUS)
		f.Sync()
	}
	close(stop)
	close(work)
	wg.Wait()
	f.Close()

	// drift: slopes after warmup, a latency comparison of the first and
	// last quarters
	w := int(float64(len(samples)) * *warm)
	if len(samples)-w < 8 {
		os.WriteFile(filepath.Join(*out, "drift.txt"), []byte("too few samples to judge drift\n"), 0o644)
		return
	}
	body := samples[w:]
	xs := make([]float64, len(body))
	rs := make([]float64, len(body))
	fs := make([]float64, len(body))
	for i, s := range body {
		xs[i], rs[i], fs[i] = s.t/3600, s.rss, s.fds
	}
	var rep strings.Builder
	bad := false
	rssSlope := slope(xs, rs)
	rssFrac := rssSlope / math.Max(rs[0], 1)
	fmt.Fprintf(&rep, "rss: %.0f KiB at warmup end, slope %.0f KiB/hour (%.1f%%/hour; limit %.1f%%)\n", rs[0], rssSlope, 100*rssFrac, 100**maxRSSGrowth)
	if rssFrac > *maxRSSGrowth {
		bad = true
		rep.WriteString("ALERT: RSS grows\n")
	}
	fdSlope := slope(xs, fs)
	fmt.Fprintf(&rep, "fds: %.0f at warmup end, slope %.2f/hour\n", fs[0], fdSlope)
	if fdSlope > 1 {
		bad = true
		rep.WriteString("ALERT: open fds grow\n")
	}
	q := len(body) / 4
	avg := func(ss []sample) float64 {
		t := 0.0
		for _, s := range ss {
			t += s.p99
		}
		return t / float64(len(ss))
	}
	first, last := avg(body[:q]), avg(body[len(body)-q:])
	fmt.Fprintf(&rep, "p99: first quarter %.0f us, last quarter %.0f us (limit +%.0f%%)\n", first, last, 100**maxLat)
	if first > 0 && last > first*(1+*maxLat) {
		bad = true
		rep.WriteString("ALERT: p99 latency grows\n")
	}
	lastS := samples[len(samples)-1]
	fmt.Fprintf(&rep, "requests %d, errors %d, drops %d\n", lastS.reqs, lastS.errs, lastS.drops)
	if lastS.errs*1000 > lastS.reqs {
		bad = true
		rep.WriteString("ALERT: more than 0.1% errors\n")
	}
	if bad {
		rep.WriteString("verdict: DRIFT\n")
	} else {
		rep.WriteString("verdict: steady\n")
	}
	os.WriteFile(filepath.Join(*out, "drift.txt"), []byte(rep.String()), 0o644)
	fmt.Print(rep.String())
	if bad {
		os.Exit(1)
	}
}
