# Windows LLVM builds split into modules do not link

Status: Observed (2026-10-18, xo-fa, cross building xo.exe on the arm64
Mac for the Windows signing trial, main d9f15415). Fixed 2026-10-19.

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
the IR gave none. (2) COFF lowers `extern_weak` to a weak external whose
default is a symbol of the object, so two objects' weak references to a
symbol nothing defines (`__tsan_*` on Windows) are a duplicate symbol.
(3) std/native/llrt pool.xo and sanhook.xo choose the platform with
`match layer()`, and sanhook's ThreadSanitizer helpers call pthread and
mmap behind `tsan()`; only an optimizer that sees layer()'s body folds
the match. In a split module (and at O0) the darwin branch's calls
stay, and Windows has no pthread or mmap to resolve them.

Fix (2026-10-19, xo-fa):

- compiler/llvm puts each shared global in its own `comdat any` on
  Windows (call.xo comdat_decl, comdat).
- A weak reference on Windows is a plain declaration plus the linker
  option `/alternatename:sym=xo_absent` in every module, xo_absent
  defined once (module asm, first module), and rt.sym_present compares
  the address with xo_absent's (call.xo weak_ref, weak_alternates;
  decision 0232).
- The SSA builder marks a function that tests the build's target
  (Func.target: `target.os`/`target.arch` matches, and sym_present of a
  sanitizer's symbol on Windows, false there), and the pass
  fold_known_calls (compiler/ssa passes.xo) replaces a call of such a
  function whose body returns one constant (layer(), tsan()) by the
  constant and folds the branches on it. Only those functions: not any
  function returning a constant.
- The LLVM path drops unreachable functions before generating
  (arm64.drop_unreached, as the arm64 backend does; XO_DCE=off keeps
  them).

Verified: hello with `XO_LLVM_SPLIT=4` (no LTO and ThinLTO) and one
module at O0, and xo.exe with the release recipe (split 16, ThinLTO,
O2, `XO_LLVM_JOBS=4` on the arm64 Mac: 294 s, 6.0 GB peak RSS, a 160 MB
xo.exe) link, and run under Wine (`xo version`, `xo check`); the sys_c
golden prints 0101 split with ThinLTO. Wine goldens 190 of 199, the
same 7 failures as main plus 2 that pass on rerun (load).

To check after a change here: build hello for windows/amd64 with
`XO_LLVM_SPLIT=4 XO_LLVM_LTO=thin` and with `XO_LLVM_OPT=O0` and run it
under Wine (toolchain.md "Windows programs under Wine in Docker").
