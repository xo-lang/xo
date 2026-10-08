# The root go.mod absorbs Go code under bench/

Status: Observed (2026-10-07, while adding the P1 parser). Resolved 2026-10-07 by `bench/go.mod` (module `xobench`), which makes `bench/` its own module so the root ignores it.

Symptom: `go test ./...` and `go build ./...` from the repo root fail with
errors in `bench/go/...` or `bench/planted/*/variants/...` (for example
`function main is undeclared in the main package`, or build failures in
planted bug variants), even though the compiler packages are fine.

Cause: the root `go.mod` (`github.com/xo-lang/xo`) owns every directory below
it that has no `go.mod` of its own. Benchmark Go code written by other agents
lands there, and planted bug variants are not meant to build on their own.
Go has no way to exclude a directory from `./...` except a nested `go.mod` or
a name starting with `.` or `_` or equal to `testdata`.

Fix: build and test the compiler with explicit patterns:
`go test ./internal/... ./cmd/...`. The durable fix belongs to whoever owns
`bench/`: give every Go tree under `bench/` its own `go.mod` (as
`bench/go/*/*` and `bench/planted/*/base` already have), for example one
`bench/planted/go.mod` covering the variants.
