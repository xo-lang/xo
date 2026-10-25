# 0371: empty the functions nothing reaches after code generation (arm64 hollow pass)

Status: Accepted (Xo Lead, 2026-10-24, as size step 1; built the same
day: compiler/arm64 hollow.xo).

## Context

compiler/arm64 reach.xo's `drop_unreached` removes dead functions before
code generation (hello world kept 441 of 1,015, 2026-10-18). It cannot
drop:

- every C ABI export (`Func.export`, the `xo_x_` stubs of
  std/native/llrt/cabi*.xo, decision 0207), since C code may call one by
  name;
- the functions only an export reaches;
- the helpers the emitter makes itself (`_xo_h*`: a type's drop or
  equality), since the emitter calls them and the runtime by symbol
  name, which the SSA does not show.

So a release hello world linked 71 export stubs and the code behind
them: netconn's dial, selects, chan sends, http stream frees. A program
whose link has no C objects never calls an export, and the assembly
names every reference the SSA hides.

## Decision

A pass over the program's assembly, after emit and before assembling
(compiler/xo main.xo, `arm64.hollow_unreached` over each part):

1. **Functions.** A function runs from a global label (`_name:`) in
   `__TEXT,__text` to the next one or the next section switch. Global
   labels with no instruction between them name one body: a drop
   helper's `_xo_f_drop_T:` and `_xo_h...:`. A shared local label in a
   function (`Lxo_drop1:`, which other functions branch to) is a name of
   that function. Its declaration lines (`.globl`, `.private_extern`,
   `.p2align`) name a symbol without using it.
2. **Roots.**
   - `_main`.
   - Every symbol named outside a function's text: data, the unwind and
     exception tables, the unsafe range tables, the export stubs' tables.
     `__DWARF` is not a root, since it names every function.
   - The unsafe range markers (`_xo_utext_*`, `_xo_utend_*`).
   - Reachability is a worklist over the symbols each function's lines
     name.
3. **Hollowing.** A function no root reaches keeps its labels and its
   layout and symbol directives, so the tables that name its labels
   still assemble. It loses its instructions, data and CFI, so no zero
   length FDE lands on the next function. Its body is one
   `brk #0x484f` ("HO", `HOLLOW_BRK`), 4 bytes: a path the scan missed
   faults at once, with an immediate that names this pass, instead of
   running into the next function.
4. **Platforms.** It runs only when all of these hold:
   - darwin/arm64 (Mach-O symbols start with `_`, which the scan relies
     on);
   - the program has no `use c` module, since a C object could call an
     export by name;
   - the build is not `xo asm`, whose output stays the emitter's.

   amd64 (AT&T text, Mach-O or ELF) and linux/arm64 ELF are left as they
   are until each is verified.
5. **Switch.** `XO_PASSES=-hollow` turns it off. The build stays correct
   either way.

## Result (2026-10-24, on 3a435412a, release builds)

| program | __text off | __text on | change |
|---|---|---|---|
| hello world | 235,612 | 164,896 | -30.0% |
| bench/perf/http/xo (CRUD) | 1,519,088 | 1,473,008 | -3.0% |
| xo itself (s2 of the fixed point) | 60,128,952 | 60,097,632 | -0.05% |

CRUD's 11 tests pass with the pass on, and hello runs; the fixed point
holds with the pass on and off (s2 and s3 identical).
internal/arm64 TestHollowUnreached builds an in-process std/http router
program. It checks that an unreached C ABI export
(`_xo_x_scope_fail_site`) and an unreached drop helper
(`_xo_f_drop_..._RootsData_3f`) are each one `brk #0x484f` (and not
with the pass off), and that the program prints the same either way.
compiler/arm64 hollow_test.xo covers the scan on a synthetic assembly:
the aliased labels, a shared local label, and a data root.

## Consequences

- Each dead function keeps its alignment padding and a 4 byte brk.
  The address space a hollowed function took is not reclaimed beyond
  that.
