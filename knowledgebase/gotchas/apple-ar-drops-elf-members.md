# Apple's ar silently drops ELF members from an archive

Status: Observed (2026-10-12, decision 0182 stage 3)

Symptom: a linux/arm64 runtime pack built on macOS with the clang
fallback of decision 0157 (`xo rtpack --target=linux/arm64
--cc="clang --target=aarch64-linux-gnu --sysroot=DIR"`, no `--ar`)
succeeds, but its `libxotls.a` is 96 bytes. Every std/http program linked
against it fails at load time: `symbol lookup error: ...: undefined
symbol: mbedtls_ssl_config_free, version GLIBC_2.17` (the self hosted ELF
linker imports what no object defines, so the missing library shows up
only when the program runs).

Cause: Apple's cctools `ar rcs` runs `ranlib`, which prints `ranlib:
warning: archive member 'x.o' not a mach-o file` and rewrites the archive
with only its `__.SYMDEF SORTED` member; `ar` still exits 0, and
`BuildRTPack` shows the archiver's output only on failure. Release packs
are not affected: `scripts/release.sh` builds the linux pack with
`zig ar`, which writes a GNU archive.

Fix: build a linux pack on macOS with a GNU format archiver: `xo rtpack
--ar=llvm-ar` or `--ar="zig ar"` (any `ar rcs LIB OBJ...` that keeps ELF
members; the stage 3 measurements used a 30 line Python writer of GNU
archives). Check the result with `ls -l libxotls.a` (about 0.8 MB) or a
member count. Next step: have `BuildRTPack` refuse an archive with fewer
members than objects it archived.
