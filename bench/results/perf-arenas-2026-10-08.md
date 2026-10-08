# Request scoped arenas: task 01 under HTTP load (2026-10-08)

Goal: test the claim in decision 0040 that structured concurrency lets Xo
cut GC cost below Go's, using per request arenas (work queue item 5;
spec core 5.5 lists request scoped arenas as P2). Measure Go against Xo on
the task 01 CRUD API under the same loopback load, add arenas behind
`xo build --arenas`, and measure again.

Host: darwin/arm64 (M4 Pro laptop, 12 cores), Go 1.26.1. Nothing in
`spec/` or `knowledgebase/decisions/` changed.

## Harness (`bench/perf/http/`)

- `go/`: copy of `bench/go/plain/01-crud-api`; `xo/`: copy of
  `bench/xo/v0.4/01-crud-api`. Both copies listen on `127.0.0.1` only; that
  is their only change. The originals are untouched.
- `loadgen/`: a Go load generator (its own module). 32 workers (`CONC`),
  each on a keep alive connection, repeat: POST /users, GET /users/{id},
  PATCH /users/{id} (nickname), GET /users/{id}, DELETE /users/{id}, and
  every eighth cycle GET /users. The store is preloaded with 64 users, so it
  holds about 64 + 32 users throughout. 2 s warmup, 10 s measured. Every
  response status is checked; all runs had 0 errors.
- `run.sh`: builds the servers, starts each fresh with `GODEBUG=gctrace=1`,
  runs the load, stops the server with `kill` of its own PID, and parses
  the GC trace. Configs are interleaved, 3 runs each, medians reported.
  `xo-base` is the Xo server built with the xo binary from before this
  change (`XO_BASE=<path>`).

Metrics: throughput and latency over the measured window; GC count, total
stop the world pause (sum of both STW phases of every cycle), peak heap
(largest heap at GC start, from gctrace, MB granularity), GC CPU percent
(gctrace's cumulative figure) over the whole run including warmup;
allocation per request from `runtime.MemStats` in a separate profiling
build (4 s of the same load).

## What changed

Shared by both Xo builds (mode independent, so `xo` differs from `xo-base`
by these only):

- `JsonW.str` wrote every JSON string through a new `json.Encoder` and
  `bytes.Buffer`. It now escapes directly (`appendJSONString`), byte for
  byte what encoding/json writes with HTML escaping off;
  `TestAppendJSONString` compares the two on fixed and 20,000 random
  strings.
- `Mutex.with` formatted its call site (`file.xo:line`, a 64 frame stack
  walk and a `Sprintf`) on every call for the re-entrancy fault. It now
  keeps 14 program counters and resolves them only when XO-F004 is
  reported; the golden `mutex_atomic` checks the message is unchanged.
- The server writes the response body with `io.WriteString` (no
  `[]byte` copy) and skips query parsing when the URL has no query.

Arenas (`--arenas`, `std/rt/arena.go`, `internal/gen/arena.go`): an
`Arena` is owned by one HTTP request. The server takes one from a free list
when the request arrives, hangs it on the request's task, and resets it
after the handler returned and the response was written. Go has no manual
free, so freeing means reuse: the next request overwrites the same arrays
instead of allocating. Two kinds of memory come from it:

1. Request plumbing built by the runtime: the Map tables of the headers,
   path parameters, and query; the body bytes; and the JSON bytes and
   header table of a response built by `http.json`/`http.text` in a
   handler's return position.
2. Expression temporaries chosen by the code generator: the List made by
   `m.values()`, `m.keys()`, `m.entries()`, or `s.to_list()` when it is
   only the receiver of a List method that keeps nothing of it (`any`,
   `all`, `find`, `fold`, `map`, `filter`, `sort_by`, `sort`, `len`, ...) or
   the iterable of a `for` loop. In task 01 these are
   `st.users.values().any(...)` (every create and patch) and
   `st.users.values().sort_by(...)` (every list).

## Soundness: a value never outlives its arena

Invariant: arena memory is reachable only from the request's own runtime
frames and compiler temporaries, both dead when the arena is reset. The
full argument is the comment on `Arena`; in short:

- Strings stored in arena tables come from net/http and net/url (heap);
  only the tables are arena memory, so `req.param(..)`, `req.header(..)`,
  `req.query(..)` results may escape freely.
- The request reaches Xo code only as a handler parameter. The code
  generator marks a route (`rt.Http_Router_arenaLast`) only if its handler
  is a closure literal that uses `req` solely as `req.json[T]()`,
  `req.param`, `req.param_int`, `req.query`, `req.header`, or the Str
  fields `method`, `path`, `remote_ip`, `url.scheme/host/port/path`, and
  never inside a nested closure (which could become a background task).
  `json` decodes through encoding/json, which copies every string. Any
  other handler, and every handler behind middleware, gets a heap copy of
  the request (`heapCopy`): the spec's "values escaping it are copied out"
  at route granularity.
- An arena response is built only by a call whose value is the handler
  literal's result (tail position through if, match, and blocks, or a
  `return`), so only the runtime holds it, and the runtime writes it
  (net/http copies the bytes) before the reset. In process dispatch
  (`r.call`) turns arena responses off while it runs.
