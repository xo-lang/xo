package rt

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Record and replay (core 11.3) and the effect traces of crash bundles
// (core 11.2).
//
// Every effect goes through a capability method of caps.go or http.go,
// which runs its I/O through fx: recording logs the call (operation,
// encoded arguments, encoded result, Xo location) and replay returns the
// recorded result without doing the I/O. Interleavings come from the
// deterministic scheduler (sched.go): a recording runs every task on it
// and logs each choice (a pick, by thread id) and each event that comes
// from outside the program (a timer firing, a signal, an HTTP request
// arriving, a server ending), and replay makes the same choices and
// delivers the same events at the same points. Thread ids are given in
// creation order and threads are created only by the running thread or by
// an event, so they are the same in both runs.
//
// Modes, chosen by RunMain from the environment:
//
//   - XO_RECORD=<file>: full recording, written to <file> when the program
//     ends (`xo run --record`).
//   - XO_RING=N: per request ring buffers of the last N effects (main and
//     its tasks are request 0); no scheduler. Attached to crash bundles.
//   - XO_REPLAY=<file.xotrace or file.xocrash>: replay (`xo replay`).
//
// The trace file is JSON lines: a header, the secrets, the effects, the
// HTTP requests, the scheduler stream (`{"s": "1 1*4 2 t3 r1"}`: picks by
// thread id with run lengths, t<id> timer, g signal, r<k> request, e<l>
// and x<l> server ends, c<n> a call of Go callback n admitted, gosched.go),
// and the end (exit status, fault or error).

const (
	fxOff = iota
	fxRecord
	fxRing
	fxReplay
)

// ReplayDiverged is the exit status of a replay that diverged from its
// trace (or could not start).
const ReplayDiverged = 3

var (
	fxMode int // set once by fxSetup, before any task runs
	rec    *recorder
	rp     *replayer

	fxTimerSeq    atomic.Int64 // record and replay: timer ids in creation order
	fxListenerSeq atomic.Int64 // listener ids
	fxReqSeq      atomic.Int64 // request ids (ring buffers)

	buildInfo json.RawMessage // SetBuildInfo
)

// SetBuildInfo is called by the generated program with the build id, the
// source hashes, the source directory, and the build options (JSON), for
// crash bundles and traces.
func SetBuildInfo(s string) { buildInfo = json.RawMessage(s) }

// fxEntry is one recorded effect.
type fxEntry struct {
	N  int64  `json:"n"`
	T  int64  `json:"t"`
	Q  int64  `json:"q,omitempty"`
	Op string `json:"op"`
	A  []any  `json:"a,omitempty"`
	R  any    `json:"r,omitempty"`
	At string `json:"at,omitempty"`
}

// reqEntry is one HTTP request that reached a server, with its response.
type reqEntry struct {
	Req  int64 `json:"req"`
	L    int64 `json:"l"`
	R    any   `json:"r"`
	Resp any   `json:"resp,omitempty"`
}

// ring is the effect buffer of one request (XO_RING).
type ring struct {
	es      []*fxEntry
	start   int
	dropped int64
	req     *reqEntry
}

func (r *ring) push(e *fxEntry, n int) {
	if len(r.es) < n {
		r.es = append(r.es, e)
		return
	}
	r.es[r.start] = e
	r.start = (r.start + 1) % n
	r.dropped++
}

func (r *ring) list() []*fxEntry {
	return append(append([]*fxEntry(nil), r.es[r.start:]...), r.es[:r.start]...)
}

// recorder holds what a recording or the ring buffers logged.
type recorder struct {
	mu       sync.Mutex
	full     bool
	path     string
	ringN    int
	seed     int64
	n        int64
	entries  []*fxEntry
	rings    map[int64]*ring
	reqs     map[int64]*reqEntry
	toks     []string
	lastPick int64
	pickRun  int
	secrets  []string
	picks    int64
	written  bool
}

// add logs one effect.
func (r *recorder) add(tk *Task, op string, args []any, res any) {
	e := &fxEntry{Op: op, At: xoLocation(2), R: fxEncode(res)}
	if tk != nil && tk.th != nil {
		e.T, e.Q = tk.th.id, tk.th.req
	}
	for _, a := range args {
		e.A = append(e.A, fxEncode(a))
	}
	r.mu.Lock()
	r.n++
	e.N = r.n
	if r.full {
		r.entries = append(r.entries, e)
	} else {
		rg := r.rings[e.Q]
		if rg == nil {
			rg = &ring{}
			r.rings[e.Q] = rg
		}
		rg.push(e, r.ringN)
	}
	r.mu.Unlock()
}

