# Byte identical objects: LLVM's symbol order and its personality field

Status: Observed (2026-10-12, decision 0182)

Symptom: objects of the self hosted assembler had every section and
relocation identical to `cc -c`'s, but 120 of the 158 golden objects
still differed: the local symbols came in another order, so relocation
symbol numbers and string table offsets differed too. Separately, the
`__eh_frame` CIE of a function with a personality held a field nobody
documents: `0xbfffff2d` where the personality's GOT pointer goes.

Cause: LLVM orders the local symbols of the symbol table by when the
assembler registered them: a label when it is defined, or earlier when
an instruction or data directive first names it (a call from `main` to
a function defined later registers that function at the call; a
`.cfi_lsda` operand does not register). Section start symbols (`ltmpN`)
register when their section is first entered, and `__TEXT,__text` is
always the first section, even when empty (then without
S_ATTR_SOME_INSTRUCTIONS). The string table is LLVM's
StringTableBuilder: names sorted by their reversed bytes, last first, a
name that ends the previous one sharing its bytes. The personality
field (ARM64_RELOC_POINTER_TO_GOT, pc relative, 4 bytes) holds minus
its own offset in the section minus 2^30; ld ignores it, the
relocation decides.

Fix: `selfhost/macho` registers symbols in that order (`Asm.register`,
`reg_order`), builds the string table the same way, and writes the
personality field as LLVM does; all 158 golden objects are byte
identical (`scripts/macho-compare.py`). When an object differs only in
the symbol table, compare sections and relocations by symbol name first:
the script does.
