# The runtime's C names made Linux system calls on FreeBSD

Status: Observed 2026-11-10 (decision 0573 `use c` stage 3, Xo Lang 1)

## Symptom

A FreeBSD `use c` program whose C called `fprintf(stderr, ...)` stopped
with no output: exit 140 (SIGSYS) on freebsd/arm64, exit 2 on
freebsd/amd64. Every other C call in the binding worked.

## Cause

In a build with no C library, std/native/llrt/cabi_libc.xo gives C the
names read, write, close, openat and the rest through libcsys.xo, which
picked each system call number with `nr(arm64, amd64)`: Linux's numbers.
FreeBSD's runtime makes its own calls through freebsd.xo, so nothing in
Xo's code reached libcsys on FreeBSD until a binding's C did; write
became syscall 64 on arm64 (FreeBSD's write is 4).

## Fix

libcsys uses `nrf(arm64, amd64, freebsd)` for every call, with FreeBSD's
numbers from its sys/syscall.h and its own flag values (O_CREAT 0x200,
AT_REMOVEDIR 0x800, TIOCGETA). A new libcsys function takes `nrf`, never
`nr`. To find the call that fails in a FreeBSD VM, mark each step on
stderr (`os.stdio.eprint`): stdout is buffered and lost when the process
dies.
