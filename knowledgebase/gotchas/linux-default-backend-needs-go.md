# xo build with no --backend needs Go on Linux

Status: Observed (2026-10-16, Docker linux/arm64, decision 0243 work)

## Symptom

In a container with no Go toolchain (scripts/docker-linux.sh's image),
`xo build .` with no `--backend` fails at once:

```
xo: go env GOROOT: NotFound(cmd: "go")
```

## Cause

The default backend is the Go backend on every host (selfhost/xo
gobackend.xo runs `go env GOROOT`), so a plain build needs `go` on PATH
even where the native backends (`--backend=arm64`, `--backend=llvm`)
would build the program alone. XO_ZONEINFO does not help: the failure is
the backend's, not the time zone data's.

## Fix

Pass `--backend=arm64` (or `llvm`) on a host without Go, as
scripts/linux-goldens.sh does. Which backend is the default is a
toolchain-wide decision (the lead has it with the owner as part of the
"native out of preview" milestone); until then, scripts and docs that
target Go-less machines name the backend.
