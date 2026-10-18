# reduce-diff.py's `go` backend was the native one

Status: Observed and fixed (Xo Lang 2, 2026-10-18).

## Symptom

`python3 -I scripts/reduce-diff.py XO go arm64 IN.xo` on a program the
arm64 backend failed to compile stopped at once with "the input does not
build with both backends", although the Go backend built it.

## Cause

The reducer passed no `--backend` for `go`, relying on the Go backend
being xo's default. Since decision 0248 the default is native (arm64 on
arm64 hosts), so its "go" side was arm64 and every go/arm64 reduction
since compared arm64 with itself.

## Fix

The reducer names every backend (`--backend=go` too). Name the backend
explicitly in any script or test that means the Go backend.
