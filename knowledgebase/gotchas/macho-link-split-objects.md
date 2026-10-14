# `--ld=macho` took only the first object of a split program

Status: Observed (2026-10-14), fixed (2026-10-14).

Symptom: the Linux arm64 milestone run at main f8ebc9ec (full tier,
TestDevGoldensELF) failed 40 of 180 programs with
`xo: --ld=macho links no C sources (<build dir>/arm64/xo_rt.c): link
with cc`, all of them the larger programs (archive_formats, http_*,
native_*, toml_basic, unicode_table, ...). The other 140 failed on the
missing runtime pack (elf-goldens-need-a-runtime-pack.md), which hid
this one on the Mac, where a checkout has no pack either.

Cause: decision 0220 stage 1 (fastbuild, merged da6a55c1) splits a large
program's assembly into parts, one object each, so the link inputs are
`[prog.0.o, prog.1.o, ..., xo_rt.c, zone data, ...]`. selfhost/xo/ld.xo
(macho_link and elf_link, decision 0182) assumed one program object:
srcs[0] the program, srcs[1] the runtime's source, the rest objects. With
two parts, xo_rt.c landed in srcs[2..] and was rejected as a C source;
with more, the later parts would have been dropped from the link. A
darwin release (bin/xo beside lib/xo/rtpack, where macho is the default
linker) would have failed the same way on any large program.

Fix: ld_objects (ld.xo) takes every input but the runtime's source
(xo_rt.c, which the pack's object replaces) and rejects any other non
object. Check: TestDevGoldensELF on linux/arm64 (EC2), which builds
these programs with `--ld=macho`.
