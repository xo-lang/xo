# C only runtime tests need the Xo exports to link

Status: Observed (2026-10-13, decision 0207)

Symptom: after std/llrt called functions of std/native/llrt/cabi.xo
(`xo_x_fmt_int`, `xo_x_fault_overflow`, ...), the internal/llvm tests
that compile a file of std/llrt/testdata (rc_threads.c, tasks.c,
evloop.c, realtime.c, fault_resume.c, crypto_diff.c, http_diff.c,
http_fuzz.c) failed to link: `Undefined symbols: _xo_x_fault_abs ...`.

Cause: those files include xo_rt.c and play the part of generated
code, so nothing defined the exports every real program carries.

Fix: internal/llvm's `cOnly(t, src)` (threads_test.go) adds the LLVM IR
of an empty Xo program, which defines the exports, to such a build.
A new C only test must go through it.
