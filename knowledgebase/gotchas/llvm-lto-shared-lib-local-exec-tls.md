# An LLVM backend shared library failed on GNU ld with R_X86_64_TPOFF32

Status: Observed (2026-11-10, decision 0574 stage 2, the linux/amd64 host)

Symptom: `xo build --backend=llvm --lib=shared` passed on the Mac and failed
on linux/amd64: `relocation R_X86_64_TPOFF32 against 'xo_tlocal' can not be
used when making a shared object; recompile with -fPIC`. `--lib=static`
failed there too: `/usr/bin/ld: bad -plugin-opt option` from the archive's
relocatable link (`cc -r`).

Cause: the single module build links with full LTO; GNU ld's LTO plugin
compiled the module as an executable (local exec thread locals), and the
unroll options for the LTO plugin (`-Wl,-plugin-opt=...`) reached `cc -r`,
which runs no LTO. darwin's ld64 gave neither error.

Fix: a library build with LLVM runs no LTO (split.single and split.lto
"none") and compiles with -fPIC off darwin (compiler/xo main.xo). Check a
new LLVM link mode on the linux hosts, not only the Mac.