// newReq logs a request that reached the server of listener l and
// returns its id.
func (r *recorder) newReq(l int64, req Http_Request) int64 {
	k := fxReqSeq.Add(1)
	e := &reqEntry{Req: k, L: l, R: fxEncode(req)}
	r.mu.Lock()
	if r.full {
		r.reqs[k] = e
	} else {
		r.rings[k] = &ring{req: e}
	}
	r.mu.Unlock()
	return k
}

// reqResp logs the response to request k.
func (r *recorder) reqResp(k int64, resp Http_Response) {
	enc := fxEncode(resp)
	r.mu.Lock()
	if e := r.reqs[k]; e != nil {
		e.Resp = enc
	}
	if rg := r.rings[k]; rg != nil && rg.req != nil {
		rg.req.Resp = enc
	}
	r.mu.Unlock()
}

// endReq drops the ring buffer of a finished request.
func (r *recorder) endReq(k int64) {
	if r.full || k == 0 {
		return
	}
	r.mu.Lock()
	delete(r.rings, k)
	r.mu.Unlock()
}

// pick logs a scheduling decision (the scheduler's lock is held).
func (r *recorder) pick(id int64) {
	r.mu.Lock()
	r.picks++
	if r.pickRun > 0 && id == r.lastPick {
		r.pickRun++
	} else {
		r.flushPickLocked()
		r.lastPick, r.pickRun = id, 1
	}
	r.mu.Unlock()
}

func (r *recorder) flushPickLocked() {
	switch {
	case r.pickRun == 1:
		r.toks = append(r.toks, strconv.FormatInt(r.lastPick, 10))
	case r.pickRun > 1:
		r.toks = append(r.toks, strconv.FormatInt(r.lastPick, 10)+"*"+strconv.Itoa(r.pickRun))
	}
	r.pickRun = 0
}

// tok logs an event of the scheduler stream.
func (r *recorder) tok(t string) {
	r.mu.Lock()
	r.flushPickLocked()
	r.toks = append(r.toks, t)
	r.mu.Unlock()
}

// fxSecret records the value of a Secret[T] so traces redact it.
func fxSecret(v any) {
	var s string
	switch x := v.(type) {
	case string:
		s = x
	case Bytes:
		s = string(x)
	default:
		return
	}
	if s == "" || rec == nil {
		return
	}
	rec.mu.Lock()
	defer rec.mu.Unlock()
	for _, o := range rec.secrets {
		if o == s {
			return
		}
	}
	rec.secrets = append(rec.secrets, s)
}

// jsonInner is the JSON string literal of s without its quotes.
func jsonInner(s string) string {
	t := fxText(s)
	return t[1 : len(t)-1]
}

// lines renders the trace. focus is the failing request of a crash
// bundle in ring mode (its buffer and main's are kept), or -1.
func (r *recorder) lines(end map[string]any, complete bool, focus int64) []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []string
	add := func(v any) {
		out = append(out, fxText(v))
	}
	mode := "ring"
	if r.full {
		mode = "full"
	}
	args := []string{}
	if len(os.Args) > 1 {
		args = os.Args[1:]
	}
	hdr := map[string]any{"xotrace": 1, "mode": mode, "complete": complete, "seed": r.seed, "args": args, "target": runtime.GOOS + "/" + runtime.GOARCH}
	if buildInfo != nil {
		hdr["build"] = buildInfo
	}
	var entries []*fxEntry
	var reqs []*reqEntry
	if r.full {
		entries = r.entries
		for _, e := range r.reqs {
			reqs = append(reqs, e)
		}
	} else {
		dropped := map[string]int64{}
		for _, q := range []int64{0, focus} {
			rg := r.rings[q]
			if rg == nil || (q == focus && q <= 0) {
				continue
			}
			entries = append(entries, rg.list()...)
			if rg.req != nil {
				reqs = append(reqs, rg.req)
			}
			if rg.dropped > 0 {
				dropped[strconv.FormatInt(q, 10)] = rg.dropped
			}
		}
		sort.Slice(entries, func(i, j int) bool { return entries[i].N < entries[j].N })
		hdr["ring"] = r.ringN
		hdr["dropped"] = dropped
	}
	sort.Slice(reqs, func(i, j int) bool { return reqs[i].Req < reqs[j].Req })
	add(hdr)
	// secrets, with where their value came from when that is an env var
	secrets := append([]string(nil), r.secrets...)
	for i, s := range secrets {
		line := map[string]any{"secret": i + 1}
		for _, e := range entries {
			if e.Op != "env.get" || len(e.A) != 1 {
				continue
			}
			if m, ok := e.R.(map[string]any); ok && m["V"] == s {
				if name, ok := e.A[0].(string); ok {
					line["src"] = "env:" + name
					line["at"] = e.At
					break
				}
			}
		}
		add(line)
	}
	for _, e := range entries {
		add(e)
	}
	for _, e := range reqs {
		add(e)
	}
	if r.full {
		toks := append([]string(nil), r.toks...)
		switch {
		case r.pickRun == 1:
			toks = append(toks, strconv.FormatInt(r.lastPick, 10))
		case r.pickRun > 1:
			toks = append(toks, strconv.FormatInt(r.lastPick, 10)+"*"+strconv.Itoa(r.pickRun))
		}
		for len(toks) > 0 {
			n := min(len(toks), 512)
			add(map[string]any{"s": strings.Join(toks[:n], " ")})
			toks = toks[n:]
		}
	}
	add(map[string]any{"end": end})
	// redact secrets, longest first
	order := make([]int, len(secrets))
	for i := range order {
		order[i] = i
	}
	sort.Slice(order, func(a, b int) bool { return len(secrets[order[a]]) > len(secrets[order[b]]) })
	for i := 1; i < len(out); i++ {
		for _, k := range order {
			if in := jsonInner(secrets[k]); in != "" {
				out[i] = strings.ReplaceAll(out[i], in, "<redacted:"+strconv.Itoa(k+1)+">")
			}
		}
	}
	if len(secrets) > 0 {
		for _, k := range order {
			if in := jsonInner(secrets[k]); in != "" {
				out[0] = strings.ReplaceAll(out[0], in, "<redacted:"+strconv.Itoa(k+1)+">")
			}
		}
	}
	return out
}