- Temporaries live in compiler temporaries no Xo code can name; the allowed
  consumers copy elements out and return fresh values; elements came from
  a Map or Set, which never holds arena memory by the points above.
- Lifetime: by structured concurrency every task started in a scope inside
  the handler has ended when the handler returns. Tasks on longer lived
  groups (`os.tasks`) run with their own Task, which never inherits an
  arena, and can only have captured heap values.
- Only the request goroutine (and scope bodies, which run on it) carry the
  arena, so the bump allocators need no locks; the free list is a channel,
  which orders one request's use before the next one's.

Value semantics are what make a syntactic, intraprocedural analysis
enough: a value can reach shared state only by being bound, passed,
captured, or returned, and the analysis rejects all four for the request
and for temporaries.

## Results (32 workers, medians of 3)

| Config | Req/s | p50 us | p99 us | p99.9 us | GCs | GC pause ms | GC pause ms per 10k req | Peak heap MB | GC CPU % | Peak RSS MB | GCs per 10k req |
|---|---|---|---|---|---|---|---|---|---|---|---|
| Go | 122,806 | 224 | 872 | 1381 | 3266 | 419 | 2.86 | 3 | 3 | 22.0 | 22.3 |
| Xo before (xo-base) | 49,982 | 666 | 1058 | 1386 | 12602 | 1231 | 20.40 | 3 | 11 | 23.2 | 208.9 |
| Xo | 52,367 | 622 | 993 | 1345 | 11871 | 1092 | 17.31 | 3 | 10 | 23.3 | 188.2 |
| Xo --arenas | 73,056 | 390 | 834 | 1149 | 4549 | 422 | 4.82 | 4 | 4 | 24.6 | 52.0 |

Allocation per request (MemStats over a 4 s run, same load):

| Config | Bytes per request | Objects per request |
|---|---|---|
| Go | 3.0 KB | 31 |
| Xo before | 15.8 KB | 158 |
| Xo | 13.1 KB | 97 |
| Xo --arenas | 6.1 KB | 68 |

8 workers (medians of 3, 8 s measured):

| Config | Req/s | p50 us | p99 us | p99.9 us | GC pause ms per 10k req | GC CPU % | GCs per 10k req |
|---|---|---|---|---|---|---|---|
| Go | 83,282 | 88 | 274 | 460 | 1.72 | 1 | 16.8 |
| Xo | 62,250 | 105 | 388 | 531 | 10.52 | 6 | 109.1 |
| Xo --arenas | 74,369 | 95 | 324 | 476 | 3.54 | 2 | 37.8 |

## Where arenas help, and why

- GC work per request falls 3.6x (188 to 52 GCs per 10k requests at 32
  workers; 2.9x at 8) and GC pause per request 3.6x (17.3 to 4.8 ms per
  10k). The live heap is tiny (3 to 4 MB, the minimum heap goal), so the
  number of cycles is proportional to bytes allocated, and arenas halve
  those (13.1 to 6.1 KB per request).
- Throughput rises 40 percent (52k to 73k req/s at 32 workers, 19
  percent at 8) and p99 falls 16 percent. Most of the gain is the GC
  cycles that no longer run (GC CPU 10 to 4 percent) plus cheaper
  allocation: reused Map tables skip index and log array allocation,
  reused JSON bytes skip `append` growth and the final string copy.
- Arena costs are small: peak RSS grows 1.3 MB (up to 128 kept arenas,
  each capped at 64 KB per buffer).
- The analysis covered every handler of task 01 without changes to the
  Xo source: all five handlers confine their request, four responses are
  arena responses, both `values()` temporaries are arena memory.

## Where arenas do not help, and why

- GC cost is not below Go's. Xo with arenas still allocates 2x Go's bytes
  and objects per request, so it runs 2.3x Go's GC cycles per request and
  pays 1.7x Go's pause per request. The claim of decision 0040 is not
  shown by this experiment; arenas closed about 85 percent of the GC gap
  (pause per 10k requests: Xo 17.3 ms, Xo --arenas 4.8 ms, Go 2.9 ms).
- What remains is not arena memory in this design (profile of the arena
  build, by objects): `JsonEnc` boxes every field through `any` (about 21
  percent of objects), `encoding/json` token parsing
  of request bodies (`jsonValue`, `Decoder.Token`, about 12 percent),
  `Mutex.with` (the copy of the protected value escapes to the heap, plus
  the deferred closure), the router splitting the path once per route
  tried (6 percent), `url.Parse` of `hr.URL.String()` (3 percent), and
  the per request Task and cancellation node with its channel (3
  percent). net/http's own request parsing (about 22 percent) is shared
  with Go.
