# 0230: Plain C name exports for the nolibc C library

Status: Accepted (2026-10-14)

## Context

A program built with `XO_NOLIBC=1` (decision 0206, Linux) links no C
library: std/llrt/xo_nolibc.c defines the C library functions the C
runtime and mbedtls call (malloc, memcpy, strlen, read, open, ...).
The zero-C audit's B04 moves that file to Xo. An Xo function reaches C
only as a C ABI export of a cabi module, and every export is named
`xo_x_<name>` (decision 0207), so a body moved to Xo still leaves a C
function under the C name that forwards to it: most of xo_nolibc.c is
one line wrappers already, and the forwarders would not shrink it.

## Decision

1. The module std/native/llrt/cabi_libc.xo is the C library of an
   XO_NOLIBC build: the SSA builders (selfhost/ssa declare_exports)
   export its top level functions under their bare names (`malloc`,
   not `xo_x_malloc`), and only in an XO_NOLIBC build, as
   cabi_nolibc.xo's exports. Its functions are one line calls of the
   implementation modules (libc*.xo), so no helper becomes a C symbol.
   internal/llvm TestNoBareLibcExportsInLibcBuild checks that a program
   built against the system's C library defines none of them (a bare
   malloc of Xo's would silently replace the system's).
2. The LLVM emitter gives the functions of cabi_libc.xo and of the
   libc*.xo modules the "no-builtins" attribute (attribute group #7), so
   the optimizer does not turn memcpy's own loop into a call of memcpy,
   or calloc's malloc and memset into a call of calloc; and it lists the
   bare exports in `@llvm.used`, so LTO keeps a definition that code
   generation may call after the optimizer dropped it (calloc, memcpy),
   as the C's `__attribute__((used))` did. arm64 needs neither.
3. The variadic C functions whose variable arguments are integers (open,
   openat, fcntl, ioctl) are exported with fixed parameters: on Linux's
   AAPCS64 and SysV x86_64 conventions a variadic integer argument
   arrives in the next integer register, which the callee reads as an
   ordinary parameter. The printf family and FILE stay C until their C
   callers move.
4. malloc is its own allocator (libcmem.xo, a port of xo_nolibc.c's: free
   lists by power of two class carved from 1 MiB mapped regions, blocks
   above 64 KiB mapped one by one with 16 kept for reuse), not alloc.xo's
   heap: alloc.xo's blocks are 8 byte aligned where C requires 16 of
   malloc; alloc.xo counts every block for the leak check and the
   resource budgets, which the C runtime's records (from malloc for the
   life of the program) must not be; and malloc must not route back into
   rt.heap_large, the runtime's malloc. Its lock and the address of its
   state are static words 30 and 31.

## Consequences

- xo_nolibc.c shrinks by each stage: (a) malloc, free, calloc, realloc;
  (b) the mem* and str* functions; (c) strerror over linux.xo's errno
  table; (d) the system call wrappers of files and processes. Each stage
  is a commit with its own CI run: nolibc builds are Linux only, so
  scripts/ci/nolibc-goldens.sh (suite=script) is their check (the LLVM
  goldens built with XO_NOLIBC=1 on both backends, the branch's std
  against main's).
- errno is not set by the Xo malloc on failure (it returns 0); the C
  runtime's callers end the program on a null result.
- A future platform without a C library (decision 0195) can reuse the
  mechanism; the bare names are a property of cabi_libc.xo alone.

## Alternatives

- Keep `xo_x_` names and one line C forwarders: no shrink of xo_nolibc.c.
- Name every export bare: collides with the C runtime's own symbols and
  with the system's in a libc build.
- malloc over alloc.xo's heap: rejected for alignment, the leak check
  and budgets, and the cycle through rt.heap_large (Decision 4).
