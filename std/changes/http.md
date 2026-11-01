# std/http changes

## 2026-11-01: feature: http.serve_component builds a whole wasi:http component, with no other tools (decision 0447)

- Kind: feature
- Commit: this commit
- Found by: xo-lang-4, decision 0447, the stage the owner accepted 2026-11-01
- Go reference: none (Go's wasip2 handlers come from componentize-go or TinyGo with ydnar/wasi-http-go)
- Test: internal/selfhost TestWasmComponentServe (testdata/wasm_http_serve under `wasmtime serve`, after `wasm-tools validate`; skips where they are not installed), TestWasmComponentHTTP (the core module under a stub host in Node)
- Timing: hello program under `wasmtime serve`, linux/arm64, 32 connections, 3 rounds, median: Xo 81,681 req/s, 257,324 bytes; Rust 89,238 req/s, 79,127 bytes; TinyGo 29,038 req/s, 2,910,541 bytes (ADR 0447)

`xo build --backend=wasm` of a program calling `http.serve_component` now
writes the component itself: wasm-tools and the preview 1 adapter are gone
from the path, and `wasmtime serve` takes the output as it is, with only the
wasi:http/proxy world. The runtime's system calls use WASI 0.2 in place of
preview 1: stdout, stderr, clocks and random work; arguments and the
environment are empty; file operations and reads of stdin return ENOSYS. A route that faults ends the
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
