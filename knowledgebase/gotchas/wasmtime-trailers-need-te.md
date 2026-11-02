# wasmtime serve sends response trailers only when the client asked for them

Observed (2026-11-02, decision 0447).

- Symptom: a wasi:http component's `http.stream` response set a trailer
  (`outgoing-body.finish` with some(fields)), the stub host saw it, but a
  Go client of `wasmtime serve` got the body and an empty
  `resp.Trailer`; no error anywhere.
- Cause: wasmtime serve's HTTP/1 server is hyper, which sends trailer
  fields only to a request that sent `TE: trailers`, and only the names
  the response's `Trailer` header declared. Go's client sends no TE.
- Fix: a test of response trailers through wasmtime sends `TE: trailers`
  and the route declares its names (`with_trailer_names`), as
  TestWasmComponentServe's GET /trailer does. A component cannot make the
  host send undeclared trailers; declare them.
