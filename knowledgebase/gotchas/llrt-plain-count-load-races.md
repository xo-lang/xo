# A plain load of an object's count in llrt is a data race

Status: Observed (2026-10-18, ThreadSanitizer on the Mac), fixed in
std/native/llrt/obj.xo (rl_copy).

Symptom: TestRuntimeThreads (internal/llvm, rc_threads test_list)
aborted with "ThreadSanitizer: data race": an atomic write of the count
word by a task's list update, and a previous read of the same word in
xo_x_obj_rl_copy_n.

Cause: rl_copy's fast path for the empty literal's list tested
`rt.mem_load(l, 0, 8) == IMMORTAL` before rc.claim. A list shared
between threads has a count other threads change with atomics; a plain
load of that word races with them even when the answer (not immortal)
cannot change.

Fix: read the count with `count(l)` (rt.slot_load, relaxed), never
rt.mem_load, and test it after rc.claim so a unique list keeps its
one-load path. An immortal count is never written, so the length word
read after it may stay a plain load.

How to spot it: any new llrt fast path that reads word 0 of a list, a
string, or a boxed object. Run the llvm gate (TestRuntimeThreads builds
with ThreadSanitizer); the arm64 tests and the fixed point do not catch
it.
