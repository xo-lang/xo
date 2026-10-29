package rt

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"runtime/trace"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

// MixHash combines a hash with another.
func MixHash(h, x uint64) uint64 { return mix(h, x) }

// DebugTo writes the Debug text of v.
func DebugTo(b *strings.Builder, v any) { debugTo(b, v) }

// FMod is `a % b` on floats.
func FMod[F ~float32 | ~float64](a, b F) F { return F(math.Mod(float64(a), float64(b))) }

// FDiv is `a / b` on floats (IEEE 754: division by zero is infinite or
// NaN, not a fault).
func FDiv[F ~float32 | ~float64](a, b F) F { return a / b }

// FNeg is unary `-x` on floats: a call, so `-0.0` is negative zero and
// not a Go constant expression (Go constants have no negative zero).
func FNeg[F ~float32 | ~float64](x F) F { return -x }

// FVal is x, as a call: `float64(2.5e300) * float64(1e10)` is +Inf at run
// time in Xo but an overflowing constant in Go, so a float `+ - *` of two
// constants reads one through FVal.
func FVal[F ~float32 | ~float64](x F) F { return x }

// IntMulDur is checked `n * d`.
func IntMulDur(n int64, d Duration, at string) Duration { return MulS(Duration(n), d, at) }

// CurrentTarget is the predeclared `target` (core 12.2): the target the
// program was built for.
func CurrentTarget() Target {
	t := Target{F_os: TargetOs{Tag: 1}, F_arch: TargetArch{Tag: 1}}
	switch runtime.GOOS {
	case "darwin":
		t.F_os = TargetOs{Tag: 2}
	case "windows":
		t.F_os = TargetOs{Tag: 3}
	case "wasip1", "wasi":
		t.F_os = TargetOs{Tag: 4}
	}
	switch runtime.GOARCH {
	case "arm64":
		t.F_arch = TargetArch{Tag: 2}
	case "wasm":
		t.F_arch = TargetArch{Tag: 3}
	}
	return t
}

func stderrWrite(s string) { outWrite(os.Stderr, s) }

func stderrFile() *os.File { return os.Stderr }

// exitUnwind unwinds a test's code under test after `os.proc.exit(code)`
// (std.md 8.4).
type exitUnwind struct{ code int64 }

