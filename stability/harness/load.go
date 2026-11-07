package main

// harness load: the app's traffic mix at fixed offered rates (open loop) and
// at saturation (closed loop), against the app built with xo and, when
// app.json names one, its Go twin, in alternating rounds (decision 0520
// section 2, check 2).
//
//	harness load -xo XO -app DIR -out OUT [-backend arm64] [-rates 1000,4000] [-sat 64] [-d 30s] [-warmup 5s] [-rounds 3] [-drop-env K,...] [-env K=V,...]
//
// When app.json names a stats path, each xo run's stats text (std/debug's
// /sched: the scheduler's counters, heap_live, fds_open, timers_pending) at
// the end of its measurement is kept in its run directory as stats.txt; set
// XO_RTSTATS=1 with -env for the scheduler's counters.
// OUT/load.csv gets one row per run, OUT/load.txt the medians, ranges and
// verdict per rate: slower or faster only when the rounds' ranges do not
// overlap (bench/perf/jitter pads.py's rule). The exit status is 1 when a
// run had errors; speed is reported, not gated.

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"io"
	"math/rand"
	"net"
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

// driver sends an app's mix to base and records each response's latency.
type driver struct {
	cl                *http.Client
	base              string
	mu                sync.Mutex
	lat               []float64
	reqs, errs, drops int64
	// dials counts the client's new connections to the app: a step in the
	// client's sockets with as many dials is the client reconnecting (a
	// reply slower than the pool's reuse, or a host starved by another
	// job), not the app keeping connections it should close
	dials int64
	// c4xx counts 4xx answers: not errors (a mix expects some), but a
	// target that answers a share of its requests 4xx does less work, so
	// its row is not comparable with the twin's
	c4xx int64
	op   func(r *rand.Rand)
}

// poolCap is the most connections the load client holds to the app: a
// soak's client sockets stay at or under it however long the run.
const poolCap = 256

// idle closes a pooled connection after that long unused (0: never,
// a hand built Transport's default, so a burst's connections stay open)
func newDriver(app *App, mix mixFn, base string, idle time.Duration) *driver {
	d := &driver{base: base}
	var dl net.Dialer
	dial := func(ctx context.Context, network, addr string) (net.Conn, error) {
		atomic.AddInt64(&d.dials, 1)
		return dl.DialContext(ctx, network, addr)
	}
	d.cl = &http.Client{Transport: &http.Transport{MaxIdleConnsPerHost: poolCap, MaxConnsPerHost: poolCap, IdleConnTimeout: idle, DialContext: dial}, Timeout: 10 * time.Second}
	d.op = mix(app, d.do)
	return d
}

func (d *driver) do(method, path, token string, body []byte) (int, []byte) {
	var rd io.Reader
	if body != nil {
		rd = bytes.NewReader(body)
	}
	req, _ := http.NewRequest(method, d.base+path, rd)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	t0 := time.Now()
	resp, err := d.cl.Do(req)
	us := float64(time.Since(t0).Microseconds())
	atomic.AddInt64(&d.reqs, 1)
	if err != nil {
		atomic.AddInt64(&d.errs, 1)
		return 0, nil
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	d.mu.Lock()
	d.lat = append(d.lat, us)
	d.mu.Unlock()
	if resp.StatusCode >= 500 {
		atomic.AddInt64(&d.errs, 1)
	} else if resp.StatusCode >= 400 {
		atomic.AddInt64(&d.c4xx, 1)
	}
	return resp.StatusCode, b
}

// takeLat returns the latencies recorded since the last call, sorted.
func (d *driver) takeLat() []float64 {
	d.mu.Lock()
	l := d.lat
	d.lat = nil
	d.mu.Unlock()
	sort.Float64s(l)
	return l
}

// open offers rate requests a second to 128 workers (a request no worker
// is free for is a drop) until the returned stop is called.
func (d *driver) open(rate int) (stop func()) {
	work := make(chan struct{}, 4096)
	quit := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < 128; i++ {
		wg.Add(1)
		go func(seed int64) {
			defer wg.Done()
			r := rand.New(rand.NewSource(seed))
			for range work {
				d.op(r)
			}
		}(int64(i + 1))
	}
	go func() {
		// by schedule, not one tick per request: a ticker under a
		// millisecond loses ticks (4000/s offered sent about 1200/s on
		// linux/arm64, 2026-11-05), so each millisecond sends what the
		// rate owes since the start
		tk := time.NewTicker(time.Millisecond)
		defer tk.Stop()
		t0 := time.Now()
		var sent int64
		for {
			select {
			case <-tk.C:
				owed := int64(time.Since(t0).Seconds()*float64(rate)) - sent
				for ; owed > 0; owed-- {
					sent++
					select {
					case work <- struct{}{}:
					default:
						atomic.AddInt64(&d.drops, 1)
					}
				}
			case <-quit:
				close(work)
				return
			}
		}
	}()
	return func() {
		close(quit)
		wg.Wait()
	}
}

// closed runs n workers that each send their next request as soon as the
// last one is answered, until the returned stop is called.
func (d *driver) closed(n int) (stop func()) {
	var quit atomic.Bool
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(seed int64) {
			defer wg.Done()
			r := rand.New(rand.NewSource(seed))
			for !quit.Load() {
				d.op(r)
			}
		}(int64(i + 1))
	}
	return func() {
		quit.Store(true)
		wg.Wait()
	}
}

