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
