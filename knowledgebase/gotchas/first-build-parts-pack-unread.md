# A large module's parts pack on its first build was never read

Status: Observed (2026-10-22, Xo Publish).

## Symptom

After pack-target-stamp-values.md's fix the back-end cache still sat at
its 16 GiB cap with every entry under 30 minutes old, and the prune still
removed about 55 GiB an hour (prune-log, 2026-10-22 01:00 to 01:25). Part
pack segments held 9.3 GB of the cap: about one 650 MB segment per self
build of xo, eleven in 16 minutes, each in a new slot, none read after it
was written (each segment's last use within a minute of its birth).

## Cause

compiler/xo bcache.xo fn_cache_store wrote a module's parts pack on its
build target's first build when the module had PART_PACK_FIRST (16) parts
or more, so an edit right after a cold build would not regenerate every
part. A target is a working directory plus the command, and the part
keys name the compile directory, so each gate's self-build in a new
worktree (xotool's xobuild, `go test` building bin/xo) is a new target
built once. A rebase does not reuse the pack either: xo built at
98f0ca721 and then at main four compiler commits later reused 0 of 116
parts (4,056 of 8,633 functions), and the second segment was appended
beside the dead first one. Only a leaf edit reuses them (115 of 116, an
8.0 s build).

## Fix

A large module's parts are packed on the target's second build (a test
build of fewer than 16 parts still on its first, e7118db80), and a build
that used none of its pack's entries rewrites the pack instead of
appending. The first edit after a cold build regenerates xo's parts once
(8.6 s against 7.6 s for an edit with them, xo, 2026-10-22); a gate's one-shot build writes no parts.

Addendum (2026-10-27, xo-lang-4): the part count was the wrong measure
for small programs. The HTTP server grew to 35 parts (8.4 MB packed),
so every user's first edit after a cold build regenerated all 35 (emit
67 ms against 24 ms, darwin/arm64 and both amd64 targets alike). A module
now also packs on its first build when its parts are under
PART_PACK_FIRST_BYTES (128 MiB) in all; the compiler's stay on the
second (internal/arm64 TestFirstEditAfterColdParts).
