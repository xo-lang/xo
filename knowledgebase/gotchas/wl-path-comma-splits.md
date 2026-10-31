# A path with a comma breaks a `-Wl,` linker option

Status: Observed (2026-10-30)

**Symptom.** cmd/xo TestSupplyChainAttacks failed its three
native/.../llvm subtests on darwin with `ld: file cannot be open()ed,
errno=2 (No such file or directory) path=_attenuated_ca2211487212/003/.build/<key>/llvm-split32files-none-w-<hash>/`
and `xo: /usr/bin/clang failed`; the path ld names is the tail of the
build directory.

**Cause.** The darwin LLVM link passed its build directory as
`-Wl,-oso_prefix,DIR/` (and `-Wl,-object_path_lto,DIR/lto`). clang splits a
`-Wl,` option's text at every comma, and Go names a subtest's temporary
directory after the subtest ("exfiltrate over net, attenuated
capability"), so DIR's text after the comma reached ld as an input file.
Any user directory with a comma breaks the same way.

**Fix.** A linker option whose argument is a path goes through
`-Xlinker OPT -Xlinker PATH` (compiler/xo link.xo wl_path), which passes
the argument whole: -oso_prefix, -object_path_lto and the race runtime's
rpath. asm_cflags drops each -Xlinker pair, as it drops `-Wl,` flags, so a
`cc -c` never sees them. Keep `-Wl,` for options whose text is fixed.
