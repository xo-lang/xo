# Benchmark run output breaks `go build ./...`

Status: Observed (2026-10-09)

Symptom: `go build ./...` fails with `package benchtask/pool is not in std`
from `runs/codex/planted/...`.

Cause: `bench/harness` writes model outputs to `runs/<model>/` (gitignored),
and those directories hold Go files with their own module paths but no
`go.mod`, so `./...` from the repo root includes them.

Fix: while a `runs/` tree exists, list packages without it:
`go test $(go list -e ./... | grep -v /runs/)` (bash; in zsh word split with
`${=PKGS}`). Do not delete `runs/`: it is the user's benchmark data.
