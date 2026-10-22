package rt

import (
	"encoding/json"
	"flag"
	"fmt"
	"math/rand/v2"
	"os"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"
)

// std/testing (std.md 8, core 9.2).

// TestCase is one test block.
type TestCase struct {
	Name   string
	Module string
	// Dir is the module's directory: the working directory of its tests.
	Dir  string
	Loc  string
	Seed int64
	Fn   func(*Task, Testing_TestOs) Result[Unit, IError]
	Skip string
	// Prop is set for a property test (`test "x" for v: T`).
	Prop PropCase
	// Regress is set for a contract fuzz test: how to write a shrunk failure
	// as a permanent regression test (core 9.2).
	Regress *RegressInfo
	// Uses lists the effects of `test "x" uses fs.read { }`: with
	// `xo test --real` the test gets real capabilities for them (core 9.2);
	// without it the test is skipped.
	Uses []string
}

// expectFail is raised by a failed `expect`; it stops the test.
type expectFail struct{ msg string }

// Expect is `expect(cond)`.
func Expect(tk *Task, ok bool, src, loc string) {
	if !ok {
		panic(&expectFail{msg: fmt.Sprintf("expect failed: %s at %s", src, loc)})
	}
}

// ExpectCmp is `expect(a) op b` with both values in the message.
func ExpectCmp[T any](tk *Task, ok bool, a, b T, op, src, loc string) {
	if !ok {
		panic(&expectFail{msg: fmt.Sprintf("expect failed: %s at %s\n      left:  %s\n      right: %s", src, loc, Debug(a), Debug(b))})
	}
}

type testOutcome struct {
	status string // pass, fail, skip
	msg    string
	// concurrent is set when the test started a task (or a runtime helper
	// thread): only then can another seed give another interleaving
	concurrent bool
	// logs is the test's log output that went to no configured Stdio
	// (decision 0057): printed when the test fails, or with -v
	logs []string
}

// RunTests runs test tables and returns the exit status: 0 when every
// test passed, 1 otherwise. Flags: -seed N, -run text, -json, -v,
// -timeout d, -explore N, -real, -cases N.
//
// Every test runs on the deterministic scheduler (core 7.4, sched.go)
// seeded with its seed (the pinned `test "x" seed N`, else -seed, else 1),
// except tests with real capabilities. -explore N runs each concurrent
// test that passes under its seed again under the next N-1 seeds and
// reports the first failing one.
func RunTests(args []string, tables ...[]TestCase) int {
	return runTests(args, tables)
}

// RunTestTables is RunTests for the generated package main: each table
// is a module's Tests ([]TestCase) as an any, so that package main names
// no runtime type (a Go package naming one compiles the method sets of
// every generic runtime type it reaches); not inlined for the same reason.
//
//go:noinline
func RunTestTables(args []string, tables ...any) int {
	ts := make([][]TestCase, len(tables))
	for i, t := range tables {
		ts[i] = t.([]TestCase)
	}
	return runTests(args, ts)
}

func runTests(args []string, tables [][]TestCase) int {
	fs := flag.NewFlagSet("xotest", flag.ContinueOnError)
	seed := fs.Int64("seed", 0, "seed for rand and the scheduler")
	explore := fs.Int("explore", 0, "run each concurrent test under this many seeds")
	run := fs.String("run", "", "run only tests whose name contains this text")
	asJSON := fs.Bool("json", false, "print JSON lines")
	verbose := fs.Bool("v", false, "print passing tests too")
	timeout := fs.Duration("timeout", 60*time.Second, "real time limit per test (0: none)")
	real := fs.Bool("real", false, "give tests that declare `uses` real capabilities")
	cases := fs.Int("cases", 100, "cases per property test")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	pass, fail, skip := 0, 0, 0
	for _, table := range tables {
		for _, tc := range table {
			if *run != "" && !strings.Contains(tc.Name, *run) {
				continue
			}
			if tc.Dir != "" {
				os.Chdir(tc.Dir)
			}
			s := tc.Seed
			if s == 0 {
				s = *seed
			}
			if s == 0 {
				s = 1
			}
			var out testOutcome
			switch {
			case tc.Prop != nil && tc.Skip == "":
				out = runProperty(tc, s, *cases, *timeout)
			case tc.Skip != "" || tc.Fn == nil:
				out = testOutcome{status: "skip", msg: tc.Skip}
			case len(tc.Uses) > 0 && !*real:
				out = testOutcome{status: "skip", msg: "uses " + strings.Join(tc.Uses, ", ") + ": run with `xo test --real` for real capabilities"}
			default:
				out = runOneTest(tc, s, *timeout)
				if out.status == "pass" && out.concurrent && *explore > 1 && tc.Seed == 0 {
					first := s
					for i := 1; i < *explore; i++ {
						o := runOneTest(tc, first+int64(i), *timeout)
						if o.status == "fail" {
							s = first + int64(i)
							// the exact command: --backend=go, since --explore
							// chose it and only it has the deterministic
							// scheduler (native `xo test --seed=N` seeds only
							// Rand, so it passed again; a 0.25.2 field report)
							o.msg = fmt.Sprintf("explored seeds %d to %d: seed %d fails; reproduce with `xo test --backend=go --seed=%d --run=%s`, or pin it with `test %q seed %d`\n%s", first, s, s, s, shellQuote(tc.Name), tc.Name, s, o.msg)
							out = o
							break
						}
					}
				}
			}
			switch out.status {
			case "pass":
				pass++
			case "fail":
				fail++
			default:
				skip++
			}
			if *asJSON {
				rec := map[string]any{"name": tc.Name, "module": tc.Module, "loc": tc.Loc, "status": out.status, "message": out.msg, "seed": s}
				if len(out.logs) > 0 && (out.status == "fail" || *verbose) {
					rec["log"] = out.logs
				}
				data, _ := json.Marshal(rec)
				fmt.Fprintln(os.Stdout, string(data))
				continue
			}
			switch {
			case out.status == "fail":
				fmt.Fprintf(os.Stdout, "--- FAIL: %s (%s, seed %d)\n", tc.Name, tc.Loc, s)
				for _, l := range strings.Split(strings.TrimRight(out.msg, "\n"), "\n") {
					fmt.Fprintf(os.Stdout, "    %s\n", l)
				}
				printTestLogs(out.logs)
			case out.status == "skip" && *verbose:
				fmt.Fprintf(os.Stdout, "--- SKIP: %s (%s)\n", tc.Name, out.msg)
			case out.status == "pass" && *verbose:
				fmt.Fprintf(os.Stdout, "--- PASS: %s\n", tc.Name)
				printTestLogs(out.logs)
			}
		}
	}
	if !*asJSON {
		status := "ok"
		if fail > 0 {
			status = "FAIL"
		}
		fmt.Fprintf(os.Stdout, "%s: %d passed, %d failed, %d skipped\n", status, pass, fail, skip)
	}
	if fail > 0 {
		return 1
	}
	return 0
}

// printTestLogs prints a test's buffered log lines under its result.
func printTestLogs(lines []string) {
	if len(lines) == 0 {
		return
	}
	fmt.Fprintf(os.Stdout, "    log:\n")
	for _, l := range lines {
		fmt.Fprintf(os.Stdout, "      %s\n", l)
	}
}

