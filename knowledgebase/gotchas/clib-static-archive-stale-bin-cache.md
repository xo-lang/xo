# A C library's static archive came from the binary cache of an older xo

Status: Observed (2026-11-10, decision 0574 stage 2)

Symptom: after a fix to compiler/xo/clib.xo's ar_static (the darwin member
of absolute symbols at 0 named `_malloc` and `_free`, so the library's own
`sys_c.weak_malloc` jumped to 0), cmd/xo TestCLibStatic still crashed at
pc 0, while the same build by hand passed. With an empty `XO_BIN_CACHE`
the test passed.

Cause: native_bin_key keys a cc link on cc's identity, the flags and the
files of the build directory. ar_static writes objects of xo's own making
during the link (the constructor and the weak symbol member), outside that
directory, so an xo whose ar_static changed found the archive the earlier
xo had cached (tests share `<user cache>/xo/testbin`).

Fix: a library link's key has xo's identity (self_id) and the mode, on the
native path and the LLVM path (compiler/xo main.xo). When a link step
writes inputs of its own, key it on xo too.
