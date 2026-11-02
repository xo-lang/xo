# A generated C file's build path reached the binary when OUT was in the checkout

Status: Observed (2026-11-01, Xo Lang 4)

**Symptom.** `bootstrap: FAIL: s2 and s3 binaries differ` on Linux at
`scripts/bootstrap-xo.sh bin/xo OUT` with OUT below the checkout. Assembly and
.text, .rodata, .data matched; `.debug_line_str` named
`.b/cache-s2/build/.../w-.../xo_buildinfo.c` and the same for xo_tzdata.c. The
same commit passed with OUT outside the checkout.

**Cause.** c_objects maps a source's directory to its path relative to the working
directory, or "." outside it. A generated source lives in the build's own
directory, so with that directory below the working directory the relative path
named the build directory, which differs per build. Whether a fixed point passes
can depend on where its output goes, so a pass elsewhere says nothing about it.

**Fix.** A source in the build's own directory maps to ".". internal/arm64
TestBuildDirBelowCwdSameBinary builds twice with the build directory below the
working directory.