// peakRSS is the process's peak resident KiB (Linux VmHWM), else its
// current resident set.
func peakRSS(pid int) float64 {
	if runtime.GOOS == "linux" {
		b, _ := os.ReadFile(fmt.Sprintf("/proc/%d/status", pid))
		for _, line := range strings.Split(string(b), "\n") {
			if v, ok := strings.CutPrefix(line, "VmHWM:"); ok {
				n, _ := strconv.ParseFloat(strings.TrimSuffix(strings.TrimSpace(v), " kB"), 64)
				return n
			}
		}
	}
	rss, _, _, _ := procStats(pid)
	return rss
}

// mapsSummary sums the process's Linux smaps: its current resident KiB,
// its transparent huge pages (AnonHugePages: khugepaged can collapse a
// range the heap gave back into a resident 2 MiB page again, decision
// 0533), and the count and resident KiB of the mappings the size of a native
// fiber stack (64 MiB above its guard page, which is a mapping of its own;
// pool.xo map_stack), which are the fibers live or cached.
func mapsSummary(pid int) string {
	b, err := os.ReadFile(fmt.Sprintf("/proc/%d/smaps", pid))
	if err != nil {
		return "no smaps\n"
	}
	var size, rss, all, huge, stacks, stackRSS, near, nearRSS int64
	flush := func() {
		all += rss
		if size == 65536 {
			stacks++
			stackRSS += rss
		} else if size > 65536 && size <= 65536+64 {
			// a stack and its guard as one mapping
			near++
			nearRSS += rss
		}
		size, rss = 0, 0
	}
	for _, line := range strings.Split(string(b), "\n") {
		f := strings.Fields(line)
		if len(f) < 2 {
			continue
		}
		switch f[0] {
		case "Size:":
			flush()
			size, _ = strconv.ParseInt(f[1], 10, 64)
		case "Rss:":
			rss, _ = strconv.ParseInt(f[1], 10, 64)
		case "AnonHugePages:":
			n, _ := strconv.ParseInt(f[1], 10, 64)
			huge += n
		}
	}
	flush()
	return fmt.Sprintf("rss_kib %d anon_huge_kib %d stack_maps %d stack_rss_kib %d near_maps %d near_rss_kib %d\n", all, huge, stacks, stackRSS, near, nearRSS)
}

// saveStats writes the app's stats text to path.
func saveStats(cl *http.Client, url, token, path string) {
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
	os.WriteFile(path, b, 0o644)
}

// loadRun is one target's run at one load.
type loadRun struct {
	target, load        string
	round               int
	rps                 float64
	p50, p99, p999      float64
	errs, drops, c4xx   int64
	reqs                int64
	cpuPerReqUS, peakKB float64
}

