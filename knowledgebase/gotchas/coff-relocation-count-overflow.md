# COFF: a section's relocation count is 16 bits

Status: Observed (2026-10-22, the 0.25.1 windows/amd64 xo.exe)

Symptom: the native windows/amd64 xo.exe passed `xo version` and `xo
check`, but every `xo build` died with 0xC0000005 (exit 139 under MSYS)
and no output. Its minidumps (lldb on the Mac opens them with xo.exe's
own DWARF) showed write_dumps(dumps=NULL), the generator's Program and
its G holding wrong words. Rebuilding with the backend cache off changed
nothing; small programs and the goldens worked; linux/amd64 with the same
backend (ELF) worked.

Cause: compiler/macho coff.xo wrote a section's NumberOfRelocations as 16
bits (o.u16) and coffread.xo read it as 16 bits. xo.exe's largest code
part has about 191,600 relocations, so the PE linker applied only the
count modulo 65,536: 132,788 calls stayed `e8 00 00 00 00`, a call of the
next instruction, and each build ran on with the callees skipped. The PE
format's extended relocations exist for this: IMAGE_SCN_LNK_NRELOC_OVFL,
the count field 0xffff, and the first relocation's VirtualAddress holding
the real count (that entry included), as LLVM's COFF writer does.

Fix: coff.xo writes extended relocations at 0xffff relocations or more
(and caps the section symbol's auxiliary count); coffread.xo reads them.
compiler/macho coff_test.xo assembles 70,000 calls (4,464 read back
before). A quick check of a PE: count `e8 00 00 00 00` in .text against
an ELF build of the same program (1,887 against 1,889 after the fix;
132,788 before).

Any 16 bit or 32 bit count in an object or image format (COFF symbols,
Mach-O nreloc is 32, ELF is 64) is worth a test at its limit when a
program the size of xo goes through it.
