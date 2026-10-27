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

## Addendum: darwin/amd64 on by default (2026-10-26, xo-lang-4)

The amd64 addendum left darwin/amd64 opt in (`XO_PASSES=+hollow`) until
an x86_64 Mach-O binary of the pass had run. It has: the Intel tester's
v0.32.0 round (macOS on an Intel Mac, darwin/amd64, release v0.32.0;
~/xo-lead/notes/lead/field/intel-v0.32.0.md, lines 20 and 21) built hello
and the web server with `XO_PASSES=+hollow`, and the hollowed server
passed CRUD, 404, about 113k requests a second and SIGINT. A second run
on the same Intel Mac, of hello and the CRUD server (bench/perf/http/xo)
built by this change with no XO_PASSES, debug and release (the Xo Lead
ran them on macOS 26.5.2, Intel Core i7-9750H, 2026-10-26): both hellos,
both servers through POST, GET, PATCH and DELETE, an unknown route's 404,
a bad JSON body's 400, a burst of 2,000 requests eight at a time (2,000
answered 200) and SIGINT (exit 0), a transcript equal byte for byte to
the same script's against darwin/arm64 builds.

So darwin/amd64 now hollows like every other native target, and
`XO_PASSES=-hollow` is the one switch. compiler/xo main.xo's hollow_on is
gone (the condition reads `pass_on(os, "hollow")`); `+NAME` still parses
(compiler/arm64 passes.xo parse_passes), turns nothing on that is not on
already, and still turns the caches off as any XO_PASSES entry does.

Release builds (`--release -s`), the same xo with XO_PASSES=-hollow and
with nothing set, darwin/amd64 (file and __text bytes):

| program | file off | file on | __text off | __text on | change (__text) |
|---|---|---|---|---|---|
| hello | 389,881 | 290,637 | 275,232 | 200,096 | -27.3% |
| bench/perf/http/xo | 2,340,168 | 2,274,028 | 1,914,944 | 1,864,416 | -2.6% |

Tests: internal/arm64 TestHollowUnreachedAmd64 builds the router program
for darwin/amd64 with no XO_PASSES and checks the export and drop helper
are one `ud2` against `XO_PASSES=-hollow` (before, it asked for the pass
with `+hollow`); TestHollowPassesChangedCaches now fills a cache with the
plain (hollowed) build and checks a `-hollow` build over it keeps the
bodies. Incremental hollow already covered darwin/amd64 (1869a496a).

## Addendum 2026-10-25: conc.built, the scheduler out of programs that cannot use it (Xo Lang 3)

