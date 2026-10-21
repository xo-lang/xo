# The Linux runners' clang could not link LTO

Status: Observed (2026-10-21)

**Symptom.** On the linux CI runners an LLVM backend build with LTO
(`-flto`, the default at -O2 for a single module) failed at the link,
though the same program built on the Mac.

**Cause.** Amazon Linux 2023's clang is 15 (XO1505 needs 16); the
`clang22` package that replaced it has no `ld.lld`, and GNU ld cannot read
LTO bitcode without the gold plugin.

**Fix.** The boot script installs the upstream LLVM release (23.1.3, a
tarball with `bin/ld.lld`, checked against a pinned SHA-256) to /opt/llvm
and writes `bin/clang.cfg` with `-fuse-ld=lld` (clang reads `clang.cfg`
beside its real path). `scripts/ci/lld-probe.sh` proves the install and
builds one LTO golden before the boot script is uploaded. When bumping the
version, update the checksums in both files from the release's asset digests.
