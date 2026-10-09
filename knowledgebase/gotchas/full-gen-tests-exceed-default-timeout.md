# Full `go test ./...` fails internal/gen at the 10 minute default timeout

Status: Observed (2026-10-09)

Symptom: `go test ./...` (without `-short`) ends with `panic: test timed
out after 10m0s` and `FAIL github.com/xo-lang/xo/internal/gen 600s`,
with goroutine dumps of `go build` subprocesses and no failing test.

Cause: the full gen suite builds every golden program three times
(TestGolden, TestGoldenNoMono, TestGoldenNoArenas) plus race and replay
builds. Alone on an idle machine it fits; with another session's
`gen.test` running at the same time it took 1450 s.

Fix: run the full suite with a longer timeout,
`go test -timeout 40m ./...`, or rerun internal/gen alone with
`-timeout 40m` to confirm a timeout is only load.