// runProperty runs a property test on generated inputs, each case in a
// fresh world, and reports the first failing input, shrunk (core 9.2).
func runProperty(tc TestCase, seed int64, cases int, limit time.Duration) testOutcome {
	for i := 0; i < cases; i++ {
		g := NewGenRand(uint64(seed)*1000003 + uint64(i))
		input, fn := tc.Prop(g)
		c := tc
		c.Fn = fn
		out := runOneTest(c, seed, limit)
		if out.status == "fail" {
			if sIn, sOut, steps := shrinkProperty(tc, g, seed, limit); steps > 0 {
				recordRegression(tc, seed, sIn)
				return testOutcome{status: "fail", msg: fmt.Sprintf("property failed on case %d; shrunk in %d steps to\n%s%s", i+1, steps, sIn, sOut.msg)}
			}
			out.msg = fmt.Sprintf("property failed on case %d with\n%s%s", i+1, input, out.msg)
			return out
		}
	}
	return testOutcome{status: "pass"}
}

// shortlexLess orders choice sequences: shorter first, then by the first
// differing choice. A shrink step must make the choices smaller in this
// order, so shrinking terminates and moves toward simpler values.
func shortlexLess(a, b []uint64) bool {
	if len(a) != len(b) {
		return len(a) < len(b)
	}
	for i := range a {
		if a[i] != b[i] {
			return a[i] < b[i]
		}
	}
	return false
}

// shrinkProperty edits the recorded choices of a failing case (deleting
// runs, zeroing, halving, decrementing) and keeps every edit after which
// the test still fails. It returns the smallest failing input found.
func shrinkProperty(tc TestCase, failing *GenRand, seed int64, limit time.Duration) (string, testOutcome, int) {
	best := append([]uint64{}, failing.rec...)
	var bestIn string
	var bestOut testOutcome
	steps, budget := 0, 2000
	try := func(cand []uint64) bool {
		if budget <= 0 {
			return false
		}
		budget--
		g := replayGen(cand, failing.Small)
		input, fn := tc.Prop(g)
		c := tc
		c.Fn = fn
		if !shortlexLess(g.rec, best) {
			return false
		}
		out := runOneTest(c, seed, limit)
		if out.status != "fail" {
			return false
		}
		best, bestIn, bestOut = append([]uint64{}, g.rec...), input, out
		steps++
		return true
	}
	for improved := true; improved && budget > 0; {
		improved = false
		for k := 8; k >= 1 && !improved; k /= 2 {
			for i := 0; i+k <= len(best) && !improved; i++ {
				cand := append(append([]uint64{}, best[:i]...), best[i+k:]...)
				improved = try(cand)
			}
		}
		for i := 0; i < len(best) && !improved; i++ {
			if best[i] == 0 {
				continue
			}
			// binary search for the smallest choice that still fails
			lo, hi := uint64(0), best[i]
			for lo < hi && budget > 0 {
				mid := lo + (hi-lo)/2
				cand := append([]uint64{}, best...)
				if i >= len(cand) {
					break
				}
				cand[i] = mid
				if try(cand) {
					hi = mid
					improved = true
				} else {
					lo = mid + 1
				}
			}
		}
	}
	return bestIn, bestOut, steps
}

// runOneTest runs a test in a fresh world: on the deterministic scheduler
// seeded with seed, unless the test has real capabilities.
func runOneTest(tc TestCase, seed int64, limit time.Duration) testOutcome {
	var w *World
	if len(tc.Uses) == 0 {
		w = NewSchedWorld(seed)
	} else {
		w = NewWorld()
	}
	// logs go to a buffer per test (std.md 6, decision 0057)
	w.log = newTestLogSink()
	done := make(chan testOutcome, 1)
	go func() {
		out := runInWorld(w, tc, seed)
		if w.sch != nil {
			w.mu.Lock()
			out.concurrent = w.sch.started > 0
			w.mu.Unlock()
		}
		done <- out
	}()
	var out testOutcome
	// the time limit is no Go timer: a pending one would keep a deadlock
	// through a Go call from being reported (gotimers_on.go)
	stop := make(chan struct{})
	defer close(stop)
	select {
	case out = <-done:
	case <-w.stuck:
		f := w.DeadlockFault()
		out = testOutcome{status: "fail", msg: "fault: " + f.Error(), concurrent: true}
	case <-limitAfter(limit, stop):
		out = testOutcome{status: "fail", msg: fmt.Sprintf("the test did not finish within %s of real time", limit)}
	}
	out.logs = testLogLines(w)
	return out
}

// limitAfter is osAfter for a test's time limit: never for 0 or less
// (`xo test --timeout=0`, no limit).
func limitAfter(limit time.Duration, stop <-chan struct{}) <-chan struct{} {
	if limit <= 0 {
		return nil
	}
	return osAfter(limit, stop)
}

func runInWorld(w *World, tc TestCase, seed int64) (out testOutcome) {
	tk := w.NewTask()
	defer w.EndTask(tk)
	s := NewScope(tk)
	tos := newTestOs(w, seed, s.M_group(tk))
	if len(tc.Uses) > 0 {
		realCaps(&tos, tc.Uses)
	}
	var res Result[Unit, IError]
	defer func() {
		r := recover()
		if r == nil {
			return
		}
		switch x := r.(type) {
		case *expectFail:
			out = testOutcome{status: "fail", msg: x.msg}
		case *exitUnwind:
			out = testOutcome{status: "pass"}
		default:
			if IsUnwind(r) {
				out = testOutcome{status: "fail", msg: "the test was canceled"}
				return
			}
			f := AsFault(r)
			out = testOutcome{status: "fail", msg: "fault: " + f.Error() + "\n" + strings.TrimRight(f.Stack, "\n")}
		}
	}()
	ctl := func() (c int) {
		body := s.Body()
		defer func() {
			// os.tasks lives for the test and is canceled when it ends
			c = s.Close(recover(), false, c)
		}()
		res = tc.Fn(body, tos)
		return 0
	}()
	if ctl == CtlScopeFail {
		e, tr := s.Failure()
		return testOutcome{status: "fail", msg: errorText(tk, e, tr)}
	}
	if !res.Ok {
		return testOutcome{status: "fail", msg: errorText(tk, res.E, res.Tr)}
	}
	return testOutcome{status: "pass"}
}

func errorText(tk *Task, e any, tr *Trace) string {
	var b strings.Builder
	b.WriteString("error: " + Display(tk, e) + "\n")
	for _, l := range tr.Lines() {
		b.WriteString(l + "\n")
	}
	return b.String()
}

func newTestOs(w *World, seed int64, tasks Task_Group) Testing_TestOs {
	fs := &memFs{w: w, caseSensitive: true, nodes: map[string]*memNode{"": {dir: true}}}
	stdio := &fakeStdio{}
	return Testing_TestOs{
		F_net:   Testing_FakeNet{n: &fakeNet{listeners: map[string]*memListener{}}},
		F_fs:    Testing_MemFs{v: &memView{fs: fs}},
		F_clock: Testing_FakeClock{w: w},
		F_rand:  &seededRand{r: rand.New(rand.NewPCG(uint64(seed), uint64(seed)^0x9e3779b97f4a7c15))},
		F_env:   Testing_FakeEnv{e: &fakeEnv{}},
		F_stdio: Testing_FakeStdio{s: stdio},
		F_proc:  Testing_FakeProc{p: &fakeProc{runs: map[string]ProcOutput{}, stdio: stdio}},
		F_tasks: tasks,
	}
}

// realCaps replaces the fakes of the effects a test declares with real
// capabilities (core 9.2, `xo test --real`). The fakes delegate to them;
// their control methods fault.
func realCaps(tos *Testing_TestOs, uses []string) {
	read, write := false, false
	for _, u := range uses {
		switch u {
		case "fs":
			read, write = true, true
		case "fs.read":
			read = true
		case "fs.write":
			write = true
		case "net", "net.dial", "net.listen":
			tos.F_net.n.real = realNet{}
		case "clock":
			tos.F_clock.real = true
		case "env":
			tos.F_env.e.real = realEnv{}
		case "stdio":
			tos.F_stdio.s.real = realStdio{}
		case "proc":
			tos.F_proc.p.real = realProc{}
		}
	}
	if read || write {
		var f Fs = &realFs{}
		if !write {
			f = f.M_read_only(nil)
		}
		if !read {
			f = f.M_write_only(nil)
		}
		tos.F_fs.v.real = f
	}
}

