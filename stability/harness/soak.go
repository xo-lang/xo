package main

// harness soak: bench/soak's open loop mixed load and drift verdict
// (decision 0325 item 2), run against an app the harness builds and starts,
// with the app's live heap blocks (/debug/sched heap_live, decision 0520)
// sampled beside its resident set and open fds.
//
//	harness soak -xo XO -app DIR [-backend arm64] [-rate 500] [-d 2h] [-every 10s] [-warmup 0.1] [-out OUT]
//
// OUT/samples.csv gets one row per interval; OUT/drift.txt the verdict; the
// exit status is 1 when a series drifts or the app's leak check at exit
// reports blocks left.

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
	"regexp"
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
	socks             float64
	csocks            float64
	heapLive, timers  float64
	cpuPerReqUS       float64
}

// procStats reads the server process: resident KiB, open fds, CPU in
// microseconds. Linux reads /proc; macOS asks ps and lsof.
func procStats(pid int) (rss, fds float64, cpuUS float64, err error) {
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
	secs := 0.0
	for _, p := range strings.Split(f[1], ":") {
		v, _ := strconv.ParseFloat(p, 64)
		secs = secs*60 + v
	}
	if lo, e := exec.Command("lsof", "-p", strconv.Itoa(pid)).Output(); e == nil {
		fds = float64(bytes.Count(lo, []byte("\n")) - 1)
	}
	return rss, fds, secs * 1e6, nil
}

