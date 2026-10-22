# 0346: DWARF embedded in Mach-O executables

Status: Accepted (owner, 2026-10-22). Proposed the same day by Xo Lang 1
at the owner's request through the lead.

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
3. The ld64 links: the post-link step or the bundle, by measurement.
4. `--dsym`, the docs (toolchain.md, the spec's mentions of PROG.dSYM),
   and the cache: `xo run`'s cached binary needs no bundle beside it.

## Consequences

- One file: copying, caching, packaging or renaming a binary keeps its
  names and lines; a crash or race report from a CI artifact names its
  frames.
- Larger debug binaries: hello is 673 KB with a 1.3 MB dSYM today, so
  about 2.0 MB embedded; xo itself measured in stage 1. Release binaries
  are unchanged (`--release` strips).
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