// write writes the trace file of a recording.
func (r *recorder) write(end map[string]any) {
	r.mu.Lock()
	done := r.written
	r.written = true
	r.mu.Unlock()
	if done || r.path == "" {
		return
	}
	data := strings.Join(r.lines(end, true, -1), "\n") + "\n"
	tmp := r.path + ".tmp"
	if err := os.WriteFile(tmp, []byte(data), 0o644); err != nil {
		stderrWrite("xo record: cannot write the trace: " + err.Error() + "\n")
		return
	}
	if err := os.Rename(tmp, r.path); err != nil {
		stderrWrite("xo record: cannot write the trace: " + err.Error() + "\n")
	}
}

// fx runs an effect. real does it; io marks an effect whose real form
// leaves the scheduler while it waits on the outside world (tk.leave and
// tk.enter), which a replay repeats so the interleaving is the same. args
// is called only when recording or replaying.
func fx[R any](tk *Task, op string, io bool, args func() []any, real func() R) R {
	switch fxMode {
	case fxOff:
		return real()
	case fxReplay:
		var a []any
		if args != nil {
			a = args()
		}
		e := rp.next(tk, op, a)
		if io {
			tk.leave()
			tk.enter()
		}
		return fxResult[R](e)
	}
	r := real()
	var a []any
	if args != nil {
		a = args()
	}
	rec.add(tk, op, a, r)
	return r
}

// fxResult decodes the recorded result of an effect.
func fxResult[R any](e *fxEntry) R {
	var r R
	v, err := fxDecode(e.R, reflect.TypeOf(&r).Elem())
	if err != nil {
		rp.fail(fmt.Sprintf("the recorded result of %s at %s cannot be decoded: %v", e.Op, e.At, err))
	}
	return v.Interface().(R)
}

// fxMono is the monotonic clock of the runtime itself (the grace period
// of a stop): an effect, so a replay takes the same branches.
func fxMono() int64 {
	if fxMode == fxOff {
		return monoNow()
	}
	return fx(nil, "rt.mono", false, nil, monoNow)
}

// fxNowTime is the wall clock of log records.
func fxNowTime(tk *Task) time.Time {
	if fxMode == fxOff {
		return time.Now()
	}
	return time.Unix(0, fx(tk, "log.time", false, nil, func() int64 { return time.Now().UnixNano() }))
}

// ---------------------------------------------------------------------------
// Scheduler events and timers

// schedEv is an event from outside the program, delivered by the
// recording scheduler at its next decision: tok runs with the scheduler
// locked and names the event in the trace ("" drops it), run runs with
// the scheduler unlocked while no thread holds the turn.
type schedEv struct {
	tok func() string
	run func()
}

// rtimer is a timer of a recording or a replay.
type rtimer struct {
	id      int64
	f       func()
	stopped atomic.Bool
}

// realAfter calls f after d of real time and returns a function that
// stops the timer. Under a recording the firing is an event of the
// scheduler; under a replay it happens when the trace says so.
func realAfter(d Duration, f func()) func() {
	s := realSched
	if s == nil || (s.rec == nil && s.rp == nil) {
		t := time.AfterFunc(time.Duration(max(d, 0)), f)
		return func() { t.Stop() }
	}
	tm := &rtimer{id: fxTimerSeq.Add(1), f: f}
	if s.rp != nil {
		s.rp.timers.Store(tm.id, tm)
		return func() { tm.stopped.Store(true) }
	}
	t := time.AfterFunc(time.Duration(max(d, 0)), func() {
		s.post(schedEv{
			tok: func() string {
				if tm.stopped.Load() {
					return ""
				}
				return "t" + strconv.FormatInt(tm.id, 10)
			},
			run: f,
		})
	})
	return func() {
		tm.stopped.Store(true)
		t.Stop()
	}
}