// Testing_run_main is `testing.run_main(main, os, args)` (std.md 8.4): runs
// a program's main against the fakes and returns its exit status: 0, 1 for
// an error (printed to the fake standard error), 70 for a fault, the code
// passed to exit, or 130 after a third signal. Signals queued with
// `os.proc.signal_after` are delivered on the virtual clock and drive the
// two phase shutdown of core 7.5 as in RunMain.
func Testing_run_main(tk *Task, main func(*Task, Os) Result[Unit, IError], tos Testing_TestOs, args List[string]) (code int64) {
	tos.F_env.e.setArgs(args)
	osv := Os{F_net: tos.F_net, F_fs: tos.F_fs, F_clock: tos.F_clock, F_rand: tos.F_rand, F_env: tos.F_env, F_proc: tos.F_proc, F_stdio: tos.F_stdio, F_tasks: tos.F_tasks}
	p := tos.F_proc.p
	st := newStopState(tk.w)
	st.proc = p
	defer func() {
		// a third signal exits with 130 however main ended (it was
		// canceled by the second, so it unwound to here)
		if st.exited.Load() {
			code = 130
			p.mu.Lock()
			p.code = Some(int64(130))
			p.mu.Unlock()
		}
	}()
	defer st.finish()
	defer func() {
		r := recover()
		if r == nil {
			return
		}
		if ex, ok := r.(*exitUnwind); ok {
			code = ex.code
			return
		}
		if _, ok := r.(*expectFail); ok {
			panic(r)
		}
		if IsUnwind(r) {
			// main unwound because the stop state canceled its root
			// scope: a clean shutdown, as in RunMain. Anything else is the
			// test's own cancellation and keeps unwinding.
			if st.canceled.Load() && !tk.Canceled() {
				code = 0
				return
			}
			panic(r)
		}
		f := AsFault(r)
		tos.F_stdio.M_eprintln(tk, "fault: "+f.Error())
		code = 70
	}()
	s := NewScope(tk)
	s.body.stop = st
	st.cancelRoot = s.bodyNode.cancel
	osv.F_tasks = s.M_group(s.body)
	p.mu.Lock()
	sigs := p.signals
	p.signals = nil
	if p.grace.Some {
		st.grace = p.grace.V
	}
	p.mu.Unlock()
	for _, d := range sigs {
		st.signalAfter(d)
	}
	var res Result[Unit, IError]
	ctl := func() (c int) {
		body := s.Body()
		completed := false
		defer func() { c = s.Close(recover(), completed, c) }()
		res = main(body, osv)
		completed = true
		return 0
	}()
	if ctl == CtlScopeFail {
		e, tr := s.Failure()
		tos.F_stdio.M_eprint(tk, errorText(tk, e, tr))
		return 1
	}
	if !res.Ok {
		tos.F_stdio.M_eprint(tk, errorText(tk, res.E, res.Tr))
		return 1
	}
	return 0
}

// ---------------------------------------------------------------------------
// Rand (seeded, std.md 3.4)

type seededRand struct {
	mu sync.Mutex
	r  *rand.Rand
}

func (s *seededRand) next() uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.r.Uint64()
}

func (s *seededRand) M_int(_ *Task, lo, hi int64) int64 { return randRange(s.next, lo, hi) }
func (s *seededRand) M_float(*Task) float64             { return float64(s.next()>>11) / (1 << 53) }
func (s *seededRand) M_bool(*Task) bool                 { return s.next()&1 == 1 }
func (s *seededRand) M_bytes(_ *Task, n int64) Bytes {
	b := make([]byte, max(n, 0))
	for i := range b {
		b[i] = byte(s.next())
	}
	return Bytes(b)
}

// XoDebug implements Debug.
func (s *seededRand) XoDebug(b *strings.Builder) { b.WriteString("<rand>") }

// ---------------------------------------------------------------------------
// FakeClock (std.md 8.2)

// Testing_FakeClock is the virtual clock of a test (real time when the
// test declares `uses clock` and runs with --real).
type Testing_FakeClock struct {
	w    *World
	real bool
}

// XoDebug implements Debug.
func (Testing_FakeClock) XoDebug(b *strings.Builder) { b.WriteString("<fake clock>") }

// M_now is `clock.now()`.
func (c Testing_FakeClock) M_now(tk *Task) Time {
	tk.Yield()
	if c.real {
		return realClock{}.M_now(tk)
	}
	return c.w.Now()
}

// M_sleep is `clock.sleep(d)` on the virtual clock.
func (c Testing_FakeClock) M_sleep(tk *Task, d Duration) {
	if c.real {
		realClock{}.M_sleep(tk, d)
		return
	}
	c.w.Sleep(tk, d)
}

// M_sleep_or_cancel is `clock.sleep_or_cancel(d)` (decision 0088).
func (c Testing_FakeClock) M_sleep_or_cancel(tk *Task, d Duration) Result[Unit, ClockErr] {
	return sleepOrCancel(tk, func() { c.M_sleep(tk, d) })
}

// M_since is `clock.since(t)`.
func (c Testing_FakeClock) M_since(tk *Task, t Time) Duration { return Duration(c.M_now(tk) - t) }

// M_advance is `clock.advance(d)` (std.md 8.2).
func (c Testing_FakeClock) M_advance(tk *Task, d Duration) { c.w.Advance(tk, d) }

// M_set is `clock.set(t)`.
func (c Testing_FakeClock) M_set(_ *Task, t Time) { c.w.SetNow(t) }

// M_sleeps is `clock.sleeps()`: every sleep requested, in order.
func (c Testing_FakeClock) M_sleeps(*Task) List[Duration] { return ListOf(c.w.Sleeps()...) }

// ---------------------------------------------------------------------------
// FakeEnv, FakeStdio, FakeProc (std.md 8.4)

type fakeEnv struct {
	mu   sync.Mutex
	vars Map[string, string]
	args List[string]
	real Env
}

func (e *fakeEnv) setArgs(a List[string]) {
	e.mu.Lock()
	e.args = a
	e.mu.Unlock()
}

// Testing_FakeEnv is the fake environment of a test.
type Testing_FakeEnv struct{ e *fakeEnv }

// XoDebug implements Debug.
func (Testing_FakeEnv) XoDebug(b *strings.Builder) { b.WriteString("<fake env>") }

func (f Testing_FakeEnv) M_get(tk *Task, name string) Option[string] {
	if f.e.real != nil {
		return f.e.real.M_get(tk, name)
	}
	f.e.mu.Lock()
	defer f.e.mu.Unlock()
	return Map_get(nil, f.e.vars, name)
}

func (f Testing_FakeEnv) M_vars(tk *Task) Map[string, string] {
	if f.e.real != nil {
		return f.e.real.M_vars(tk)
	}
	f.e.mu.Lock()
	defer f.e.mu.Unlock()
	// marked under the lock: M_set overwrites an unshared map in place
	return Map_Own(f.e.vars)
}

func (f Testing_FakeEnv) M_args(*Task) List[string] {
	f.e.mu.Lock()
	defer f.e.mu.Unlock()
	return f.e.args
}

func (f Testing_FakeEnv) M_program(*Task) string { return "test" }

func (f Testing_FakeEnv) M_home(tk *Task) Result[string, EnvErr] {
	if h := f.M_get(tk, "HOME"); h.Some {
		return Ok[string, EnvErr](h.V)
	}
	return Err[string](EnvErr{Tag: 1, P: EnvErr_Missing{F_name: "HOME"}})
}

