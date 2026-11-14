# A std module generated into std/rt cannot call a merged std module the Go backend builds as a program package

Status: Observed (2026-11-14, Xo Lang 3, std/url's URL type)

## Symptom

`xo test --std-module=url` passed on arm64 and LLVM, but after
`go run ./scripts/stdsync`, `go build ./std/rt` failed:
`std/rt/std_gen_url.go: undefined: MISSING_PKG_x_std_pathF_clean`.

## Cause

stdsync generates some std modules into the Go backend's runtime
(std/rt/std_gen_*.go, rt_hosted). A call from one of them reaches
another module through gen.xo's pkg_ref: a std module the runtime hosts
resolves (netip did), but std/path is merged and listed in
std/go_compiled.txt, so the Go backend builds it as a program package,
which the runtime cannot import; pkg_ref writes a MISSING_PKG_ name.

## Fix

From a module stdsync hosts, call only modules the runtime hosts too, or
keep a private copy of the small function (std/url's clean_path is
path.Clean). After any new `use` in such a module, run stdsync and
`go build ./std/rt`; grep the generated file for MISSING_PKG.
