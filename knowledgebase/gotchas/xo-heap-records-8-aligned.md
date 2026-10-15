# Records from the Xo heap are 8 aligned, not 16

Status: Observed (2026-10-15, the zero-C audit's B18; TestGoldenASan)

Symptom: TestGoldenASan failed in every program that spawns a task
(net_echo, net_only) with UndefinedBehaviorSanitizer's "misaligned
address ... for type 'xo_ctx', which requires 16 byte alignment" in
xo_rt.c.

Cause: tasks.xo run_claimed allocates the task's context with
obj.alloc_obj, and alloc.xo puts an 8 byte header before every block of
a 16 byte size class, so a block's address is 8 mod 16. xo_ctx embedded
a `struct _Unwind_Exception`, which the unwinder declares with the
target's largest alignment (16 on arm64 and x86-64), making the whole
struct 16 aligned. Any C struct with such a member (`long double`,
`__int128`, `max_align_t`, an `_Alignas(16)` field, SSE vector types)
that is placed in a record from the Xo heap has the same problem; on
x86-64 the compiler may copy or clear it with `movaps`, which faults on
an 8 aligned address.

Fix: keep C structs that Xo allocates at 8 byte alignment. xo_ctx's
fault_exc is now an 8 aligned byte area 8 bytes larger than the
exception, and ctx_exc(c) returns its first 16 byte aligned address
(the context never moves, so the address is stable). Check new records
with `_Static_assert(_Alignof(T) <= 8, ...)` when Xo code allocates
them.
