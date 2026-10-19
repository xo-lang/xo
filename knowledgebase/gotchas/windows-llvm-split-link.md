# Windows LLVM builds split into modules do not link

Status: Observed (2026-10-18, xo-fa, cross building xo.exe on the arm64
Mac for the Windows signing trial, main d9f15415).

Symptom: `xo build --backend=llvm --target=windows/amd64` with
`XO_LLVM_SPLIT=N` stops in lld-link, two ways:

- `XO_LLVM_LTO=none`: `duplicate symbol: .xo.static`, `.xo.sys_words`,
  `xo_empty_list`, and, for xo, `__tsan_*` and `__errno_location`.
- `XO_LLVM_LTO=thin` (the release recipe in toolchain.md, split 16):
  `undefined symbol: pthread_attr_init`, `pthread_create`, `pthread_join`,
  `mmap`, `mprotect`, `munmap`, referenced from `x.pool.thread_start`,
  `x.sanhook.thread_start`, `x.pool.thread_run`.

One module (`XO_LLVM_SPLIT` unset) at O1 or O2 links; one module at O0
fails like the ThinLTO case. A small hello shows all of them
(`XO_LLVM_SPLIT=4`).

Cause: (1) compiler/llvm defines the shared runtime globals in every
module as `weak` or `linkonce_odr` (call.xo: `.xo.static`,
`.xo.sys_words`, the placeholders such as `xo_empty_list`). ELF and
Mach-O fold those; COFF folds a definition only through a comdat, and
the IR gives none. (2) std/native/llrt pool.xo and sanhook.xo choose the
platform with `match layer()`; only an optimizer that sees layer()'s
body folds the match. In a split module (and at O0) the darwin branch's
calls stay, and Windows has no pthread or mmap to resolve them.

Fix (to do, xo-fa, before decision 0254 stage 6c): put each such global
in its own `comdat any` on COFF targets, and make the platform choice a
compile time constant the front end folds (or declare the darwin and
linux externs only for their targets); verify with the split 16 ThinLTO
xo.exe. Until then build Windows binaries as one module at O1 or O2.
