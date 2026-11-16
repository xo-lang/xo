# Concurrent Go backend builds on a cold Go cache each compiled the runtime

Status: Observed 2026-11-16 (Xo Lang 2)

## Symptom

The hourly `go test -short` on main timed out (2026-11-16 05:53, 20f38df45),
with TestDevGoldensGo builds hitting their 120 s limit. On the Mac,
`go test -short ./internal/gen` with an empty `XO_GOCACHE` ran 26 builds
at once (load 60, 19 GB of compilers) and was stopped at heavy.sh's 32 GB
limit.

## Cause

Every Go backend build compiles the generated runtime package
`xoprog/rt` (about 240 files, 25 to 36 CPU seconds, 1.2 GB). Go's build
cache is shared, but it has no in-flight dedupe: builds started together
on a cold cache all miss and all compile the same package. Warm, a build
takes 0.6 s; cold and alone, 8 to 14 s; 26 cold at once, past 120 s each.

## Fix

`go_build` (compiler/xo/gobackend.xo `cold_rt_lock`) takes an exclusive
lock per Go cache and runtime variant (a hash of the target, race, tags
and the rt tree's file names and sizes) unless a marker says that variant
was built in the last hour. The first build compiles the runtime; the
others wait, then hit Go's cache. `XO_GO_RT_LOCK=off` turns it off.

Measured on the Mac, empty `XO_GOCACHE`,
`go test -short ./internal/gen ./internal/devgoldens`: without the lock
stopped at 37 GB after 132 s; with it, both pass in 136 s, peak 24 GB.
Four concurrent cold builds of one program: 4 compiles of xoprog/rt in
15.4 s without the lock, 1 in 8.3 s with it
(cmd/xo TestGoBackendColdRuntimeCompiledOnce; `XO_TEST_RT_LOCK_OFF=1`
shows the count without the lock).

The rt tree differs per program (decision 0551 prunes it), so programs
using different parts of the runtime still compile it once each; one
shared tree for all programs is the open design question.
