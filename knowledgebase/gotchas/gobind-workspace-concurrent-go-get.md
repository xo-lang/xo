# Concurrent binds ran go get in one workspace

Status: Observed and fixed (2026-10-30)

## Symptom

`error XO1101: cannot bind Go package `github.com/golang-jwt/jwt/v5`: go:
updating go.mod: existing contents have changed since last read` in one
of several builds that bound the same Go module at once
(internal/gen TestGoldenNoMono/go_jwt_mapclaims beside TestGolden's run
of it, which share the test binding cache; two terminals or an editor
and a test run of one program do the same). It passed alone.

## Cause

Every bind into a binding directory runs `go get` and `go list` in its
one workspace module (`<dir>/_mod`, internal/gobind ensureWorkspace), and
go get rewrites that go.mod; Go refuses to write a go.mod that changed
after it read it. The binding files themselves were already safe (each
written to a temporary name and renamed, writeAtomic).

## Fix

Bind holds an exclusive lock on `<workspace>/.xo-bind.lock` (flock, or
LockFileEx on Windows, released by the kernel if xo-gobind dies) for the
whole bind; the bindings of the packages its types come from run inside
it through bind and loadOrBind, which do not lock again (a second flock
on its own descriptor blocks, even in one process). Test: internal/gobind
TestConcurrentBinds (eight local modules at once). The Go backend's
build tree had the same shape: concurrent-builds-shared-build-dir.md.
