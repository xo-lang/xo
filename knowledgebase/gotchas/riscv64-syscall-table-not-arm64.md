# riscv64's syscall numbers are arm64's, but not all of arm64's syscalls exist

Status: Observed (2026-11-08, decision 0571 stage 1)

## Symptom

On linux/riscv64 (under qemu-user), goldens fs_atomic, fs_native,
fs_ops_edges and fs_read_mapped exited 1 at their first rename:
`Fs.rename` returned an error for paths that exist. Every other file
operation worked.

## Cause

Both arm64 and riscv64 use the asm-generic syscall table, so std/native's
`nr(arm64, amd64)` helpers give riscv64 the arm64 number. But the
generic table's optional entries depend on each architecture's
`__ARCH_WANT_*` defines: arm64 defines `__ARCH_WANT_RENAMEAT`, and
riscv64 does not, so syscall 38 (renameat) is ENOSYS there. Both define
`__ARCH_WANT_NEW_STAT`, `__ARCH_WANT_SET_GET_RLIMIT` and
`__ARCH_WANT_SYS_CLONE3`; the 32-bit time calls only matter for 32-bit
targets. libcsys.xo already used renameat2 (276) and was unaffected.

## Fix

std/native/llrt/linux.xo `rename` calls renameat2 (276) with no flags
on riscv64. When a new syscall goes into `nr(...)`, check it against
arch/riscv/include/uapi/asm/unistd.h's `__ARCH_WANT_*` list, not only
the arm64 number.