// post delivers an event of a recording at the next decision.
func (s *sched) post(ev schedEv) {
	s.mu.Lock()
	s.inbox = append(s.inbox, ev)
	if s.cur == nil && !s.picking {
		s.pickLocked()
	}
	s.mu.Unlock()
}

// drainLocked runs the pending events of a recording.
func (s *sched) drainLocked() {
	for len(s.inbox) > 0 {
		ev := s.inbox[0]
		s.inbox = s.inbox[1:]
		t := ev.tok()
		if t == "" {
			continue
		}
		s.rec.tok(t)
		s.mu.Unlock()
		ev.run()
		s.mu.Lock()
	}
}

// replayLocked makes the trace's next decision: it fires the events due
// and hands the turn to the recorded thread, or waits (returns true with
// no thread running) until that thread is ready. It returns false when
// the trace has no more decisions.
func (s *sched) replayLocked() bool {
	p := s.rp
	for {
		if p.ti >= len(p.toks) {
			return false
		}
		tok := p.toks[p.ti]
		if id, n, ok := pickTok(tok); ok {
			for i, th := range s.ready {
				if th.id != id {
					continue
				}
				p.runUsed++
				if p.runUsed >= n {
					p.ti++
					p.runUsed = 0
				}
				p.decisions.Add(1)
				s.ready = append(s.ready[:i], s.ready[i+1:]...)
				th.st = stRunning
				s.moves++
				s.cur = th
				th.turn <- struct{}{}
				return true
			}
			return true
		}
		if tok[0] == 'c' {
			// a Go callback call (gosched.go): wait until it arrives
			seq, err := strconv.ParseInt(tok[1:], 10, 64)
			if err != nil {
				p.fail("bad scheduler event " + strconv.Quote(tok))
			}
			if !s.replayArrivalLocked(seq) {
				return true
			}
			p.ti++
			p.runUsed = 0
			p.decisions.Add(1)
			continue
		}
		p.ti++
		p.runUsed = 0
		p.decisions.Add(1)
		s.mu.Unlock()
		p.fire(tok)
		s.mu.Lock()
	}
}

// pickTok parses a pick token: `5` or `5*12`.
func pickTok(t string) (id int64, n int, ok bool) {
	if t == "" || t[0] < '0' || t[0] > '9' {
		return 0, 0, false
	}
	a, b, rep := strings.Cut(t, "*")
	id, _ = strconv.ParseInt(a, 10, 64)
	n = 1
	if rep {
		n, _ = strconv.Atoi(b)
	}
	return id, n, true
}

// ---------------------------------------------------------------------------
// Replay

// rpServer is a server of a replay: its requests come from the trace.
type rpServer struct {
	router  Http_Router
	tk      *Task
	reqRoot *cnode
	ended   chan struct{}
	once    sync.Once
	wg      sync.WaitGroup
}

func (s *rpServer) end() { s.once.Do(func() { close(s.ended) }) }

// replayer replays a trace.
type replayer struct {
	mu       sync.Mutex
	path     string
	full     bool // effects per thread and a scheduler stream; else per request (ring)
	complete bool // the trace ends where the program ended
	bundle   bool
	seed     int64
	queues   map[int64][]*fxEntry
	pos      map[int64]int
	used     int
	reqs     map[int64]*reqEntry
	end      map[string]any
	servers  map[int64]*rpServer
	timers   sync.Map // id -> *rtimer
	stop     *stopState

	// the scheduler stream, read with the scheduler's lock held
	toks      []string
	ti        int
	runUsed   int
	decisions atomic.Int64

	finished atomic.Bool
}

// fxSetup reads XO_RECORD, XO_RING, and XO_REPLAY. They are removed from
// the environment so the program (and the processes it starts) do not
// see them.
func fxSetup() {
	recPath, ringV, repPath := os.Getenv("XO_RECORD"), os.Getenv("XO_RING"), os.Getenv("XO_REPLAY")
	os.Unsetenv("XO_RECORD")
	os.Unsetenv("XO_RING")
	os.Unsetenv("XO_REPLAY")
	switch {
	case repPath != "":
		p, err := loadReplay(repPath)
		if err != nil {
			stderrWrite("xo replay: " + err.Error() + "\n")
			os.Exit(ReplayDiverged)
		}
		rp = p
		fxMode = fxReplay
	case recPath != "":
		rec = &recorder{full: true, path: recPath, reqs: map[int64]*reqEntry{}, rings: map[int64]*ring{}}
		fxMode = fxRecord
	case ringV != "":
		n, err := strconv.Atoi(ringV)
		if err != nil || n <= 0 {
			stderrWrite("xo: XO_RING must be a positive number of effects, got " + strconv.Quote(ringV) + "\n")
			return
		}
		rec = &recorder{ringN: n, rings: map[int64]*ring{}, reqs: map[int64]*reqEntry{}}
		fxMode = fxRing
	}
}

