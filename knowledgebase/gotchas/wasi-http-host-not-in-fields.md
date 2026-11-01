# wasi:http gives Host as the authority, and a stub host hides it

Status: Observed (2026-11-01, Xo Lang 4, decision 0447)

**Symptom.** A component's `req.host()` and `req.url.host` were empty under
`wasmtime serve`, while TestWasmComponentHTTP's stub host passed.

**Cause.** wasmtime leaves Host out of the request's fields and gives it through
`[method]incoming-request.authority`. The stub host, written from the canonical
ABI, passed Host as a field, so it could not show this. The same first version
kept only a repeated field's first value and left the path escaped, which a
stub written by the same author agreed with.

**Fix.** serve_component reads the authority. Compare a component with the native
server rather than with expectations written by hand: TestWasmComponentGoldens
runs one request script against both, on a host that has wasmtime (the Mac skips
it).