// RunMain runs a program's `main` (core 6.3, 7.5, 8, 11.2) and returns the
// process exit status: 0, 1 for an error returned from main, 70 for a
// fault. main runs as the body of the root scope, whose group is
// `os.tasks`. Shutdown has two phases (core 7.5, stop.go): the first
// SIGINT or SIGTERM requests a stop, the grace period or a second signal
// cancels the root scope, and a third exits with status 130.
//
// Live inspection (core 11.4, sched.go): SIGQUIT prints the task tree to
// standard error and to DumpPath (`xo dump <pid>`) and the program goes
// on. A monitor faults with XO-F001 and exit status 70 when every task is
// blocked on another task (a channel, mutex, await, or scope wait).
// XO_SEED=N (`xo run --seed=N`) runs every task on the deterministic
// scheduler with the real clock (core 7.4).
func RunMain(main func(*Task, Os) Result[Unit, IError]) (code int) {
	fxSetup()
	// the heap goal: a floor of 2 MiB per P, at least 16 MiB, unless GOGC
	// or xo.mod says otherwise (gc.go, decision 0065)
	gcSetup()
	if path := os.Getenv("XO_CPUPROFILE"); path != "" {
		// CPU profile of the whole run, for `go tool pprof` (toolchain.md)
		if f, err := os.Create(path); err == nil && pprof.StartCPUProfile(f) == nil {
			defer func() { pprof.StopCPUProfile(); f.Close() }()
		}
	}
	if path := os.Getenv("XO_TRACE"); path != "" {
		// execution trace of the whole run, for `go tool trace` (where a
		// slow request waited: GC, scheduling, locks, the network)
		if f, err := os.Create(path); err == nil && trace.Start(f) == nil {
			defer func() { trace.Stop(); f.Close() }()
		}
	}
	if path := os.Getenv("XO_MEMPROFILE"); path != "" {
		// where the run allocated (pprof -sample_index=alloc_space or
		// alloc_objects); XO_MEMPROFILERATE=N samples one allocation per N
		// bytes (1: every allocation) instead of Go's 512 KiB default
		if v, err := strconv.Atoi(os.Getenv("XO_MEMPROFILERATE")); err == nil && v > 0 {
			runtime.MemProfileRate = v
		}
		defer func() {
			if f, err := os.Create(path); err == nil {
				pprof.Lookup("allocs").WriteTo(f, 0)
				f.Close()
			}
		}()
	}
	if path := os.Getenv("XO_MEMSTATS"); path != "" {
		// allocation counters of the whole run, for allocations per
		// request (bench/perf/http/allocs.sh)
		defer func() {
			var ms runtime.MemStats
			runtime.ReadMemStats(&ms)
			os.WriteFile(path, fmt.Appendf(nil, "mallocs %d\ntotal_alloc %d\nnum_gc %d\n", ms.Mallocs, ms.TotalAlloc, ms.NumGC), 0o644)
		}()
	}
	if path := os.Getenv("XO_BLOCKPROFILE"); path != "" {
		// where goroutines wait (channels, locks, select), same tool
		runtime.SetBlockProfileRate(1000)
		defer func() {
			if f, err := os.Create(path); err == nil {
				pprof.Lookup("block").WriteTo(f, 0)
				f.Close()
			}
		}()
	}
	// XO_LOCKSTATS: mutex wait and hold times per call site (lockstats.go)
	defer lockStatsWrite()
	root := NewRootTask()
	th := root.th
	mon := &monitor{quiet: 200 * time.Millisecond, root: root.node}
	mon.report = func(f *FaultErr) {
		c := &crashInfo{fault: f}
		reportCrash(c)
		fxFinish(70, c)
		os.Exit(70)
	}
	th.mon = mon
	runMon.Store(mon)
	defer runMon.Store(nil)
	mon.started()
	seed, seeded := int64(0), false
	if v := os.Getenv("XO_SEED"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			seed, seeded = n, true
		}
	}
	switch fxMode {
	case fxRecord:
		// a recording runs every task on the deterministic scheduler and
		// logs its decisions (core 11.3); without XO_SEED the seed is
		// random, so recordings see varied interleavings
		if !seeded {
			seed = time.Now().UnixNano()%1_000_000_000 + 1
		}
		seeded = true
		rec.seed = seed
	case fxReplay:
		seed, seeded = rp.seed, rp.full
	}
	if seeded {
		realSched = newSched(new(sync.Mutex), nil, seed)
		switch {
		case fxMode == fxRecord:
			realSched.rec = rec
		case fxMode == fxReplay:
			realSched.rp = rp
			go rp.watch(realSched)
		}
		realSched.start(th)
		<-th.turn
	}
	defer func() {
		th.mu.Lock()
		th.done = true
		th.mu.Unlock()
		mon.ended()
		if realSched != nil {
			realSched.end(th)
		}
	}()
	// the outcome of the run, for the trace of a recording and the check
	// of a replay (core 11.3)
	var end *crashInfo
	defer func() { fxFinish(code, end) }()
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGQUIT)
	go func() {
		for range quit {
			writeDump(root.node)
		}
	}()
	defer signal.Stop(quit)
	st := newStopState(nil)
	root.stop = st
	if rp != nil {
		rp.stop = st
	}
	s := NewScope(root)
	st.cancelRoot = s.bodyNode.cancel
	st.exit = func() {
		fxFinish(130, nil)
		os.Exit(130)
	}
	defer st.finish()
	osv := RealOs(s.M_group(root))
	if fxMode != fxReplay {
		// a replay takes its signals from the trace
		sig := make(chan os.Signal, 3)
		signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
		requestStop = func() {
			if !st.requested.Load() {
				st.signal()
			}
		}
		go func() {
			for s := range sig {
				if sigTaken(s) {
					// a Proc.notify subscription takes it (decision 0407)
					continue
				}
				if rs := realSched; rs != nil && rs.rec != nil {
					rs.post(schedEv{tok: func() string { return "g" }, run: st.signal})
				} else {
					st.signal()
				}
			}
		}()
		defer signal.Stop(sig)
	}
	var res Result[Unit, IError]
	defer func() {
		r := recover()
		if r == nil {
			return
		}
		if IsUnwind(r) {
			code = 0
			return
		}
		end = &crashInfo{fault: AsFault(r)}
		reportCrash(end)
		code = 70
	}()
	ctl := func() (c int) {
		tk := s.Body()
		completed := false
		defer func() { c = s.Close(recover(), completed, c) }()
		res = main(tk, osv)
		completed = true
		return 0
	}()
	if ctl == CtlScopeFail {
		e, tr := s.Failure()
		end = errorInfo(root, e, tr)
		reportCrash(end)
		return 1
	}
	if !res.Ok {
		end = errorInfo(root, res.E, res.Tr)
		reportCrash(end)
		return 1
	}
	return 0
}

