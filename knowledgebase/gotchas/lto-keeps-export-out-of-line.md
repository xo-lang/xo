# LTO keeps a C ABI export out of line where the C was inlined

Status: Observed (2026-10-13, decision 0207 stage 3, the heap in Xo)

Symptom: with the heap in Xo (std/native/llrt/alloc.xo through
cabi.xo's `heap_alloc` export), bench/perf/text int_fmt on the LLVM
backend ran 1.077 times the C path's instructions less setup, and
float_fmt 1.054, while every allocation workload was faster than the C.
The Xo function itself compiled to good code (frameless, tail calls).

Cause: in the C path `alloc_in` is `always_inline` C in the same
translation unit, so clang inlined the whole allocator into hot string
builders (`xo_rs_push_word`, `rs_append`). The export lives in the
program's LLVM IR, another module until LTO, where LLVM's cost model
left `xo_x_heap_alloc` out of line in those callers: a call per small
string. The function attribute `always_inline` on the C wrapper does
not reach across modules.

Fix: mark the call itself: `[[clang::always_inline]] p =
xo_x_heap_alloc(n, h);` (std/llrt/xo_rt.c `XO_INLINE_CALL`), which
clang emits as an `alwaysinline` call site attribute that LTO's inliner
honors. int_fmt went to 1.014 and float_fmt to 1.004. Use it for an
export the C calls on a hot path where the C copy was inline; not for
large or rare ones. Guard it by the clang version (17, Apple 16), not
by `__has_c_attribute(clang::always_inline)`: Amazon Linux 2023's clang
15 fails to parse that test itself ("missing ')' after ':'"), which
broke every C runtime build of the first EC2 run.
