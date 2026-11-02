# A 208 byte smaller frame cost 36% more CPU: the fiber stack's 4 KiB phase

Status: Observed (2026-11-02, Xo Lang 1, 3, 4 and 2)

## Symptom

On the first linux/arm64 offload host (a Linux VM on an M4 Pro), bench/perf/jitter's
/alloc ran 36 to 40% more CPU a request with linger plus narrow B (a slot_group change
that made http.respond's frame 208 bytes smaller) than with linger alone. No code but
respond's frame differed; the darwin build on the same cores showed no gap.

## Cause

Where the handler's frames sit in a 4 KiB page. respond calls the handler, so every frame of
the JSON encoder moved 208 bytes; at that stack phase the copy of each field into the out
buffer (obj.rs_append_bytes) cost about 3x a byte, all user time, with the same allocations,
no faults and no extra switches. XO_STACK_SHIFT (every fiber's first frame N bytes lower)
moved the cost with the stack alone: linger shifted -208 mod 4 KiB was as slow, linger + B
shifted down 208 bytes as fast, and the bad offsets form a comb inside about 128 bytes of
the 4096. Code placement, .data, huge pages, the kernel and the stack Str buffers of
decision 0395 (XO_PASSES=-stack_small) were ruled out. On a bare metal Graviton 4 the
same phases are bad but cost under 1%, and the one counter that follows them is
st_align_lat (about 30x at exactly the bad phases), sampled in json_layout, rs_append,
dbox and the handler: the arm64 prologue stored callee saved registers in pairs (stp) from
spill_base + 8 * slots, often 8 mod 16, and a 16 byte pair at 8 mod 16 crosses a 4 KiB page
at one sp phase in 256. Hot leaf callees run hundreds of times a request, each at its own
offset (the comb). The M4 Pro under a VM with 4 KiB guest pages prices a page-crossing
store far higher; darwin's 16 KiB pages put no boundary at those offsets
(bench/results/stack-phase-2026-11-02.md).

## Fix

compiler/arm64 saves callee saved registers from a 16 byte aligned offset and pairs them
only there (decision 0459 item 5), so no prologue pair crosses a page: linger + B runs at
v0.35.0's cost at every former bad phase. When a speed change has no code cause, measure it
at XO_STACK_SHIFT 0, 16, 208, 1024 and 2048 (and XO_DATA_SHIFT 0 and 64) before bisecting;
a cost that moves with the stack alone is a store or load crossing a page or aliasing at 4
KiB. On Linux VMs with 4 KiB guest pages on Apple cores a crossing store is far dearer than
on Graviton, so a cliff seen on one host should be checked on a second before chasing code.
