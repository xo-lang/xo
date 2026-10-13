# 0213: std/internal and a shared file lock for the toolchain

Status: Accepted (owner, 2026-10-13: internal only, no public std API
change; std's public `Fs.lock` stays exclusive only), implemented

## Context

The Go implementation's go commands on the shared Xo Go cache hold
`internal/gotool.HoldCache()` (gate `xo-gate.lock` shared, then
`xo-cache.lock` shared; a trim takes both exclusively and may then remove
any entry; gotchas/go-commands-outside-build-race-trim.md). xo1's Go
backend (`go_build` in selfhost/xo/gobackend.xo) ran go build on the same
cache without it, because Xo's public `Fs.lock` and `Fs.try_lock` are
exclusive only. A Go built tool trimming while xo1 builds could remove
the entries xo1's go build was reading.

xo1 runs on every backend: arm64 (darwin, linux with glibc or selfhost's
ELF linker), LLVM (linux, Windows through the C runtime's flock shim),
optionally without libc (XO_NOLIBC=1), and the Go backend (the linux/amd64
release xo is built with `--backend=go`). The primitive has to exist in
all of them, and the toolchain's modules are ordinary Xo modules, so the
native intrinsics (`use native/rt`, decision 0074) alone do not reach
them, and do not exist on the Go backend.

## Decision

- A std module `std/internal` (std/internal.xo) holds primitives for
  std's implementation and the toolchain. The checkers (internal/check
  load.go `internalVisible`, selfhost/check load.xo `internal_visible`)
  resolve `use std/internal` only for std modules, native implementation
  modules, and modules whose path is `github.com/xo-lang/xo` or below it;
  for any other module it is `XO0601` module not found (as `native/rt`),
  and it is never suggested as a near miss. spec/std.md does not list it;
  spec/core.md 4.1 states the rule.
- Its first primitive: `internal.lock_shared(f: Fs, path) -> SharedLock !
  FsErr uses fs.write` and `SharedLock.unlock()`: flock LOCK_SH on Unix,
  LockFileEx without LOCKFILE_EXCLUSIVE_LOCK on Windows, waiting (polling
  every 10 ms, a cancellation point) while the file is locked
  exclusively. Only the unrestricted root Fs takes it; a sub, read only,
  write only, or memory Fs is `Denied`.
  - Go backend: std/rt internal.go over `realFs.lockFileMode` (fsfile.go)
    and `tryLockFile(f, shared)` (flock_unix.go, flock_windows.go).
  - Native backends: std/native/internal.xo over the new intrinsic
    `rt.fs_lock_shared` (std/native/rt.xo), the C runtime's
    `xo_fs_lock_shared` (std/llrt/xo_fs.c, `xo_fs_lock` with LOCK_SH; the
    Windows flock shim in xo_win.c and the no libc shim in xo_nolibc.c
    already pass LOCK_SH through).
- selfhost/xo holds the Go cache exactly as internal/gotool does:
  `hold_go_cache` takes `xo-gate.lock` shared, then `xo-cache.lock`
  shared, releases the gate, and holds the cache lock across the go build
  (`release_go_cache` after it, on success and on a failed start). Any
  error runs unlocked, as gotool does.

## Consequences

- A Go built `xo` (or test binary) trimming the cache waits for xo1's go
  builds, and xo1 waits at the gate for a trim in progress.
  `TestXo1GoBuildSurvivesTrim` (internal/selfdriver) runs xo1 `build
  --backend=go` three times while trimming the cache every 20 ms through
  `gotool.LockCacheExclusive`, and fails when no trim found the cache
  held.
- On Windows the Go implementation still takes no cache lock
  (internal/gotool cachelock_other.go) and trims only old entries; xo1's
  shared locks there are harmless and ready for a Go side lock.
- std/internal is a place for further toolchain-only primitives without
  growing std's public surface.

## Alternatives

- `Fs.lock_shared` in the prelude: a public API change; the owner kept
  `Fs.lock` exclusive only.
- Letting toolchain modules `use native/rt`: no Go backend
  implementation, so the linux/amd64 xo (Go backend) and the Go backend
  builds of selfhost/xo in tests would not compile.
- An exclusive `Fs.lock` on xo-cache.lock in xo1: safe against trims but
  serializes every concurrent go build of every xo1 and Go tool.
- A trim that knows about xo1 (keep recent entries only): leaves the race
  for any entry older than the window.