// M_set is `env.set(name, value)`.
func (f Testing_FakeEnv) M_set(_ *Task, name, value string) {
	f.e.mu.Lock()
	Map_put(nil, &f.e.vars, name, value)
	f.e.mu.Unlock()
}

// M_set_args is `env.set_args(args)`.
func (f Testing_FakeEnv) M_set_args(_ *Task, args List[string]) { f.e.setArgs(args) }

type fakeStdio struct {
	real   Stdio
	mu     sync.Mutex
	out    strings.Builder
	errb   strings.Builder
	input  string
	offset int
	width  int64 // set_tty: terminals of this width (0: not terminals)
}

// Testing_FakeStdio is the captured standard streams of a test.
type Testing_FakeStdio struct{ s *fakeStdio }

// XoDebug implements Debug.
func (Testing_FakeStdio) XoDebug(b *strings.Builder) { b.WriteString("<fake stdio>") }

func (f Testing_FakeStdio) write(err bool, s string) {
	if f.s.real != nil {
		if err {
			f.s.real.M_eprint(nil, s)
		} else {
			f.s.real.M_print(nil, s)
		}
	}
	f.s.mu.Lock()
	defer f.s.mu.Unlock()
	if err {
		f.s.errb.WriteString(s)
	} else {
		f.s.out.WriteString(s)
	}
}

func (f Testing_FakeStdio) M_print(tk *Task, s string)    { tk.Yield(); f.write(false, s) }
func (f Testing_FakeStdio) M_println(tk *Task, s string)  { tk.Yield(); f.write(false, s+"\n") }
func (f Testing_FakeStdio) M_eprint(tk *Task, s string)   { tk.Yield(); f.write(true, s) }
func (f Testing_FakeStdio) M_eprintln(tk *Task, s string) { tk.Yield(); f.write(true, s+"\n") }

func (f Testing_FakeStdio) M_write(tk *Task, data Bytes) Result[Unit, IoErr] {
	tk.Yield()
	if tk.Canceled() {
		return Err[Unit](IoErr{Tag: 3})
	}
	f.write(false, string(data))
	return Ok[Unit, IoErr](Unit{})
}

func (f Testing_FakeStdio) M_read_line(tk *Task) Result[Option[string], IoErr] {
	if stdinStopped(tk) {
		return Err[Option[string]](IoErr{Tag: 3})
	}
	f.s.mu.Lock()
	defer f.s.mu.Unlock()
	rest := f.s.input[f.s.offset:]
	if rest == "" {
		return Ok[Option[string], IoErr](Option[string]{})
	}
	line := rest
	if i := strings.IndexByte(rest, '\n'); i >= 0 {
		line = rest[:i]
		f.s.offset += i + 1
	} else {
		f.s.offset = len(f.s.input)
	}
	return Ok[Option[string], IoErr](Some(strings.TrimSuffix(line, "\r")))
}

func (f Testing_FakeStdio) M_read_all(tk *Task) Result[string, IoErr] {
	if stdinStopped(tk) {
		return Err[string](IoErr{Tag: 3})
	}
	f.s.mu.Lock()
	defer f.s.mu.Unlock()
	rest := f.s.input[f.s.offset:]
	f.s.offset = len(f.s.input)
	return Ok[string, IoErr](rest)
}

func (f Testing_FakeStdio) M_lines(tk *Task) Result[List[string], IoErr] {
	r := f.M_read_all(tk)
	if !r.Ok {
		return Err[List[string]](r.E)
	}
	return Ok[List[string], IoErr](Str_lines(tk, r.V))
}

// M_set_input is `stdio.set_input(text)`.
func (f Testing_FakeStdio) M_set_input(_ *Task, text string) {
	f.s.mu.Lock()
	f.s.input, f.s.offset = text, 0
	f.s.mu.Unlock()
}

// M_output is everything printed to standard output.
func (f Testing_FakeStdio) M_output(*Task) string {
	f.s.mu.Lock()
	defer f.s.mu.Unlock()
	return f.s.out.String()
}

// M_errors is everything printed to standard error.
func (f Testing_FakeStdio) M_errors(*Task) string {
	f.s.mu.Lock()
	defer f.s.mu.Unlock()
	return f.s.errb.String()
}

type fakeProc struct {
	real    Proc
	mu      sync.Mutex
	code    Option[int64]
	runs    map[string]ProcOutput
	signals []Duration       // signals for the next run_main, from its start
	grace   Option[Duration] // grace period for the next run_main
	stdio   *fakeStdio       // the test's, where Cmd.run writes (decision 0161)
	killed  []string         // Proc.kill's signals, "NAME PID" (decision 0190)
}

// Testing_FakeProc is the fake process control of a test.
type Testing_FakeProc struct{ p *fakeProc }

// XoDebug implements Debug.
func (Testing_FakeProc) XoDebug(b *strings.Builder) { b.WriteString("<fake proc>") }

// M_exit records the code and unwinds the code under test (std.md 8.4).
func (f Testing_FakeProc) M_exit(_ *Task, code int64) Never {
	f.p.mu.Lock()
	f.p.code = Some(code)
	f.p.mu.Unlock()
	panic(&exitUnwind{code: code})
}

func (f Testing_FakeProc) M_run(tk *Task, cmd string, args List[string]) Result[ProcOutput, ProcErr] {
	if f.p.real != nil {
		return f.p.real.M_run(tk, cmd, args)
	}
	if tk.Canceled() {
		return Err[ProcOutput](ProcErr{Tag: 3})
	}
	f.p.mu.Lock()
	defer f.p.mu.Unlock()
	if out, ok := f.p.runs[cmd]; ok {
		return Ok[ProcOutput, ProcErr](out)
	}
	return Err[ProcOutput](ProcErr{Tag: 1, P: ProcErr_NotFound{F_cmd: cmd}})
}

// M_exit_code is set when exit was called.
func (f Testing_FakeProc) M_exit_code(*Task) Option[int64] {
	f.p.mu.Lock()
	defer f.p.mu.Unlock()
	return f.p.code
}

// M_signal_after delivers a signal (SIGTERM) to a program run by
// testing.run_main after d of virtual time (core 7.5): called before
// run_main, d counts from the start of the next run_main; called by the
// program itself, from now. Each call delivers one more signal: the first
// requests a stop, the second cancels main's root scope, the third exits
// with status 130.
func (f Testing_FakeProc) M_signal_after(tk *Task, d Duration) {
	if st := tk.stop; st != nil && st.proc == f.p {
		st.signalAfter(d)
		return
	}
	f.p.mu.Lock()
	f.p.signals = append(f.p.signals, d)
	f.p.mu.Unlock()
}

// M_set_grace is `os.proc.set_grace(d)` (core 7.5): inside run_main it sets
// the running program's grace period; before, the next run_main's.
func (f Testing_FakeProc) M_set_grace(tk *Task, d Duration) {
	if st := tk.stop; st != nil && st.proc == f.p {
		st.setGrace(d)
		return
	}
	f.p.mu.Lock()
	f.p.grace = Some(max(d, 0))
	f.p.mu.Unlock()
}

// M_on_run registers the output of a command.
func (f Testing_FakeProc) M_on_run(_ *Task, cmd string, out ProcOutput) {
	f.p.mu.Lock()
	f.p.runs[cmd] = out
	f.p.mu.Unlock()
}

// ---------------------------------------------------------------------------
// FakeNet (std.md 8.1)

type fakeNet struct {
	real      Net
	mu        sync.Mutex
	handlers  []*fakeHandler
	fails     []*fakeFail
	reqs      []Http_Request
	listeners map[string]*memListener
	nextPort  int
}

