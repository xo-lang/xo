# arm64: one call anywhere gives an Xo function a frame and saved registers

Status: Observed (2026-10-13, decision 0207 stage 3, the heap in Xo)

Symptom: the first version of std/native/llrt/alloc.xo's `alloc`
compiled by the arm64 emitters saved x29, x30, x19, x20, and x21 on
every call (seven extra instructions on the free list pop), though the
pop path calls nothing: only the rare chunk refill called
`rt.heap_chunk` and then kept using h, the size, and the class.

Cause: the arm64 emitters (internal/arm64, selfhost/arm64) make a
function a leaf only when it has no call at all, and a value live
across a call goes to a callee saved register that the prologue saves
on every path (no shrink wrapping, no tail calls).

Fix: make each call last, with nothing live after it, so no value
needs a callee saved register: alloc.xo returns `rt.heap_grow(h, n)`
(C installs a new chunk and calls the export again) instead of
carving after the call. The frame record (three instructions) stays
while any call remains; the C path has the same frame. Also: constant
offsets of `rt.mem_load`/`mem_store` and constant addends of
`rt.int_wrap` were a `mov` plus a register form until both emitters
learned the immediate forms (same commit). A tail call in the
emitters (a call whose result is returned, in a function that is
otherwise a leaf, as `b`) would remove the remaining frame of `free`.

Seen again (2026-10-14, zero-C audit B13,
bench/results/string-list-objects-2026-10-14.md): the list and string
objects in Xo missed the bar on arm64 (alloc small 1.266, nested walk
1.115, text split 1.098) mostly through this: rl_free's drop loop saved
five register pairs on the path with no drop, and an inlined
alloc.refill five pairs on every allocation. Two workarounds, both
measured: test the cheap case in the C entry before calling Xo, and
keep a rare large path out of the arm64 inliner with a call of itself
on a path never taken (selfhost/arm64/inline.xo does not inline a
function on a call cycle). The fix is shrink wrapping in the emitters (below).

Update (2026-10-14, decision 0224): selfhost/arm64 now shrink wraps
(the frame made only in the region of blocks that need it, entered and
left through save and restore blocks) and emits tail calls (`b sym`
after the epilogue). alloc.alloc and alloc.free have no frame at all;
a rare call still costs the frame on its own path. The rules above
still help where shrink wrapping does not apply: functions with landing
pads, stack arguments or results, more than six region edges, or a
value kept across a call on the common path.

Update (2026-10-14, decision 0226): B13's list objects meet the bar
once arm64 generated code calls obj.xo directly (no C level) and its
common cases make no call; a new caller of a large function can undo
an inlining elsewhere (gotchas/arm64-inline-few-sites-new-callers.md).

Seen again (2026-10-15, the zero-c perf sweep): `mutex.held_add`
(std/native/llrt/mutex.xo) built XO-F004's fault message inline in its
scan loop. A call inside a loop is not covered by shrink wrapping, so
every Mutex.with saved nine registers. Moving the message into
`held_fault`, and adding a call-free fast path (no Mutex held yet,
room for one) that calls `held_add_slow` otherwise, took ctx's `mutex`
workload on arm64 from 1.121G to 1.000G instructions. `held_del`'s
single-entry fast path took it to 0.972G (main: 0.934G).
