# std/http changes

## 2026-11-02: perf: a slow keep-alive connection stops lingering before it parks (decision 0449)

- Kind: perf
- Commit: this commit
- Found by: Xo Lang 2's memory review of the 20 ms linger
- Go reference: none
- Test: Xo Lang 2's linger-mem runs (think time 10 and 30 ms at 2,000 and 10,000 connections, a 40,000 connection burst) and bench/perf/jitter /alloc on the linux/arm64 host
- Timing: 30 ms think time at 2,000 connections: RssAnon 70 to 71 MB to 56 to 63 MB, req/s 64k to 52 to 56k; busy connections unchanged (178k req/s); /alloc req/s 1.072 at 32 connections and 1.037 at 256 against main in the same job, CPU a request 0.965 and 0.961 (bench/perf/jitter, first linux/arm64 host, 3 rounds, 2026-11-02, both on main with a1 P, 25d23d18e, since reverted)

A connection whose 20 ms linger timed out parks at once on its later
idles, until it comes back within 20 ms of its last park, so slow
keep-alive traffic no longer holds a stack for 20 ms of every gap.

## 2026-11-02: fix: an HTTP/2 request whose trailers arrive late no longer hangs its streaming handler

- Kind: fix
- Commit: this commit
- Found by: Xo Lang 1, http_trailers under load (1 run in about 1,900, 2026-11-02)
- Go reference: net/http's http2 server (processTrailerHeaders ends the body and wakes its reader)
- Test: internal/llvm TestH2Diff's raw "late trailers" case (HEADERS and DATA, a 300 ms pause, then the trailers): the native server never answered before this fix
- Timing: none

