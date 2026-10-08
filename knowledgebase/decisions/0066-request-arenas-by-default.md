# 0066: Request scoped arenas are on by default

Status: Accepted (2026-10-08)

## Context

Request scoped arenas (`std/rt/arena.go`, `internal/gen/arena.go`) were
added behind `xo build --arenas` as the experiment of decision 0040
(`bench/results/perf-arenas-2026-10-08.md`). They cut task 01's allocation
per request by about half and its GC cycles per request about 3x, but a
flag that must be remembered is a flag nobody sets. Making them the
default requires that no value made during a request can still be
reachable after its arena is reset and reused, whatever the program does
with it. Spec core 5.5 listed arenas as P2.

The audit enumerated every way a value allocated during a request can
outlive the request or be used beside it, and for each either pointed at
the mechanism and the test that covers it, or wrote a test that proves it
(golden `arena_holes`, which, like `arena_escape`, runs many requests from
concurrent clients against a real loopback server so arenas are reused
while escaped values are alive, and must print the same with and without
arenas).

## Audit

Kinds of arena memory: (R) the request's header, parameter, and query
tables and its body, only for a route whose handler literal confines its
request; (S) a response built by `http.json`/`http.text` that is directly
the handler literal's result; (T) a `values()`/`keys()`/`entries()`/
`to_list()` List consumed in its own expression. Everything else is heap.

