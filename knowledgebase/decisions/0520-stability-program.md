# 0520: Stability program: a corpus of real services, tested continuously

Status: Accepted (owner, 2026-11-05, through the lead: "go with your recommendations"). Night soaks may
take one long lease each (OFF_MAX_MIN raised for that job only) between 22:00 and 07:00 local time. Phase 1
starts after release-vs-go-v0.40.0. Proposed by Xo Lang 2 the same day.

## Context

Xo's tests are strong on short programs and weak on long-running ones. Every differential check compares the
standard output of a short program: progen's generators against the Go backend (internal/gen
compileprop_test.go:44, scripts/coll-diff.sh), scripts/stdfuzz over pure std functions on three backends,
the goldens, scripts/twincheck. The std parsers are fuzzed against Go (internal/stdtest *fuzz_test.go,
internal/llvm http_diff_test.go:567 FuzzHTTPDiff, TestHTTPFuzz). What breaks in production is different: a
service that runs for hours, under load, with clients that misbehave. The 2026-11-05 race stress
(bench/results/race-stress-2026-11-05.md) showed the gap: a heap corruption in the HTTP server (cbe600955:
heap_large blocks released 8 bytes off) was found by running the concurrency goldens many times at once under
load, not by any test of a real service.

What exists to build on (surveyed 2026-11-05):

- **One real service.** examples/notes-service (main.xo 287 lines, main_test.xo 242): users and expiring notes
  over JSON, Argon2id passwords, SHA-256 token digests, a purge loop, graceful shutdown. Its storage is std/sql's
  in-memory driver, so nothing survives a restart.
- **A soak harness.** bench/soak (decision 0325 item 2): soak.sh runs notes-service under XO_LEAKCHECK=1 for
  DURATION (1 h) at RATE (500/s), driven by bench/soak/client (Go, open loop, 128 workers, a five-request mix),
  sampling RSS, fds, latency and CPU per request every 10 s into samples.csv and failing on drift (RSS over
  10%/h, an fd slope over 1/h, last-quarter p99 20% over the first, errors over 0.1%). It has run only on the
  Mac and in Docker (a 2 h Docker run: 2.07M requests, 0 errors, 0 live of 769M allocations, a drift verdict
  confounded by VM load) and never on the offload hosts; no result is committed.
- **Load generators.** bench/perf/jitter/loadgen (open loop with no coordinated omission, p50 to max,
  CPU per request) and bench/perf/http/loadgen (closed loop CRUD cycles, h2, TLS resume, connect storms), run on
  host 1 by scripts/release-vs-go-http.sh, with pads.py's placement-aware verdict.
- **A response parity check.** bench/perf/http/parity.sh diffs status, headers and body of 26 requests against
  two servers (written for Go against Rust, Node and Python twins).
- **Candidate apps.** bench/xo/v0.4 and v0.6 hold ten task solutions with Go twins in bench/go (crud API,
  worker pool, retry client, file sync, rate limiter, job queue, cache, notes service, inventory, log
  pipeline), the buildable versions of bench/paper's blind-from-the-spec programs.
- **Runtime signals.** XO_LEAKCHECK (live allocations at exit), XO_RTSTATS (scheduler, net and fiber counters
  at exit), std/debug's router (/debug/sched live counters, /debug/profile, /debug/heap), debug.mem_* (mapped,
  peak, budget, trim), std/metrics.
- **Fault injection in tests only.** FakeNet.fail_next, MemFs.fail_next and set_disk_usage (spec/std.md), the
  limit and budget goldens (http_server_limits, task_budget, handle_drop_leak).

What is missing: a corpus of more than one service; a differential of a service's responses across backends;
soak runs on the offload hosts with a verdict; chaos against a live process (resets, slow clients, a full
disk, memory pressure, kill and restart); fuzzing of a service end to end; a live count of allocations and
handles (leakcheck reports only at exit, and is skipped on `os.proc.exit`); no refcount cycle detector; and
crash bundles from native builds (spec/core.md 3644: native builds write a fault trace to stderr, no
`.xocrash`).

