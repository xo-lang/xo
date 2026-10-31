# A clang-only flag in a `cc -c` breaks every Linux build

Status: Observed (2026-10-30)

**Symptom.** After 7408d0c15 xo no longer built on the linux/amd64 host:
`cc: error: unrecognized command-line option '-fdebug-compilation-dir=.'`
then `/usr/bin/cc failed`. The darwin gates, which run clang as cc, passed.

**Cause.** compiler/xo cffi.xo c_objects compiles each `use c` source
(and xo_buildinfo.c) with the build's cc, which is gcc on the Linux hosts.
-fdebug-compilation-dir is clang's; gcc does not know it.

**Fix.** Use flags both compilers take: `-ffile-prefix-map=CWD=.` for the
compile directory, then the source directory's own map (both apply the
last map that matches). A change to the flags xo passes to cc runs the
linux/amd64 sample (`~/xo-lead/agents/amd64-run.sh REV NAME sample`) or a
linux/arm64 build before it pushes, not only the darwin gates.
