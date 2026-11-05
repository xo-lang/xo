# 0524: Request arenas on the native backends

Status: Accepted (owner, 2026-11-05; drafted by xo-lang-4): native request arenas are not built now. Extends decision 0066 to
arm64, amd64 and LLVM. The explicit `arena scope` block (spec core 5.5) stays P2.

## Context

Decision 0066 made request scoped arenas the default on the Go backend: the request's
tables and body (R), a response built by http.json/http.text as the handler's direct
result (S), and a values()/keys()/entries()/to_list() List consumed in its own expression
(T) live in a per-request arena reset after the response. There they gave +40% req/s at
32 workers and 3.6x fewer GC cycles (bench/results/perf-arenas-2026-10-08.md:140-155),
because the Go GC's cycles follow bytes allocated.

The native backends (the default since decision 0074's successors) ignore arenas
(compiler/gen/arena.xo is the Go generator's; no native arena exists in std/native/llrt).
Native memory is reference counted: no GC cycles, and a T temporary is already freed at
its last use. What an arena can save natively is the allocator's alloc/free calls and
the RC traffic on R, S and T objects.

Re-measure on main 56e08ee4b (bench/results/native-arena-gap-2026-11-05.md; linux/arm64 host 1, bench/perf/http run.sh, REPS=3, 32
workers, 10 s; medians):

| Config | Req/s | p50 us | p99 us | Peak RSS MB | CPU us/req |
|---|---:|---:|---:|---:|---:|
| go | 125,138 | 186 | 1,148 | 19.1 | 27.17 |
| xo native arm64 | 190,552 | 134 | 847 | 13.1 | 11.54 |
| xo native LLVM | 189,114 | 135 | 842 | 12.8 | 11.82 |

Native already serves 1.51x Go's requests at 0.42x its CPU per request with no arena
(run.sh's xo and xo-arenas configs now build natively and match these rows; the Go
backend's arena gain, 0066's 1.40, is no longer in run.sh's default table).
Where a native request's CPU goes (XO_PROF=cpu at 2000 Hz on srv-xo-arm64 under the
same load; 32,768 samples kept, 21,500 lost to a full table, so shares are indicative):
the write syscall (hconn.send_at) 24.8%, sock.read
9.1%, futex wake and poll waits about 10%, fiber switches 3.8%; the allocator and RC
(every alloc., drop and inc node with no cutoff: drop Str 268 ms, inc thunk 189 ms, drop
List[Str] 150 ms, free_push 150 ms, alloc 110 ms, ...) 1,438 of 16,384 ms, 8.8%; the
idle workers' spin in pool.find_work reading the clock (sysio.mono_ns, 11.5%, 79% of it
from find_work; the clocksource is arch_sys_counter) is waiting, not request work. Dispatch with the
handler is 16% cumulative, header parsing 4.9%.

## Decision (proposed)

Do not build native request arenas now. The most they can save is the allocator and RC
share of a request's CPU (8.8%), before the bump allocation, the reset and the
poison check they add; the native server already serves 1.51x Go's requests at 0.42x its
CPU per request. The lever this profile ranks first is the write path (24.8%, about
three times anything an arena can reach), then the read path (9.1%) and the scheduler's
wakes and waits (about 10%).

Decision 0066 stays as is for the Go backend. The design below is kept for the day a
workload's profile shows allocation and RC above about 20% of its CPU (a JSON-heavy
handler, a large request table); that workload's profile reopens this.

Kept design (not to be built now):

1. Placement analysis on the SSA path: port confines_req, mark_resp_calls and the T
   producer/consumer marks (compiler/gen/arena.xo) to compiler/ssa, on the checker's
   records (the same inputs the Go generator reads), producing per call site an
   "arena" flag. Same rules as 0066, so its audit table holds unchanged.
2. Runtime: a per-request arena in std/native/llrt (bump chunks, 64 KB, a small free list
   per worker, reset after the response is written, before the next request's parse).
   Arena objects carry the IMMORTAL count (obj.xo): every RC inc/dec and release already
   skips it, and a write copies, so codegen gains no new RC path; the analysis only routes
   the allocation (an arena variant of the std producers and http.json/http.text, and the
   request parse in hserve for a confined route).
3. When the analysis is wrong: an escaped arena object looks immortal and is read after
   reset. XO_ARENA_CHECK=1 (and --race builds) poisons each object's count and payload at
   reset; the RC and load paths that meet the poison count fault with "arena object used
   after its request ended" and the route. The differential tests of 0066 (arena_escape,
   arena_holes, both ways against one want.txt) run on arm64 and LLVM, plus with the check on.
4. --no-arenas parity: the flag turns the SSA analysis off; goldens build both ways
   (TestGoldenRun and a NoArenas twin) against the same want.txt.

## Measurement plan (when reopened)

bench/perf/http on linux/arm64 (ab-host.sh BASE NEW NAME -- bench/perf/http ...), arm64
and LLVM backends, arenas vs --no-arenas: req/s, p99, CPU per request, max RSS;
allocations per request. Darwin only for darwin claims. Each stage carries its --ab.

## Consequences / Alternatives
- (alt) allocator-side arena mode around the handler (0435's heap record ARENA mode):
  needs no analysis but frees nothing early and was a negative result for the front end.
- (alt) runtime-only arena for R inside hserve (no compiler change): smaller win, safe only
  for confined routes, which needs the analysis anyway.
- (alt) no native arenas: if the re-measure shows the native gap is allocation-light.
