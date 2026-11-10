# ARM64 COFF atoms mode branches resolved in place

Status: Observed 2026-11-10 (fixed in the same change).

**Symptom.** On windows/arm64, a program built against std's separately
compiled store (`XO_SEPCOMP=1`, `xo dev stdcache`) faulted 0xc0000005
before main; the whole program build of the same source ran. `xo debug`
showed the fault address in .rdata. The internal/arm64 TestStd* tests
(StdRecordDefaultArgs and others) failed on the winarm VM for this reason.

**Cause.** compiler/macho/coffa64.xo `coff_a64_fixes` resolved every
branch to a non weak symbol of the same section in place. In an atoms mode
object (`.subsections_via_symbols`, every std store part), pestrip.xo
`coff_split` cuts the section into atoms and the link lays them out anew,
so a resolved branch between two atoms kept its old distance and landed
in other code or data (entry.init's `bl heapos.mag_caps` went into
.rdata). The ELF and Mach-O path (asm.xo `apply_fixes`) already relocated
a branch to another atom; the COFF ARM64 writer, added later, did not.

**Find it.** Count branches whose target lies outside .text in the
linked exe (llvm-objdump -d, b/bl/b.cond/cbz/tbz targets against the
.text range): 29 in the separate build against 5 in the whole build.

**Fix.** `coff_a64_fixes` takes the same locality test as `apply_fixes`
(`atom_at(site) == target's atom`), so a branch between atoms becomes a
relocation; regression test in coffa64_test.xo. A new object writer
for an atoms mode format must take this test too.
