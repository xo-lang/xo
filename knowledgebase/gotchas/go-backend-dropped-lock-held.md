# A FileLock dropped while held stays held on the Go backend

Status: Observed (2026-10-20, golden handle_drop_leak, decision 0295)

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

Fix: release explicitly: `l.unlock()`, or `defer { l.unlock() }` right
after `lock`. That is correct on every backend; spec/std.md 3.1 says so.
Tests that drop held locks on purpose use a different path each time.

Options for the Go backend's owner, ranked:

1. The Go generator emits the release at the handle's last use, as the
   native RC pass drops it. compiler/gen already tracks borrows and moves
   (own.xo), so it can find the last use of a local handle. That would be
   prompt and deterministic, and matches native.
2. A `runtime.SetFinalizer` (or Go 1.24's `runtime.AddCleanup`) on
   flockH that unlocks. That adds nothing over the existing `os.File`
   finalizer, since both run only after a collection, so it would not
   fix the hang. Calling `runtime.GC()` in `lock` before each wait poll
   would release dropped locks within one poll, at the cost of a full
   collection per poll while a lock is contended.