// loadReplay reads a trace or a crash bundle.
func loadReplay(path string) (*replayer, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	p := &replayer{path: path, queues: map[int64][]*fxEntry{}, pos: map[int64]int{}, reqs: map[int64]*reqEntry{}, servers: map[int64]*rpServer{}}
	var lines []string
	var bundle map[string]json.RawMessage
	if json.Unmarshal(data, &bundle) == nil && bundle["kind"] != nil {
		p.bundle = true
		var raw []json.RawMessage
		if bundle["trace"] == nil || json.Unmarshal(bundle["trace"], &raw) != nil || len(raw) == 0 {
			return nil, fmt.Errorf("%s has no effect trace: the run was not recorded (xo run --record) and kept no ring buffer (XO_RING=N)", path)
		}
		for _, l := range raw {
			lines = append(lines, string(l))
		}
	} else {
		sc := bufio.NewScanner(bytes.NewReader(data))
		sc.Buffer(nil, 1<<30)
		for sc.Scan() {
			if l := strings.TrimSpace(sc.Text()); l != "" {
				lines = append(lines, l)
			}
		}
	}
	if len(lines) == 0 || !strings.Contains(lines[0], `"xotrace"`) {
		return nil, fmt.Errorf("%s is not an Xo trace or crash bundle", path)
	}
	// secrets: put the values back from the environment
	for _, l := range lines {
		var s struct {
			Secret int    `json:"secret"`
			Src    string `json:"src"`
			At     string `json:"at"`
			Op     string `json:"op"`
		}
		if json.Unmarshal([]byte(l), &s) != nil || s.Secret == 0 || s.Op != "" {
			continue
		}
		key := "XO_REPLAY_SECRET_" + strconv.Itoa(s.Secret)
		v, ok := os.LookupEnv(key)
		name, fromEnv := strings.CutPrefix(s.Src, "env:")
		if !ok && fromEnv {
			v, ok = os.LookupEnv(name)
		}
		if !ok {
			where := "a value the program wrapped in Secret[T]"
			hint := "set " + key
			if fromEnv {
				where = fmt.Sprintf("the value of env.get(%q)", name)
				if s.At != "" {
					where += " at " + s.At
				}
				hint = "set " + name + " (or " + key + ")"
			}
			return nil, fmt.Errorf("the trace redacts secret %d (%s); %s to replay it", s.Secret, where, hint)
		}
		ph := "<redacted:" + strconv.Itoa(s.Secret) + ">"
		in := jsonInner(v)
		for i := range lines {
			lines[i] = strings.ReplaceAll(lines[i], ph, in)
		}
	}
	for i, l := range lines {
		dec := json.NewDecoder(strings.NewReader(l))
		dec.UseNumber()
		var m map[string]json.RawMessage
		if err := json.Unmarshal([]byte(l), &m); err != nil {
			return nil, fmt.Errorf("%s line %d: %v", path, i+1, err)
		}
		switch {
		case m["xotrace"] != nil:
			var h struct {
				Mode     string           `json:"mode"`
				Complete bool             `json:"complete"`
				Seed     int64            `json:"seed"`
				Ring     int              `json:"ring"`
				Dropped  map[string]int64 `json:"dropped"`
			}
			if err := json.Unmarshal([]byte(l), &h); err != nil {
				return nil, fmt.Errorf("%s header: %v", path, err)
			}
			p.full, p.complete, p.seed = h.Mode == "full", h.Complete, h.Seed
			for q, n := range h.Dropped {
				who := "request " + q
				if q == "0" {
					who = "main"
				}
				return nil, fmt.Errorf("the ring buffer of %s dropped its first %d effects (XO_RING=%d keeps the last %d); a replay needs all of them: raise XO_RING or record the run with xo run --record", who, n, h.Ring, h.Ring)
			}
		case m["op"] != nil:
			var e fxEntry
			if err := dec.Decode(&e); err != nil {
				return nil, fmt.Errorf("%s line %d: %v", path, i+1, err)
			}
			key := e.Q
			if p.full {
				key = e.T
			}
			p.queues[key] = append(p.queues[key], &e)
		case m["req"] != nil:
			var e reqEntry
			if err := dec.Decode(&e); err != nil {
				return nil, fmt.Errorf("%s line %d: %v", path, i+1, err)
			}
			p.reqs[e.Req] = &e
		case m["s"] != nil:
			var s struct {
				S string `json:"s"`
			}
			json.Unmarshal([]byte(l), &s)
			p.toks = append(p.toks, strings.Fields(s.S)...)
		case m["end"] != nil:
			var s struct {
				End map[string]any `json:"end"`
			}
			if err := dec.Decode(&s); err != nil {
				return nil, fmt.Errorf("%s line %d: %v", path, i+1, err)
			}
			p.end = s.End
		}
	}
	for _, q := range p.queues {
		sort.SliceStable(q, func(i, j int) bool { return q[i].N < q[j].N })
	}
	return p, nil
}

