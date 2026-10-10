# A test's os.fs is a MemFs, not an Fs

Status: Observed (2026-10-10, decision 0120)

Symptom: with native `xo test --real`, a `uses fs.read` test reading a
real file got `NotFound`, while a `uses fs` test that wrote a file and
read it back passed.

Cause: `os` in a test is a `TestOs`, whose `fs` field is a `MemFs`
(std/testing). Its methods resolve to `MemFs` methods of std/testing,
not to the prelude's `Fs` methods, so the run time switch to the real
file system, placed only on `Fs` methods, never ran: the write and the
read both went to the in memory file system, and the read only test saw
an empty one.

Fix: `memFsCall` (internal/ssa build_io.go) routes every Fs or MemFs
method that has a real implementation (`fs_<name>` of the native
prelude) through the run time switch; MemFs's own control methods have
none and stay fake. A test of real capabilities should check something
only the real one can show (a file that exists on disk), not a round
trip a fake also passes.
