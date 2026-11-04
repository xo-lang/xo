# The binding format number lives in two places

Status: Observed (Xo Lang 2, 2026-11-04), guarded by a test

Symptom: after bumping `gobind.Version` (internal/gobind/gobind.go) for a
change to generated bindings, every `use go` test in cmd/xo failed with
XO1101 "cannot bind Go package ...: xo-gobind at ... wrote binding
format 36, this xo reads 35", followed by XO0601 for each bound name.
internal/gobind's own tests passed.

Cause: xo checks the format of a binding against its own constant,
`GOBIND_VERSION` in compiler/check/goimport.xo, so that a binding from
another binder version is treated as stale (bindmiss.xo). The two
numbers must move together; the Go side alone was bumped.

Fix: bump both in the same commit. internal/gobind
TestVersionMatchesChecker (4fd274f57) reads goimport.xo and fails
when they differ, in `go test -short`. Since GOBIND_VERSION is in
compiler/, a bump is a fixed point set change: the gate builds xo
(xobuild) and runs `go test ./cmd/xo -run 'TestUseGo|TestGobind'` with it.
