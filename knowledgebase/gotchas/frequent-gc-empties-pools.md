# A GC every millisecond empties every sync.Pool, so bytes per request grow with GC rate

Status: Observed (2026-10-08, bench/perf/http, Go 1.26.1, Apple M-series, 12 P)

Symptom: the Xo server built with `--no-arenas` allocated 4,137 bytes
per request at `GOMAXPROCS=1` and 5,295 at 12 (Go: 2,901 and 3,016), with
the same request mix. The 12 P runs collected every 1.3 ms. Its allocation
size histogram (`runtime/metrics` `/gc/heap/allocs-by-size:bytes`) had
objects of 1.5 to 8 KiB that a single request never allocates.

Cause: every sync.Pool is emptied by two GCs (the victim cache keeps one
generation), and each P has its own pool slot. At a GC every millisecond
the pools that net/http relies on per request miss all the time, and a
miss allocates:

- `io.Discard`'s 8 KiB copy buffer (`io.init.func1`): net/http drains a
  request body the handler did not close, with
  `io.CopyN(io.Discard, body, 256<<10)` in `chunkWriter.writeHeader`, even
  when the body was already read to its end (it checks `closed`, not
  `sawEOF`). That also allocates an `io.LimitReader` every request.
- the per P slot arrays of each pool after a cleanup (`sync.(*Pool).pinSlow`,
  1.5 KiB at 12 P), the 2 KiB response bufio writers, `fmt`'s printer
  (used by `chunkWriter.Write` for each chunk header of a response larger
  than net/http's 2 KiB buffer, which goes out chunked).

More allocation means more GCs, which means more misses: a feedback loop
that grows with GOMAXPROCS.

Fix: the server closes the request body right after reading it (std/rt
http.go `serve`), so net/http skips the drain, and sets `Content-Length`
on any non streamed body over 2 KiB, so it is not chunked. Measured:
`--no-arenas` 5,300 to 4,700 bytes per request and 80k to 92k req/s at
default GOGC; with arenas (whose body reader already reads to EOF) 3,570
to 3,500 bytes. A larger heap goal (decision 0065) removes the loop's
cause. To look for more: `XO_MEMPROFILE` with `XO_MEMPROFILERATE=2048`,
then `go tool pprof -traces -focus 'pinSlow|io.init.func1'`.