// who names the effect stream of a key for messages.
func (p *replayer) who(tk *Task, key int64) string {
	if !p.full {
		if key == 0 {
			return "main (request 0)"
		}
		return fmt.Sprintf("request %d", key)
	}
	if tk != nil && tk.th != nil && tk.th.id == key {
		return tk.th.label()
	}
	if key == 0 {
		return "the runtime"
	}
	return fmt.Sprintf("thread %d", key)
}

// next returns the recorded effect that answers this call, or ends the
// replay with a divergence.
func (p *replayer) next(tk *Task, op string, args []any) *fxEntry {
	key := int64(0)
	if tk != nil && tk.th != nil {
		key = tk.th.id
		if !p.full {
			key = tk.th.req
		}
	}
	at := xoLocation(2)
	call := make([]any, len(args))
	for i, a := range args {
		call[i] = fxEncode(a)
	}
	p.mu.Lock()
	q, i := p.queues[key], p.pos[key]
	if i >= len(q) {
		p.mu.Unlock()
		if !p.complete {
			p.prefixEnd(fmt.Sprintf("the trace ends before %s calls %s at %s", p.who(tk, key), fxCall(op, call), at))
		}
		p.diverge(fmt.Sprintf("%s calls %s at %s, but the trace has no more effects for it (it recorded %d)", p.who(tk, key), fxCall(op, call), at, len(q)))
	}
	e := q[i]
	if e.Op != op || (len(e.A) > 0 || len(call) > 0) && fxText(e.A) != fxText(call) {
		p.mu.Unlock()
		p.diverge(fmt.Sprintf("effect %d of %s differs:\n  the program calls  %s at %s\n  the trace recorded %s at %s", i+1, p.who(tk, key), fxCall(op, call), at, fxCall(e.Op, e.A), e.At))
	}
	p.pos[key] = i + 1
	p.used++
	p.mu.Unlock()
	return e
}

// fire delivers an event of the scheduler stream (the scheduler's lock
// is not held; no thread runs).
func (p *replayer) fire(tok string) {
	if tok == "" {
		return
	}
	n, err := strconv.ParseInt(tok[1:], 10, 64)
	if err != nil && tok != "g" {
		p.fail("bad scheduler event " + strconv.Quote(tok))
	}
	switch tok[0] {
	case 't':
		v, ok := p.timers.LoadAndDelete(n)
		if !ok {
			p.diverge(fmt.Sprintf("the trace fires timer %d at decision %d, but the program has not started it", n, p.decisions.Load()))
		}
		if tm := v.(*rtimer); !tm.stopped.Load() {
			tm.f()
		}
	case 'g':
		if p.stop != nil {
			p.stop.signal()
		}
	case 'r':
		p.fireRequest(n)
	case 'e', 'x':
		p.mu.Lock()
		srv := p.servers[n]
		p.mu.Unlock()
		if srv == nil {
			p.diverge(fmt.Sprintf("the trace ends the server of listener %d, but the program does not serve it", n))
		}
		if tok[0] == 'x' {
			srv.reqRoot.cancel()
		}
		srv.end()
	default:
		p.fail("bad scheduler event " + strconv.Quote(tok))
	}
}

// addServer registers a server of the replay.
func (p *replayer) addServer(l int64, s *rpServer) {
	p.mu.Lock()
	p.servers[l] = s
	p.mu.Unlock()
}

// fireRequest delivers recorded request k to its server.
func (p *replayer) fireRequest(k int64) {
	p.mu.Lock()
	re := p.reqs[k]
	var srv *rpServer
	if re != nil {
		srv = p.servers[re.L]
	}
	p.mu.Unlock()
	if re == nil {
		p.fail(fmt.Sprintf("the trace delivers request %d but does not record it", k))
	}
	if srv == nil {
		p.diverge(fmt.Sprintf("the trace delivers request %d to listener %d, but the program does not serve it", k, re.L))
	}
	v, err := fxDecode(re.R, reflect.TypeFor[Http_Request]())
	if err != nil {
		p.fail(fmt.Sprintf("request %d cannot be decoded: %v", k, err))
	}
	req := v.Interface().(Http_Request)
	stk := srv.tk
	reqTask := &Task{stop: stk.stop, th: stk.th.newChild("request")}
	th := reqTask.th
	th.req = k
	node := newNodeOf(srv.reqRoot, nil, nodeRequest, reqTask)
	reqTask.node = node
	if th.mon != nil {
		th.mon.started()
	}
	srv.wg.Add(1)
	if sch := th.sch; sch != nil {
		sch.start(th)
	}
	go func() {
		defer srv.wg.Done()
		defer node.detach()
		if sch := th.sch; sch != nil {
			<-th.turn
		}
		defer endThread(th)
		resp := srv.router.call(reqTask, req).collect(reqTask)
		p.checkResp(reqTask, k, re, resp)
	}()
}

