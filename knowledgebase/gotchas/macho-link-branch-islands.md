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

Mach-O split link (Observed 2026-11-08): the same GCP tests (and
aiplatform/v1) failed on darwin/arm64 with xo 0.44.0's default,
separately compiled link (`prog-darwin.0.o: branch out of range`;
XO_SEPCOMP=0 linked). A split link puts the program's code in its own
output section, `__TEXT,_ptext` (prog_name, decision 0409), after std's
whole __text, and `islands` looked only at `__text`, so the program's
far calls got no veneers. The Mach-O islands now work as the ELF ones:
the span of __text, _ptext and __stubs decides, each code section gets
an island after every half reach and one at its end, and only an island
in std's __text refuses the half or rejects a link from one
(`isl_text`). The assumption to keep: every output section that holds
code must be in the islands' list; a new code section (as _ptext was)
needs adding there. The islands goldens only ran whole program
(XO_BACKEND_CACHE=off); internal/arm64 TestGoldenRunIslandsSplit runs
them through the split link and from std's half.