| Escape route | How it is handled | Evidence | Result |
|---|---|---|---|
| Stored in a `Mutex` (or `Atomic`) | Stored values are copies; R strings are heap (net/http), R tables are only reachable through `req`, which a confined handler never binds; S and T cannot be named | `arena_escape` (names, headers, params, keys List), `arena_holes` (inner responses, closures) | Clean |
| Sent on a channel | Same: copies of heap values | `arena_escape` (`ch.send`) | Clean |
| Captured by `spawn_background` / `os.tasks`, running after the response | Nested closures may not mention `req` (confinesReq); spawned tasks never get an arena (`Task.child`) | `arena_escape` (late background tasks read values 2 ms after the response), `TestArenaEscapeAnalysis` "request captured by a nested closure" | Clean |
| Returned in the response body | S is written to the connection (net/http copies it) before reset | `arena_escape`, `arena_holes` (bodies checked by every client) | Clean |
| Streamed response (`http.stream`) produced after the handler returned | The stream closure runs on the request task before release; it cannot mention `req` | `arena_holes` `/stream` | Clean |
| Global state, `task.Once` filled in a handler | A Once value is the result of its closure; no closure result is ever arena memory (T is never a result, S only of the handler literal) | `arena_holes` `/once` (value compared on every request and at exit) | Clean |
| Go interop (`use go`) values | Only heap values can be named and passed; Bytes and slices are copied at the boundary | Code (`internal/gobind`), `go_*` goldens under both lowerings | Clean |
| Go interop callbacks run by Go on another goroutine (`sync.WaitGroup.Go`, timers), during or after the request | **Hole 2.** The callback adapter captured the request's task, arena included: two goroutines bump allocated in one arena, and a late callback could allocate in a reset arena | `arena_holes` `/go`: 36 to 48 wrong responses and 67 to 96 wrong callback checks per run before the fix, 0 after (without arenas: 0) | Fixed: callbacks run with `rt.GoCallbackTask(tk)`, a task without the arena (gobind format version 2) |
| Logged | `log.*` formats synchronously (`logRecord`) and only Str values | Code (`std/rt/log.go`); handler fault logs in `arena_holes` | Clean |
| Recorded (`XO_RECORD`, `XO_RING` crash bundles) | **Hole 1.** `fxEncode` kept the request body string and the S response body without copying: a recorded body was overwritten by the next request (a trace showed `{"name":"b"}st-and-much-longer"}`; even a non confined route's recorded body, since the recorder gets the request before dispatch) | `TestReplayServer` (new `POST /item` requests; the replay diverged before the fix) | Fixed: serve hands the recorder heap copies of the bodies (`recCopyReq`, `recCopyResp`) |
| `std/sql` | Arguments are `SqlValue`s of heap Str and Int; rows decode to fresh values | `arena_holes` `/sql` (rows read back at exit) | Clean |
| Closure stored elsewhere and called later | Captures are copies of heap values; a closure mentioning `req` un-confines the route | `arena_holes` `/inner` (closures called after all requests), `TestArenaConfineRandom` | Clean |
| Response kept by middleware | A router with middleware of its own gets no arena (serve). **Hole 3.** A *mounted* router's middleware ran behind a server without top level middleware, so its handler's S response was arena memory that the middleware could keep (a cache) | `arena_holes` `/m/c`: 72 of 72 cached responses corrupted before the fix, 0 after | Fixed: a mounted route with middleware runs with arena responses off (`noArenaResp`, as `r.call` already did) |
| In process dispatch (`r.call`) inside a request, response kept | `M_call` turns arena responses off; params tables in the arena are only read by confined handlers | `arena_holes` `/inner`, `TestArenaResponseOnlyInServerDispatch` | Clean |
| Fault in the middle of a handler | `isolateHandler` turns it into a 500 (heap); the arena is released after the response as usual | `arena_holes` `/fault` (9 faults among reused arenas) | Clean |
| Cancellation: client gives up, handler continues | The handler keeps its arena until it returns; release happens after | `arena_holes` `/slow` (values stored after the client's timeout) | Clean |
| Cancellation: shutdown cut short (unwinding) | The unwinding panic skips `release`; the arena is left to the GC, never reused | Code (`serve`, comment at `a.release()`) | Clean |
| Value semantics of elements copied out of T | Std call results are marked (own.go); consumers mark the elements they copy | `arena_holes` `/vs` (writes through first, find, fold, entries, loop copies, `to_list().get`, `reverse().first()`) | Clean |
| Scope bodies, `Mutex.with` closures | Run on the request goroutine; share the arena; children get none | `TestGoldenRace` with arenas (`arena_escape`, `arena_holes`, servers) | Clean |
| Typed request values (decision 0055) | `req.value`/`with_value` are not confined forms, so such a handler gets a heap copy; middleware routes get no arena | `TestArenaConfineRandom` | Clean |

Differential evidence: `TestGolden` builds every golden program with arenas
(the default) and `TestGoldenNoArenas` without, both against the same
want.txt; `TestGoldenRace` and `TestGoldenRaceNoArenas` likewise under the
race detector; `TestExampleArenaDiff` runs examples/notes-service's tests
both ways and `TestExampleArenaServeDiff` its real server (scripted plus
4x30 concurrent requests through the mounted auth middleware); all pass.
`TestArenaConfineRandom` generates 160 random handler bodies (12 confined
forms, 10 escaping ones, nested in if, match, scope, and loops) and
requires a route to be arena backed exactly when it has no escaping form.

## Decision

Arenas are on by default for programs built with the Go backend: `xo
build`, `run`, and `test` take `--no-arenas` to turn them off (`--arenas`
stays accepted, as `--mono` did in its switch). `build.Options.NoArenas`
replaces `build.Options.Arenas`, and the build info records `no_arenas`.
The llvm and arm64 backends have no HTTP server and ignore arenas. Spec
core 5.5 describes arenas as an unobservable implementation detail; a user
written `arena scope` stays P2.

Measured with allocation counts (load independent; MemStats deltas around a
loadgen run of task 01, 8 workers, 3 runs each; the host had load average
23 to 38 from another agent's work, so req/s is not reported):

| Build | Objects per request | Bytes per request | GCs per 10k requests |
|---|---|---|---|
| `--no-arenas` | 76.8 | 9.46 KB | 76 to 80 |
| arenas (default) | 49.3 | 5.47 KB | 41 |
| arenas before the fixes (`--arenas`, previous commit) | 49.7 | 5.43 KB | 37 to 38 |

The fixes cost nothing on task 01 (its routes have no mounted middleware,
no Go callbacks, and it is not recorded).

## Consequences

- Every HTTP service gets the arena gain without a flag; programs without
  `http.server` call `rt.EnableArenas()` and pay nothing (temporaries fall
  back to the heap off a request task).
- Runtime code that keeps a request, a response, or a body past the
  request (recorder, middleware, caches) must copy arena memory first; Go
  callbacks must never run with a request task. The comment on `Arena`
  lists the cases; a new keeper needs a line there and a case in
  `arena_holes`.
- Cached `use go` bindings of format version 1 are regenerated (version 2).
- Remaining risks, not holes found: the analysis trusts that net/http and
  net/url return heap strings and that HTTP/1 writes copy the body
  synchronously; an HTTP/2 or custom transport would need its own check.
  A Go callback run on another goroutine still shares the request's
  thread record and held mutex map, as before arenas. (Update
  2026-10-08: resolved by decision 0067; such a call is a task of its
  own.)

## Alternatives

- Keep the flag (status quo): rejected, the audit found three holes and
  fixed them, and the differential and race runs pass both ways.
- Copy everything out at the end of each request (the spec's old wording,
  "values escaping it are copied out"): needs a precise escape set at run
  time; the static confinement plus heap copies at route granularity is
  simpler and already measured.
- Default on only for routes without middleware: that is already what the
  runtime does for the request; the response side needed the mounted
  middleware fix either way.