- The dead functions' unwind entries, exception tables and
  `_xo_unsafe_*` rows stay, now naming an empty range. Dropping them is
  a follow-up.
- A reference the scan does not see shows up as a SIGTRAP at a
  `brk #0x484f`. Two examples: an address computed at run time, or a
  name looked up by profiler symbols. The first cut hit one such case
  (a `bl Lxo_drop1` into a helper), which the shared local label rule
  fixes.
- The per-function backend cache is unaffected: the pass runs on the
  assembled parts' text, after the cache.

## Alternatives

- Teach reach.xo the emitter's symbol references (drop helpers, runtime
  calls) and drop exports when no C is linked. Rejected for step 1:
  every emitter change would have to keep that list in step, and the
  assembly already has it.
- Linker dead stripping (`-dead_strip`). The internal Mach-O linker has
  no atom model. Its sections are whole, so it would first need
  subsections_via_symbols per function. That is larger than this pass,
  and a candidate for a later step.
- An empty body (no brk). A missed reference would then run into the
  next function's code. Rejected (Lead, 2026-10-24).

## Addendum: data objects nothing reaches (2026-10-25)

Lang 3 measured a release hello with the pass on. 270 of its 819
fault-location strings in __cstring (8,581 bytes) were named only by
hollowed bodies, and so were many Str literal objects in __const.

The pass now treats each local data object as a chunk of its own:
- A data object is a `Lxo_` label outside the text: a location string,
  or a literal's object. Its chunk runs to the next label or section.
- An object nothing live names (no surviving body, and no data that is
  itself reached) loses its label and data. Its alignment and symbol
  directives stay, since the next object's alignment can sit in its
  range.
- An object that __DWARF names stays, so its label still assembles.
- Global data labels are unchanged: their data are roots.

Release builds, main 303dc270d against the change:

| program | __cstring | __const | file |
|---|---|---|---|
| hello | 26,180 to 17,343 | 47,505 to 17,121 | 332,314 to 299,289 (-9.9%) |
| bench/perf/http/xo | 94,566 to 88,266 | 253,721 to 222,337 | 2,030,217 to 1,997,192 (-1.6%) |

__text is unchanged. compiler/arm64 hollow_test.xo now also checks:
- a location string named only by a hollowed function goes;
- a Str object named by main stays;
- a string named only by DWARF stays.

## Addendum: linux/arm64 ELF (2026-10-25)

The pass now runs on linux/arm64 too (hollow_unreached_elf). It reads
ELF assembly text:
- symbols with no leading `_` (`xo_...`, `main`);
- local labels `.Lxo_...`;
- `.text`, `.data`, `.bss` and `.section` switches, and `.debug_*`
  sections in place of __DWARF;
- `.hidden`, `.type` and `.size` lines, which name a symbol without
  using it and stay with a hollowed function.

Calls into libc (`bl memcpy`) are not the program's symbols and stay
calls.

Release builds for linux/arm64, the same xo with XO_PASSES=-hollow as
the base:

| program | file off | file on | change |
|---|---|---|---|
| hello | 379,320 | 283,672 | -25.2% |
| bench/perf/http/xo | 2,098,608 | 2,018,400 | -3.8% |

compiler/arm64 hollow_test.xo has an ELF case: an unreached export and
its location string go, while a Str object main names and a libc call
stay. The Docker linux/arm64 goldens are the gate. x86_64 (AT&T text) is
still not covered.

## Addendum: amd64 (2026-10-25)

The pass reads x86-64 assembly too, Mach-O and ELF, with the same scan:
the amd64 emitter spells a function's global label, its `Lxo_`/`.Lxo_`
shared labels, the data objects, the section switches and the
`.globl`/`.private_extern`/`.p2align`/`.hidden`/`.type`/`.size` lines as
arm64's; its `callq`, `jmp`, `lea` and RIP-relative operands carry the
same `_xo_`/`xo_` names. One difference: an x86 function's own branch
labels are `LpGL1jtk_f1_b0:` (Mach-O) or `.LpGL...` (ELF), not `Lxo_`,
so they stay inside their function's chunk and the scan never takes one
for a function. A hollowed body is one `ud2` (an invalid opcode, SIGILL):
x86 has no immediate to carry a marker as arm64's `brk #0x484f` does.