// checkResp compares a replayed response with the recorded one.
func (p *replayer) checkResp(tk *Task, k int64, re *reqEntry, resp Http_Response) {
	if re.Resp == nil {
		return
	}
	got := fxText(fxEncode(resp))
	if want := fxText(re.Resp); got != want {
		p.diverge(fmt.Sprintf("the response to request %d differs:\n  replayed %s\n  recorded %s", k, clip(got), clip(want)))
	}
}

func clip(s string) string {
	if len(s) > 300 {
		return s[:297] + "..."
	}
	return s
}

// endThread marks a request thread as finished.
func endThread(th *thread) {
	th.mu.Lock()
	th.done = true
	th.mu.Unlock()
	if th.mon != nil {
		th.mon.ended()
	}
	if sch := th.sch; sch != nil {
		sch.end(th)
	}
}

// replayServe is srv.serve() under a replay: the requests come from the
// trace, and the serve ends when the trace says it did.
func replayServe(tk *Task, s Http_Server) Result[Unit, Http_HttpErr] {
	p := rp
	srv := &rpServer{router: s.r, tk: tk, reqRoot: newNodeOf(nil, tk.w, nodeServer, nil), ended: make(chan struct{})}
	extraRoots.Store(srv.reqRoot, true)
	defer extraRoots.Delete(srv.reqRoot)
	p.addServer(s.l.id, srv)
	tk.leave()
	if p.full {
		<-srv.ended
	} else {
		// ring buffers: deliver the failing request of the bundle
		var focus *reqEntry
		p.mu.Lock()
		for _, re := range p.reqs {
			if re.L == s.l.id {
				focus = re
			}
		}
		p.mu.Unlock()
		if focus != nil {
			p.fireRequest(focus.Req)
		}
		srv.wg.Wait()
	}
	tk.enter()
	e := p.next(tk, "http.serve", []any{s.l.id})
	return fxResult[Result[Unit, Http_HttpErr]](e)
}

// runOutcome is the outcome of a run: its exit status and fault or error.
func runOutcome(code int, c *crashInfo) map[string]any {
	m := map[string]any{"code": code}
	if c == nil {
		return m
	}
	if c.fault != nil {
		m["fault"] = c.fault.Msg
		if c.fault.Code != "" {
			m["fcode"] = c.fault.Code
		}
		if c.fault.Loc != "" {
			m["loc"] = c.fault.Loc
		}
	}
	if c.errText != "" {
		m["error"] = c.errText
	}
	if c.isolated {
		m["isolated"] = true
		m["req"] = c.req
	}
	return m
}

// describe renders an outcome for messages.
func describeEnd(m map[string]any) string {
	var parts []string
	if f, ok := m["fault"].(string); ok {
		s := "fault: "
		if c, ok := m["fcode"].(string); ok {
			s += c + ": "
		}
		s += f
		if l, ok := m["loc"].(string); ok {
			s += " at " + l
		}
		if iso, _ := m["isolated"].(bool); iso {
			return s + " (isolated, in the handler of request " + fmt.Sprint(m["req"]) + ")"
		}
		parts = append(parts, s)
	}
	if e, ok := m["error"].(string); ok {
		parts = append(parts, "error: "+strings.SplitN(e, "\n", 2)[0])
	}
	parts = append(parts, "exit status "+fmt.Sprint(m["code"]))
	return strings.Join(parts, ", ")
}

// sameFailure compares the fault or error of two outcomes.
func sameFailure(a, b map[string]any) bool {
	for _, k := range []string{"fault", "fcode", "loc", "error"} {
		if fmt.Sprint(a[k]) != fmt.Sprint(b[k]) {
			return false
		}
	}
	return true
}

