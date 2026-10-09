# A thread local's address must not live across a fiber switch

Status: Anticipated (2026-10-08, decision 0069)

Symptom (expected if the rule is broken): a task on a fiber parks on one
worker thread and resumes on another, then reads the old thread's
current context, worker, or errno: wrong unwinding flags, a deque of
another worker, a stale EAGAIN.

Cause: compilers treat the address of a `__thread` variable (and the
result of `__errno_location`, which glibc declares `const`) as constant
within a function, also across calls, and the LLVM backend's link time
optimization inlines runtime functions into generated code. A function
that parks between two reads of a thread local may use the address
computed before the switch.

Fix: `std/llrt/xo_rt.c` reads and writes thread locals only through
`noinline` accessors (`cur_get`/`cur_set`, `fib_get`/`fib_set`,
`w_get`/`w_set`; the heap's thread local is read in `heap_slow`, also
`noinline`), and calls that report errors in errno go through `noinline`
wrappers (`x_read`, `x_write`, `x_accept`, `x_connect`, `x_errno`, ...)
that return -errno. Keep new runtime code to the same rule.
