# A Linux system call number on FreeBSD does something else, or returns

Status: Observed (2026-11-10, Xo Lang 1, the FreeBSD golden tiers)

## Symptom

On freebsd/amd64, golden proc_cmd hung until its timeout: a command whose exec failed
never reported "executable file not found". On freebsd/arm64 the same golden passed.
unix_kill died with rc 140 (SIGSYS) on amd64 and left its socket file on arm64.

## Cause

FreeBSD's runtime goes through std/native/llrt/linux.xo for most calls, and a wrapper
that still uses `nr(arm64, amd64)` passes Linux's number to FreeBSD's kernel, whose
table is its own (the same on both architectures). Linux's exit_group is 94 on arm64
(nosys on FreeBSD: SIGSYS kills the child, which looked like an exit) and 231 on amd64
(shmget on FreeBSD: it returns, so the failed child ran on into the parent's code).
unlinkat (35 on arm64, 263 on amd64) is not FreeBSD's 503. A wrong number shows as SIGSYS when
FreeBSD has nothing there, and as silent misbehavior when it has a different call.

## Fix

Use `nrf(arm64, amd64, freebsd)` (linux.xo, libcsys.xo) or a freebsd.xo wrapper for any
call FreeBSD reaches, with FreeBSD's flag values and structure layouts where they
differ (AT_REMOVEDIR 0x800, SIG_SETMASK 3, sigaction's 32 byte struct). To find the rest:
`grep -n 'rt.sys6(nr(' std/native/llrt/linux.xo` and check each caller's FreeBSD path.
Test on both architectures: the same wrong number fails differently on each.