func loadMain(args []string) int {
	fs := flag.NewFlagSet("load", flag.ExitOnError)
	xo := fs.String("xo", "xo", "the xo binary")
	appDir := fs.String("app", "", "the app's directory (holds app.json)")
	backend := fs.String("backend", "arm64", "the backend to build with")
	rates := fs.String("rates", "1000,4000", "offered rates (requests per second), comma separated; empty for none")
	sat := fs.Int("sat", 64, "closed loop workers for the saturation run (0: none)")
	dur := fs.Duration("d", 30*time.Second, "measured duration of each run")
	warm := fs.Duration("warmup", 5*time.Second, "load before each run's measurement")
	idle := fs.Duration("idle", 6*time.Second, "wait after each run's load before mem.txt's idle line (past the runtime's 3 s fiber trim; decision 0533's age passes need 30 to 60 s)")
	rounds := fs.Int("rounds", 3, "rounds (each runs every target at every load)")
	addEnv := fs.String("env", "", "K=V pairs, comma separated, added to the xo build's environment (e.g. XO_RTSTATS=1)")
	dropEnv := fs.String("drop-env", "", "app.json env keys left out for every target (e.g. NOTES_DATA when the twin has no persistence)")
	owners := fs.Int("owners", 1, "users the traffic is spread over (1: the admin user alone, as rows before 2026-11-06 ran; above 1 needs app.json's owners)")
	out := fs.String("out", "", "output directory")
	fs.Parse(args)
	if fs.NArg() > 0 {
		// a value dropped by a caller's unquoted empty argument (`-rates ""`)
		// made the next flag a value and stopped parsing, so every later
		// flag kept its default and the run measured something else
		// (2026-11-06: a 8/32/64/128 worker sweep ran 64 workers four times)
		fmt.Fprintf(os.Stderr, "harness %s: unexpected argument %q (flags stop at the first non-flag: check for a dropped empty value)\n", fs.Name(), fs.Arg(0))
		return 2
	}
	if *appDir == "" || *out == "" {
		fmt.Fprintln(os.Stderr, "harness load: -app and -out are required")
		return 2
	}
	app, _, err := loadApp(*appDir)
	if err != nil {
		fmt.Fprintln(os.Stderr, "harness load:", err)
		return 2
	}
	mix, ok := mixes[app.Name]
	if !ok {
		fmt.Fprintf(os.Stderr, "harness load: no traffic mix for app %q\n", app.Name)
		return 2
	}
	for _, k := range strings.Split(*dropEnv, ",") {
		delete(app.Env, strings.TrimSpace(k))
	}
	if *owners > 1 {
		if app.Owners.Env == "" || !strings.Contains(app.Owners.Token, "{n}") {
			fmt.Fprintln(os.Stderr, "harness load: -owners above 1 needs app.json's owners.env and an owners.token with {n}")
			return 2
		}
		app.Env[app.Owners.Env] = strconv.Itoa(*owners)
		for i := 1; i <= *owners; i++ {
			app.OwnerTokens = append(app.OwnerTokens, strings.ReplaceAll(app.Owners.Token, "{n}", strconv.Itoa(i)))
		}
	}
	if err := os.MkdirAll(*out, 0o755); err != nil {
		fmt.Fprintln(os.Stderr, "harness load:", err)
		return 2
	}
	xoEnv := map[string]string{}
	for _, kv := range strings.Split(*addEnv, ",") {
		if k, v, ok := strings.Cut(strings.TrimSpace(kv), "="); ok {
			xoEnv[k] = v
		}
	}
	type target struct {
		name, bin string
		env       map[string]string
		stats     bool
	}
	xoBin, _ := filepath.Abs(filepath.Join(*out, app.Name+"-"+*backend))
	cmd := exec.Command(*xo, "build", "--backend="+*backend, "-o", xoBin, ".")
	cmd.Dir = *appDir
	if b, err := cmd.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "harness load: build: %v\n%s", err, b)
		return 2
	}
	targets := []target{{"xo-" + *backend, xoBin, xoEnv, true}}
	if app.Twin != "" {
		twinBin, _ := filepath.Abs(filepath.Join(*out, app.Name+"-go"))
		cmd := exec.Command("go", "build", "-o", twinBin, ".")
		cmd.Dir = filepath.Join(*appDir, app.Twin)
		if b, err := cmd.CombinedOutput(); err != nil {
			fmt.Fprintf(os.Stderr, "harness load: twin build: %v\n%s", err, b)
			return 2
		}
		targets = append(targets, target{"go", twinBin, nil, false})
	}
	var loads []string
	for _, r := range strings.Split(*rates, ",") {
		if r = strings.TrimSpace(r); r != "" {
			loads = append(loads, "rate"+r)
		}
	}
	if *sat > 0 {
		loads = append(loads, fmt.Sprintf("sat%d", *sat))
	}

	f, _ := os.Create(filepath.Join(*out, "load.csv"))
	fmt.Fprintln(f, "target,load,round,rps,p50_us,p99_us,p999_us,errs,drops,cpu_us_per_req,peak_rss_kib,reqs,c4xx")
	var runs []loadRun
	bad := false
	n := 0
	for round := 1; round <= *rounds; round++ {
		for _, ld := range loads {
			// alternate which target goes first, so a drift in the host's
			// state over a round does not always favor the same one
			order := targets
			if round%2 == 0 && len(targets) == 2 {
				order = []target{targets[1], targets[0]}
			}
			for _, t := range order {
				n++
				ta := *app
				ta.Env = map[string]string{}
				for k, v := range app.Env {
					ta.Env[k] = v
				}
				for k, v := range t.env {
					ta.Env[k] = v
				}
				if !t.stats {
					ta.Stats = ""
				}
				r, err := loadOnce(&ta, mix, t.bin, filepath.Join(*out, fmt.Sprintf("run-%s-%s-%d", t.name, ld, round)), n, ld, *warm, *dur, *idle)
				if err != nil {
					fmt.Fprintf(os.Stderr, "harness load: %s %s round %d: %v\n", t.name, ld, round, err)
					return 2
				}
				r.target, r.load, r.round = t.name, ld, round
				runs = append(runs, r)
				if r.errs > 0 {
					bad = true
				}
				fmt.Fprintf(f, "%s,%s,%d,%.0f,%.0f,%.0f,%.0f,%d,%d,%.2f,%.0f,%d,%d\n", r.target, r.load, r.round, r.rps, r.p50, r.p99, r.p999, r.errs, r.drops, r.cpuPerReqUS, r.peakKB, r.reqs, r.c4xx)
				f.Sync()
			}
		}
	}
	f.Close()
	rep := loadReport(app.Name, runs, loads, targets[0].name, len(targets) == 2)
	os.WriteFile(filepath.Join(*out, "load.txt"), []byte(rep), 0o644)
	fmt.Print(rep)
	if bad {
		return 1
	}
	return 0
}

