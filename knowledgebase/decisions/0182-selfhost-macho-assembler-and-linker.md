# 0182: A Mach-O assembler and linker in Xo, so native builds need no `cc`

Status: Accepted (2026-10-12), stages 1 and 2 implemented in
`selfhost/macho` (asm.xo, enc.xo, cfi.xo, obj.xo, out.xo: the
assembler; lin.xo, link.xo, unwind.xo, sign.xo: the linker; main.xo:
`macho asm`, `macho link`; macho_test.xo) and `selfhost/xo` (`--as=macho`,
`--ld=macho`, ld.xo); `scripts/macho-compare.py`; `XO1_FLAGS` and
`XO1_PATH` in `scripts/selfhost-goldens.py`; `TestSelfhostMachoXoTests`.
Stage 3 (ELF) not started. Evidence: `bench/results/macho-2026-10-12.md`.

## Context

xo1 (`selfhost/xo`, decisions 0158 and 0174) writes arm64 assembly and
hands it to the C compiler driver, which assembles it and links it with
the C runtime (a prebuilt pack object, decision 0157). Decision 0157
noted that a compiler with no external tools needs the arm64 backend to
write object files and link them itself. The Go implementation is being
retired (decision 0150 as amended), so this is written in Xo, in
`selfhost/macho`, and xo1 calls it.

What `cc` does for a native arm64 build today (darwin/arm64, Apple clang
21, `cc -v` on `xo build --backend=arm64` of the recursion golden,
2026-10-12), with internal/build's and xo1's arguments
`-O2 -fexceptions -funwind-tables -o BIN -isysroot SDK prog.s RT.o -lm -lpthread`:

1. **Preprocess** the `.s` (`clang -cc1 -E -x assembler-with-cpp`): a
   no-op for the emitter's output, which has no `#` directives at the
   start of a line; `//` comments are dropped.
2. **Assemble** (`clang -cc1as -triple arm64-apple-macosx26.5.0
   -filetype obj -mrelocation-model pic`) into an MH_OBJECT:
   - instructions encoded, branches and `adr` within `__text` resolved;
     `adrp`/`add` with `@PAGE`/`@PAGEOFF` become ARM64_RELOC_PAGE21 and
     ARM64_RELOC_PAGEOFF12 against the target's atom (the nearest
     preceding non temporary symbol of its section, or the section's
     `ltmpN` symbol) with an ARM64_RELOC_ADDEND for the offset, except
     that temporary labels of a `cstring_literals` section stay symbols;
     `bl` to an undefined symbol becomes ARM64_RELOC_BRANCH26;
   - `.uleb128` label differences (call site tables) and `.long` label
     differences (jump tables) computed, ULEB128s minimal;
   - the `.cfi_*` directives become one 32 byte `__LD,__compact_unwind`
     entry per function (LLVM's AArch64 compact encoding: frame mode
     when the CFA is fp with the fp/lr pair at its base and callee
     saved pairs below, otherwise DWARF mode, always DWARF with a non
     canonical personality such as `_xo_personality`), and an
     `__TEXT,__eh_frame` CIE and FDE for every DWARF mode function
     (augmentation `zR`, or `zPLR` with a POINTER_TO_GOT personality and
     a pc relative LSDA; SUBTRACTOR/UNSIGNED pairs for the pc fields);
   - symbol table: locals, then the defined externals and the undefined
     symbols sorted by name; LC_BUILD_VERSION, LC_SYMTAB, LC_DYSYMTAB.
3. **Link** (`ld -demangle -dynamic -arch arm64 -platform_version macos
   26.5.0 26.5 -syslibroot SDK -O2 -o BIN prog.o RT.o -lm -lpthread
   -lSystem .../libclang_rt.osx.a`; `-lm` and `-lpthread` are libSystem
   on macOS) into an MH_EXECUTE (MH_PIE): segments `__PAGEZERO`,
   `__TEXT` (`__text`, `__stubs`, `__gcc_except_tab`, `__const`,
   `__cstring`, `__unwind_info`, `__eh_frame`), `__DATA_CONST` (`__got`,
   `__const`), `__DATA` (`__data`, `__thread_vars`, `__thread_data`,
   `__thread_bss`, `__bss`), `__LINKEDIT`; load commands
   LC_DYLD_CHAINED_FIXUPS (binds of libSystem imports and rebases of
   absolute pointers), LC_DYLD_EXPORTS_TRIE, LC_SYMTAB, LC_DYSYMTAB,
   LC_LOAD_DYLINKER, LC_UUID, LC_BUILD_VERSION, LC_SOURCE_VERSION,
   LC_MAIN, LC_LOAD_DYLIB (libSystem.B.dylib), LC_FUNCTION_STARTS,
   LC_DATA_IN_CODE, LC_CODE_SIGNATURE (ad hoc, required for arm64
   macOS). The runtime object adds what the program's object does not
   use: GOT loads (ARM64_RELOC_GOT_LOAD_PAGE21/PAGEOFF12), thread local
   variables (TLVP_LOAD_PAGE21/PAGEOFF12, `__tlv_bootstrap`),
   PAGEOFF12 on scaled loads and stores, `__literal8`, `__literal16`,
   `__DATA,__const`, `__bss`; 107 undefined symbols, all in libSystem
   (`_malloc`, `_pthread_*`, `__Unwind_*` of libunwind,
   `___chkstk_darwin`, `___stack_chk_guard`, ...).

## Decision

Three stages, each committed with its check, in `selfhost/macho` (a
package xo1 imports, with a command line of its own):

