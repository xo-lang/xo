# A FileLock dropped while held stays held on the Go backend

Status: Observed (2026-10-20, golden handle_drop_leak, decision 0295); fixed 2026-10-21 (std/rt fsfile.go newFlockH, collectDropped; golden lock_drop_relock)

Symptom: a program that drops a held `FileLock` (or lets it go out of
scope) and then calls `fs.lock` on the same path runs natively but hangs
on the Go backend. The golden handle_drop_leak locked "h.lock" in a loop,
dropping it held every other round: arm64 and LLVM passed, and the Go
backend timed out after 120 s at the second round's `lock`.

Cause: native handles are reference counted, and the last drop calls
the handle's free hook (llrt fsops.xo lock_free), which closes the
descriptor and so releases the `flock`. The Go backend has no drops. Its
`FileLock` (std/rt fsfile.go flockH) holds an `*os.File`, and Go closes
that descriptor only from the `os.File` finalizer when the garbage
collector collects it. Until then the lock stays held. A dropped open
`File` keeps its descriptor the same way.

Fix (2026-10-21): std/rt gives each FileLock handle a finalizer that
unlocks it (fsfile.go newFlockH), and a `lock` or `try_lock` that finds
the lock held runs the collector (collectDropped: runtime.GC and a 1 ms
pause for the finalizer goroutine) on its first try and every tenth poll
on a real Fs, and up to three times before queueing on a fake Fs (whose
in-memory lock had no finalizer at all, so a dropped holder there was
never released). Golden lock_drop_relock drops a held lock 20 times and
locks the same path again; on the Go backend it hung at the second round
before the fix (checked by hand, 2026-10-21). Releasing explicitly with
`l.unlock()` or `defer { l.unlock() }` is still the clearer program.

The other options considered:

1. The Go generator emits the release at the handle's last use, as the
   native RC pass drops it. compiler/gen's own.xo tracks List ownership,
   not the last use of locals. Doing this would need escape analysis
   (stored, returned, captured, passed) and a release on every block
   exit (return, `?`, break), so it is more work for the same result.
2. A finalizer alone: it adds nothing over the existing `os.File`
   finalizer until a collection runs; the collection on contention is
   what makes it prompt.

Decision (owner, 2026-10-21): the fix stays as it is (d446345d: the
finalizer unlock plus a collection on the first and every tenth contended
poll). The trade-off: a contended `lock` on the Go backend may run a full
garbage collection (runtime.GC and a 1 ms pause), a cost native builds
never pay, accepted because the Go backend is not the production target
and a program that releases its locks explicitly never contends on a
dropped one. If a Go backend user hits that cost (a hot path that
contends on locks while holding a large heap), option 1 above becomes the
next step: compiler/gen emits the unlock at the handle's last use, as the
native RC pass drops it.
