# 0230: Plain C name exports for the nolibc C library

Status: Proposed (2026-10-14)

Stub: number claimed for xo-4b's B04 (the xo_nolibc.c shrink): a module
std/native/llrt/libc.xo whose functions are exported under their bare C
names in XO_NOLIBC builds (malloc, memcpy, strerror, open, ...), the
LLVM emitter's no-builtins attribute on them, and fixed arity exports
for the variadic C functions; filled in by that change.
