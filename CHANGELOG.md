# Changelog

Xo stays on 0.x until the owner declares 1.0 (decision 0089). The toolchain
minor matches the spec draft; the full list of language changes for each
release is the "Changes in 0.N" section of `spec/core.md`.

## Unreleased
- PERF (native arm64): a `var` parameter's memory stays held at a merge
  where one edge passes it in place, the other edges storing their value
  into it (decisions 0148, 0151). A merge after a call that can fault no
  longer loads the whole receiver into registers on every path and
  stores it back word by word. xo's stage 2 __text 38,082,764 to
  36,494,348 bytes (-4.2%). New golden byref_fault_merge.
- Self build: an SSA value is 9 words, was 11 (decision 0448): its position is an
  index into its function's table, sharing a word with its line. Mac peak footprint
  about -3%; max RSS on linux/arm64 unchanged (1.002).
- `xo bench --ab` runs 3 rounds by default, was 10 (decision 0340). `--early` alone
  now has nothing to stop: it needs `--rounds` above 5.
- Self build: the heap counter's peak moves from hollow back to emit (about 4440
  to 4000 MB on the Mac, decision 0448): a part's hollowed text is dropped once
  its object and symbol facts exist, and those facts copy their names rather
  than pin the text. Max RSS on linux/arm64 is unchanged (0.995, noise).

- FIX (Windows, std/http): a server connection parked between requests
  (decision 0449) was never watched by the poller, so its next request
  timed out at the client. It is watched now.
- FIX (std/http): a parked HTTP/2 connection that fired as its server stopped
  leaked a 64 byte record (decision 0449).
- std/http (wasm): `http.serve_component(r)` builds a program as the core
  module of a `wasi:http` 0.2 component (decision 0447 step 1): its
  `handle` export runs main per request and answers through the router,
  with the request's method, path, query, fields and body and the
  response's status, fields and body. Wrapped with the preview 1 adapter,
  it serves under `wasmtime serve`.
- FIX (wasm): the scheduler's task list is empty again after main ends,
  so a module whose main runs more than once on one instance (a wasi:http
  component) no longer traps on the freed list.