- Throughput stays below Go (0.6x at 32 workers, 0.89x at 8): beyond
  allocation, Xo pays a Mutex with explicit waiters (32 workers contend on
  one store mutex; Go's version uses an RWMutex), the O(n) email scan of
  this Xo solution (Go keeps an email index), and checked arithmetic.
- Arenas do nothing for handlers that pass `req` to a function, read
  `req.headers` or `req.body` directly, capture `req` in a closure, or sit
  behind middleware: they get a heap copy (correct, just not faster). See
  `knowledgebase/gotchas/arena-handler-not-confined.md`.

## Confounds

- The load is closed loop: each worker waits for its response, so a slower
  server sees less queueing. Latency is comparable between the two Xo
  builds (same server code path), less so against Go at different
  throughput (`knowledgebase/gotchas/closed-loop-latency-confound.md`).
- Laptop timing drifts (`gotchas/bench-host-timing-drift.md`); configs
  were interleaved and the three runs agree within about 6 percent.
- The heap stays at the 4 MB minimum goal; with a large live heap each
  cycle marks more, so the savings per avoided cycle would be larger.
  Not measured here.
- The first arena version used `sync.Pool`, which drops its items at every
  GC; at hundreds of GCs per second reuse collapsed (9.6 KB per request
  against 8.5 KB with a channel free list, both measured before the
  shared fixes). A bounded channel free list
  fixed it (`gotchas/sync-pool-drops-on-gc.md`).

## Correctness

- `go test ./...` passes, including `TestGoldenArenas` (every golden
  program built with `--arenas`, same output) and `TestGoldenRaceArenas`
  (concurrent and HTTP goldens with `--arenas` under `-race`).
- Golden `arena_escape`: 320 requests from 4 concurrent clients to a real
  loopback server; values derived from requests escape through a Mutex,
  a channel, the response, background tasks on `os.tasks` that run after
  the response, an element copied out of an arena temporary, and a
  `keys()` List stored as a value. Same output with and without arenas,
  also under `-race`. Checked that it catches early frees: forcing every
  handler to count as confined makes it fault (a background task reads a
  reset parameter table); treating every `values()`/`keys()` call as a
  temporary corrupts 160 of 160 stored Lists.
- `TestArenaEscapeAnalysis`: table of handlers and functions with the
  expected decisions (request confined or not, response in return
  position or not, temporaries or not), and no arena code without the
  flag. `std/rt/arena_test.go`: no reuse before reset, emptied reuse
  after, heap copies independent of the arena, arena responses only in
  server dispatch.
- `xo test` on every solution in `bench/xo/v0.4` and `bench/xo/v0.6`
  (run on copies) prints the same results with the previous xo, the new
  xo, and the new xo with `--arenas` (03-retry-client: 1 contract fuzz
  failure in all three; 06-job-queue: XO0410 check error in all three; the
  rest pass).

## Next experiments, ranked by likelihood of getting Xo below Go

1. Typed JSON encoding: have `internal/gen/json.go` emit `w.Str(v.F_name)`
   and `w.Int(v.F_id)` style calls in derived `XoJsonEnc` instead of
   `rt.JsonEnc(tk, w, v.F_x)`, which boxes through `any`. Largest single
   object source left. Measure: `CONFIGS="go xo xo-arenas"
   bench/perf/http/run.sh`.
2. Arena backed JSON decoding: replace `jsonParse`'s encoding/json token
   loop (`std/rt/json.go`) with a hand written parser whose tree nodes come
   from the arena and whose strings are copied only in `JsonDec` for
   `*string` (where a value leaves the arena). About 1 KB and 15 objects
   per POST or PATCH.
3. Router and URL: split the path once per dispatch instead of once per
   route (`route.match` in `std/rt/http.go`), and build `Http_Url` from
   `hr.URL` instead of re-parsing its string.
4. `Mutex.with`: keep the protected value's copy off the heap (pass it by
   value and return it, or reuse an arena slot per with call) and drop the
   deferred closure (`std/rt/sync.go`).
5. Recycle the request Task and cancellation node through the arena; needs
   a proof that no goroutine keeps the Task (the HTTP client's cancel
   watcher ends before `M_send` returns).
6. Interprocedural confinement: per function summaries so handlers that
   pass `req` to helpers stay arena backed, and arena responses through
   `with_header`.
7. A large live heap run (`PRELOAD=5000`, and `GOGC=50/200`) to measure
   how the saved cycles scale when each cycle marks more.

With 1 to 4 the profile suggests Xo --arenas would land near 3 to 4 KB per
request, around Go's 3.0 KB; whether it goes below is the open question
for decision 0040.
