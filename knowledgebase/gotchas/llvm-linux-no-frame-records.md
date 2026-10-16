# LLVM on Linux: no frame records to walk

Status: Observed (2026-10-16, fault tracebacks, decision 0239).

Symptom: XO_TRACEBACK on an LLVM build on Linux printed
`fault_raise_msg` and then `_fini` frames; on darwin and with the arm64
backend the frames were right.

Cause: the traceback first walked the x29 frame record chain; clang on
Linux omits frame pointers, so the chain breaks (darwin's ABI keeps
them). Keeping them ("frame-pointer"="non-leaf") cost fib 9.1% more
instructions on linux/arm64 LLVM.

Fix: the traceback walks the unwind tables (unwind.xo trace, the same
walk raise does), which every backend emits; no frame pointers needed.