## Decision

### 1. The corpus

Eight services under a new top-level directory `stability/` in this repository, each a normal Xo module with
its own xo.mod, built only from std (no `use go`, so all three backends build it):

| app | what it exercises | origin |
|---|---|---|
| notes | JSON REST with auth, sql, a purge timer, graceful shutdown; persistence added | examples/notes-service, extended |
| shortener | redirects, a counter per link, persistence to a file (append log plus snapshot), restart recovery | written blind from the spec |
| jobs | job queue: enqueue over HTTP, a worker pool of tasks, retries with backoff, deadlines, cancellation | bench/xo v0.4 06-job-queue, grown into a service |
| chat | HTTP/2 and WebSocket rooms: broadcast fan-out, slow consumers, backpressure | written blind from the spec |
| files | static file server: ranges, ETag and If-None-Match, listings, large files, TLS | a faithful port of a small permissively licensed Go file server (license checked in phase 2) |
| proxy | reverse proxy: HTTP/1.1 and h2 upstreams, streamed bodies, timeouts, retries, header rewriting | a faithful port of a small permissively licensed Go reverse proxy |
| logs | text-heavy CLI: parse, filter and aggregate large log files (no network) | bench/xo v0.6 10-log-pipeline, grown |
| cron | scheduler: cron expressions, timers, overlapping runs, a persistent run history | written blind from the spec |

The blind apps are written by a fresh agent given only spec/core.md, spec/std.md and a task text, as
bench/paper does; their NOTES.md of ambiguities feeds the spec. The ports keep the Go original beside them as
the twin and the oracle. The rest grow existing bench solutions. Every app has a Go twin, which the load
section measures against.

### 2. The harness, per app

One Go program, `stability/harness` (bench/soak's client generalized; Go, so it does not share the runtime
under test), reads each app's `stability/APP/app.toml` (build and start, ports, a readiness URL, a traffic mix,
a scenario file, a chaos list, fuzz targets) and runs five checks:

1. **Differential.** Builds the app with `--backend=go`, `arm64` and `llvm` (and amd64 on the amd64 host),
   starts each, replays the app's scenario (a recorded request sequence, with fixed clocks and seeds through
   environment knobs), and diffs status, headers (minus Date and others listed per app) and bodies, as
   parity.sh does; the Go twin is a fourth column where its responses are meant to match. CLI apps compare
   stdout and exit status over an input corpus.
2. **Load.** jitter's open loop loadgen with the app's mix at fixed rates and at saturation: req/s, p50 to
   p99.9, errors, CPU per request and peak RSS, against the Go twin and against the previous run, judged by
   pads.py's rule (slower only when the ranges over code placements do not overlap).
3. **Soak.** bench/soak's sampler and drift verdict over 2 hours (and a weekly 8 hour run of one app), plus
   live allocations and open handles from /debug/sched once the runtime provides them (section 4).
4. **Chaos.** A harness-side TCP proxy between client and app that injects resets mid-request, half-open
   connections, slow readers and writers, and truncated or malformed requests; a full disk (the app's data
   directory on a small dedicated filesystem where the host allows a mount, else a std fs fault knob added for
   this); memory pressure (XO_MAX_MEM at 1.2 times the soak's observed peak, and a cgroup memory.max where
   available); and kill -9 then restart for the persistent apps, checking that the recovered state matches what
   was acknowledged. The app must keep serving well behaved clients, with no fault, hang or leak.
5. **Fuzz.** Route and body mutation from the scenario's grammar (Go's fuzzing engine driving the live app,
   as TestHTTPFuzz drives a std program), under XO_LEAKCHECK, with a corpus kept per app in
   `stability/corpus/APP/`.

### 3. Failures become regression tests

