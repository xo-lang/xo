# 0325: Production observability in std: live profiles, traces, metrics

Status: Accepted (owner, 2026-10-21; Xo Lang 3's note from the language
poll). Built in stages 1 to 3 below (Xo Lang 2 reviews the runtime change
of stage 1).

## Context

A Xo server can be profiled only by starting it with `XO_PROF=cpu` (or
`heap`) and stopping it: the raw profile is written when the process
ends (decision 0238, 0240, 0245: `xo prof FILE` converts it). A server
in production cannot do that. Go's `net/http/pprof` gives a running
server `/debug/pprof/profile?seconds=30`, `/heap`, `/goroutine`, and
`expvar` and Prometheus-style counters give its health; nothing of this
exists in std. The runtime already has the parts: a per thread SIGPROF
sampler with a lock free ring (0238), an allocation sampler, the
scheduler's counters (`XO_RTSTATS`: spawn, steal, wake, park, futex
calls, lock slow paths; decision 0233), an execution trace (0240) and the
leak counters (`XO_LEAKCHECK`).

What the runtime lacks is the shape: starting and stopping a profile at
run time, reading it from inside the process, and a std surface a
handler can mount.

## Decision (proposed)

Three parts, each independent, none a dependency of the others.

### 1. `std/debug`: a router a server mounts (profiles)

```xo
use std/debug

var r = http.router()
r.mount("/debug", debug.router(auth: fn(req) { req.client_ip() == loopback }))
```

- `GET /debug/profile?seconds=N&hz=H` starts the CPU sampler for N
  seconds (default 30, at most 300), then answers the profile. Only one
  profile runs at a time (409 while one is running). The handler
  parks its fiber, not a worker.
- `GET /debug/heap` answers the sampled heap profile of the live objects
  (inuse) and, with `?alloc=1`, the allocations since the sampler
  started (rate from `XO_PROF_HEAP_RATE`, started on first request).
- `GET /debug/sched` answers the scheduler counters (the `XO_RTSTATS`
  set) as `text/plain` lines `name value`, for a before and after
  difference, and `GET /debug/threads` the threads' states.
- Format: the XORAW container of 0240 (`Content-Type:
  application/x-xo-profile`), which `xo prof URL` already converts to
  pprof and a text summary (the command gains an http(s) source), so the
  server needs neither a protobuf nor a gzip writer; `?format=pprof` is a
  later extension that adds both to std (std/compress has gzip).
- Access: the router is not mounted by default and takes an `auth`
  function; mounting without one is a compile time warning in
  `xo vet`. Profiles leak function names and, for the heap, sizes, and a
  30 s CPU profile costs about 1% of CPU at 100 Hz.

Runtime change (the one real piece of work): `prof.start()`/`finish()`
(std/native/llrt/prof.xo) run once per process at entry and exit. They
become `prof.begin(kind, hz) -> Int` and `prof.snapshot_stop() -> Bytes`
callable from a handler: arm the per thread timers (`setitimer` is per
process, so one call; the rings already exist when mapped), stop them
after the duration, collect every thread's ring into the container and
name the locations (what `finish` does at exit), unmap or reset the
rings. A release build without `--profile-support` has no profiler
(`prof.supported()` is false), and `debug.router` answers 501 for the
profile routes; the scheduler and thread routes need only the counters
and are always built.

Go backend: the same routes over `runtime/pprof` (profile bytes in the
same container is not possible, so on the Go backend the profile routes
answer pprof directly and `xo prof` passes it through). Parity is in the
route names and parameters, not the bytes.

### 2. Metrics: `std/metrics` (counters, gauges, histograms, text exposition)

Dependency free, about 300 lines, the Prometheus text format since it is
the one every collector reads:

```xo
use std/metrics

let reqs = metrics.counter("http_requests_total", ["route", "status"])
let lat = metrics.histogram("http_request_seconds", ["route"], metrics.latency_buckets)
reqs.inc(["/notes", "200"])
lat.observe(["/notes"], elapsed.seconds())
r.handle("GET /metrics", metrics.handler())
```

- Instruments are registered in a registry (a value, `metrics.default()`
  for the simple case; tests build their own); a counter and a gauge
  are one `Atomic[Int]`/`Atomic[Float]` per label set, a histogram an
  array of atomic bucket counts, a sum and a count. Label sets are keyed
  in a `Mutex[Map]` (the map write only on the first use of a label
  set; reads and increments take the read lock then the atomic), so the
  hot path allocates nothing after warmup.
- `http.server(...).with_metrics(registry)` registers per route request
  counts by status and a latency histogram, and gauges for connections
  and in flight requests (the server already counts the latter for
  drain), with no handler code.
- The runtime's counters are exported as `xo_` metrics on request:
  `xo_sched_wake_total`, `xo_futex_wake_total`, `xo_heap_inuse_bytes`,
  `xo_goroutine`-like `xo_fibers_live`, `xo_gc_*` on the Go backend; read
  from the same words as `XO_RTSTATS` (always counted for the cheap
  ones; the hot-path counters of rtstats stay behind their flag, so
  `/metrics` shows them as absent rather than slowing every server).
