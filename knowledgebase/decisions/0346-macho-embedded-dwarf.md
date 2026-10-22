# 0346: DWARF embedded in Mach-O executables

Status: Accepted (owner, 2026-10-22). Proposed the same day by Xo Lang 1
at the owner's request through the lead. Stage 1 implemented
2026-10-22: Xo's Mach-O linker writes the `__DWARF` segment (prot 0,
before `__LINKEDIT`; compiler/macho link.xo, dsym.xo dwarf_data shared
with the bundle) and the runtime reads the executable first (dwline.xo
dsym_sects). With no dSYM present: the crash report's traceback, `xo dev
symbolize`, atos (`xo_f_poke (in cp) (main.xo:5)`) and lldb (a breakpoint
at main.xo:9, a line lookup) name file and line; `dwarfdump --verify`
finds no error; `codesign -v --strict` passes. Measured: hello 2,126,114
bytes (its DWARF, 1,401,811 bytes, page aligned in the executable); xo
60.6 MB to 102.7 MB (its dSYM is 40.6 MB); the link phase 637 to 911 ms
before and 912 to 1,116 ms after, the dSYM still written too until stage
4 makes it `--dsym`'s; `-s` and `--release` leave both out as before. A
link through ld64 (`use c`, the LLVM backend) has no segment: atos and
lldb find its lines through ld64's debug map (the build directory's
objects), the runtime through its dSYM (stage 3).

Stage 4 implemented 2026-10-22: no PROG.dSYM by default; `--dsym`
(compiler/xo opts.xo, macho.LinkOpts.dsym) writes it too, and is part
of the binary cache's key (an entry linked without it has no bundle);
an earlier --dsym build's bundle beside the output is removed. The
arm64 debug tests read the executable (dwarfFile in debugline_test.go)
and check that the default writes no bundle, `--dsym` writes one that
verifies, and `-s` writes neither and has no `__DWARF` segment.

Release binaries (owner, 2026-10-22, through the lead): ship with their
DWARF. scripts/release.sh builds the darwin xo with neither `--release`
nor `-s`, so it now carries its `__DWARF` segment: the xo binary 60.6 MB
to 102.7 MB, its tar.gz 23,178,098 to 34,366,894 bytes (+11.2 MB, +48%).
Users' crash reports, tracebacks and race reports name file and line;
the alternatives (strip with `-s`, or publish a separate dSYM per
release) were declined.

Stage 2 done 2026-10-22, every outside reader against the embedded copy
with no dSYM present: atos names file and line (`xo_f_entry_2emain (in
hello) (entry.xo:97)`), and ThreadSanitizer's reports through it
(TestRaceStacksArm64 passes); lldb resolves a breakpoint at main.xo:9
and prints frame variables (TestDebugVars, TestDebugTypes); `dwarfdump
--verify` finds no error in hello; `codesign --verify --strict` passes.
A Developer ID signed hello and xo (hardened runtime, timestamp,
scripts/sign-macos.sh) were notarized with their `__DWARF` segments, as
the owner approved: submission 0ff4dfa7-c4e4-4117-b05d-9470e93ef0c0,
Accepted, no issues; `spctl -a -t install` accepts hello as "Notarized
Developer ID" (a bare executable is "not an app" to the default
assessment, as code-signing.md says), and the signed copies run and
symbolize. The checks found three older DWARF faults, fixed in
0226ad6dd: atos named nothing at the first function of each part (a
second symbol, the part's `xo_utext_` marker, at its address; the
linker now leaves the markers out of the symbol table,
gotchas/atos-two-symbols-one-address.md); DW_AT_comp_dir was the
working directory, not the line root the names are relative to, so
lldb opened the wrong main.xo for `xo build DIR` from elsewhere; and a
type with no words (a List[()]'s items, a field of type ()) referred to
offset 0, which `dwarfdump --verify` rejected (101 times in xo).

Stage 3 done 2026-10-22, the post-link step: after an ld64 link (the
LLVM backend, `--ld=cc`, `use c`) and its dsymutil, compiler/macho
embed.xo `embed_dwarf` copies the dSYM's `__DWARF` sections into the
executable as the same kind of segment (vmaddr `__LINKEDIT`'s, vmsize 0,
as Go's; every `__LINKEDIT` file offset in the load commands moved by
the DWARF's page aligned size) and signs it again ad hoc under ld64's
identifier; compiler/xo link.xo cc_dsym keeps the bundle only for
`--dsym` (in the binary cache's key) or when embedding fails (a warning
says why). ld64 leaves 32 bytes after the load commands, so these links
pass `-Wl,-headerpad,0x1000` (gotchas/ld64-header-room.md). Both
ld64 outputs of hello: `__DWARF` present, `dwarfdump --verify` no
errors, `codesign --verify --strict` passes, lldb resolves main.xo:9,
and atos names file and line with the debug map's stabs stripped
(`xo_f_num_2eappend__int (in nostab) (num.xo:74)`, `x.cabi_c.main (in
nostab) (cabi_c.xo:15)`), so the copy, not the build directory's
objects, is what it read. Measured on internal/llvm's http_server
golden (three runs each, XO_BIN_CACHE=off, `--phase-times` cc phase):
`--ld=cc` 210, 214, 588 ms before and 251, 226, 260 ms after; the LLVM
backend 19.3, 19.3, 23.9 s before and 19.3, 19.6, 18.8 s after (its
link is dominated by clang). The executable grows by its bundle:
`--ld=cc` 2,505,944 bytes plus a 3.75 MB dSYM to 6,205,125 bytes, LLVM
2,652,304 plus 2.24 MB to 4,815,612. Keeping the bundle for these links
was the other way measured: no link time saved worth the second
artifact, so every darwin link now gives one file.

## Context

A darwin program's DWARF lives in a separate bundle today
(decision 0239): Xo's Mach-O linker leaves the objects' `__DWARF`
sections out of the executable, as ld64 does, and writes
PROG.dSYM/Contents/Resources/DWARF/PROG itself (compiler/macho dsym.xo:
the sections joined and relocated, the executable's LC_UUID); a link
through cc runs dsymutil (compiler/xo link.xo cc_dsym). Everything that
names a frame looks for that bundle beside the executable:

- the runtime's fault tracebacks and crash reports (decision 0343) and
  the profiler (std/native/llrt dwline.xo `dsym_sects`: the dSYM whose
  LC_UUID is the running image's);
- `xo debug`, `xo dev dwarf`, `xo dev symbolize` (compiler/debug);
- lldb, and atos, which `--race` reports call through ThreadSanitizer's
  symbolizer.

The bundle is a second artifact to keep with the binary. A copied,
renamed, cached or packaged executable loses its names silently: `xo
run`'s cached binary, a binary moved into a container image or a CI
artifact without its .dSYM, a release archive. The program still runs;
its reports just name nothing.

Go embeds the DWARF in the executable. The lead measured Go on this Mac
(2026-10-22):

- Go's default, compressed DWARF (`__zdebug_*` sections in a `__DWARF`
  segment): atos gives names only (no file:line), lldb finds no line
  entry, dwarfdump finds nothing. Apple's tools do not read compressed
  sections.
- With `-ldflags=-compressdwarf=false`, the same DWARF uncompressed:
  `atos` gives `main.hello (in p2) (main.go:6)`; dwarfdump and lldb work.

## Decision

Darwin executables carry their DWARF uncompressed in a `__DWARF`
segment of the executable, by default, instead of PROG.dSYM.

- What is embedded: what dsym.xo writes to the bundle today (the joined
  `__debug_info`, `__debug_abbrev`, `__debug_line`, `__debug_str`,
  `__debug_line_str`, `__debug_addr`, `__debug_loclists`,
  `__debug_aranges`), the same bytes, at the executable's own addresses.
  The segment is not mapped for reading, writing or execution (`maxprot`
  and `initprot` 0, as Go's) and comes before `__LINKEDIT`, which
  codesign requires to be last.
- Uncompressed: Apple's atos, lldb and dwarfdump read only that, and
  atos is ThreadSanitizer's symbolizer for `--race` on darwin.
- `--dsym` also writes PROG.dSYM, for tools or workflows that want the
  bundle; `-s` and `--release` leave the DWARF out as today and write no
  bundle.
- Readers look in the executable first, then a matching dSYM: dwline.xo
  (the running image's own `__DWARF`, read through its file, as on Linux
  and Windows), compiler/debug (sections() already reads a Mach-O file's
  `__debug_` sections: the executable instead of its dSYM), `xo dev
  symbolize`.
- Links through ld64 (the LLVM backend, `--ld=cc`) cannot keep
  `__DWARF` in their output. Stage 3 measures two ways: a post-link step
  that copies dsymutil's DWARF into the executable as a segment before
  `__LINKEDIT` and signs it again, or keeping the bundle for those links.

### Stages

1. Xo's Mach-O linker embeds the segment; dwline.xo and compiler/debug
   read it. Tests: TestTraceback, TestCrashReport, the debug info tests,
   and `xo dev symbolize`, each with no dSYM present. Measured: hello's
   and xo's size, link time.
2. Every outside reader against the embedded copy: atos (ThreadSanitizer's
   reports, TestRaceStacksArm64), lldb (a breakpoint at a line, frame
   variables, TestDebugVars' lldb path), dwarfdump --verify. Code signing
   (`codesign --verify --strict` of an ad hoc signed debug binary and of
   the cached ones) and notarization of a Developer ID signed binary
   (submitted only with the owner's approval, through the lead).
3. The ld64 links: the post-link step or the bundle, by measurement
   (the post-link step, measured above).
4. `--dsym`, the docs (toolchain.md, the spec's mentions of PROG.dSYM),
   and the cache: `xo run`'s cached binary needs no bundle beside it.

## Consequences

- One file: copying, caching, packaging or renaming a binary keeps its
  names and lines; a crash or race report from a CI artifact names its
  frames.
- Larger debug binaries: hello is 673 KB with a 1.3 MB dSYM today, so
  about 2.0 MB embedded; xo itself measured in stage 1. A `--release`
  build strips; the released xo is not built with it and ships its
  DWARF (owner, 2026-10-22: +11 MB per darwin archive).
- The link writes into the executable the bytes it writes into the
  bundle today: about the same link time (measured in stage 1).
- A signed debug binary's signature covers its DWARF (the segment is
  before `__LINKEDIT`).

## Alternatives

- **Keep the dSYM** (today): two artifacts, and the names are lost
  whenever the bundle does not travel with the binary.
- **Compressed embedded DWARF** (Go's default): smaller, but atos, lldb
  and dwarfdump read none of it (measured with Go), so `--race` reports
  and lldb would lose their lines.
- **Both always** (embedded and a dSYM): doubles the debug output for
  no reader that needs the bundle; `--dsym` covers that case.