// finish checks the end of a replayed run against the trace.
func (p *replayer) finish(code int, c *crashInfo) {
	if !p.finished.CompareAndSwap(false, true) {
		return
	}
	got := runOutcome(code, c)
	if p.complete {
		p.mu.Lock()
		var keys []int64
		for k, q := range p.queues {
			if p.pos[k] < len(q) {
				keys = append(keys, k)
			}
		}
		sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
		p.mu.Unlock()
		if len(keys) > 0 {
			k := keys[0]
			e := p.queues[k][p.pos[k]]
			p.divergeNow(fmt.Sprintf("the program ended (%s), but %s has %d more recorded effects; the next is %s at %s", describeEnd(got), p.who(nil, k), len(p.queues[k])-p.pos[k], fxCall(e.Op, e.A), e.At))
		}
	}
	if p.end != nil {
		want := p.end
		if iso, _ := want["isolated"].(bool); iso {
			p.divergeNow(fmt.Sprintf("the program ended (%s) without the recorded %s", describeEnd(got), describeEnd(want)))
		}
		if fmt.Sprint(want["code"]) != fmt.Sprint(code) || !sameFailure(want, got) {
			p.divergeNow(fmt.Sprintf("the program ended with %s; the recording ended with %s", describeEnd(got), describeEnd(want)))
		}
	}
	what := "identical run"
	if p.bundle {
		what = "reproduced the crash"
	}
	stderrWrite(fmt.Sprintf("xo replay: %s: %s (%d effects, %d scheduling decisions)\n", what, describeEnd(got), p.used, p.decisions.Load()))
}

// isolatedFault checks a fault isolated in a request handler against the
// crash of a bundle: the replay is done when it is the one.
func (p *replayer) isolatedFault(tk *Task, f *FaultErr) {
	if p.end == nil {
		return
	}
	if iso, _ := p.end["isolated"].(bool); !iso {
		return
	}
	got := runOutcome(70, &crashInfo{fault: f, isolated: true, req: tk.th.req})
	if !sameFailure(p.end, got) {
		return
	}
	if !p.finished.CompareAndSwap(false, true) {
		return
	}
	stderrWrite(fmt.Sprintf("xo replay: reproduced the crash: %s (%d effects, %d scheduling decisions)\n", describeEnd(got), p.used, p.decisions.Load()))
	os.Exit(0)
}

// prefixEnd ends the replay of a trace that stops before the program did
// (a crash bundle written while the program went on).
func (p *replayer) prefixEnd(why string) {
	if !p.finished.CompareAndSwap(false, true) {
		return
	}
	want := "the recorded outcome"
	if p.end != nil {
		want = describeEnd(p.end)
	}
	stderrWrite(fmt.Sprintf("xo replay: %s without reproducing %s\n", why, want))
	os.Exit(ReplayDiverged)
}

// diverge reports the first divergence and exits.
func (p *replayer) diverge(msg string) {
	p.finished.Store(true)
	p.divergeNow(msg)
}

func (p *replayer) divergeNow(msg string) {
	stderrWrite(fmt.Sprintf("xo replay: diverged from %s after %d effects and %d scheduling decisions: %s\n", filepath.Base(p.path), p.used, p.decisions.Load(), msg))
	os.Exit(ReplayDiverged)
}

// fail reports a damaged trace.
func (p *replayer) fail(msg string) {
	stderrWrite("xo replay: " + msg + "\n")
	os.Exit(ReplayDiverged)
}

// watch reports a replay that stopped making progress: the thread the
// trace runs next never became ready.
func (p *replayer) watch(s *sched) {
	last, lastUsed, idle := int64(-1), -1, 0
	for {
		time.Sleep(250 * time.Millisecond)
		d := p.decisions.Load()
		p.mu.Lock()
		used := p.used
		p.mu.Unlock()
		s.mu.Lock()
		running := s.cur != nil
		next := ""
		if p.ti < len(p.toks) {
			next = p.toks[p.ti]
		}
		var want *thread
		if id, _, ok := pickTok(next); ok {
			for _, th := range s.threads {
				if th.id == id {
					want = th
				}
			}
		}
		s.mu.Unlock()
		if d != last || used != lastUsed || running {
			last, lastUsed, idle = d, used, 0
			continue
		}
		idle++
		if idle < 12 {
			continue
		}
		if next == "" {
			if !p.complete {
				p.prefixEnd("the trace ends here")
			}
			p.diverge("the program waits, but the trace has no more scheduling decisions")
		}
		what := "the trace's next decision runs " + next
		if want != nil {
			what = "the trace's next decision runs " + want.label()
			if sn, ok := want.snap(nil); ok {
				d, _ := sn.describe()
				what += ", which is " + d
			}
		} else if _, _, ok := pickTok(next); ok {
			what += ", a thread the program has not started (or that has ended)"
		}
		if !p.complete {
			p.prefixEnd(what)
		}
		p.diverge(what)
	}
}

// fxFinishOnce ends a recording or replay once.
var fxFinishOnce sync.Once

// fxFinish writes the trace of a recording or checks the end of a replay.
func fxFinish(code int, c *crashInfo) {
	fxFinishOnce.Do(func() {
		switch fxMode {
		case fxRecord:
			rec.write(runOutcome(code, c))
		case fxReplay:
			rp.finish(code, c)
		}
	})
}