type fakeHandler struct {
	method string
	prefix string // scheme://host[:port]
	r      *route
	h      func(*Task, Http_Request) Result[Http_Response, Never]
}

type fakeFail struct {
	method string
	prefix string
	r      *route
	err    Http_ClientErr
	once   bool
}

// parseFakePattern splits "[METHOD ]https://host/path/{x}".
func parseFakePattern(p string) (string, string, *route) {
	method := ""
	if i := strings.IndexByte(p, ' '); i > 0 && !strings.Contains(p[:i], "/") {
		method, p = strings.ToUpper(p[:i]), strings.TrimSpace(p[i+1:])
	}
	prefix := ""
	rest := p
	if i := strings.Index(p, "://"); i >= 0 {
		j := strings.IndexByte(p[i+3:], '/')
		if j < 0 {
			prefix, rest = p, "/"
		} else {
			prefix, rest = p[:i+3+j], p[i+3+j:]
		}
	}
	_, segs := parsePattern(rest)
	return method, prefix, &route{segs: segs}
}

// Testing_FakeNet is the fake network of a test.
type Testing_FakeNet struct{ n *fakeNet }

// XoDebug implements Debug.
func (Testing_FakeNet) XoDebug(b *strings.Builder) { b.WriteString("<fake net>") }

// M_handle registers a handler for "[METHOD ]<absolute url pattern>".
func (f Testing_FakeNet) M_handle(_ *Task, pattern string, h func(*Task, Http_Request) Result[Http_Response, Never]) {
	m, prefix, r := parseFakePattern(pattern)
	f.n.mu.Lock()
	f.n.handlers = append(f.n.handlers, &fakeHandler{method: m, prefix: prefix, r: r, h: h})
	f.n.mu.Unlock()
}

// M_fail makes every matching request fail until clear_failures.
func (f Testing_FakeNet) M_fail(_ *Task, pattern string, err Http_ClientErr) {
	m, prefix, r := parseFakePattern(pattern)
	f.n.mu.Lock()
	f.n.fails = append(f.n.fails, &fakeFail{method: m, prefix: prefix, r: r, err: err})
	f.n.mu.Unlock()
}

// M_fail_next makes the next matching request fail.
func (f Testing_FakeNet) M_fail_next(_ *Task, pattern string, err Http_ClientErr) {
	m, prefix, r := parseFakePattern(pattern)
	f.n.mu.Lock()
	f.n.fails = append(f.n.fails, &fakeFail{method: m, prefix: prefix, r: r, err: err, once: true})
	f.n.mu.Unlock()
}

// M_clear_failures removes every failure.
func (f Testing_FakeNet) M_clear_failures(*Task) {
	f.n.mu.Lock()
	f.n.fails = nil
	f.n.mu.Unlock()
}

// M_requests returns every request seen, including failed ones.
func (f Testing_FakeNet) M_requests(*Task) List[Http_Request] {
	f.n.mu.Lock()
	defer f.n.mu.Unlock()
	return ListOf(append([]Http_Request(nil), f.n.reqs...)...)
}

// isFake reports whether requests are served in process.
func (f Testing_FakeNet) isFake() bool { return f.n.real == nil }

// M_listen gives an in memory listener (":0" picks a port).
func (f Testing_FakeNet) M_listen(tk *Task, addr string) Result[Listener, NetErr] {
	if f.n.real != nil {
		return f.n.real.M_listen(tk, addr)
	}
	f.n.mu.Lock()
	defer f.n.mu.Unlock()
	if _, ok := unixPath(addr); ok {
		// an in memory Unix domain socket (decision 0190)
		if old := f.n.listeners[addr]; old != nil && !old.closed {
			return Err[Listener](netErr(4, "address already in use"))
		}
		l := &memListener{n: f.n, addr: addr, served: newEvent(), q: memQueue{changed: newEvent()}}
		f.n.listeners[addr] = l
		return Ok[Listener, NetErr](Listener{mem: l})
	}
	host, port := "127.0.0.1", strings.TrimPrefix(addr[strings.LastIndexByte(addr, ':')+1:], ":")
	if i := strings.LastIndexByte(addr, ':'); i > 0 {
		host = addr[:i]
	}
	if port == "" || port == "0" {
		f.n.nextPort++
		port = fmt.Sprint(49151 + f.n.nextPort)
	}
	if old := f.n.listeners[host+":"+port]; old != nil && !old.closed {
		return Err[Listener](netErr(4, "address already in use"))
	}
	pn, _ := strconv.Atoi(port)
	l := &memListener{n: f.n, addr: host + ":" + port, served: newEvent(), q: memQueue{changed: newEvent()}, port: pn}
	f.n.listeners[l.addr] = l
	if host == "127.0.0.1" {
		f.n.listeners["localhost:"+port] = l
	}
	return Ok[Listener, NetErr](Listener{mem: l})
}

// M_dial connects to the in memory listener at addr (decision 0292):
// Refused when there is none; else the connection waits in its backlog
// for accept.
func (f Testing_FakeNet) M_dial(tk *Task, addr string) Result[Conn, NetErr] {
	if f.n.real != nil {
		return f.n.real.M_dial(tk, addr)
	}
	if tk.Canceled() {
		return Err[Conn](netErr(5, ""))
	}
	f.n.mu.Lock()
	l := f.n.listeners[addr]
	if l != nil && l.closed {
		l = nil
	}
	if l != nil {
		f.n.nextPort++
	}
	port := 49151 + f.n.nextPort
	f.n.mu.Unlock()
	if l == nil {
		return Err[Conn](netErr(2, ""))
	}
	client, server := newMemPair(l.addr, port)
	if !l.q.push(server) {
		return Err[Conn](netErr(2, ""))
	}
	return Ok[Conn, NetErr](Conn{c: client})
}

func matchFake(method, prefix string, r *route, req Http_Request, rawURL string) (Map[string, string], bool) {
	if method != "" && method != strings.ToUpper(req.F_method) {
		return Map[string, string]{}, false
	}
	if prefix != "" && !strings.HasPrefix(rawURL, prefix) {
		return Map[string, string]{}, false
	}
	params, _, ok := r.match(nil, req.F_path)
	return params, ok
}

// dispatch serves a client request in process.
func (f Testing_FakeNet) dispatch(tk *Task, req Http_Request, rawURL string) Result[Http_Response, Http_ClientErr] {
	tk.Yield()
	n := f.n
	n.mu.Lock()
	n.reqs = append(n.reqs, req)
	for i, fl := range n.fails {
		if _, ok := matchFake(fl.method, fl.prefix, fl.r, req, rawURL); ok {
			if fl.once {
				n.fails = append(n.fails[:i], n.fails[i+1:]...)
			}
			n.mu.Unlock()
			return Err[Http_Response](fl.err)
		}
	}
	var best *fakeHandler
	var params Map[string, string]
	for _, h := range n.handlers {
		if p, ok := matchFake(h.method, h.prefix, h.r, req, rawURL); ok {
			if best == nil || h.r.literals() > best.r.literals() {
				best, params = h, p
			}
		}
	}
	var srv *memListener
	if best == nil {
		hostport := req.F_url.F_host
		if req.F_url.F_port.Some {
			hostport += fmt.Sprint(":", req.F_url.F_port.V)
		}
		srv = n.listeners[hostport]
	}
	n.mu.Unlock()
	if best != nil {
		req.XParams = params
		return Ok[Http_Response, Http_ClientErr](best.h(tk, req).V)
	}
	if srv != nil && !srv.served.isSet() {
		// like a real socket after listen: the connection waits in the
		// backlog until http.server(l, r).serve() starts
		if !srv.served.wait(tk, true, waitAccept, nil) {
			return Err[Http_Response](clientErr(5, ""))
		}
	}
	if srv != nil {
		if r, st, ok := srv.begin(); ok {
			defer srv.end()
			htk := tk
			if st != nil {
				// the handler sees the serving program's stop state
				htk = &Task{node: tk.node, w: tk.w, held: tk.held, shield: tk.shield, stop: st, th: tk.th}
			}
			return Ok[Http_Response, Http_ClientErr](r.M_call(htk, req))
		}
	}
	return Err[Http_Response](clientErr(2, "no fake handler for "+req.F_method+" "+rawURL))
}