// fdList reads where each of the server's open fds points (Linux:
// /proc/PID/fd's links, "socket:[N]" for a socket), sorted, and how many
// are sockets; elsewhere nil and -1. It tells a socket kept open from a
// file left open when the fds column climbs.
func fdList(pid int) ([]string, float64) {
	if runtime.GOOS != "linux" {
		return nil, -1
	}
	dir := fmt.Sprintf("/proc/%d/fd", pid)
	ents, _ := os.ReadDir(dir)
	var out []string
	socks := 0.0
	for _, e := range ents {
		l, err := os.Readlink(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		if strings.HasPrefix(l, "socket:") {
			socks++
		}
		out = append(out, e.Name()+" "+l)
	}
	sort.Strings(out)
	return out, socks
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

// schedStats reads heap_live and timers_pending from the app's stats path
// (/debug/sched); -1 for each it does not report.
func schedStats(cl *http.Client, url, token string) (heapLive, timers float64) {
	heapLive, timers = -1, -1
	req, _ := http.NewRequest("GET", url, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := cl.Do(req)
	if err != nil {
		return
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	for _, line := range strings.Split(string(b), "\n") {
		k, v, ok := strings.Cut(line, " ")
		if !ok {
			continue
		}
		n, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
		if err != nil {
			continue
		}
		switch k {
		case "heap_live":
			heapLive = n
		case "timers_pending":
			timers = n
		}
	}
	return
}

var leakLine = regexp.MustCompile(`xo leakcheck: (\d+) live of (\d+) allocations`)

func soakMain(args []string) int {
	fs := flag.NewFlagSet("soak", flag.ExitOnError)
	xo := fs.String("xo", "xo", "the xo binary")
	appDir := fs.String("app", "", "the app's directory (holds app.json)")
	backend := fs.String("backend", "arm64", "the backend to build with")
	rate := fs.Int("rate", 500, "requests per second offered")
	dur := fs.Duration("d", time.Hour, "total duration")
	every := fs.Duration("every", 10*time.Second, "sampling interval")
	warm := fs.Float64("warmup", 0.1, "fraction of the run before drift is judged")
	out := fs.String("out", "", "output directory")
	maxRSSGrowth := fs.Float64("max-rss-growth", 0.10, "drift when RSS grows by more than this fraction per hour (after warmup)")
	maxHeapGrowth := fs.Float64("max-heap-growth", 0.05, "drift when heap_live grows by more than this fraction per hour (after warmup)")
	addEnv := fs.String("env", "", "K=V pairs, comma separated, added to the app's environment (e.g. XO_THP=off, XO_HEAPSTATS=1)")
	clientIdle := fs.Duration("client-idle", 0, "close the load client's pooled connections after this long unused (0: never)")
	maxLat := fs.Float64("max-lat-growth", 0.20, "drift when the last quarter's p99 exceeds the first quarter's by this fraction")
	fs.Parse(args)
	if *appDir == "" || *out == "" {
		fmt.Fprintln(os.Stderr, "harness soak: -app and -out are required")
		return 2
	}
	app, _, err := loadApp(*appDir)
	if err != nil {
		fmt.Fprintln(os.Stderr, "harness soak:", err)
		return 2
	}
	mix, ok := mixes[app.Name]
	if !ok {
		fmt.Fprintf(os.Stderr, "harness soak: no traffic mix for app %q\n", app.Name)
		return 2
	}
	if err := os.MkdirAll(*out, 0o755); err != nil {
		fmt.Fprintln(os.Stderr, "harness soak:", err)
		return 2
	}
	bin := filepath.Join(*out, app.Name+"-"+*backend)
	cmd := exec.Command(*xo, "build", "--backend="+*backend, "-o", bin, ".")
	cmd.Dir = *appDir
	if b, err := cmd.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "harness soak: build: %v\n%s", err, b)
		return 2
	}
	// the leak check counts from the start, which heap_live needs too
	app.Env["XO_LEAKCHECK"] = "1"
	for _, kv := range strings.Split(*addEnv, ",") {
		if k, v, ok := strings.Cut(strings.TrimSpace(kv), "="); ok {
			app.Env[k] = v
		}
	}
	// a fault or crash report is kept in OUT/crash.txt as well as the
	// run's log (XO_CRASH_FILE, decision 0520)
	crashFile, _ := filepath.Abs(filepath.Join(*out, "crash.txt"))
	app.Env["XO_CRASH_FILE"] = crashFile
	p, err := start(app, bin, filepath.Join(*out, "run"), 1)
	if err != nil {
		fmt.Fprintln(os.Stderr, "harness soak:", err)
		return 2
	}
	pid := p.cmd.Process.Pid
	base := "http://" + p.addr
	d := newDriver(app, mix, base, *clientIdle)
	cl := d.cl
	stopLoad := d.open(*rate)

	f, _ := os.Create(filepath.Join(*out, "samples.csv"))
	fmt.Fprintln(f, "t_s,reqs,errs,drops,p50_us,p99_us,p999_us,rss_kib,fds,heap_live,timers,cpu_us_per_req,socks,client_socks")
	statsURL := ""
	if app.Stats != "" {
		statsURL = base + app.Stats
	}
	start := time.Now()
	var samples []sample
	_, _, cpu0, _ := procStats(pid)
	var lastReqs int64
	tk := time.NewTicker(*every)
	for time.Since(start) < *dur {
		<-tk.C
		l := d.takeLat()
		rss, fds, cpu, _ := procStats(pid)
		fl, socks := fdList(pid)
		// the harness's own sockets: the load client's connections to the app
		_, csocks := fdList(os.Getpid())
		if len(samples) == 0 {
			os.WriteFile(filepath.Join(*out, "fds-first.txt"), []byte(strings.Join(fl, "\n")+"\n"), 0o644)
		}
		os.WriteFile(filepath.Join(*out, "fds-last.txt"), []byte(strings.Join(fl, "\n")+"\n"), 0o644)
		hl, tm := -1.0, -1.0
		if statsURL != "" {
			hl, tm = schedStats(cl, statsURL, app.Tokens[app.StatsToken])
		}
		n := atomic.LoadInt64(&d.reqs)
		s := sample{t: time.Since(start).Seconds(), reqs: n, errs: atomic.LoadInt64(&d.errs), drops: atomic.LoadInt64(&d.drops),
			p50: pct(l, 0.5), p99: pct(l, 0.99), p999: pct(l, 0.999), rss: rss, fds: fds, socks: socks, csocks: csocks, heapLive: hl, timers: tm}
		if n > lastReqs {
			s.cpuPerReqUS = (cpu - cpu0) / float64(n-lastReqs)
		}
		cpu0, lastReqs = cpu, n
		samples = append(samples, s)
		fmt.Fprintf(f, "%.0f,%d,%d,%d,%.0f,%.0f,%.0f,%.0f,%.0f,%.0f,%.0f,%.2f,%.0f,%.0f\n", s.t, s.reqs, s.errs, s.drops, s.p50, s.p99, s.p999, s.rss, s.fds, s.heapLive, s.timers, s.cpuPerReqUS, s.socks, s.csocks)
		f.Sync()
	}
	stopLoad()
	f.Close()
	stopErr := p.stop("term")

	var rep strings.Builder
	bad := false
	fmt.Fprintf(&rep, "harness soak %s on %s: rate %d/s for %s\n", app.Name, *backend, *rate, *dur)
	if stopErr != nil {
		fmt.Fprintf(&rep, "ALERT: stop: %v\n", stopErr)
		bad = true
	}
	logText, _ := os.ReadFile(p.log.Name())
	if m := leakLine.FindSubmatch(logText); m != nil {
		fmt.Fprintf(&rep, "leakcheck at exit: %s live of %s allocations\n", m[1], m[2])
		if string(m[1]) != "0" {
			bad = true
			rep.WriteString("ALERT: blocks left at exit\n")
		}
	} else {
		rep.WriteString("leakcheck at exit: no report (the app did not return from main)\n")
	}
	w := int(float64(len(samples)) * *warm)
	if len(samples)-w < 8 {
		rep.WriteString("too few samples to judge drift\n")
	} else {
		body := samples[w:]
		xs := make([]float64, len(body))
		rs := make([]float64, len(body))
		fds := make([]float64, len(body))
		hs := make([]float64, len(body))
		ts := make([]float64, len(body))
		for i, s := range body {
			xs[i], rs[i], fds[i], hs[i], ts[i] = s.t/3600, s.rss, s.fds, s.heapLive, s.timers
		}
		rssSlope := slope(xs, rs)
		rssFrac := rssSlope / math.Max(rs[0], 1)
		fmt.Fprintf(&rep, "rss: %.0f KiB at warmup end, slope %.0f KiB/hour (%.1f%%/hour; limit %.1f%%)\n", rs[0], rssSlope, 100*rssFrac, 100**maxRSSGrowth)
		if rssFrac > *maxRSSGrowth {
			bad = true
			rep.WriteString("ALERT: RSS grows\n")
		}
		if hs[0] >= 0 {
			hSlope := slope(xs, hs)
			hFrac := hSlope / math.Max(hs[0], 1)
			fmt.Fprintf(&rep, "heap_live: %.0f blocks at warmup end, slope %.0f/hour (%.1f%%/hour; limit %.1f%%)\n", hs[0], hSlope, 100*hFrac, 100**maxHeapGrowth)
			if hFrac > *maxHeapGrowth {
				bad = true
				rep.WriteString("ALERT: live heap blocks grow\n")
			}
		} else {
			rep.WriteString("heap_live: not reported by the app\n")
		}
		if ts[0] >= 0 {
			// a server's timers follow its open connections' deadlines, so
			// only a steady climb past both 5 an hour and 5% an hour counts
			tSlope := slope(xs, ts)
			fmt.Fprintf(&rep, "timers_pending: %.0f at warmup end, slope %.2f/hour\n", ts[0], tSlope)
			if tSlope > math.Max(5, 0.05*ts[0]) {
				bad = true
				rep.WriteString("ALERT: pending timers grow\n")
			}
		} else {
			rep.WriteString("timers_pending: not reported by the app\n")
		}
		fdSlope := slope(xs, fds)
		fmt.Fprintf(&rep, "fds: %.0f at warmup end, slope %.2f/hour\n", fds[0], fdSlope)
		if fdSlope > 1 {
			bad = true
			rep.WriteString("ALERT: open fds grow\n")
		}
		if body[0].socks >= 0 {
			ss := make([]float64, len(body))
			for i, s := range body {
				ss[i] = s.socks
			}
			// which kind grows: fds-first.txt and fds-last.txt list them
			fmt.Fprintf(&rep, "sockets: %.0f at warmup end, slope %.2f/hour; files: %.0f, slope %.2f/hour\n", ss[0], slope(xs, ss), fds[0]-ss[0], fdSlope-slope(xs, ss))
			cs := make([]float64, len(body))
			for i, s := range body {
				cs[i] = s.csocks
			}
			// equal to the app's sockets: each is a connection the client holds
			fmt.Fprintf(&rep, "client sockets: %.0f at warmup end, slope %.2f/hour, %.0f at the end (the app's: %.0f)\n", cs[0], slope(xs, cs), cs[len(cs)-1], ss[len(ss)-1])
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
	}
	if len(samples) > 0 {
		lastS := samples[len(samples)-1]
		fmt.Fprintf(&rep, "requests %d, errors %d, drops %d\n", lastS.reqs, lastS.errs, lastS.drops)
		if lastS.errs*1000 > lastS.reqs {
			bad = true
			rep.WriteString("ALERT: more than 0.1% errors\n")
		}
	}
	if b, err := os.ReadFile(crashFile); err == nil && len(b) > 0 {
		bad = true
		fmt.Fprintf(&rep, "ALERT: the app wrote a fault or crash report (crash.txt):\n%s", b)
	}
	if bad {
		rep.WriteString("verdict: DRIFT\n")
	} else {
		rep.WriteString("verdict: steady\n")
	}
	os.WriteFile(filepath.Join(*out, "drift.txt"), []byte(rep.String()), 0o644)
	fmt.Print(rep.String())
	if bad {
		return 1
	}
	return 0
}

// A mix makes one request of the app's traffic per call.
type mixFn func(app *App, do func(method, path, token string, body []byte) (int, []byte)) func(r *rand.Rand)

var mixes = map[string]mixFn{"notes": notesMix}

// notesMix is bench/soak's: 22% create (ttl 60 to 180 s, so the purge job
// removes some), 28% get, 10% list, 22% delete, 18% health.
func notesMix(app *App, do func(method, path, token string, body []byte) (int, []byte)) func(r *rand.Rand) {
	tok := app.Tokens["admin"]
	var idMu sync.Mutex
	ids := make([]int64, 0, 1024)
	return func(r *rand.Rand) {
		x := r.Intn(100)
		switch {
		case x < 22:
			b, _ := json.Marshal(map[string]any{"body": fmt.Sprintf("note %d %s", r.Int63(), strings.Repeat("x", r.Intn(200))), "ttl_secs": 60 + r.Intn(120)})
			if st, rb := do("POST", "/notes", tok, b); st == 201 {
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
		case x < 50:
			idMu.Lock()
			var id int64 = -1
			if len(ids) > 0 {
				id = ids[r.Intn(len(ids))]
			}
			idMu.Unlock()
			if id >= 0 {
				do("GET", fmt.Sprintf("/notes/%d", id), tok, nil)
			}
		case x < 60:
			do("GET", "/notes", tok, nil)
		case x < 82:
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
				do("DELETE", fmt.Sprintf("/notes/%d", id), tok, nil)
			}
		default:
			do("GET", "/health", "", nil)
		}
	}
}
