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
test (compiler/macho link_islands_test.xo).

ELF (Observed 2026-11-07): Azure 1's GCP compute/v0.alpha test binary
failed the same way on linux/arm64 (`prog-linux.0.o: branch out of
range`): elflink.xo had no islands. It now has them too (elflink.xo
`islands`, `veneer`, `ElfLinkOpts.branch_reach`;
compiler/macho elflink_islands_test.xo). They go in when the code
(.text, .plt, a split link's .text.xoprog) spans more than 7/8 of the
reach, an island after every half reach of each code output section and
one at its end, and every ELF path shares them (link_elf, the split and
store links: rest_layout). An island in std's .text moves std's
sections, so that link keeps no half and a link from a half that would
need one falls back to the full link (half_refused, half_pins_ok);
islands in .text.xoprog alone keep the half.
