# The build directory kept every arm64 build's assembly text

Status: Observed (2026-10-21).

Symptom: free disk fell from 46 GB to 26 GB in 40 minutes while agents
built xo; `~/Library/Caches/xo/build` grew 24.7 GB, about 15 self builds
at 1.1 to 2.3 GB each, almost all of it `.s` files in
`<build>/<key>/arm64/`.

Cause: the arm64 (and amd64) driver path wrote the whole program's
assembly as `prog-<goos>.s` (about 1.1 GB for xo), and with parallel
parts wrote every part again as `prog-<goos>.N.s`. On Xo's own assembler
path (`--as=macho`, the default with Xo's linker) nothing read them: the
assembler takes the texts in memory. The build cache keeps one
directory per program key, so each edit of xo left another 2.2 GB.

Fix: compiler/xo/main.xo writes the files only where a tool reads them:
cc (or clang for windows/amd64) assembling, which takes the parts' files
when there are parts and the whole file otherwise, and a cross build
that stops at its assembly (it prints the path). cc's files are removed
after the link. `XO_KEEP_ASM=1` writes and keeps them all;
`--asm=FILE` and `xo asm` give the text anywhere. Directories made
before the fix still hold their files until the cache prune removes
them.
