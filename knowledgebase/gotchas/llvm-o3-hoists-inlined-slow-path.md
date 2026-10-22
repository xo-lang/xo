# LLVM -O3 hoists a pure loop from an inlined slow path onto the fast path

Status: Observed (2026-10-21, the LLVM -O2 tuning experiment)

Symptom: bench/perf/llvm strings took 13% more instructions at
`XO_LLVM_OPT=O3` than at O2 (1,137 M against 1,005 M on the Mac; 1,007 M
against 876 M under callgrind on linux/arm64), with the same output.
The Xo program's own code was the same size at both levels, and the
prologues of alloc.alloc and xo_x_heap_release matched, so a static
diff showed nothing.

Cause: alloc.refill_alloc (the magazine take, reached only when a size
class's free list is empty) was inlined into alloc.alloc through its tail
call `alloc(h, n)`, at both levels, although its doc says it stays out
of line. Its magazine count, `MAG_BYTES / size` written as a 13 step
long division (no `/`, which can fault), depends only on the size
class. At O3 LLVM unrolled it fully and hoisted the branch free steps
above the free list test, so every allocation ran them: about 80
instructions a call, alloc.alloc's own count 80 M to 242 M (callgrind
`--dump-instr=yes`, most of it on line 0). Found with a per function
callgrind diff of the two builds in Docker (the LLVM backend builds only
for the host, so a linux/arm64 xo cross built from the Mac builds the
program inside the container), then a per instruction one.

Fix: the division is its own function, alloc.mag_count, and it and
refill_alloc are in ssa.cold_fn (noinline cold in LLVM, never inlined by
the arm64 inliner). strings: O2 1,005 M to 945 M, O3 1,137 M to 933 M,
native arm64 unchanged (844.9 M to 844.3 M), against the same xo with
refill_alloc renamed so the cold list misses it. A slow path that a fast
path reaches through a tail call is inlined unless it is on the cold
list; a doc comment saying "out of line" does not keep it there.
