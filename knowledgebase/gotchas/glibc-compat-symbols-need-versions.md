# Unversioned imports cannot bind glibc's compatibility symbols

Status: Anticipated (2026-10-12, decision 0182 stage 3); the versioned
imports below pass every golden on glibc 2.31 and 2.41

Symptom (expected): an executable that imports `__xstat` (or
`__fxstat`, `__lxstat`) without a version, as a linker that reads no
`.so` files is tempted to write, starts on glibc 2.31 but fails on 2.33
and later with `symbol lookup error: undefined symbol: __xstat`.

Cause: the linux runtime pack is compiled against glibc 2.31's headers,
whose `stat` is an inline call of `__xstat`. glibc 2.33 made `stat` a
real function and kept `__xstat` only as a compatibility symbol (a
hidden version, `__xstat@GLIBC_2.17`). The dynamic loader binds an
unversioned reference only to a default (`@@`) version, or to the one
non hidden version; a hidden version is matched only by a reference that
names it.

Fix: the ELF linker gives every import the version glibc 2.31 defines
for it, as `cc` would bind it there: `compiler/macho/glibc.xo`, generated
by `scripts/elf-glibc-versions.py` from Debian bullseye's libc.so.6,
libm.so.6, libpthread.so.0, and libgcc_s.so.1 (libc.so.6 and GLIBC_2.17,
the arm64 baseline, for every name not listed). The same rule keeps
`pthread_create@GLIBC_2.17` and `__libc_start_main@GLIBC_2.17` working on
glibc 2.34 and later, where both moved to new default versions.