// memListener is an in memory listener served by http.server in tests.
type memListener struct {
	n      *fakeNet
	addr   string
	served *event // fired when serve starts
	// the raw connections dialed to it (decision 0292)
	q      memQueue
	port   int
	closed bool

	mu       sync.Mutex
	r        *Http_Router
	stop     *stopState // of the serving program
	inflight int
	drained  *event // fired when inflight drops to 0 while draining
}

// close stops the listener's raw connections (decision 0292): a waiting
// accept fails, the queued connections are reset, and its address is
// free again.
func (l *memListener) close() {
	l.n.mu.Lock()
	l.closed = true
	l.n.mu.Unlock()
	l.q.close()
}

// begin starts a request on the listener: it reports the router and the
// serving program's stop state, or false when nothing is served.
func (l *memListener) begin() (Http_Router, *stopState, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.r == nil {
		return Http_Router{}, nil, false
	}
	l.inflight++
	return *l.r, l.stop, true
}

// end finishes a request started by begin.
func (l *memListener) end() {
	l.mu.Lock()
	l.inflight--
	var ev *event
	if l.inflight == 0 && l.drained != nil {
		ev, l.drained = l.drained, nil
	}
	l.mu.Unlock()
	if ev != nil {
		ev.fire()
	}
}

// serve routes requests for the listener's address to r until a stop is
// requested (then it waits for the requests in flight, core 7.5) or the
// task is canceled.
func (l *memListener) serve(tk *Task, r Http_Router) Result[Unit, Http_HttpErr] {
	l.mu.Lock()
	l.r = &r
	l.stop = tk.stop
	l.mu.Unlock()
	l.served.fire()
	stopped := false
	if tk.stop != nil {
		stopped = tk.stop.ev.wait(tk, true, waitStop, nil)
	} else if !tk.Canceled() {
		// a task canceled before it got here (a background serve whose
		// scope ended first) must not block: nothing would wake it
		tk.blockOn(waitServe, nil, true)
		<-tk.node.doneCh()
		tk.unblock()
	}
	l.mu.Lock()
	l.r = nil
	var drained *event
	if stopped && l.inflight > 0 {
		drained = newEvent()
		l.drained = drained
	}
	l.mu.Unlock()
	if drained != nil {
		// cancellation (grace period, second signal) cuts the wait short
		drained.wait(tk, true, waitDrain, nil)
	}
	return Ok[Unit, Http_HttpErr](Unit{})
}

// ---------------------------------------------------------------------------
// MemFs (std.md 8.3)

type memNode struct {
	dir  bool
	data Bytes
	mod  Time
	name string // path with original case
	// set_mode's permission bits (decision 0163); without, 0o644 or 0o755
	mode    int64
	hasMode bool
	link    string // a symbolic link's target
}

type memFail struct {
	op      int // FsOp tag
	pattern string
	err     FsErr
}

type memFs struct {
	mu            sync.Mutex
	w             *World
	nodes         map[string]*memNode // canonical path relative to the root; "" is the root
	caseSensitive bool
	fails         []memFail
	tempSeq       int
	locks         map[string]*lock // FileLock, by key (decision 0163)
}

type memView struct {
	real      Fs // `uses fs` tests with --real
	fs        *memFs
	root      string // relative to the fs root; "" for os.fs
	sub       bool
	readOnly  bool
	writeOnly bool
}

// Testing_MemFs is the in memory filesystem of a test.
type Testing_MemFs struct{ v *memView }

// XoDebug implements Debug.
func (Testing_MemFs) XoDebug(b *strings.Builder) { b.WriteString("<mem fs>") }

// FsOp tags (std.md 8.3).
const (
	opRead = iota + 1
	opWrite
	opRename
	opRemove
	opCreateDir
	opReadDir
)

func (fs *memFs) key(p string) string {
	if fs.caseSensitive {
		return p
	}
	return strings.ToLower(p)
}

// resolve maps a path given to the view to a path relative to the fs root.
func (v *memView) resolve(p string) (string, bool) {
	p = strings.ReplaceAll(p, "\\", "/")
	if v.sub {
		depth := 0
		for _, seg := range strings.Split(p, "/") {
			switch seg {
			case "..":
				depth--
			case "", ".":
			default:
				depth++
			}
			if depth < 0 {
				return "", false
			}
		}
	}
	clean := strings.TrimPrefix(path.Clean("/"+p), "/")
	full := strings.Trim(path.Join(v.root, clean), "/")
	if full == "." {
		full = ""
	}
	return full, true
}

// rel is a path relative to the fs root shown relative to the view root.
func (v *memView) rel(full string) string {
	if v.root == "" {
		return full
	}
	return strings.TrimPrefix(strings.TrimPrefix(full, v.root), "/")
}

// globMatch matches `*` within one path segment (std.md 8.3).
func globMatch(pattern, name string) bool {
	ok, err := path.Match(pattern, name)
	return err == nil && ok
}

// failed consumes a matching fail_next. Caller holds fs.mu.
func (fs *memFs) failed(op int, paths ...string) *FsErr {
	for i, f := range fs.fails {
		if f.op != op {
			continue
		}
		for _, p := range paths {
			if globMatch(fs.key(f.pattern), fs.key(p)) {
				fs.fails = append(fs.fails[:i], fs.fails[i+1:]...)
				e := f.err
				return &e
			}
		}
	}
	return nil
}

func (fs *memFs) now() Time {
	if fs.w == nil {
		return Time(time.Now().UnixNano())
	}
	return fs.w.Now()
}

func (fs *memFs) get(full string) *memNode { return fs.nodes[fs.key(full)] }

func (fs *memFs) put(full string, n *memNode) {
	n.name = full
	fs.nodes[fs.key(full)] = n
}

// mkdirs creates the directories on the way to full (inclusive when
// self). It reports the first prefix that is a file.
func (fs *memFs) mkdirs(full string, self bool) (string, bool) {
	if full == "" {
		return "", true
	}
	segs := strings.Split(full, "/")
	end := len(segs) - 1
	if self {
		end = len(segs)
	}
	for i := 1; i <= end; i++ {
		p := strings.Join(segs[:i], "/")
		n := fs.get(p)
		if n == nil {
			fs.put(p, &memNode{dir: true, mod: fs.now()})
			continue
		}
		if !n.dir {
			return p, false
		}
	}
	return "", true
}

// children lists the direct children of a directory, sorted by name.
func (fs *memFs) children(full string) []*memNode {
	var out []*memNode
	prefix := fs.key(full)
	if prefix != "" {
		prefix += "/"
	}
	for k, n := range fs.nodes {
		if k == "" || !strings.HasPrefix(k, prefix) {
			continue
		}
		if strings.Contains(k[len(prefix):], "/") {
			continue
		}
		out = append(out, n)
	}
	sortNodes(out)
	return out
}

func sortNodes(ns []*memNode) {
	for i := 1; i < len(ns); i++ {
		for j := i; j > 0 && path.Base(ns[j].name) < path.Base(ns[j-1].name); j-- {
			ns[j], ns[j-1] = ns[j-1], ns[j]
		}
	}
}