- Process: RSS, open fds, CPU seconds, start time (`/proc` on Linux,
  `proc_pidinfo` on macOS, the Windows equivalents later), as
  `process_*` as the Prometheus client does.

### 3. Request traces: a span log, not a tracing system

OpenTelemetry's wire protocol is out of scope (a dependency, and a large
surface). The minimal useful shape:

- The server assigns each request an id (from the `X-Request-Id` header
  or a random 64 bit id) and puts it in the request context
  (`req.id()`), and `std/log` includes it in every record written while
  handling the request (the logger reads the request context key).
- `trace.span(ctx, "name") -> Span` / `span.end()` record
  `(request id, name, start, duration, parent)` into a bounded ring in
  memory; `GET /debug/requests?slow=100ms` answers the slowest and the
  failed requests' spans as JSON (the last N, default 1000), and the
  `XO_TRACE` execution trace of 0240 stays the tool for scheduler
  level analysis.
- A W3C `traceparent` header is read and passed on by the HTTP client
  (so a collector can join spans written by other services), with
  nothing exported by Xo itself: an exporter is a user's handler that
  reads the ring.

## Safety

- `/debug` is off unless mounted; the profile and heap routes cost CPU
  and memory while running (rings of 17 MB for the CPU sampler per
  process, mapped only while profiling) and are limited to one at a time.
- Metrics label cardinality is bounded: a registry refuses more than
  `max_series` (default 10,000) label sets per instrument and counts the
  refusals in `xo_metrics_dropped_total`.
- The heap sampler's rate and the profiling hz are clamped.

## Staging

1. `prof.begin/snapshot_stop` and `std/debug` profile, heap, sched
   routes with the XORAW answer; `xo prof http://...`; tests: a golden
   that profiles itself over a FakeNet request and checks the container,
   and one that runs two profiles at once (409).
2. `std/metrics` with the text exposition and `with_metrics`; a golden of
   the exposition and a concurrent increment test under `--race`.
3. Request ids and spans; `/debug/requests`.
4. pprof protobuf+gzip answers for tools outside Xo.
5. The soak harness (below) runs 1 to 3 against examples/notes-service
   and watches the series for drift.

## Soak harness (the owner's second item)

`bench/soak/` runs examples/notes-service under a steady mixed load for
hours: the load generator of bench/perf/http (loadgen, mixed CRUD) at a
fixed request rate (not closed loop, so latency is not hidden by
backpressure); a sampler records every 10 s the server's RSS, open fds,
CPU seconds per request, p50/p99/p99.9 latency of the last interval, and
the runtime counters (from `/metrics` once part 2 exists, from `/proc`
and `XO_RTSTATS` at exit before), plus the leak counters (`XO_LEAKCHECK`
at exit); drift is a least squares slope over a sliding window on RSS
and fd count (alert when positive beyond a threshold after warmup) and a
latency percentile rising by more than 20% against the first hour. The
Mac natively and Docker linux/arm64 first, Windows later through CI with
the owner's approval. See `bench/soak/README.md`.

## Alternatives

- Embedding OpenTelemetry or a pprof protobuf encoder in std first: more
  code and a format fight before any use; the XORAW container plus
  `xo prof` is what agents already use.
- Profiling by a signal (SIGUSR1 dumps a profile file): no HTTP, works
  for servers without a debug port, but needs a file path and an extra
  channel; it can be added over the same `prof.begin/snapshot_stop`.
- Metrics only through `/proc` scraping by an agent: no per route
  numbers.

## Consequences

- A running server can be profiled and watched without a restart.
- Three new std modules/routers and one runtime change (the profiler
  becomes restartable); the leak and scheduler counters gain readers.
- Cost when unused: none (nothing registered, `/debug` unmounted, the
  rings unmapped).

## Built (2026-10-21)

Stage 1 0c17dba55 (std/debug, the restartable profiler), stage 2
81c91200c and b9b44a29f (std/metrics, `Registry.middleware`), stage 3
dd2628eb7 (std/trace). Differences from the plan above:

- `xo prof URL` is not built: the compiler would need an HTTP client.
  `curl -o p.xoraw http://HOST/debug/profile?seconds=30` then `xo prof
  p.xoraw` does the same.
- `with_metrics` on the server is `Registry.middleware(clock, label)` for
  `router.wrap`: a middleware needs no server change, and the `route`
  label is the program's own bounded name for a request.
- The runtime counters (`xo_sched_*`) are not exported by `/metrics`; the
  scheduler's counters are `GET /debug/sched`.
- `/debug/requests` is `Ring.handler(auth)` of std/trace, mounted by the
  program.
- No program wide default registry: Xo has no program state, so main
  makes one and passes it.
- The profiler's ring stays mapped after a live profile (17 MB virtual);
  profile_collect returns the used span's pages with madvise (linux.madvise,
  sys_darwin.madvise), so a read profile is not resident.
