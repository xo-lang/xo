# `stat` is not in glibc 2.31's libc.so.6: the ELF linker must supply it

Status: Observed (2026-10-12, the switch point B flip's linux/arm64
archive on EC2, Amazon Linux 2023, glibc 2.34; bench/results/
retire-go-2026-10-12.md "Switch point B: the flip").

## Symptom

The linux/arm64 xo1 linked by selfhost/macho (`--ld=macho`) failed every
command at load time:
`bin/xo: symbol lookup error: bin/xo: undefined symbol: stat, version GLIBC_2.17`.
Decision 0182's 159 goldens had passed in glibc 2.31 and 2.41
containers; none of them calls `stat`, `fstat`, or `lstat` directly
from the runtime pack's objects, which xo1 does (the pack's xo_fs.c).

## Cause

Before glibc 2.33, `stat`, `fstat`, `lstat`, `fstatat` (and `mknod`,
`atexit`, `pthread_atfork`) are not exported by libc.so.6: they live in
libc_nonshared.a, which cc links statically, as wrappers calling
`__xstat(0, path, buf)` and its siblings. selfhost/macho reads no
libraries, so it imported `stat` with libc's default version
GLIBC_2.17, which no glibc has (2.33 and later export `stat@GLIBC_2.33`).

## Fix

The ELF linker synthesizes the libc_nonshared wrappers the objects call
(elflink.xo `nonshared`: `stat`, `fstat`, `lstat`, `fstatat` and their
64 names shift the arguments and branch to `__xstat`, `__fxstat`,
`__lxstat`, `__fxstatat`), which bind as GLIBC_2.17 on 2.31 and as the
compatibility symbols on 2.33 and later; the macho Xo test "stat, fstat,
and lstat call glibc's __xstat family". `mknod`, `atexit`, and
`pthread_atfork` are not called by the pack yet; add them to `nonshared`
when an import of one appears (`__xmknod`, `__cxa_atexit` with
`__dso_handle`, `__register_atfork`). Check a new link's imports
against libc_nonshared.a's members, not only against glibc.xo.
