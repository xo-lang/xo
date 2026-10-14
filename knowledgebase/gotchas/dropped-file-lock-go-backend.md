# A dropped FileLock is released at once natively, not on the Go backend

Status: Observed (2026-10-14, work item B16)

Symptom: a program that drops a `FileLock` without `unlock()` (for
example `match fs.try_lock("l")? { Some(_) => ... }`) and then calls
`try_lock` on the same file gets the lock again on the arm64 and LLVM
backends, but `None` (still held) on the Go backend. A golden program
with that sequence cannot have one want.txt for every backend.

Cause: a native lock handle is a reference counted runtime record
(std/llrt/xo_fs.c xo_fslock, made by std/native/llrt/fsops.xo through
rt.fs_raw_lock); the last release closes its descriptor, which drops
the flock. The Go backend's lock is an `*os.File` that only a garbage
collector finalizer closes, at some later time. The spec (std.md 3.1)
promises neither: it says only that `unlock` releases the lock and that
an exiting program releases its locks.

Fix: in programs and goldens, call `unlock()` before relying on the
lock being free (internal/llvm/testdata/golden/fs_ops_edges does). Do
not write a test that depends on a dropped handle closing its file.
