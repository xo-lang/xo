# An alwaysinline call site inlines a noinline function on LLVM

Status: Observed (2026-10-15, decision 0233's rt.frame_addr).

Symptom: the golden rt_addrs printed 010001 on LLVM (010101 on arm64):
a function's rt.frame_addr was not above the frame address of the
function it called, which had attributes #9 (`noinline
"frame-pointer"="all"`).

Cause: compiler/llvm marks every call of a function in its inlinable set
(ranges.xo inlinable_set, the SSA cost walk) with the call site
attribute #6 (`alwaysinline`), and LLVM honours a call site's
alwaysinline over the callee's noinline: the callee was inlined, so
both frame addresses were the caller's.

Fix: keep such a function out of the inlinable set (ranges.xo
inline_walk marks a function that calls rt.frame_addr as not
inlinable, as arm64's inline.xo does). Any future reason for a function
never to be inlined has to go into that cost walk, not only into the
function's attributes.
