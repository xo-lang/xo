# A value kept by address is not freed by a release from Xo

Status: Observed (2026-10-14, the zero-C audit's B14)

Symptom: after the test world moved to std/native/llrt/testworld.xo,
`TestNoGCTestLeaks` (cmd/xo) found live allocations at exit on arm64 and
LLVM: 7 to 50 for the MemFs and FakeNet tests of
cmd/xo/testdata/nogc/same and examples/notes-service, then 2 for
"fake net handle and fail" once the end of test cleanup was fixed.

Cause: testworld.xo keeps an `rt.Any` (a FakeNet handler, failure, or
request; the MemFs state) as its object's address with one count
(`rc.inc(rt.obj_addr(v))`, the C's h_retain). It dropped one with
`rt.obj_at[rt.Any](a)`, `rc.dec(a)`, and the binding going out of
scope, which left the value live, both from the end of test cleanup
(on the C main thread, after main's thread ended) and from a test's
own thread (fail_next's failure when it is used). Not traced further.

Fix: Xo never releases a kept value. testworld.cleanup hands every
address (the ones still kept and the ones dropped during the test,
collected in a vector) to the C in a malloc'd block, and xo_testing.c's
tw_cleanup calls h_release on each, as before the port. Dropped values
live until the test ends, which nothing observes. A runtime module that
needs to release a value it keeps by address should do the same until
an intrinsic for it exists.
