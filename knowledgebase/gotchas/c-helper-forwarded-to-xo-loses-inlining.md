# A hot C helper forwarded to an Xo export costs its C callers on LLVM

Status: Observed (2026-10-14, B13 stage 3, the string objects)

Symptom: making the C runtime's `rs_new` (a new string object) a
forwarder to obj.xo's export took bench/perf text lines to 1.053 and
1.054 and split to 1.049 of main's instructions on the LLVM backend
(arm64 unchanged, 1.000), over decision 0180's bar; int_fmt went from
1.003 to 1.024.

Cause: rs_new's C callers (xo_str_slice above all, every slice of every
split line) had the allocation inlined by clang: xo_alloc's fast path
and the size class read. Through the export it became a call into the
Xo function and its own fast path, which LTO did not fold back into
them. Generated code's calls of the exports are not hurt the same way:
they replaced a call of a C entry with a call of the export.

Fix: keep a hot helper that C code calls in C until its callers move
too (std/llrt/xo_rt.c rs_new stays; rs_append, which only cold C calls,
forwards). Move the C callers to Xo first, then delete the helper.
Measure both backends: arm64 alone would not have shown it.
