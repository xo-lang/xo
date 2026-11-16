# The age pass walked the solo record's lists from another thread

Status: Observed (Xo Lang 1, 2026-11-16, decision 0676 stage 2)

## Symptom

The page sweep of decision 0676 crashed with SIGSEGV at address 0x1 in
`heapos.sweep` (the darwin debug build with the sweep allowed; on
linux/arm64 the run hung in the crash report) while TestRuntimeHeapIdle's
second burst allocated: a free list held a live block, whose first word
the walk followed as its link.

## Cause

`heapos.age` took the record whose lists it walks as `obj.solo_heap()`
when set, else the calling thread's TLS_HEAP. The solo record is the
single threaded heap's, and it stays set while idle pool workers run
(a probe faulting when it was set fired on every run): the age pass runs
on an idle worker (pool.xo age_check), so it walked, marked and changed
the lists of the program's one thread while that thread allocated from
them. The page-class age walk of decision 0533 (age_list: PSEEN marks
and advice on the blocks it finds) has done the same since it landed;
the sweep, which unlinks blocks, made it crash.

## Fix

The pool's pass (`age_from(central, false)`, from age_check through
cabi's AGE) walks the calling thread's own record only, and none while
a solo record is set (the central lists, under their locks, are still
aged). A direct `age` (own: a test, the owning thread itself) still
walks the solo record: the first fix skipped it there too and
heapos_test's "an age pass marks a free page block" failed (the gate,
2026-11-16). Any pass run from an idle worker must take the record from
TLS_HEAP, never from `obj.solo_heap()`.
