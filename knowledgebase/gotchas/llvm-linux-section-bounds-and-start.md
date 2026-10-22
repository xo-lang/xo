# Linux LLVM builds: linker-defined section bounds and code before the thread pointer

Status: Observed (2026-10-21)

## Symptom

After decision 0318 stage 4 (e518ea7af), every Linux `--backend=llvm`
build failed to link:
`LLVM gold plugin: symbol '__start_xo_text' can not be undefined in a subtraction expression`
(ld.bfd with the LLVM gold plugin, Debian trixie's default; lld says the
same without the plugin prefix). The darwin gates passed.
With the link fixed, every binary, hello included, segfaulted at start in
`xo_tlocal_get` called from `xo_x_nl_start`.

## Cause

1. The unsafe range table's initializer held
   `sub (ptrtoint @__stop_xo_text, ptrtoint @__start_xo_text)`. Under LTO
   the linker defines `__start_`/`__stop_` only after code generation, and
   a difference of two undefined symbols cannot become a relocation.
   Each symbol alone is a plain absolute relocation and links.
2. The no-preemption guard reads the thread local table. A no-libc Linux
   build starts in cabi_nolibc.xo (`xo_nolibc.c`'s `_start`) before the
   main thread has a thread pointer, so the guard's read faulted. darwin
   links the C library, whose start sets the thread pointer first.

## Fix

1. The LLVM table stores the section's end, and -1 as the ranges' count
   marks that format; pool.xo `async_pc_ok` turns the end into a size.
   Native tables are unchanged.
2. apre_mark leaves unguarded every function reachable from
   cabi_nolibc.xo's exports (that code may not touch thread state in any
   case).

Proof: Docker linux/arm64 and linux/amd64, ld.bfd with the plugin and
ld.lld: hello, async_preempt_alloc and async_preempt_regs (output as
want.txt) and the bench/perf/http server.

## Avoid

A change to what LLVM programs link or run at start needs a Docker Linux
LLVM build of hello and a golden: the darwin gates use the C library's
start and Apple's linker, and see neither failure.
