# A handler that lets its request escape gets no arena

Status: Anticipated (2026-10-08, `xo build --arenas`)

Symptom: building with `--arenas` makes no difference for one route, or
a service gets slower than expected with arenas on.

Cause: the request of a route is arena memory only when internal/gen
proves its handler confines it (`confinesReq` in `internal/gen/arena.go`):
the handler must be a closure literal passed to `r.handle`, and it may use
`req` only as `req.json[T]()`, `req.param(..)`, `req.param_int(..)`,
`req.query(..)`, `req.header(..)`, `req.method`, `req.path`,
`req.remote_ip`, or `req.url.scheme/host/port/path`, never inside a
nested closure. Passing `req` to a helper function, reading `req.headers`
or `req.body` directly, capturing `req` in a closure, or a router with
middleware (`r.wrap`) makes dispatch copy the request to the heap
(`heapCopy`), which costs a little more than no arena at all. The same
holds for responses: only `http.json`/`http.text` whose value is directly
the handler's result are arena responses; `http.json(..).with_header(..)`
or a response bound to a `let` first is a heap response.

Fix: extract what the handler needs at the top (`let id =
req.param_int("id")`, `let body = req.json[T]()`) and pass the values,
not the request, to helpers; return the response expression directly.
Check the generated Go for `rt.Http_Router_arenaLast` after the route's
`Http_Router_handle` and for `Http_json_arena`.