1. **Assembler** (`macho.assemble`): the subset of arm64 assembly the
   emitter writes (the instructions, directives, and relocation
   operators of every arm64 golden and of xo1's own assembly) into an
   MH_OBJECT as in step 2 above. Check: every arm64 golden assembled by
   it, linked by the system `ld` exactly as `cc` links, behaves as the
   `cc` build (`scripts/selfhost-goldens.py` with xo1 using it), and the
   `__text` bytes compared with `as`'s object function by function.
2. **Linker** (`macho.link`): the program's object and the runtime
   pack's objects (and `libxotls.a` members) into an MH_EXECUTE as in
   step 3, with libSystem imports bound through chained fixups,
   `__unwind_info` built from the compact unwind entries, and an ad hoc
   code signature (the format of Go's cmd/link
   `macho_code_signature.go`: a SuperBlob with a CodeDirectory of SHA-256
   page hashes, 4 KiB pages). Check: the arm64 goldens run identically
   with no `cc` on `PATH`.
3. **ELF** for linux/arm64, later.

Choices made on the way:

- **Byte identical objects are the target**, not just equivalent ones:
  the assembler reproduces LLVM's section order (`__text` always
  first), its symbol registration order, its string table, its compact
  unwind algorithm, its FDE grouping and padding, and its odd
  POINTER_TO_GOT personality field (gotchas/
  llvm-symbol-order-and-personality-field.md), so any difference from
  `cc -c` is a bug the comparison shows.
- **The linker concatenates** input sections by name (no atoms, no dead
  stripping, no literal merging), allocates common symbols in
  `__DATA,__common`, moves `__DATA,__const` into `__DATA_CONST`, turns
  TLVP loads of in image descriptors into adds and the descriptors'
  offsets into template offsets, and builds `__unwind_info` with regular
  second level pages of 511 entries (DWARF mode entries hold their FDE's
  offset in the merged `__eh_frame`).
- **Imports without the SDK**: every import binds to libSystem by
  ordinal 1 (it re-exports libsystem_*), or by flat lookup when more
  dylibs are linked (std/http's Security and CoreFoundation). No `.tbd`
  is read, so linking needs no SDK; a missing symbol fails at launch.
- **LC_BUILD_VERSION 11.0/11.0** (as Go's linker writes), the deployment
  target of the pack's objects.
- **xo1**: `--as=macho` assembles in process and lets cc link;
  `--ld=macho` also links, from a pack only (the runtime object cache's
  key needs the C compiler), and writes std/time's zone data as an
  assembled object (`tz_object`) instead of `xo_tzdata.c`.

Programs that still need a C compiler with the linker in place: `use c`
sources. A build without a Go toolchain also needs `XO_ZONEINFO` for
std/time (decision 0157, next step 1).

## Consequences

- Stage 1: on the 158 arm64 goldens and on xo1's own assembly (13.8
  million lines), every section, function (3,792 and 2,825), and
  relocation is identical to `cc -c`'s, and all 159 objects are byte
  identical; assembling xo1 takes 4.92 s instead of 35.40 s. xo1 with
  `--as=macho` passes 158 of 158 goldens.
- Stage 2: xo1 with `--ld=macho` and an empty PATH passes 158 of 158
  goldens; xo1 links itself with no cc (B), B links C, and B and C are
  byte identical and pass 158 of 158; std/time zone data links. The
  binaries are 0.1 percent smaller than ld's; a small link is 5 times
  faster (0.010 s against 0.054 s), linking xo1 4 times slower (0.44 s
  against 0.11 s).
- A self hosted toolchain on macOS now needs only the pack, not the
  Command Line Tools; the released `xo` (Go) is unchanged and still
  uses cc (decision 0150: switching is the owner's call).
- Found on the way: a struct named `Target` anywhere in a program breaks
  the native build's predeclared `target` (gotchas/
  struct-named-target-breaks-predeclared-target.md), and the TLS
  library holds common symbols (gotchas/tls-library-common-symbols.md).

Next steps, ranked by likelihood of working:

1. Make `--ld=macho` xo1's default when a pack is found and no `use c`
   source is linked, keeping cc as the fallback (one line in
   selfhost/xo/main.xo, then the goldens and the fixed point again).
2. Ship zoneinfo.zip with the pack (decision 0157 next step 1) so std/time
   needs no Go toolchain either: `tz.xo` `zoneinfo_zip` looks beside the
   pack first.
3. ELF: an ELF64 relocatable writer for the emitter's linux syntax (the
   same parser; `.type`, `:lo12:`, `.gcc_except_table`, the
   `DW.ref.xo_personality` comdat; R_AARCH64_* relocations; `.eh_frame`
   only, no compact unwind), checked against `zig cc -c -target
   aarch64-linux-gnu` byte for byte as stage 1 was; then a static or
   glibc dynamic linker with the linux pack.
4. Port internal/ssa's `target` resolution to compare the prelude type
   by identity (the gotcha above), and the same in selfhost/ssa.
5. LC_FUNCTION_STARTS and local symbols for profilers, and `.tbd`
   reading to report a missing import at link time.

## Alternatives

- **Port the assembler and linker in Go first** (internal/arm64): the Go
  implementation is being retired (decision 0150 as amended); a Go
  version would be thrown away.
- **Ship LLVM's `llvm-mc` and `ld64.lld`**: two large binaries per
  host, and still an external tool.
- **Write the object directly from the emitter, skipping text**: the
  `.s` is the emitter's tested interface (byte identical with the Go
  emitter's); assembling it keeps that comparison and lets the
  assembler be checked against `as` on the same input.