func (v *memView) info(n *memNode) FileInfo {
	return FileInfo{F_name: path.Base(n.name), F_path: v.rel(n.name), F_is_dir: n.dir, F_size: int64(len(n.data)), F_modified: n.mod}
}

// begin checks cancellation and access, resolves p, and locks the fs.
func (v *memView) begin(tk *Task, p string, write bool) (string, *FsErr) {
	tk.Yield()
	if tk != nil && tk.Canceled() {
		e := FsCanceled
		return "", &e
	}
	if write && v.readOnly || !write && v.writeOnly {
		e := fsErr(6, p)
		return "", &e
	}
	full, ok := v.resolve(p)
	if !ok {
		e := fsErr(6, p)
		return "", &e
	}
	v.fs.mu.Lock()
	return full, nil
}

func (f Testing_MemFs) M_sub(tk *Task, dir string) Fs {
	if f.v.real != nil {
		return f.v.real.M_sub(tk, dir)
	}
	full, ok := f.v.resolve(dir)
	if !ok {
		return Testing_MemFs{v: &memView{fs: f.v.fs, root: "\x00denied", sub: true, readOnly: true, writeOnly: true}}
	}
	return Testing_MemFs{v: &memView{fs: f.v.fs, root: full, sub: true, readOnly: f.v.readOnly, writeOnly: f.v.writeOnly}}
}

func (f Testing_MemFs) M_cwd(*Task) Fs { return f }

// M_cwd_path is "/": a MemFs's working directory is its root (M_cwd).
func (f Testing_MemFs) M_cwd_path(tk *Task) Result[string, FsErr] {
	if f.v.real != nil {
		return f.v.real.M_cwd_path(tk)
	}
	return Ok[string, FsErr]("/")
}

func (f Testing_MemFs) M_read_only(*Task) Fs {
	c := *f.v
	c.readOnly = true
	return Testing_MemFs{v: &c}
}

func (f Testing_MemFs) M_write_only(*Task) Fs {
	c := *f.v
	c.writeOnly = true
	return Testing_MemFs{v: &c}
}

func (f Testing_MemFs) M_case_sensitive(*Task) bool {
	f.v.fs.mu.Lock()
	defer f.v.fs.mu.Unlock()
	return f.v.fs.caseSensitive
}

func (f Testing_MemFs) M_stat(tk *Task, p string) Result[Option[FileInfo], FsErr] {
	if f.v.real != nil {
		return f.v.real.M_stat(tk, p)
	}
	full, e := f.v.begin(tk, p, false)
	if e != nil {
		return Err[Option[FileInfo]](*e)
	}
	defer f.v.fs.mu.Unlock()
	if e := f.v.fs.failed(opRead, full); e != nil {
		return Err[Option[FileInfo]](*e)
	}
	n := f.v.fs.get(full)
	if n == nil {
		return Ok[Option[FileInfo], FsErr](Option[FileInfo]{})
	}
	if n.link != "" {
		// a symbolic link: the target's information under the link's name
		t, ok := f.v.fs.follow(full)
		tn := f.v.fs.get(t)
		if !ok || tn == nil {
			return Ok[Option[FileInfo], FsErr](Option[FileInfo]{})
		}
		info := f.v.info(tn)
		info.F_name, info.F_path = path.Base(n.name), f.v.rel(n.name)
		return Ok[Option[FileInfo], FsErr](Some(info))
	}
	return Ok[Option[FileInfo], FsErr](Some(f.v.info(n)))
}

func (f Testing_MemFs) M_exists(tk *Task, p string) Result[bool, FsErr] {
	if f.v.real != nil {
		return f.v.real.M_exists(tk, p)
	}
	r := f.M_stat(tk, p)
	if !r.Ok {
		return Err[bool](r.E)
	}
	return Ok[bool, FsErr](r.V.Some)
}

func (f Testing_MemFs) M_read(tk *Task, p string) Result[Bytes, FsErr] {
	if f.v.real != nil {
		return f.v.real.M_read(tk, p)
	}
	full, e := f.v.begin(tk, p, false)
	if e != nil {
		return Err[Bytes](*e)
	}
	defer f.v.fs.mu.Unlock()
	if e := f.v.fs.failed(opRead, full); e != nil {
		return Err[Bytes](*e)
	}
	if t, ok := f.v.fs.follow(full); ok {
		full = t
	}
	n := f.v.fs.get(full)
	switch {
	case n == nil:
		return Err[Bytes](fsErr(1, p))
	case n.dir:
		return Err[Bytes](fsErr(5, p))
	}
	return Ok[Bytes, FsErr](n.data)
}

func (f Testing_MemFs) M_read_text(tk *Task, p string) Result[string, FsErr] {
	if f.v.real != nil {
		return f.v.real.M_read_text(tk, p)
	}
	r := f.M_read(tk, p)
	if !r.Ok {
		return Err[string](r.E)
	}
	return Ok[string, FsErr](string(r.V))
}

func (f Testing_MemFs) M_read_dir(tk *Task, dir string) Result[List[FileInfo], FsErr] {
	if f.v.real != nil {
		return f.v.real.M_read_dir(tk, dir)
	}
	full, e := f.v.begin(tk, dir, false)
	if e != nil {
		return Err[List[FileInfo]](*e)
	}
	defer f.v.fs.mu.Unlock()
	if e := f.v.fs.failed(opReadDir, full); e != nil {
		return Err[List[FileInfo]](*e)
	}
	n := f.v.fs.get(full)
	switch {
	case n == nil:
		return Err[List[FileInfo]](fsErr(1, dir))
	case !n.dir:
		return Err[List[FileInfo]](fsErr(4, dir))
	}
	var out []FileInfo
	for _, c := range f.v.fs.children(full) {
		out = append(out, f.v.info(c))
	}
	return Ok[List[FileInfo], FsErr](ListOf(out...))
}

func (f Testing_MemFs) M_walk(tk *Task, dir string) Result[List[FileInfo], FsErr] {
	if f.v.real != nil {
		return f.v.real.M_walk(tk, dir)
	}
	full, e := f.v.begin(tk, dir, false)
	if e != nil {
		return Err[List[FileInfo]](*e)
	}
	defer f.v.fs.mu.Unlock()
	if e := f.v.fs.failed(opReadDir, full); e != nil {
		return Err[List[FileInfo]](*e)
	}
	n := f.v.fs.get(full)
	switch {
	case n == nil:
		return Err[List[FileInfo]](fsErr(1, dir))
	case !n.dir:
		return Err[List[FileInfo]](fsErr(4, dir))
	}
	var out []FileInfo
	var walk func(d string)
	walk = func(d string) {
		for _, c := range f.v.fs.children(d) {
			out = append(out, f.v.info(c))
			if c.dir {
				walk(c.name)
			}
		}
	}
	walk(full)
	return Ok[List[FileInfo], FsErr](ListOf(out...))
}

func (f Testing_MemFs) M_write(tk *Task, p string, data Bytes) Result[Unit, FsErr] {
	if f.v.real != nil {
		return f.v.real.M_write(tk, p, data)
	}
	full, e := f.v.begin(tk, p, true)
	if e != nil {
		return Err[Unit](*e)
	}
	defer f.v.fs.mu.Unlock()
	if e := f.v.fs.failed(opWrite, full); e != nil {
		return Err[Unit](*e)
	}
	if full == "" {
		return Err[Unit](fsErr(5, p))
	}
	if bad, ok := f.v.fs.mkdirs(full, false); !ok {
		return Err[Unit](fsErr(4, f.v.rel(bad)))
	}
	if n := f.v.fs.get(full); n != nil && n.dir {
		return Err[Unit](fsErr(5, p))
	}
	f.v.fs.put(full, &memNode{data: data, mod: f.v.fs.now()})
	return Ok[Unit, FsErr](Unit{})
}