`native/llrt/concflag.built()` is folded by the SSA builder to its `conc`
(ssa/expr.xo target_test): true when the program has a scope, a task, a
channel, `select`, a Mutex, an Atomic, a sleep, the network or standard
input (ssa/conc.xo), else false, like `dwline.built()`.
Elsewhere it is true (the Go, wasm and LLVM runtimes keep everything). Three
runtime sites ask it, and a development build keeps all three (the fold is
`conc || !release`; `xo dump` and the signal handlers stay where a developer
looks for a hang; a release build gives them up for size, as it does the
DWARF readers, ADR 0354's addendum):

- entry.xo main: with it false, main runs on its own 1 GiB thread (the path
  XO_MAIN_FIBER=0 always had), not on a fiber under the root's scheduler loop;
- ctx.xo init: the fiber, worker and stdin pump entries are not stored in the
  services record (their addresses kept the scheduler's whole call graph);
- entry.xo signals: no event loop (no poller thread, no self pipe) and no
  SIGINT, SIGTERM or SIGQUIT handler, only SIGPIPE ignored. The handlers
  count a signal and wake the poller, which a program with nothing to wake does
  not need: SIGINT and SIGTERM end it as the system does.

Changes in a release build of a program that never spawns, sleeps, waits on
I/O, takes a Mutex or asks `task.stop_requested` (that call is `conc` too):
SIGQUIT no longer writes the task tree (`xo dump` has no poller to
ask; the system's default SIGQUIT, a core dump, applies), and the monitor and
poller threads do not exist. `proc.run` and the network and stdin set `conc`;
file reads, `clock.now` and the rest of std do not and ran unchanged.

Release hello on darwin/arm64 (`__text`): 164,872 to 114,212 bytes (-30.7%),
file 308 to 228 KB; the CRUD server (conc) 1,404,588 to 1,410,412 (+0.4%, the
rest of this branch). All 379 goldens (every one built both ways by the same
run), the arm64 run corpus (26) and the std corpora (38) pass; hello builds
and runs with `--race`. Mutex-only, sleep-only, stdin-only, `proc.run`,
network-only and file-read programs were built and run by hand.
Open: the unwinder (21 KB, any fault raises through it), the crash reporter
(12 KB) and the profiler (14 KB with debug info) stay.

Known baseline of the goldens built with `--release` (2026-10-25, the same on
the merge-base xo and on this branch): of the 375 programs of
internal/llvm/testdata/golden, two differ from their `want.txt`, dbg_values and
lazy_const. Both print `dbg(x)` lines (`[main.xo:23:3] p = Point{x: 1, y: -2}`)
to standard error that a release build does not write, since `--dbg` is off
there and `dbg(x)` is `x` (core 11.4, 2.x flags table). The goldens are meant
to run as development builds; this is the expected release difference, not a
miscompile. Run them with `xo dev goldens --flags=--release` and expect exactly
these two.

## Addendum (2026-10-25, Xo Lead): the symbol tables of a release build

A release binary kept every local symbol: 856 of hello's, 2,971 of the CRUD
server's, of which the `__TEXT,__text` function names are the only ones
anything reads at run time (std/native/llrt/prof.xo's traceback: a release
build has no profiler, `profile_support_eff` is false, so tracebacks and
crash reports are the readers). Two changes, measured on release
darwin/arm64 builds of bench/size/native/xo/hello and bench/perf/http/xo:

- the LSDA labels the arm64/x86 emitters made on darwin
  (`GCC_except_table_...`) are assembler-local, as they already were on ELF:
  nothing looks them up (the unwinder takes the LSDA pointer from the FDE's
  CIE, and prof reads __text symbols only). Hello loses 67 of its 856
  symbols and 2,037 bytes of string table, CRUD 559 of 2,971 and 16,905
  bytes. All the fault, defer and landing-pad goldens and the traceback
  tests pass unchanged.
- `LinkOpts.release` (set by `--release`, part of the binary cache key)
  keeps local symbols only in `__TEXT,__text`: the function names a
  traceback prints. Local data and TLS symbols go, in the Mach-O and ELF
  linkers; debug builds keep everything. Hello ends at 780 symbols, CRUD at
  2,397.

Measured on release darwin/arm64 builds of bench/size/native/xo/hello and
bench/perf/http/xo against the same tree before the change: hello
216,777 to 213,337 bytes (-1.6%), CRUD 1,949,960 to 1,923,368 (-1.4%).
`sizebreak` __text grows by 360 bytes in both, the price of the no-symbol
check below; the win is in __LINKEDIT (symbol and string tables). On
linux/arm64 the ELF linker already wrote neither the LSDA labels nor
non-text locals, so only the 360 bytes show there (hello 204,656 to
205,024, its __text 133,968 to 134,256).

`-s` still drops every local symbol. That used to make a traceback name
every frame `_mh_execute_header` (the image header is a defined symbol at
the image base, and the nearest symbol for every pc), and it never stopped
at main; prof now skips that symbol, and the first frame past the runtime's
own that no symbol covers prints
`traceback: (no function symbols: built with -s, or stripped)`
(gotchas/stripped-traceback-header-symbol.md). A compact function-name
table (sorted, prefix-compressed, read by prof instead of LC_SYMTAB) was
designed and parked: about 15.8 KB more on hello and 54.8 KB on CRUD, but
nm, atos, sample and crash reports lose function names, an owner decision.
