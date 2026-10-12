# 0182: A Mach-O assembler and linker in Xo, so native builds need no `cc`

Status: Proposed (2026-10-12), stage 0 (this plan).

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

Programs that still need a C compiler with the linker in place: `use c`
sources, and std/time's zone data (`xo_tzdata.c`) until it is written
as data by xo1.

## Consequences

To be filled in as the stages land
(`bench/results/macho-2026-10-12.md`).

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