// loadOnce starts bin, offers load ld for warm then dur, and stops it.
func loadOnce(app *App, mix mixFn, bin, work string, n int, ld string, warm, dur, idleWait time.Duration) (loadRun, error) {
	var r loadRun
	if err := os.MkdirAll(work, 0o755); err != nil {
		return r, err
	}
	p, err := start(app, bin, work, n)
	if err != nil {
		return r, err
	}
	pid := p.cmd.Process.Pid
	d := newDriver(app, mix, "http://"+p.addr, 0)
	var stop func()
	if v, ok := strings.CutPrefix(ld, "rate"); ok {
		rate, _ := strconv.Atoi(v)
		stop = d.open(rate)
	} else {
		w, _ := strconv.Atoi(strings.TrimPrefix(ld, "sat"))
		stop = d.closed(w)
	}
	time.Sleep(warm)
	d.takeLat()
	reqs0, errs0, drops0, c4xx0 := atomic.LoadInt64(&d.reqs), atomic.LoadInt64(&d.errs), atomic.LoadInt64(&d.drops), atomic.LoadInt64(&d.c4xx)
	_, _, cpu0, _ := procStats(pid)
	t0 := time.Now()
	time.Sleep(dur)
	l := d.takeLat()
	el := time.Since(t0).Seconds()
	if app.Stats != "" {
		saveStats(d.cl, d.base+app.Stats, app.Tokens[app.StatsToken], filepath.Join(work, "stats.txt"))
	}
	reqs := atomic.LoadInt64(&d.reqs) - reqs0
	_, _, cpu1, _ := procStats(pid)
	r.peakKB = peakRSS(pid)
	mem := "busy " + mapsSummary(pid)
	stop()
	// idle: every connection closed, past the runtime's 3 s fiber trim
	time.Sleep(idleWait)
	mem += "idle " + mapsSummary(pid)
	if app.Stats != "" {
		saveStats(d.cl, d.base+app.Stats, app.Tokens[app.StatsToken], filepath.Join(work, "stats-idle.txt"))
	}
	os.WriteFile(filepath.Join(work, "mem.txt"), []byte(mem), 0o644)
	r.errs = atomic.LoadInt64(&d.errs) - errs0
	r.drops = atomic.LoadInt64(&d.drops) - drops0
	r.c4xx = atomic.LoadInt64(&d.c4xx) - c4xx0
	r.reqs = reqs
	if err := p.stop("term"); err != nil {
		return r, err
	}
	r.rps = float64(reqs) / el
	r.p50, r.p99, r.p999 = pct(l, 0.5), pct(l, 0.99), pct(l, 0.999)
	if reqs > 0 {
		r.cpuPerReqUS = (cpu1 - cpu0) / float64(reqs)
	}
	return r, nil
}

