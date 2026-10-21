# arm64's list length cache goes stale on an in-place append

Status: Observed (2026-10-21, decision 0291)

**Symptom.** Built with XO_MAP_LOG=1 on arm64, the Map model test
crashed in the allocator (alloc_fast, EXC_BAD_ACCESS) after a few
thousand steps; seed 5 at step 391. The same program passed on LLVM and
with every optional pass off it still crashed (`xo dev bisect-pass -ref
off`: no divergence).

**Cause.** The arm64 backend remembers each register's list length
(compiler/arm64 calls.xo list_len, `g.cm.lens`), and the facts of a
block hold in every block it dominates: a list's length is assumed fixed
for its value. `log.push` (decision 0291) appends into a list in place,
so a list keeps its register while its length grows. The Map's chunk
directory, once built inline with three `list.push` calls (whose result
has a known length, 3), was log-pushed in the flatten's loop without
being reassigned: every iteration's `c == list.len(dir)` read the 3
known before the loop, no new chunk was added past the first, and the
entries went into the wrong chunk and past its capacity.

**Fix.** The directory is made by a call (`xo.mlog.newdir`, kept out of
line by ssa cold_fn), whose result has no known length in the caller;
and the `log.push` lowering records the new length for its own block
(`g.cm.lens[l] = n1`, as `list.push` does). A list that is grown in
place must never have a length known in a block that dominates the
growth: make it with a call, or load its length from memory. LLVM and
wasm keep no such cache.