// crashInfo is what went wrong: a fault (in main, or isolated in a request
// handler) or an error returned from main.
type crashInfo struct {
	fault    *FaultErr
	errText  string // the error's display text, then its frames (core 11.1)
	isolated bool   // a fault isolated in a request handler (std.md 4.2)
	req      int64  // the request of an isolated fault
}

// errorInfo describes an error returned from main.
func errorInfo(tk *Task, e any, tr *Trace) *crashInfo {
	lines := append([]string{Display(tk, e)}, tr.Lines()...)
	return &crashInfo{errText: strings.Join(lines, "\n")}
}

// reportCrash writes a fault or an error that reached main, with its
// frames, and its crash bundle (core 11.1, 11.2).
func reportCrash(c *crashInfo) {
	var b strings.Builder
	if f := c.fault; f != nil {
		b.WriteString("fault: " + f.Error() + "\n")
		b.WriteString(f.Stack)
	} else {
		lines := strings.Split(c.errText, "\n")
		b.WriteString("error: " + lines[0] + "\n")
		for _, l := range lines[1:] {
			b.WriteString(l + "\n")
		}
	}
	if fxMode != fxReplay {
		if p := writeCrashBundle(c); p != "" {
			b.WriteString("crash bundle: " + p + "\n")
		}
	}
	stderrWrite(b.String())
}

// isolatedCrash handles a fault isolated in the handler of a request of a
// real server: a crash bundle (core 11.2), or under a replay of one, the
// check that it is the recorded crash.
func isolatedCrash(tk *Task, f *FaultErr) {
	if tk == nil || tk.w != nil || tk.th == nil || tk.th.kind != "request" {
		return
	}
	if fxMode == fxReplay {
		rp.isolatedFault(tk, f)
		return
	}
	if p := writeCrashBundle(&crashInfo{fault: f, isolated: true, req: tk.th.req}); p != "" {
		stderrWrite("crash bundle: " + p + "\n")
	}
}

// isolatedBundles counts the crash bundles of isolated faults: a server
// writes at most maxIsolatedBundles.
var isolatedBundles atomic.Int32

const maxIsolatedBundles = 100

// maxCrashBundles is how many bundles the default directory keeps.
const maxCrashBundles = 100

// crashDir is the default directory of crash bundles (owner,
// 2026-10-22): <user cache>/xo/crash, where they are not lost among the
// system temporary directory's files (0.25.2 wrote them to
// /var/folders/...), with the oldest past maxCrashBundles removed; the
// temporary directory when there is no user cache. `xo clean -cache`
// keeps it (compiler/prune clean.xo).
func crashDir() string {
	base, err := os.UserCacheDir()
	if err != nil || base == "" {
		return os.TempDir()
	}
	dir := filepath.Join(base, "xo", "crash")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return os.TempDir()
	}
	pruneCrashBundles(dir, maxCrashBundles-1)
	return dir
}

