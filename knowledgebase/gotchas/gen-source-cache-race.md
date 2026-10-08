# Parallel golden tests crashed in gen with concurrent map writes

Status: Observed (2026-10-08, `go test ./internal/gen -run 'TestGolden$|TestGoldenNoMono'`)

Symptom: `fatal error: concurrent map writes` in `gen.(*gen).source`
(internal/gen/fn.go) during the golden tests, once.

Cause: `source` (the source text of contracts and `expect` messages)
cached files in a package level map without a lock, and the golden
programs are generated in parallel (`t.Parallel()`), several of them
with contracts. Latent since the cache was added; it needs two programs
with contracts generated at the same moment.

Fix: a mutex around the cache (`sourceMu`). Package level state in the
generator must be safe for concurrent `Generate` calls.