func (f Testing_MemFs) M_create_temp(tk *Task, dir, prefix string) Result[string, FsErr] {
	if f.v.real != nil {
		return f.v.real.M_create_temp(tk, dir, prefix)
	}
	full, e := f.v.begin(tk, dir, true)
	if e != nil {
		return Err[string](*e)
	}
	defer f.v.fs.mu.Unlock()
	fs := f.v.fs
	fs.tempSeq++
	name := fmt.Sprintf("%s%06d", prefix, fs.tempSeq)
	file := strings.Trim(path.Join(full, name), "/")
	if e := fs.failed(opWrite, full, file); e != nil {
		return Err[string](*e)
	}
	if n := fs.get(full); n == nil || !n.dir {
		if n == nil {
			return Err[string](fsErr(1, dir))
		}
		return Err[string](fsErr(4, dir))
	}
	fs.put(file, &memNode{mod: fs.now()})
	return Ok[string, FsErr](f.v.rel(file))
}

func (f Testing_MemFs) M_rename(tk *Task, from, to string) Result[Unit, FsErr] {
	if f.v.real != nil {
		return f.v.real.M_rename(tk, from, to)
	}
	src, e := f.v.begin(tk, from, true)
	if e != nil {
		return Err[Unit](*e)
	}
	defer f.v.fs.mu.Unlock()
	fs := f.v.fs
	dst, ok := f.v.resolve(to)
	if !ok {
		return Err[Unit](fsErr(6, to))
	}
	if e := fs.failed(opRename, src, dst); e != nil {
		return Err[Unit](*e)
	}
	n := fs.get(src)
	if n == nil {
		return Err[Unit](fsErr(1, from))
	}
	if parent := path.Dir(dst); parent != "." {
		if pn := fs.get(parent); pn == nil || !pn.dir {
			return Err[Unit](fsErr(1, to))
		}
	}
	if d := fs.get(dst); d != nil && d.dir && !n.dir {
		return Err[Unit](fsErr(5, to))
	}
	if !n.dir {
		delete(fs.nodes, fs.key(src))
		fs.put(dst, &memNode{data: n.data, mod: n.mod})
		return Ok[Unit, FsErr](Unit{})
	}
	moved := map[string]*memNode{}
	prefix := fs.key(src)
	for k, c := range fs.nodes {
		if k == prefix || strings.HasPrefix(k, prefix+"/") {
			moved[k] = c
		}
	}
	for k := range moved {
		delete(fs.nodes, k)
	}
	for _, c := range moved {
		rest := strings.TrimPrefix(c.name, src)
		fs.put(dst+rest, &memNode{dir: c.dir, data: c.data, mod: c.mod})
	}
	return Ok[Unit, FsErr](Unit{})
}

func (f Testing_MemFs) M_remove(tk *Task, p string) Result[Unit, FsErr] {
	if f.v.real != nil {
		return f.v.real.M_remove(tk, p)
	}
	full, e := f.v.begin(tk, p, true)
	if e != nil {
		return Err[Unit](*e)
	}
	defer f.v.fs.mu.Unlock()
	fs := f.v.fs
	if e := fs.failed(opRemove, full); e != nil {
		return Err[Unit](*e)
	}
	n := fs.get(full)
	if n == nil || full == "" {
		return Err[Unit](fsErr(1, p))
	}
	if n.dir && len(fs.children(full)) > 0 {
		return Err[Unit](fsErr(3, p))
	}
	delete(fs.nodes, fs.key(full))
	return Ok[Unit, FsErr](Unit{})
}

func (f Testing_MemFs) M_remove_all(tk *Task, p string) Result[Unit, FsErr] {
	if f.v.real != nil {
		return f.v.real.M_remove_all(tk, p)
	}
	full, e := f.v.begin(tk, p, true)
	if e != nil {
		return Err[Unit](*e)
	}
	defer f.v.fs.mu.Unlock()
	fs := f.v.fs
	if e := fs.failed(opRemove, full); e != nil {
		return Err[Unit](*e)
	}
	prefix := fs.key(full)
	for k := range fs.nodes {
		if k == "" {
			continue
		}
		if prefix == "" || k == prefix || strings.HasPrefix(k, prefix+"/") {
			delete(fs.nodes, k)
		}
	}
	return Ok[Unit, FsErr](Unit{})
}

func (f Testing_MemFs) M_create_dir_all(tk *Task, p string) Result[Unit, FsErr] {
	if f.v.real != nil {
		return f.v.real.M_create_dir_all(tk, p)
	}
	full, e := f.v.begin(tk, p, true)
	if e != nil {
		return Err[Unit](*e)
	}
	defer f.v.fs.mu.Unlock()
	if e := f.v.fs.failed(opCreateDir, full); e != nil {
		return Err[Unit](*e)
	}
	if bad, ok := f.v.fs.mkdirs(full, true); !ok {
		return Err[Unit](fsErr(4, f.v.rel(bad)))
	}
	return Ok[Unit, FsErr](Unit{})
}

func (f Testing_MemFs) M_set_modified(tk *Task, p string, t Time) Result[Unit, FsErr] {
	if f.v.real != nil {
		return f.v.real.M_set_modified(tk, p, t)
	}
	full, e := f.v.begin(tk, p, true)
	if e != nil {
		return Err[Unit](*e)
	}
	defer f.v.fs.mu.Unlock()
	if e := f.v.fs.failed(opWrite, full); e != nil {
		return Err[Unit](*e)
	}
	n := f.v.fs.get(full)
	if n == nil {
		return Err[Unit](fsErr(1, p))
	}
	n.mod = t
	return Ok[Unit, FsErr](Unit{})
}

// M_fail_next makes the next matching operation fail once (std.md 8.3).
func (f Testing_MemFs) M_fail_next(_ *Task, op Testing_FsOp, p string, err FsErr) {
	full, _ := f.v.resolve(p)
	if strings.Contains(p, "*") {
		full = strings.Trim(path.Join(f.v.root, strings.TrimPrefix(p, "/")), "/")
	}
	f.v.fs.mu.Lock()
	f.v.fs.fails = append(f.v.fs.fails, memFail{op: op.Tag, pattern: full, err: err})
	f.v.fs.mu.Unlock()
}

// M_set_case_sensitive switches case sensitivity.
func (f Testing_MemFs) M_set_case_sensitive(_ *Task, on bool) {
	fs := f.v.fs
	fs.mu.Lock()
	defer fs.mu.Unlock()
	if fs.caseSensitive == on {
		return
	}
	old := fs.nodes
	fs.caseSensitive = on
	fs.nodes = map[string]*memNode{}
	for _, n := range old {
		fs.nodes[fs.key(n.name)] = n
	}
}

// M_files returns every file by path, sorted by path.
func (f Testing_MemFs) M_files(*Task) Map[string, Bytes] {
	fs := f.v.fs
	fs.mu.Lock()
	defer fs.mu.Unlock()
	var names []string
	byName := map[string]*memNode{}
	for _, n := range fs.nodes {
		if !n.dir {
			names = append(names, n.name)
			byName[n.name] = n
		}
	}
	sortStrings(names)
	var m Map[string, Bytes]
	for _, name := range names {
		if rel := f.v.rel(name); f.v.root == "" || strings.HasPrefix(name, f.v.root+"/") {
			Map_put(nil, &m, rel, byName[name].data)
		}
	}
	return m
}

// shellQuote quotes s for a POSIX shell when it needs quoting.
func shellQuote(s string) string {
	if s != "" && strings.IndexFunc(s, func(r rune) bool {
		return !(r == '-' || r == '_' || r == '.' || r == '/' || r == ':' || r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z')
	}) < 0 {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