// pruneCrashBundles removes the oldest *.xocrash files of dir past keep
// (their names start with the program and end with the time, so the
// modification time orders them).
func pruneCrashBundles(dir string, keep int) {
	ms, _ := filepath.Glob(filepath.Join(dir, "*.xocrash"))
	if len(ms) <= keep {
		return
	}
	type f struct {
		p string
		t time.Time
	}
	fs := make([]f, 0, len(ms))
	for _, p := range ms {
		if st, err := os.Stat(p); err == nil {
			fs = append(fs, f{p, st.ModTime()})
		}
	}
	sort.Slice(fs, func(i, j int) bool { return fs[i].t.After(fs[j].t) })
	for _, x := range fs[min(keep, len(fs)):] {
		os.Remove(x.p)
	}
}

// writeCrashBundle writes `<name>-<time>.xocrash` (core 11.2) into
// $XO_CRASH_DIR, or crashDir (<user cache>/xo/crash): the fault or error
// with its frames, the task tree, the build id and source hashes, the
// target, and with a recording (XO_RECORD) or ring buffers (XO_RING=N)
// the effect trace of the failing request, which `xo replay` replays.
func writeCrashBundle(c *crashInfo) string {
	if c.isolated && isolatedBundles.Add(1) > maxIsolatedBundles {
		return ""
	}
	dir := os.Getenv("XO_CRASH_DIR")
	if dir == "" {
		dir = crashDir()
	}
	name := filepath.Base(os.Args[0])
	now := time.Now().UTC()
	path := filepath.Join(dir, fmt.Sprintf("%s-%s.xocrash", name, now.Format("20060102T150405.000000000")))
	code := 70
	bundle := map[string]any{
		"target": runtime.GOOS + "/" + runtime.GOARCH,
		"time":   now.Format(time.RFC3339Nano),
		"args":   os.Args,
	}
	if f := c.fault; f != nil {
		bundle["kind"] = "fault"
		bundle["fault"] = f.Msg
		bundle["code"] = f.Code
		bundle["location"] = f.Loc
		bundle["stack"] = strings.Split(strings.TrimRight(f.Stack, "\n"), "\n")
		if f.Detail != "" {
			bundle["detail"] = strings.Split(f.Detail, "\n")
		}
	} else {
		code = 1
		lines := strings.Split(c.errText, "\n")
		bundle["kind"] = "error"
		bundle["error"] = lines[0]
		bundle["frames"] = lines[1:]
	}
	if c.isolated {
		bundle["isolated"] = true
		bundle["request"] = c.req
	}
	if m := runMon.Load(); m != nil {
		// the task tree (core 11.2)
		bundle["tasks"] = strings.Split(strings.TrimRight(dumpText(m.root), "\n"), "\n")
	}
	if buildInfo != nil {
		bundle["build"] = buildInfo
	}
	data, err := json.MarshalIndent(bundle, "", "  ")
	if err != nil {
		return ""
	}
	if rec != nil {
		// the effect trace, one JSON object per line
		focus := int64(-1)
		if c.isolated {
			focus = c.req
		}
		lines := rec.lines(runOutcome(code, c), false, focus)
		var b bytes.Buffer
		b.Write(bytes.TrimRight(bytes.TrimSuffix(bytes.TrimRight(data, "\n"), []byte("}")), "\n"))
		b.WriteString(",\n  \"trace\": [\n")
		for i, l := range lines {
			b.WriteString("    " + l)
			if i < len(lines)-1 {
				b.WriteByte(',')
			}
			b.WriteByte('\n')
		}
		b.WriteString("  ]\n}\n")
		data = b.Bytes()
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return ""
	}
	return path
}
