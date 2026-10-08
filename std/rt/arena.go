package rt

import (
	"io"
	"strings"
	"unsafe"
)

// Request scoped arenas (spec core 5.5): on by default since decision 0066
// (`--no-arenas` turns them off); first measured in
// bench/results/perf-arenas-2026-10-08.md (decision 0040). The audit of
// every way a value can leave a request is the table of decision 0066;
// the goldens arena_escape and arena_holes run it with and without arenas.
//
// An Arena is memory owned by one HTTP request. The server takes one from
// a pool when a request arrives, hangs it on the request's task, and puts
// it back (reset) after the handler returned and the response was
// written. Go has no manual free, so "freeing" means reuse: the next
// request overwrites the same arrays instead of allocating new ones, and
// the garbage collector sees fewer, longer lived objects. Two kinds of
// memory come from an arena:
//
//  1. Request plumbing built by the runtime: the Map tables of the request
//     headers, the path parameters, and the query; the request body bytes;
//     and the JSON bytes and header table of a response built by
//     `http.json`, `http.text`, or `http.empty` in a handler's return
//     position.
//  2. Expression temporaries chosen by the code generator: the List made
//     by `m.values()`, `m.keys()`, `m.entries()`, or `s.to_list()` when it
//     is only the receiver of a List method that does not keep its
//     receiver, or only the iterable of a `for` loop.
//
// # Soundness: a value never outlives its arena
//
// The invariant is: no arena memory is reachable from anything except the
// request's own runtime frames and compiler temporaries, and both are dead
// when the arena is reset. The argument, case by case:
//
//   - Strings stored in arena tables (header names and values, parameter
//     and query values) are never arena memory: they come from net/http
//     and net/url, which allocate them on the heap. So a `req.header(n)`,
//     `req.param(n)`, or `req.query(n)` result may escape freely; only the
//     tables themselves are arena memory.
//   - The request (whose tables and body are arena memory) reaches Xo code
//     only as the parameter of a route handler. internal/gen marks a route
//     (Http_Router_arenaLast) only when its handler is a closure literal
//     that uses the parameter solely as `req.json[T]()`, `req.param(..)`,
//     `req.param_int(..)`, `req.query(..)`, `req.header(..)`, or the Str
//     fields `method`, `path`, `remote_ip`, `url.scheme`, `url.host`,
//     `url.port`, `url.path`, and not inside a nested closure. None of
//     these return arena memory: `json` decodes through encoding/json,
//     which copies every string out of the body. For any other route, and
//     for every route of a router with middleware, dispatch copies the
//     request to the heap first (heapCopy), so the handler sees no arena
//     memory at all.
//   - An arena response is built only by a call that internal/gen found in
//     return position of a handler literal, so its value is the handler's
//     result and nothing else holds it. The runtime writes it to the
//     connection (net/http copies the bytes into its own buffer) before the
//     arena is reset. In process dispatch (`r.call(req)`, whose response
//     goes back to Xo code) and the middleware of a mounted router (which
//     sees the response and may keep it) clear Arena.resp while they run
//     (noArenaResp), so a handler reached that way builds a heap response.
//     A router with middleware of its own gets no arena at all (serve).
//   - The recorder (XO_RECORD, XO_RING) keeps the request and response of
//     every request past it: serve hands it heap copies of the bodies
//     (recCopyReq, recCopyResp).
//   - A temporary List of the second kind is bound to a compiler temporary
//     the program cannot name. The consumers allowed (internal/gen
//     arenaConsumers) copy elements out (Go value copies) and return fresh
//     values; none returns or stores its receiver or a slice of it.
//     Elements were copied in from a Map or Set, whose contents are never
//     arena memory by the first and second points, so element copies do
//     not point into the arena either.
//   - Lifetime: the arena is reset after the handler returned and the
//     response was written. Structured concurrency (core 7.1) means every
//     task started in a scope inside the handler has finished by then.
//     Tasks started on longer lived groups (`os.tasks`) run with their own
//     Task, which has no arena, and can only have captured values the
//     points above show are heap values.
//   - One goroutine at a time: only the request task and the bodies of
//     scopes it opens (which run on the same goroutine) carry the arena;
//     spawned tasks get none (Task.child), and an Xo function passed to Go
//     as a callback, which Go may run on any goroutine, runs with a copy of
//     the task without it (GoCallbackTask), so the bump allocators need no
//     locks. The free list is a channel, so a send by one request
//     happens before the receive of the next request that reuses it.
//
// Memory is reused only after reset; inside one request an arena only
// grows (bump allocation), so a temporary is never overwritten while the
// request runs, however often its expression is evaluated. Oversized
// buffers are dropped at reset (arenaKeep) so one large request does not
// pin memory in the pool.
type Arena struct {
	body     []byte
	jbuf     []byte
	jcount   []int
	resp     bool // arena responses allowed: the server's own dispatch is running
	respUsed bool
	maps     []*mtable[string, string]
	nmaps    int
	slabs    map[any]arenaSlab
}