The harness minimizes each failure (shrinking the request sequence, then the inputs, as `xo test` shrinks
contract cases) and saves it with its build and environment under `stability/failures/DATE-APP-CHECK/`. It
becomes a regression test where it belongs: a golden under internal/llvm/testdata/golden for a compiler or
runtime bug, a std module test for a std bug, a test in the app for an app bug. The program's owner (Xo Lang 2)
reports each one to the lead with the minimized repro, and the lead routes it to the owning agent; the fix
lands with its regression test and, for std, a std/changes entry (decision 0276).

### 4. Runtime work it needs first (each its own small change)

- A live count of allocations and of open handles (fds, sockets, timers, tasks) in /debug/sched, so a soak
  sees growth while it runs; leakcheck at exit stays.
  Partly done 2026-11-05 (Xo Lang 2): /debug/sched reports `heap_live` (the live heap blocks) and
  `fds_open` (the open descriptors); sockets, timers and tasks are not counted apart yet.
- Native crash bundles, or at least the fault trace written to a file named by an environment variable, so a
  chaos or soak failure on a native build keeps more than stderr.
  Done 2026-11-05 (Xo Lang 2): `XO_CRASH_FILE=PATH` (std/native/llrt/crashfile.xo), Linux and macOS, the
  hardware fault report and an uncaught fault's report appended to PATH; the harness's soak sets it.

### 5. Where it runs and what it costs

On the offload hosts, through offload.sh and heavyd's lanes, never on the Mac. Host 2 (18 CPUs) takes the
soaks and fuzzing; host 1 (12 CPUs, the measured HTTP host) takes load, so load figures stay comparable with
release-vs-go's; the amd64 host takes the amd64 differential only (its WSL loopback is too slow for load).

Estimates per app, to be replaced by measured times in phase 1: differential 5 minutes, load 10, chaos 15,
fuzz 30, soak 120. A daily pass of the four short checks over eight apps is about 8 host hours (under 3 hours
of wall time in host 2's three slots); two soaks a night add 4 host hours; the weekly 8 hour soak adds 8 a
week: about 13 host hours a day in all. The short checks run in leases of at most 10 minutes (the lead's
2026-11-05 guidance); each soak takes one long lease at night, between 22:00 and 07:00 local time, with
OFF_MAX_MIN raised for that job only (owner, 2026-11-05). Results go to `bench/results/stability-DATE.md` (one table per app and check), and a
failing check opens an item on the board.

### 6. Phases

1. The harness and the starter app: notes with persistence, the differential on three backends, load, and a
   2 hour soak on host 2; the two runtime signals. Exit: one clean soak verdict committed.
2. shortener (blind) and files (port); chaos for all three.
3. jobs, chat and proxy; fuzz for all.
4. logs and cron; the nightly schedule (only with the owner's go: no cron or launchd job is installed without
   it).

The starter app is notes: it is the only real service today, bench/soak already drives it, and it needs only
persistence to exercise restart recovery.

## Consequences

- Bugs that need hours, load or misbehaving clients to appear get a standing place to be found before a
  release rather than after.
- Each app is also a realistic program for the language: the blind apps keep surfacing spec gaps, and the
  ports show where Go idioms do not carry over.
- The repository grows by eight services and a harness (a rough estimate: 8,000 to 12,000 lines of Xo and
  2,000 of Go).
- About 13 offload host hours a day once every phase runs; the lead schedules them around measurements.
- The apps must keep building as the language changes: a change that breaks one updates it in the same push,
  as goldens are updated today.

## Alternatives

- **Its own repository.** Keeps the main repository smaller, but the apps would build against a released xo
  instead of HEAD, so a regression would be found a release late, and every language change would need a
  second push. Rejected; the directory can move out later if it grows.
- **More goldens instead of services.** Goldens are short by design; the bugs this targets need time and load.
- **Soak only, no chaos or fuzz.** Cheaper, but misbehaving clients exercise the error paths a well behaved
  load never reaches.
- **The Mac.** Rejected: long runs go to the offload hosts (decision 0423 and the owner's 2026-10-30 rule).
