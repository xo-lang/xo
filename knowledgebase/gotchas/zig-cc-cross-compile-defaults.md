# zig cc and Apple clang cross compiles differ from the host compiler's defaults

Status: Observed (2026-10-11, decision 0157)

Symptom: cross compiling std/llrt for linux/arm64 on the Mac:

- `zig cc -target aarch64-linux-gnu.2.31 -O2 -c` wrote 2.7 MB objects
  (0.5 MB expected): zig adds debug information unless told `-g0`.
- `zig cc -target aarch64-linux-gnu.2.31 --version` exits 1 ("version
  '.2.31' in target triple ... is invalid") after printing the clang
  version on stdout, so a manifest that required success recorded no
  compiler.
- `clang --target=aarch64-linux-gnu --sysroot=DIR` (Apple clang) and zig
  cc emit no outline atomics (`__aarch64_*` helpers), while Debian's
  clang turns them on by default (`+outline-atomics`); zig emits no
  stack protector on darwin, while Apple clang adds `-fstack-protector`
  and `___chkstk_darwin` probes.
- A Linux header sysroot copied with `tar -cf - usr/include` from a
  container fails with `'asm/errno.h' file not found`: the
  `aarch64-linux-gnu/asm/*` headers are symlinks out of the tree.

Cause: a cross compiler's defaults are its own, not the target
distribution's.

Fix: the pack pins its flags (`rtPackFlags`: `-g0`, `-fPIC
-moutline-atomics` on Linux, `-mmacosx-version-min=11.0` on darwin);
darwin packs are built with Apple clang so the hardening matches the
local fallback; the manifest takes the first line of `--version`
whatever the exit status; copy sysroots with `tar -chf` (dereference).
Compare `nm -u` of a pack object with a local compile when changing the
compiler or flags.
