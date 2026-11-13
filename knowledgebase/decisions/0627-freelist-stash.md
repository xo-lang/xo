# 0627: Emit allocates from fresh memory: a per-thread free list stash

Status: Rejected (Xo Lang 1, 2026-11-13; lead's timebox): neither probe pays. The stash costs 1.6 to 1.7x the self build's cycles and 1.9x its memory; sorting the small free lists by address has no effect (aiplatform CPU 0.999). The argument pool's remaining aiplatform CPU (about 1.12) stands, as decision 0448 took it.

## Context

Decision 0448's argument pool (stage 2, d29005fa2) moved every SSA
value's argument list into one pool per function. The self build got
better on every axis (CPU 0.953 to 0.973, max RSS 0.873 to 0.885), but
the gcp aiplatform/apiv1 test build took about 12% more CPU (1.12 to
1.137 on three linux/arm64 hosts) for 18% less memory. The lead took
the trade (memory is the binding limit) and asked for the CPU back.

What the follow-up found (ADR 0448 "The CPU follow-up"):

- The cost is in emit alone: with one emit job on host 1, emit takes
  72.6 to 73.4 s on the base and 80.5 to 82.3 s with the pool (+10.5%);
  every other phase is equal, so it is per thread work, not contention.
- Its direct part, the pool's accessors (an iterator call per argument),
  is gone (cabff5ec0: CPU 0.990 on aiplatform, 0.983 on the self build).
- The rest is unchanged code that reads only MIR running slower
  (regalloc +0.8 s, overlaps_sorted +0.5 s, hoist_invariants +0.4 s,
  gen_kill +0.3 s): data placement, not code (a `--layout` build of each
  keeps the gap; huge pages everywhere keep it).
- Keeping the old per-value argument lists alive next to the pool
  (throwaway) gives back nearly all of it: aiplatform CPU 0.913, wall
  0.968, but max RSS 1.214 (host 1, n=9). XO_HEAPSTATS at exit: the
  base holds 1,669 MB of free 32 and 48 byte blocks against the pool's
  151 MB.

So with the per-value lists gone, the SSA passes' frees fill the small
classes' free lists with blocks scattered over the whole heap, and
emit's MIR (instructions' operand lists, regalloc's per-value ranges,
isel's small lists) is allocated from them in LIFO order: neighbors in
the MIR land far apart, and the passes that walk the MIR in order
(liveness, regalloc's range searches) stall on them. In the base, those
classes' lists were mostly held by live argument lists, so emit carved
fresh memory in allocation order.

## Decision (proposed, rejected)

A per-thread free list stash in the native allocator (llrt alloc.xo),
which a caller brackets around work whose allocations should come from
fresh memory:

1. stash_on (the outermost of nested calls): each small class's free
   list (classes 1 to 64, blocks up to 1 KiB) and its length move to
   words of the heap record (heapos.xo H_STASH, after the record's last
   field); the lists are then empty.
2. While the stash is on, an allocation of those classes pops only
   blocks freed since the stash began, else carves from the bump chunk;
   it skips the magazine and central refill, and heap_grow skips
   cut_for (which refills the class lists the stash bypasses; without
   it the probe looped alloc, heap_grow, alloc).
3. stash_off (the outermost): each class's current list (the blocks
   freed inside the bracket, recent and in cache) is kept first and the
   stashed list is linked behind it; the lengths add. Nothing leaks: the
   stashed blocks are reused by the next allocations outside a bracket.
4. The compiler brackets each function's emit (arm64 G.function: isel,
   regalloc, emit), so a function's MIR comes from fresh memory and
   from its own frees.

How the compiler reaches it: TBD after the probe (a debug.* function
needs std/debug.xo, the Go backend's std/rt and wasm's stubs; an rt
intrinsic needs seed care, gotchas/seed-older-than-intrinsics.md). The
probe uses debug.mem_label with two magic texts.

## Measurements

The stash probe (lang1-stash, never pushed; the compiler reached it
through debug.mem_label with two magic texts):

- Its first build hung 65 minutes: emit threads looped alloc,
  heap_grow, cut_for, alloc (cut_for refills the class lists the stash
  bypasses). Skipping cut_for while stashed fixed it.
- Its self build passed 8 GB in 21 s (normal peak 4.9 GB): once a
  list reached a magazine, free_mt moved it to the spare magazine and
  then to the central list, which the stashed alloc never took. Taking
  the thread's own spare magazine did not stop it; taking the central
  list too did, in a warm run (4.8 GB peak heap).
- Cold, two interleaved rounds on the Mac (caches off), against main:
  cycles 1,151 and 1,106 G against 660 and 708 G (1.6 to 1.7x), max RSS
  8.87 and 8.85 GB against 4.65 and 4.64 GB (1.9x), wall 36.4 s against
  24.7 and 26.7 s. The brackets run back to back (every function's
  emit), so each function stashes the blocks its predecessor freed and
  carves fresh memory again: the stashed blocks never come back. Fresh
  memory for every function and a bounded heap cannot both hold.

The address order probe (same branch, 7b4b3498b): no stash; at each
function's emit start the thread's 32 and 48 byte free lists are sorted
by address (an in place bottom up merge sort of the linked blocks) when
their length moved by 256 or more since the last sort (lead: a sort per
function would eat the gain), so emit reuses the same blocks in address
order.

- Self build, Mac, cold, two interleaved rounds: cycles 676 and 665 G
  against 669 and 681 G, the same max RSS: neutral.
- aiplatform/apiv1 test build, `xo bench --ab` on linux/arm64 host 1
  against its parent, n=9: wall 0.996, CPU 0.999, max RSS 1.011, all
  within noise.

So the order of reuse within the 32 and 48 byte classes is not what
the kept-live lists bought. What they bought is that emit's allocations
did not reuse the SSA passes' blocks at all, which costs the memory the
pool saved (CPU 0.913 at RSS 1.214).

## Consequences

None: no allocator change lands. The aiplatform trade of decision 0448
stage 2 stands (CPU about 1.12, max RSS about 0.82 against the base;
1.11 after cabff5ec0's accessor change). Further CPU would need emit's
own data structures to stop depending on the heap's order (MIR in flat
per function pools reused across functions, as the argument pool did
for SSA), which ADR 0448's probes found only partly (copying the
instructions' operands alone did nothing).

## Alternatives

- Keep the per-value lists alive (CPU 0.913, RSS 1.214): gives the
  memory back.
- The stash and the address order, above.
- Decision 0435's arena (refs/recover/xl91-arena): a mode with no-op
  frees and a sweep; larger (549 lines), its footprint result was
  negative for the front end, and it needs a sweep per function here.
- Compiler-side pools for MIR (Inst operand lists, regalloc ranges):
  copying the instructions' operands into fresh memory before regalloc
  changed nothing (CPU 1.002); every MIR structure would need it.
- Address-ordered free lists: a sort per class, costly on every free or
  a periodic pass.