// arenaKeep is the largest buffer (bytes or elements) an arena keeps
// across requests.
const arenaKeep = 1 << 16

// arenaFree is the free list of reset arenas. It is a bounded channel
// rather than a sync.Pool because a Pool drops its items at every garbage
// collection, and request heavy programs collect often, which defeats
// reuse. At most arenaFreeMax arenas are kept.
const arenaFreeMax = 128

var (
	arenasOn  bool
	arenaFree = make(chan *Arena, arenaFreeMax)
)

// EnableArenas turns request scoped arenas on; the generated main of a
// program built without `--no-arenas` calls it first.
func EnableArenas() { arenasOn = true }

func getArena() *Arena {
	select {
	case a := <-arenaFree:
		return a
	default:
		return &Arena{slabs: map[any]arenaSlab{}}
	}
}

// release resets a and returns it to the pool. The caller must hold no
// arena memory any more (see the soundness argument above).
func (a *Arena) release() {
	if cap(a.body) > arenaKeep {
		a.body = nil
	}
	a.body = a.body[:0]
	if cap(a.jbuf) > arenaKeep {
		a.jbuf = nil
	}
	a.jbuf, a.jcount = a.jbuf[:0], a.jcount[:0]
	a.resp, a.respUsed = false, false
	for _, t := range a.maps[:a.nmaps] {
		resetTable(t)
	}
	a.nmaps = 0
	for _, s := range a.slabs {
		s.reset()
	}
	select {
	case arenaFree <- a:
	default: // enough arenas kept: leave this one to the GC
	}
}

// resetTable empties a table for reuse, keeping its log array and index.
func resetTable[K, V any](t *mtable[K, V]) {
	if p := t.arr.Load(); p != nil {
		if cap(*p) > arenaKeep {
			t.arr.Store(nil)
		} else {
			clear((*p)[:min(int(t.used.Load()), len(*p))])
		}
	}
	if ix := t.idx.Load(); ix != nil {
		if len(ix.slots) > arenaKeep {
			t.idx.Store(nil)
		} else {
			for i := range ix.slots {
				ix.slots[i].Store(0)
			}
		}
	}
	t.used.Store(0)
	t.keys = 0
}

// strMap returns an empty Map[Str, Str] whose table is arena memory, or a
// plain empty map when a is nil.
func (a *Arena) strMap() Map[string, string] {
	if a == nil {
		return Map[string, string]{}
	}
	var t *mtable[string, string]
	if a.nmaps < len(a.maps) {
		t = a.maps[a.nmaps]
	} else {
		t = &mtable[string, string]{}
		a.maps = append(a.maps, t)
	}
	a.nmaps++
	m := Map[string, string]{t: t}
	if p := t.arr.Load(); p != nil {
		m.es = (*p)[:0]
	}
	return m
}

// readBody reads the request body into arena memory.
func (a *Arena) readBody(r io.Reader) Bytes {
	b := a.body[:0]
	for {
		if len(b) == cap(b) {
			b = append(b, 0)[:len(b)]
		}
		n, err := r.Read(b[len(b):cap(b)])
		b = b[:len(b)+n]
		if err != nil {
			break
		}
	}
	a.body = b
	if len(b) == 0 {
		return ""
	}
	return Bytes(unsafe.String(&b[0], len(b)))
}

// copyMap copies a map into fresh heap tables.
func copyMap(m Map[string, string]) Map[string, string] {
	var out Map[string, string]
	for k, v := range m.All() {
		out.M_put(nil, k, v)
	}
	return out
}

// heapCopy moves every arena backed part of a request to the heap, for a
// handler that may let the request escape.
func (r Http_Request) heapCopy() Http_Request {
	r.F_headers = copyMap(r.F_headers)
	r.XParams = copyMap(r.XParams)
	r.F_url.F_query = copyMap(r.F_url.F_query)
	r.F_body = Bytes(strings.Clone(string(r.F_body)))
	return r
}

// recCopyReq returns req for the recorder (replay.go), which keeps it
// past the request: its body, arena memory when a is not nil, copied to
// the heap. The tables need no copy: the recorder encodes their entries,
// and the strings in them are heap strings. Decision 0066, hole 1: before
// this copy, a recorded body was overwritten by the next request that
// reused the arena.
func recCopyReq(a *Arena, req Http_Request) Http_Request {
	if a != nil {
		req.F_body = Bytes(strings.Clone(string(req.F_body)))
	}
	return req
}

// recCopyResp is recCopyReq for a response, whose body is arena memory
// when it was built by Http_json_arena.
func recCopyResp(a *Arena, resp Http_Response) Http_Response {
	if a != nil {
		resp.F_body = Bytes(strings.Clone(string(resp.F_body)))
	}
	return resp
}

// Http_Router_arenaLast marks the route registered last as one whose
// handler confines its request (internal/gen proved it), so the request it
// gets may be arena memory.
func Http_Router_arenaLast(r *Http_Router) {
	if n := len(r.routes.s); n > 0 {
		r.routes.s[n-1].arena = true
	}
}