// loadReport gives each target's median and range per load and metric and,
// with a twin, xo's ratio to it and whether the ranges overlap.
func loadReport(app string, runs []loadRun, loads []string, xoName string, twin bool) string {
	var b strings.Builder
	fmt.Fprintf(&b, "harness load %s: %d runs\n", app, len(runs))
	type metric struct {
		name   string
		get    func(loadRun) float64
		higher bool // higher is better
	}
	metrics := []metric{
		{"req/s", func(r loadRun) float64 { return r.rps }, true},
		{"p50 us", func(r loadRun) float64 { return r.p50 }, false},
		{"p99 us", func(r loadRun) float64 { return r.p99 }, false},
		{"p99.9 us", func(r loadRun) float64 { return r.p999 }, false},
		{"cpu us/req", func(r loadRun) float64 { return r.cpuPerReqUS }, false},
		{"peak RSS KiB", func(r loadRun) float64 { return r.peakKB }, false},
	}
	stats := func(target, ld string, m metric) (med, lo, hi float64, n int) {
		var v []float64
		for _, r := range runs {
			if r.target == target && r.load == ld {
				v = append(v, m.get(r))
			}
		}
		if len(v) == 0 {
			return 0, 0, 0, 0
		}
		sort.Float64s(v)
		return v[len(v)/2], v[0], v[len(v)-1], len(v)
	}
	for _, ld := range loads {
		fmt.Fprintf(&b, "\n%s\n", ld)
		var errs, drops int64
		var c4, n4 = map[string]int64{}, map[string]int64{}
		for _, r := range runs {
			if r.load == ld && r.target == xoName {
				errs += r.errs
				drops += r.drops
			}
			if r.load == ld {
				c4[r.target] += r.c4xx
				n4[r.target] += r.reqs
			}
		}
		for _, m := range metrics {
			xm, xl, xh, xn := stats(xoName, ld, m)
			fmt.Fprintf(&b, "  %-13s %s %.1f (%.1f to %.1f)", m.name, xoName, xm, xl, xh)
			if gm, gl, gh, gn := stats("go", ld, m); twin && gn > 0 {
				ratio := 0.0
				if gm != 0 {
					ratio = xm / gm
				}
				verdict := "ranges overlap"
				switch {
				case xn < 2 || gn < 2:
					// one run is a point, not a range
					verdict = "one run, no verdict"
				case m.name == "req/s" && strings.HasPrefix(ld, "rate"):
					// an open loop's rate is what was offered
					verdict = "offered"
				case xh < gl || gh < xl:
					if (xm > gm) == m.higher {
						verdict = "better"
					} else {
						verdict = "worse"
					}
				}
				fmt.Fprintf(&b, "  go %.1f (%.1f to %.1f)  xo/go %.3f %s", gm, gl, gh, ratio, verdict)
			}
			b.WriteString("\n")
		}
		fmt.Fprintf(&b, "  %s errors %d, drops %d\n", xoName, errs, drops)
		// a differing share of 4xx answers means the targets did different
		// work: the mix's later requests depend on earlier answers
		for _, t := range []string{xoName, "go"} {
			if n4[t] > 0 {
				fmt.Fprintf(&b, "  %s 4xx %d of %d (%.2f%%)\n", t, c4[t], n4[t], 100*float64(c4[t])/float64(n4[t]))
			}
		}
	}
	return b.String()
}
