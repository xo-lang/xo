# Mach-O x86_64 relocations: atoms, SIGNED_n, and an order dependence

Status: Observed (2026-10-18)

## Symptom

Xo's Mach-O x86_64 assembler (compiler/macho asm_amd64.xo
x86_macho_fixes, decision 0254 stage 6a) wrote objects that differed from
`clang -c -arch x86_64` in ways no golden showed at first: text 16 bytes
longer (short branches clang makes long), 15 relocations fewer, then
after a fix 7,400 fewer; then, on a hand written test, SIGNED_4 fields
off by 4, a cstring label referenced by section where clang names it.

## Cause

LLVM's X86MachObjectWriter rules, which differ from ELF's and arm64
Mach-O's:

- Atoms count even without `.subsections_via_symbols`: an atom spans from
  one non-temporary symbol to the next. A pc relative reference resolves
  in the object only when its target (a symbol, or a temporary label) is
  in the fixup's own atom. A call to any other function, a
  `.private_extern` one too, is a BRANCH relocation, and so a long jmp.
- A temporary label in another atom is relocated against the atom's
  symbol, its offset from it in the field; one in no atom (a literal
  section's) by section (r_extern 0, the field the displacement within
  the object) only while no earlier relocation has named the label
  itself and the field's value is 0; otherwise against the label, which
  then appears in the symbol table.
- The field is the target's offset from the relocated symbol less the
  bytes after the field; SIGNED_1, SIGNED_2, SIGNED_4 only for an
  expression of no constant of its own (`movl $7, sym(%rip)` is SIGNED_4
  with -4; `movl $7, sym+4(%rip)` is SIGNED with 0).
- __eh_frame has an FDE for every function, pc fields resolved in the
  object (no relocations), sorted stably by CIE (no personality first);
  the personality's GOT word holds 4. __compact_unwind skips functions
  of encoding 0 (no CFI instructions); the emitter's `.cfi_def_cfa`
  frames are DWARF mode.
- No ltmpN section symbols (arm64's writer makes them), code as bytes,
  CPU_TYPE_X86_64 with subtype 3.

The linker's side (compiler/macho link.xo x86_reloc, x86_eh_frame;
2026-10-18): those resolved fields stay correct only while their section
moves as a unit, and __eh_frame's do not: an FDE's function and LSDA
pointers are pc relative into __text and __gcc_except_tab with no
relocation, and the output puts __stubs, __const, __cstring and the
unwind info between, so each pointer is moved by hand (the FDE test of
link_amd64_test.xo fails without it). A SIGNED_n field is read as
target plus field less the address after the field, whatever n is; a
section relocation's target is the field's address plus 4 plus n plus
the field.

## Fix

Probe clang rather than reason from the documentation: assemble one
instruction per form and read the relocation and field
(compiler/macho/testdata/amd64-macho/signed_forms.s keeps 27 such forms);
`scripts/amd64-enc-goldens.py macho-objects` regenerates the test's
objects. All 193 goldens' darwin/amd64 objects are byte identical.