A trailing HEADERS frame that ended a request's stream marked the body
done but did not wake a `handle_stream` handler already waiting for more
of it (a DATA frame's END_STREAM did), so a request whose trailers came
in a later read than its last DATA hung until the client gave up. The
code is the same in v0.35.0, where a client sending HTTP/2 request
trailers (a gRPC client, for one) to a `handle_stream` route could hit
it.

## 2026-11-02: perf: an idle HTTP/1.1 connection parks only after 20 ms without a request (decision 0449)

- Kind: perf
- Commit: this commit
- Found by: xo-lang-4, the /alloc regression bisect for 0.36 (Lead, 2026-11-02)
- Go reference: none (net/http keeps a goroutine per connection and never parks)
- Test: bench/perf/jitter /alloc on the linux/arm64 host (alloc-points, exclusive, 3 rounds) and the idle connection tests (bench/perf/idleconn at 2,000 and 40,000 connections)
- Timing: /alloc req/s against v0.35.0, c32 0.775 to 1.023, c256 0.651 to 1.027; CPU a request 1.327 to 0.972 and 1.536 to 0.973 (linux/arm64, 2026-11-02)

A plain HTTP/1.1 keep-alive connection decided to park from the gap since
its last response (2 ms). Under load on Linux, readiness reaches a
connection one poll later, so busy connections looked idle and parked on
most requests (57% at 32 connections, 91% at 256), each park and resume
costing a closure, the server's parked list and a task. Now the
connection, after its peek finds no bytes, waits on its own stack with a
20 ms timer and parks only when nothing came. Idle connections still
park, 20 ms later. TLS and HTTP/2 connections are unchanged.

## 2026-11-02: feature: handle_stream and trailers in a wasi:http component (decision 0447)

- Kind: feature
- Commit: this commit
- Found by: xo-lang-4, decision 0447's next stage (owner, 2026-11-02)
- Go reference: net/http's Request.Body and Request.Trailer, and the TrailerPrefix response header
- Test: internal/selfhost TestWasmComponentHTTP (the stub host: a body in pieces, a declared and an undeclared trailer, a streamed response in two writes with a trailer and a forbidden one) and TestWasmComponentServe (wasmtime serve: 100,000 bytes in several reads, a body in three pieces 50 ms apart read as they arrive, a request trailer, a response trailer, a 1 MiB streamed response, a route that answers after one read)
- Timing: none (the host holds the connection)

On `--targets=wasip2/wasm`, a `handle_stream` route's `BodyReader` reads the
request body from the host's input-stream as it arrives, in place of the whole
body read first, and `BodyReader.trailers()` has the trailers the request's
`Trailer` field declared (`incoming-body.finish`), as the native server keeps
them. A route that answers without reading to the end leaves the rest to the
runtime. An `http.stream` response goes to the host as it is written: its
head with the first write or flush (a missing Content-Type detected from
those bytes, as net/http does), its body through the output-stream, then
`BodyWriter.set_trailer`'s trailers through `outgoing-body.finish`, names a
trailer may not carry dropped (a host may send only declared trailers, and only to a
client that sent `TE: trailers`, as wasmtime serve does). In process (`Router.call`) the stream is
collected as before, its trailers as Go TrailerPrefix headers. A client's
request trailers still fail.

## 2026-11-02: feature: http.Client in a wasi:http component (decision 0447)

- Kind: feature
- Commit: this commit
- Found by: xo-lang-4, decision 0447's next stage (owner, 2026-11-01)
- Go reference: none (Go has no wasi:http client in net/http)
- Test: internal/selfhost TestWasmComponentClient (wasmtime serve against a Go server)
- Timing: none (the host holds the connection; no native comparison)

On `--targets=wasip2/wasm`, `Client.send` and `Client.open` go through
`wasi:http/outgoing-handler`, with the same redirects, Net.only checks,
Authorization stripping and gzip as native. Limits of this stage: a request
blocks the instance while it runs; request and response are held whole, so `open`
reads its response before it returns and a piped body is read before it is sent;
each timeout is the host's per-phase one, not one deadline across redirects;
request trailers fail; `with_roots` and a client with `with_cert` fail, since the
host does TLS. A component that does not reach the client imports nothing new.

## 2026-11-01: fix: an HTTP/2 connection whose idle park was refused re-reads before it waits (decision 0449 stage 1c)

- Kind: fix
- Commit: this commit
- Found by: TestH2Diff (a raw case answered nothing for 30 s under load, native "open" where Go answers)
- Go reference: none (net/http's http2 server blocks in a goroutine and has no park)
- Test: internal/llvm TestH2Diff, 3 rounds of 4 concurrent runs on the Mac: 12 of 12 clean (about 39 to 41 s each), where the default failed about 1 in 4 and ran 79 s once; XO_THREADS=1 and parking off were clean
- Timing: none

hconn.park_ready clears the socket's read readiness and peeks; bytes the peek found refuse the park, but no edge announces them any more, and the h2 loop waited at once, so the connection sat silent until a deadline. The loop now reads once more after a refused park.

## 2026-11-01: feature: http.serve_component builds a whole wasi:http component, with no other tools (decision 0447)

- Kind: feature
- Commit: this commit
- Found by: xo-lang-4, decision 0447, the stage the owner accepted 2026-11-01
- Go reference: none (Go's wasip2 handlers come from componentize-go or TinyGo with ydnar/wasi-http-go)
- Test: internal/selfhost TestWasmComponentGoldens (18 native goldens answered the same under `wasmtime serve`), TestWasmComponentTargets, TestWasmComponentServe (testdata/wasm_http_serve under `wasmtime serve`, after `wasm-tools validate`; skips where they are not installed), TestWasmComponentHTTP (the core module under a stub host in Node)
- Timing: hello program under `wasmtime serve`, linux/arm64, 32 connections, 3 rounds, median: Xo 83,339 req/s, 281,733 bytes; Rust 89,794 req/s, 79,127 bytes; TinyGo 29,114 req/s, 2,910,541 bytes (ADR 0447)

`xo build --targets=wasip2/wasm` of a program calling `http.serve_component`
writes the component itself (XO1514 on wasip1/wasm, XO1515 for files, processes
or sockets, XO1516 with no router): wasm-tools and the preview 1 adapter are gone
from the path, and `wasmtime serve` takes the output as it is, with only the
wasi:http/proxy world. The runtime's system calls use WASI 0.2 in place of
preview 1: stdout, stderr, clocks and random work; arguments and the
environment are empty; a read of stdin faults. A request now reaches the router
as the native server's does (escaped path, repeated fields, Host), the response
keeps repeated fields and sniffs a missing Content-Type as the server does, and
`http.stream` works, going out whole. A route that faults ends the
request with an error from the host and the next request gets a new instance.

## 2026-11-01: fix: a parked idle connection answers its next request on Windows (decision 0449)

- Kind: fix
- Commit: this commit
- Found by: the Lead's tier, windows/amd64 sample at 28a0dbcf2 (http_server, http_tls)
- Go reference: none (the runtime's own poller)
- Test: goldens http_server and http_tls on windows/amd64 (`OFFLOAD_ENV=offload-win.env amd64-run.sh REV NAME http_server http_tls`)

On Windows the poller watches only descriptors someone waits on, and a
connection parked between requests counted as nobody, so its next request
was never seen and the client timed out.
A parked connection is now watched for reads, with the AFD poll and with
WSAPoll. Other systems are not affected: their poller registers every
descriptor when it opens.
## 2026-11-01: fix: no continuation record outlives a stopped server (decision 0449)

- Kind: fix
- Commit: this commit
- Found by: the Lead's tier, TestGoldenTasks/http_conn_fault (1 live at exit)
- Go reference: none (parking is Xo's own)
- Test: golden http_conn_fault, repeated: 0 of 600 runs leak in its two cut down forms that leaked 8 to 9 and 5 of 200 before

An HTTP/2 connection that fired, or parked, after the stopping server's
resumer had taken its last look left its 64 byte record on the server's lists
with no one to free it, so XO_LEAKCHECK sometimes reported 1 live block at
exit. The server now frees what is left when it is freed.

## 2026-11-01: feature: http.serve_component serves a wasi:http component's request, wasm (decision 0447)

- Kind: feature
- Commit: this commit
- Found by: xo-lang-4, decision 0447 step 1
- Go reference: none (Go's wasip2 handlers come from componentize-go or TinyGo with ydnar/wasi-http-go)
- Test: internal/selfhost TestWasmComponentHTTP (testdata/wasm_http: a stub wasi:http host under Node, an instance per request and one for every request)
- Timing: the hello handler under wasmtime serve, linux/arm64 offload host, one 20 s loadgen run at 32 connections: Xo 75,620 req/s, Rust 85,410, TinyGo 28,109 (Go has no row: standard Go needs componentize-go); decision 0447 Step 1 results

A program whose main builds a router and calls `http.serve_component(r)`
builds, on the wasm backend, as the core module of a `wasi:http/proxy`
0.2 component: it exports `wasi:http/incoming-handler@0.2.0#handle` and
`cabi_realloc` in place of `_start`, runs main for each request, and
answers through `Router.call`. The request's method, path with query,
fields (keyed by Go's canonical name, a repeated name's first value) and
body reach the router; the response's status, fields and body go back.
Step 1 keeps the preview 1 imports of the rest of the runtime, so the
host wraps the module with the preview 1 adapter (wasm-tools component
embed and new). Off wasm, serve_component faults.
