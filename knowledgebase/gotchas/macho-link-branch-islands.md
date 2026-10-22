# Xo's Mach-O linker: a __text past a bl's ±128 MiB reach

Status: Observed (2026-10-22, xo's self-build with every optional pass off)

Symptom: `xo: link: .../prog-darwin.0.o: branch out of range` linking xo
built with `XO_PASSES` turning every optional pass off: 182 MB of text
(about 125 objects), past the ±128 MiB a `bl` or `b` (BRANCH26) reaches.
The normal build is far below it, so only a pass bisection hit it.

Cause: compiler/macho link.xo patched BRANCH26 directly and failed past
its reach; ld64 inserts branch islands there.

Fix: the linker adds islands when __text passes 7/8 of the reach: a
section of veneers (adrp x16, add x16, br x16; x16 is the intra
procedure call register a call may clobber) after each half reach of
input sections, and a far branch goes through its region's veneer
(`islands`, `veneer`). `LinkOpts.branch_reach` shrinks the reach for a
test (compiler/macho link_islands_test.xo). The ELF linker (elflink.xo)
still fails the same way past the reach.
