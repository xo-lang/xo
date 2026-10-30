# 0446: `use c` sources are compiled with debug information

Status: Proposed (xo-lang-6, 2026-10-30)

## Context

A `use c` binding's C sources (the adapter cbind writes and the
binding's `link` files) went on the link line as .c files, compiled by
the C compiler inside the link command with the link's flags and no
debug information. A Linux LLVM build links with full LTO, and the LTO
inlines a C function and its `xo_c_` thunk into the Xo caller
(gotchas/linux-lto-inlines-c-frames.md): the function is then no frame
of a crash report or a fault's traceback, and with no debug information
for the C unit the binary's DWARF has no `DW_TAG_inlined_subroutine` for
it either, so dwline's inline chain cannot name it. TestCrashReport's
helpers were marked noinline for the walker's sake; a user's C function
still vanished from reports. On darwin, where ld64 keeps the function
out of line, the frame had a name but no file and line.

## Decision

The C sources are compiled to objects with debug information before the
link, on both native backends:

- compiler/xo cffi.xo `c_objects`: each `.c` among the link inputs is
  compiled with `cc -g -O2 -c` and the link's own compile flags
  (asmparts.xo asm_cflags: the link flags without -l, -L, -Wl and
  objects, so the target, sysroot, sanitizer and `-mllvm` flags carry
  over) into `<private dir>/cffi/NAME-<key>.o`, named by the source
  bytes and flags and reused while they are the same; the other inputs
  (objects, archives) pass through in their order.
- The LLVM backend (main.xo, before the link) adds the link's LTO flag
  (`-flto` for a single module build's full LTO, `-flto=thin` for a
  split build with thin LTO), so the object is bitcode and the LTO
  inlines it as before; the arm64 backend compiles a plain object.
- The C unit's DWARF rides into the binary with the Xo unit's: a crash
  report or traceback names a C frame with its file and line
  (`crashc_write (crashc.c:11)`), and an inlined one as `name
  (file:line) [inlined]` through dwline's inline chain (prof.xo), which
  already walks every unit.
- Windows (lld-link, PDB) keeps compiling the C sources in the link:
  not changed here.
- compiler/debug (dwarf.xo read, the symbolizer and debugger's reader)
  names an inlined call whose abstract origin is in another unit: the
  thunk LLVM's LTO inlines into poke has its origin in the C unit, a
  DW_FORM_ref_addr reference, and the per unit name table gave `?`
  (`xo dev symbolize` printed `crash+0x73ed4` where the report said
  `xo_c_..._crashc_write (xo_c_....c:37) [inlined]`, linux/arm64); the
  table and the calls to name now span every unit.

TestCrashReport: `crashc_write_inl` (no noinline, kind `inl`) is
reported with its file and line as its own frame or as an inlined frame
of poke; on Linux with the LLVM backend the inlined form is required.

## Consequences

- A `use c` program's binary carries the C units' debug information:
  TestCrashReport's program (three small C functions and the adapter) on
  darwin/arm64 grew from 1623118 to 1624430 bytes on the arm64 backend
  (+1.3 KB) and from 1087053 to 1103693 on the LLVM backend (+16.6 KB:
  the bitcode's debug information survives the LTO link in full); a
  stripped or --release build drops it as it drops the Xo unit's.
- The C compile is a step of its own before the link, cached by content
  in the build's private directory: the first build of a program runs
  one `cc -c` per C source, later builds reuse the objects. The binary
  cache keys on the objects' paths as it did on the sources.
- A crash report's C frames gain `(file:line)`: the symbolizer tests and
  TestCrashReport's patterns accept it.

## Alternatives

- `-g` on the link line: the same flags feed the assembly of the .s
  parts (cc_assemble), and `as -g` on an assembly that already carries
  `.debug_*` directives duplicates or rejects the sections.
- Marking every C function noinline (the TestCrashReport fix): keeps a
  frame at the cost of the inlining users expect from `use c`, and
  still gives the frame no file and line.
- Leaving it: a crash in C code names the Xo caller only on Linux LLVM
  builds.
