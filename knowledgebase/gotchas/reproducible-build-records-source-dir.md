# Byte identical builds only from the same source directory

Status: Observed (2026-10-09)

Symptom: two `xo build`s of identical sources in two different
directories give binaries with different hashes, although spec 12.1 said
"the same source, lockfile, and toolchain version produce byte identical
output". Builds of the same directory (even with fresh build directories)
are identical.

Cause: the generated program calls `rt.SetBuildInfo` with the absolute
source directory (crash bundles and `xo replay` find the sources with it),
so the path is in the binary. `-trimpath` does not cover it.

Fix: spec 12.1 now says "the same source in the same directory";
`internal/build` `TestReproducibleBuild` builds one directory twice. To
make copies elsewhere identical too, the build info would have to carry
a relative or user supplied directory (`xo replay --dir` already
overrides it).
