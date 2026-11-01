# std/http changes

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