**Targets.** linux/amd64 runs the pass by default: the Docker amd64
goldens build and run those binaries. darwin/amd64 is opt in, with
`XO_PASSES=+hollow`, because nothing here can run an x86_64 Mach-O
binary (the development Mac has no Rosetta; the owner's Intel laptop is
offline), and a pass that only a size table checks should not change a
shipped build. `+NAME` is new in `XO_PASSES` (compiler/arm64
passes.xo): `-NAME` still turns a pass off, `+NAME` turns on the one a
target leaves off, and naming one both ways is an error.

Release builds, the same xo with XO_PASSES=+hollow and XO_PASSES=-hollow
as the two sides (sizebreak's __text, 2026-10-25):

| program | target | __text off | __text on | change |
|---|---|---|---|---|
| hello | linux/amd64 | 280,696 | 203,960 | -27.3% |
| bench/perf/http/xo | linux/amd64 | 1,946,024 | 1,895,320 | -2.6% |
| hello | darwin/amd64 (+hollow) | 259,728 | 187,728 | -27.7% |
| bench/perf/http/xo | darwin/amd64 (+hollow) | 1,880,336 | 1,832,560 | -2.5% |

For comparison, the same builds on arm64: hello 254,544 to 184,448
(-27.5%, linux) and 242,340 to 170,832 (-29.5%, darwin); CRUD 1,522,560
to 1,476,116 (-3.1%, linux) and 1,456,556 to 1,410,348 (-3.2%, darwin).
The arm64 numbers are the same as the ELF addendum's, so the shared scan
and the trap parameter changed nothing there.

Tests: compiler/arm64 hollow_test.xo gains an amd64 Mach-O case and an
amd64 ELF case (an unreached export and its drop helper become one
`ud2`, a `jmp` to a shared `Lxo_drop1` keeps it, a RIP-relative `lea`
and a `.quad` table are roots, a libc `memcpy:` label is not a function
of the program), and XO_PASSES' `+`/`-` parsing. internal/arm64
TestHollowUnreachedAmd64 cross builds the router program for
darwin/amd64 with `XO_PASSES=+hollow`, disassembles it with otool (the
assembler pads a 2 byte body with nops, which the test does not count)
and checks the same export and drop helper against `XO_PASSES=-hollow`;
the binary itself is not runnable here, and the test says so. The
Docker amd64 goldens build and run linux/amd64 with the pass on, and
internal/arm64 TestAmd64CrossBuilds builds four goldens for the three
amd64 targets.

Gates (2026-10-25): the fixed point (bootstrap-xo.sh, s1 = s2 = s3,
identical assembly and binary, hello runs); the full internal/arm64
package; every golden cross built for darwin/amd64 (375 of 375); and the
full Docker amd64 run, all 375 goldens: 372 ok, 1 skip
(proc_children_load hangs under Rosetta), 1 build note (use_c_add needs
a C compiler), and 1 failure, slp_f64, which fails the same way with
`XO_PASSES=-hollow` (a NaN payload sign bit: 0x7ff8... against
0xfff8...), so it is x86 hardware behaviour in that arm64 SLP golden and
not this pass. It is reported as a known linux/amd64 issue.

`+NAME` changes which passes run without making the module's
`passes_off` non-empty, so the four sites that turn the caches off
when XO_PASSES is set (compiler/xo main.xo lazy inlining, the
per-function cache, its whole-build condition, and bcache.xo's
reference count cache) now read `passes_changed(os)`, which is true for
either a `-` or a `+` entry: a `+hollow` build neither stores nor
reuses entries of a plain build.
