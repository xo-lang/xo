# 0362: Every released xo is built by the native backends

Status: Accepted (owner, 2026-10-23)

## Context

Since decision 0254 6b the release's windows/amd64 xo.exe is built by
the amd64 backend with Xo's PE linker, and darwin/arm64, darwin/amd64
and linux/arm64 by the native backends. linux/amd64 was the last one
built with the Go backend (`--backend=go --targets=linux/amd64`,
scripts/release.sh, from before the amd64 backend existed), and
scripts/seed-cut.sh cross built every seed but darwin/arm64 with the Go
backend.

A Go backend xo is a different compiler build of the same source: on
2026-10-23 a Go backend xo.exe (xobuild's on Windows) spent 538 to 605
s in each build's link, where the native one took under a second (a
Go backend copy per call, 53950e801; decision 0351's addendum). The
releases are also what users and the next seed run, so each should be
the build the toolchain is tested as.

## Decision

Every released and seeded xo is built by the native backend of its
platform:

- release.sh: linux/amd64 is `--backend=amd64 --targets=linux/amd64`,
  linked by compiler/macho's ELF linker with no C library, as
  linux/arm64 (`--ld=macho`).
- seed-cut.sh: each cross built seed uses its platform's native backend
  (linux/arm64 with `--ld=macho`); `XO_SEED_CROSS_BACKEND=go` builds
  them with the Go backend as before, a fallback only.
- xobuild on Windows builds xo natively (094a4be96).
- The released Linux xo has no C library. The toolchain's own Linux
  self builds (internal/xotool hostBackend, bootstrap-xo.sh) keep
  XO_NOLIBC=0 until decision 0243's Linux self rebuild with none is
  recorded as passing; a Docker linux/amd64 run of the new release
  build, s2 = s3 with none, is that evidence for amd64.

Development builds of xo on x86-64 hosts are native too
(internal/xotool hostBackendNative, scripts/bootstrap-xo.sh), replacing
decision 0220's LLVM -O0 there: in Docker linux/amd64 (Rosetta, so
slow in absolute terms) the native build of xo took 212 s against 893 s
for LLVM -O0 split 12, and the native xo built compiler/xo cold in
211 s against the LLVM -O0 xo's 532 s. The seed chain's hops keep LLVM
there (chainBackend: those commits predate amd64 self builds), with
XO_NOLIBC=0 on Linux as before.

Cutting a seed or a release stays the owner's step; this changes the
scripts and xobuild only.

## Consequences

- No released xo needs the Go toolchain to be built (xo-gobind still
  does: it is Go).
- A linux/amd64 xo built by the native backend is what the Docker amd64
  goldens and progen already test the backend with.
- The Go backend stays a supported target and the parity reference; it
  just builds no released xo.

## Alternatives

- Keep linux/amd64 on the Go backend: the one release built by a
  compiler path the toolchain does not otherwise self host, slower, and
  the trap that cost the Windows runs.
- The LLVM backend for linux/amd64: needs clang on the release host
  for Linux and a sysroot; the native backend needs neither.