// arenaResp returns the arena of tk if a response may be built in it now.
func arenaResp(tk *Task) *Arena {
	if tk == nil || tk.arena == nil || !tk.arena.resp || tk.arena.respUsed {
		return nil
	}
	return tk.arena
}

// Http_json_arena is `http.json(status, v)` in return position of a
// confined handler: the body and header table are arena memory.
func Http_json_arena[T any](tk *Task, status int64, v T) Http_Response {
	a := arenaResp(tk)
	if a == nil {
		return Http_json(tk, status, v)
	}
	a.respUsed = true
	w := JsonW{b: a.jbuf[:0], count: a.jcount[:0]}
	JsonEnc(tk, &w, v)
	a.jbuf, a.jcount = w.b[:0], w.count[:0]
	h := a.strMap()
	h.M_put(tk, "Content-Type", "application/json")
	var body Bytes
	if len(w.b) > 0 {
		body = Bytes(unsafe.String(&w.b[0], len(w.b)))
	}
	return Http_Response{F_status: status, F_headers: h, F_body: body}
}

// Http_text_arena is `http.text(status, s)` in return position of a
// confined handler: the header table is arena memory.
func Http_text_arena(tk *Task, status int64, s string) Http_Response {
	a := arenaResp(tk)
	if a == nil {
		return Http_text(tk, status, s)
	}
	a.respUsed = true
	h := a.strMap()
	h.M_put(tk, "Content-Type", "text/plain; charset=utf-8")
	return Http_Response{F_status: status, F_headers: h, F_body: Bytes(s)}
}

// ---------------------------------------------------------------------------
// Expression temporaries

// arenaSlab is a typed bump allocator of an arena.
type arenaSlab interface{ reset() }

type slab[T any] struct {
	buf []T
	n   int
}

// reset clears the used part (so the arena does not keep values alive)
// and starts over; an oversized buffer is dropped.
func (s *slab[T]) reset() {
	clear(s.buf[:s.n])
	s.n = 0
	if len(s.buf) > arenaKeep {
		s.buf = nil
	}
}

type slabKey[T any] struct{}

// arenaSlice returns n elements of arena memory (zeroed), or nil when they
// do not fit the arena's limits (the caller allocates normally). Memory
// handed out is never handed out again before the arena is reset.
func arenaSlice[T any](a *Arena, n int) []T {
	if n > arenaKeep {
		return nil
	}
	k := any(slabKey[T]{})
	s, _ := a.slabs[k].(*slab[T])
	if s == nil {
		s = &slab[T]{}
		a.slabs[k] = s
	}
	if s.n+n > len(s.buf) {
		// the old buffer may still back live temporaries: leave it to
		// the GC and continue in a larger one
		s.buf = make([]T, max(64, 2*len(s.buf), n))
		s.n = 0
	}
	out := s.buf[s.n : s.n+n : s.n+n]
	s.n += n
	return out
}

// tempArena is the arena for an expression temporary of the current task.
func tempArena(tk *Task) *Arena {
	if tk == nil {
		return nil
	}
	return tk.arena
}

// Map_values_arena is `m.values()` as a temporary (see Arena).
func Map_values_arena[K, V any](tk *Task, m Map[K, V]) List[V] {
	a := tempArena(tk)
	if a == nil {
		return m.M_values(tk)
	}
	out := arenaSlice[V](a, m.n)
	if out == nil {
		return m.M_values(tk)
	}
	i := 0
	m.each(func(_ *mentry[K, V], v V) bool {
		out[i] = v
		i++
		return true
	})
	return List[V]{s: out[:i]}
}

// Map_keys_arena is `m.keys()` as a temporary.
func Map_keys_arena[K, V any](tk *Task, m Map[K, V]) List[K] {
	a := tempArena(tk)
	if a == nil {
		return m.M_keys(tk)
	}
	out := arenaSlice[K](a, m.n)
	if out == nil {
		return m.M_keys(tk)
	}
	i := 0
	m.each(func(e *mentry[K, V], _ V) bool {
		out[i] = e.k
		i++
		return true
	})
	return List[K]{s: out[:i]}
}

// Map_entries_arena is `m.entries()` as a temporary.
func Map_entries_arena[K, V any](tk *Task, m Map[K, V]) List[Tuple2[K, V]] {
	a := tempArena(tk)
	if a == nil {
		return Map_entries(tk, m)
	}
	out := arenaSlice[Tuple2[K, V]](a, m.n)
	if out == nil {
		return Map_entries(tk, m)
	}
	i := 0
	m.each(func(e *mentry[K, V], v V) bool {
		out[i] = Tuple2[K, V]{e.k, v}
		i++
		return true
	})
	return List[Tuple2[K, V]]{s: out[:i]}
}

// Set_to_list_arena is `s.to_list()` as a temporary.
func Set_to_list_arena[T any](tk *Task, s Set[T]) List[T] {
	return Map_keys_arena(tk, s.m)
}
