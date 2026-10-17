# Cross building for Windows: xwin's arch names, and the TLS library missed the sysroot

Status: Observed (2026-10-13, windows3 agent, Linux EC2 host cross
building xo1 for windows/amd64; bench/results/windows-2026-10-12.md,
"xo1 built by Xo for Windows").

## 1. lld-link finds no system library

Symptom: with a sysroot made by `xwin splat --use-winsysroot-style`,
`clang --target=x86_64-pc-windows-msvc -Xmicrosoft-windows-sys-root DIR
-fuse-ld=lld` compiles (the headers are found) but the link stops with
`lld-link: error: could not open 'uuid.lib'` and `'ws2_32.lib'`.

Cause: xwin 0.10.0 names the library directories `x86_64` and `aarch64`
by default (`Windows Kits/10/Lib/<ver>/um/x86_64`), while clang's
winsysroot search uses Microsoft's `x64` and `arm64`.

Fix: splat with `--preserve-ms-arch-notation` as well:
`xwin --accept-license --arch x86_64,aarch64 splat --use-winsysroot-style
--preserve-ms-arch-notation --output DIR` (19 s, 1.1 GB).

## 2. `'string.h' file not found` in the TLS library

Symptom: the first cross build of xo1 compiled every IR module, then
stopped compiling the TLS library (`src/library/debug.c ... fatal error:
'string.h' file not found`). xo1 links std/http's TLS (it fetches
modules), so a build of xo1 always builds the library.

Cause: the TLS library is built once per "flavor" of the link flags
(sanitizers and the cross `--target=`, compiler/xo tls.xo `tls_args`);
the sysroot pair `-Xmicrosoft-windows-sys-root DIR` was not part of the
flavor, so the library compiled for Windows without its headers.

Fix (0a6b96d7): the flavor keeps the sysroot pair too. Any new cross
flag that the C compiles need must be added there and in
`llvm_cross_flags` (clang.xo).

## 3. Split modules compiled for the host

Anticipated, avoided in 2d30dabb: the IR names no target triple, so a
split build's per module `clang -c prog.N.ll` used the host's default
target (an ELF object on Linux); split_build now passes the link's
`--target=` to each module. The Go driver's split build
(internal/build llvmsplit.go) still omits it, which matters only for
windows/arm64 split builds on windows/amd64 by the Go driver (frozen).
