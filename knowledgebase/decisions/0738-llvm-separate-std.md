# 0738: Std compiled once for the LLVM backend too

Status: Accepted (owner, 2026-11-21, xw decision #253; proposed by Xo
Lang 1 the same day, task #607; the direction was the owner's, xw
decision #250; lands on main for 0.56.0, not 0.55.x, xw decision #255)

## Context

Every `--backend=llvm` build is a whole program: compiler/xo sepenv.xo
refuses the separately compiled std when the backend is not the host's
native one ("std variant: not used (another target or backend)"), so
std's functions are emitted as LLVM IR and compiled by clang in every
program that reaches them.

Task #593 ranked the 861 LLVM goldens by build time at 63b5d3c46
(linux/arm64 host 2, three at a time, ~/xo-lead/notes/xo-lang-1/p593):
clang is 9,043 s of 9,679 s. No superlinear pass is left after decision
0731. 696 goldens spend 2 to 10 s in clang (3,530 s); 126 spend 30 to
100 s (4,989 s, 55%): every http, tls and net program, and small ones
that reach all of crypto (list_swap_guard, 20 lines, 43.8 s). Such a
program is one module of about 690,000 lines of IR (the median of the
126) holding about 4,000 std functions; `clang -ftime-report` on one
spends its time evenly in code generation (instruction selection 21%,
register allocation 15%, scheduling 11.5%, InstCombine 11%).

Reach, measured (task #607, p607/reach1.out: the kept IR of each of the
126, every function counted by its owner):

| Owner | IR lines | Share |
|---|---:|---:|
| plain std functions (the same in every program that reaches them) | 76,150,868 | 88.6% |
| std generic instances (per program) | 7,157,302 | 8.3% |
| the runtime | 1,637,318 | 1.9% |
| std clones (specialize_args, #once) | 635,884 | 0.7% |
| the program's own functions | 383,263 | 0.4% |

Per golden the plain std share is 71.3% to 92.0%, median 88.8%. If
clang's time follows lines, 4,424 s of the 4,989 s are std's and 566 s
remain. That is an estimate from line counts, not a measurement: stage
0 below measures it.

The native backends have had this since decision 0388: std compiled
once per variant into an indexed store, with exported facts, and a
program compiles its own functions, its generic instances and its
specialize_args clones against it.

## Decision

1. Std is compiled once per variant for the LLVM backend: std built
   alone, every std function a root (0388's XO_STD_ROOTS), emitted as
   LLVM IR, split into modules, compiled by clang with
   `-ffunction-sections -fdata-sections`, and kept in the backend cache
   as std's LLVM objects. The variant key is the native one's (xo's
   identity, std's sources, target, conc, test mode) plus what changes
   clang's output: the optimization level, debug information, and
   clang's identity.
2. A program's LLVM build takes 0388's facts and export records as the
   native build does (they are made from SSA and the checker, before
   any backend), builds SSA for the program's functions, its generic
   instances and its clones only, and emits those. A std function it
   calls is a `declare`; std's functions get hidden external linkage
   and the names the store's objects define, where they are `internal`
   today.
3. The link is clang's, of the program's object with std's objects,
   with dead stripping (`-dead_strip` on darwin, `--gc-sections` on
   ELF), which replaces the whole program's DCE and hollow for std.
4. Small std functions stay inlinable: the bodies 0388 exports for the
   native inliner are emitted into the program's module as
   `available_externally` definitions, LLVM's form for a body that may
   be inlined and is never emitted.
5. Whole program stays for `--release` (decision 0220, as native),
   for `--race` and the sanitizer builds (std would need an
   instrumented variant each), for wasm, and for a target that is not
   the host's until its variant exists. `XO_SEPCOMP=0` turns it off.

Stages, each measured before the next:

0. Oracle, no compiler change: std's IR alone through clang once (the
   store's cold cost and size), and one slow golden's IR with std's
   plain functions made declarations, compiled and linked against that
   object with dead stripping (the warm cost: clang and the link). The
   stage ends the work if the warm build is not under about a quarter
   of today's.
1. std's LLVM objects: the cache kind, `xo dev stdcache --backend=llvm`.
2. The program's side: emit against the facts, the link, behind
   `XO_SEPCOMP_LLVM=1`; the LLVM goldens pass both ways.
3. `available_externally` bodies; `xo bench --ab` of the scorecard's
   LLVM rows against whole program.
4. On by default for dev builds; the tier's llvm golden step measured
   before and after.

## Stage 0: the oracle (2026-11-21)

No compiler change: a golden's kept whole program IR split with
`llvm-extract` into its plain std functions and the rest (the program,
generic instances, clones, the runtime), each compiled with `clang -O2
-c` and the code sinking option of decision 0731; then xo's own clang
line (logged by a wrapper, its `-flto` dropped) linking the whole
program's one object, and the rest's object with the std part's.
linux/arm64 host 2, clang 23.1.3, one run each
(~/xo-lead/notes/xo-lang-1/p607/oracle3.out, the host's load 1.7 at the
end; oracle1.out has a third golden under load).

| Golden | clang, whole | clang, the rest | clang, the std part | link, whole | link, rest + std |
|---|---:|---:|---:|---:|---:|
| list_swap_guard | 22.8 s | 1.0 s | 19.6 s | 23 ms | 21 ms |
| http_tls | 32.6 s | 2.0 s | 28.9 s | 27 ms | 27 ms |

So a warm build compiles 4% to 6% of today's (5% to 7% in the loaded
run, tls_dial 40.1 s to 2.6 s there), and the link costs the same. The
split binaries ran with the whole program's exit status and output
(http_tls's bytes the same; list_swap_guard's stderr differed after its
first line, not compared further). Under the bar of a quarter by a wide
margin.

What the oracle leaves open: the std part here is only what these
programs reach, already pruned by the whole program passes, so the
store's cold cost and the link against all of std are stage 1's first
measurements; the rest was compiled with no std body to inline, so
item 4's bodies add to its time; and today's build runs clang with full
LTO in one step, which this oracle's plain objects do not.

## Stages 1 and 2 built, opt in (2026-11-21, task #607)

`XO_SEPCOMP_LLVM=1` on `xo build` or `xo run` with `--backend=llvm`,
for the host's target, not `--release` or `--race`:

- std's LLVM objects (compiler/xo stdcache.xo std_variant_llvm): the
  directory `KEY.llvm-ID` beside the native variant's `KEY.store` in the
  backend cache, `std.N.o` and `objs.txt`, built once per concurrency
  variant by `xo dev stdobj --backend=llvm` (an `XO_STD_OBJ` build with
  the LLVM backend). The facts are the native variant's `KEY.stdf`:
  they are made before any backend.
- std's own build in the emitter and the driver: no process entry, the
  program entry `xo_entry` declared (the runtime takes its address),
  every Xo function exported by the IR splitter (split.xo
  split_ir_store), every plain std function a root of the dead
  function removal (arm64 reach.xo drop_unreached_store), the modules
  compiled several at once with `-ffunction-sections -fdata-sections`
  and, on ELF, `-fseparate-named-sections`.
- The program's side in the emitter (compiler/llvm): a std function
  with facts is declared where it is called or its address taken and
  not lowered, its C export declared; parameters passed by reference
  are decided from the parameter types (as the native backend does),
  which a function with no body here has too.
- The link: std's objects after the program's, with `-Wl,--gc-sections
  -Wl,-z,start-stop-gc` (`-Wl,-dead_strip` on darwin).

Measured, linux/arm64 host 2, an empty cache, one run each
(~/xo-lead/notes/xo-lang-1/p607/drv4.out): the first build takes 103 s
(the native variants 10.7 and 12.6 s, std's LLVM objects 36.7 and 42.3
s, 52 and 55 MB); every later build about a second or less. clang's
time (phase cc), whole program against the opt in:

| Golden | Whole | Opt in | Output |
|---|---:|---:|---|
| arith | 3,319 ms | 70 ms | same |
| json_text | 4,157 ms | 371 ms | same |
| conc_channels | 3,380 ms | 107 ms | same |
| list_swap_guard | 25,241 ms | 49 ms | 2 traceback lines |
| http_tls | 37,355 ms | 704 ms | same |
| native_sql | 27,881 ms | 530 ms | same |
| tls_dial | 33,552 ms | 244 ms | same |
| otlp_export | 30,560 ms | 448 ms | same |
| net_only | 31,364 ms | 364 ms | same |
| crypto_prims | 25,136 ms | 190 ms | same |

list_swap_guard's fault report names the same fault and location; its
traceback shows `listswap.probe_list_swap_at` as a frame of its own,
not `[inlined]`. No golden's want.txt holds a traceback. On darwin/arm64
TestSepcompLLVM passes (five goldens against want.txt, the first with
the objects' build in 70 s).

What these builds are not yet:

- Small on darwin, where the sizes are not measured yet. On ELF std's
  objects are compiled with split DWARF since 2026-11-21 (in place in
  the cache, under a lock, so each skeleton's `.dwo` path holds): a
  binary keeps std's line tables, a constant 1.85 MB (concurrent) or
  3.05 MB of `.debug` sections, where it carried 4.8 or 7.8 MB before
  and the whole program has 0.75 to 3.8 MB. Files, whole against opt
  in (~/xo-lead/notes/xo-lang-1/p607/drv5.out): arith 1.30 and 4.39
  MB, conc_channels 1.41 and 3.39, list_swap_guard 6.07 and 8.30,
  native_sql 7.39 and 8.65, http_tls 9.43 and 9.60. The `.dwo` files
  are 3.3 and 5.4 MB a variant; a debugger reads std's types from
  them while the cache keeps them, and fault reports need only the
  line tables.
- Inlined across the boundary: stage 3's `available_externally` bodies
  are not built, so no speed claim is made for these binaries.
- The default, or for tests: `xo test` builds whole. (The objects'
  directory is named by clang's version and XO_LLVM_OPT too since
  the commit after: `KEY.llvm-ID`.)

Two things the first measurements had wrong. An XO_STD_ROOTS build
with dead function removal on is not all of std: its roots are main,
the exports and the entries, so it kept 6,368 functions (662,055 lines
of IR) and a program linked against it found no `x.prelude.llrt_println`;
all of std is 4.4 million lines in 7 modules. And std's functions sit
in the named section `xo_text` (the runtime's code range), which
function sections alone do not divide and whose `__start_` and
`__stop_` symbols keep every input section alive: a program that used
no time zone still needed `xo_tz_table` until the two options above.

## What the opt in costs at run time (2026-11-21)

Measured before anything is inlined across the boundary, linux/arm64
host 1 run alone, xo at 57e3364ad, each bench of the native against
LLVM set built with `--backend=llvm` whole program and with
`XO_SEPCOMP_LLVM=1`; 30 interleaved rounds a workload, the median of the
per round ratios (~/xo-lead/notes/xo-lang-1/p607/rt-time-57e3364ad.out):

| Bench | Opt in / whole program (geometric mean) | Workloads |
|---|---:|---:|
| portable | 1.034 | 10 |
| atomic | 1.048 | 8 |
| digest | 1.247 | 5 |
| fault | 1.472 | 3 |
| text | 1.523 | 25 |
| json | 1.582 | 11 |
| ctx | 2.075 | 5 |
| rsa | 4.389 | 4 |
| all | 1.486 | 71 |

The worst rows: rsa sign4096 158.3 ms to 1,001 ms (6.3), rsa verify4096
4.8, ctx budget 2.5, text cut 2.2. clang's time on the same programs
went from 4.1 to 30.8 s to 0.08 to 0.64 s.

This is more than the calls from the program into std: std's own
objects are seven modules with nothing inlined between them, compiled
without LTO and with hidden instead of internal linkage, and none of
the whole program's passes ran across them. So the opt in is for build
time (goldens, edit loops) and is not timed: `xo bench` refuses to run
with `XO_SEPCOMP_LLVM=1` set (compiler/cli/bench sepcomp_llvm_refusal),
and it does not become the default until stage 3 covers the calls
inside std as well as those into it.

## Stage 3, the first half: std's modules optimized together (2026-11-21)

The cost above was mostly inside std. An experiment on the worst rows
(~/xo-lead/notes/xo-lang-1/p607/thin2.out, thin3.out; a shared host,
ratios to the whole program): rsa sign4096 5.39 with the plain objects
and 1.06 with std's seven modules optimized across each other once;
text cut 1.96 and 1.30.

So on ELF std's objects are made by distributed ThinLTO (compiler/xo
main.xo, the XO_STD_OBJ branch): each module to bitcode (`-flto=thin`),
one index step (`ld.lld -r --thinlto-index-only`, `-r` so every symbol
stays), then each module's own code generation with its imports
(`-x ir -fthinlto-index=`), in parallel, with the same section flags
and split DWARF. A relocatable ThinLTO link into one object was tried
first and lost the function sections inside `xo_text`. The clang
driver given the index options exits 0 and writes no index, and a
backend given a missing index exits 0 with an empty object, so xo runs
lld itself and looks at each step's files; without lld, or when a step
leaves nothing, the objects are the plain ones (`mode.txt`, printed by
--phase-times as `(thinlto)` or `(plain)`). darwin links the plain
objects still.

Measured again, linux/arm64 host 1 run alone, xo at 059490a56, the
same 71 workloads (rt-time-059490a56.out): the opt in over the whole
program 1.062 as the geometric mean (1.486 before): portable 1.021,
json 1.051, atomic 1.062, text 1.062, fault 1.064, rsa 1.069 (4.389
before), digest 1.072, ctx 1.151. The worst rows are calls from the
program into std, the second half of this stage: atomic add 1.470, ctx
defer 1.417, ctx budget 1.326, text cut 1.297. clang's time per program
is as before (0.08 to 0.64 s); std's objects take about 44 s a variant.

The price is size: imported functions bring their line tables, so a
binary's `.debug` sections are a constant 2.95 MB (concurrent) or 6.06
MB (1.85 and 3.05 MB with the plain objects), and arith is 7.40 MB
where the whole program is 1.30 MB (drv7.out). Ten of ten goldens gave
the whole program's output but for list_swap_guard's `[inlined]` mark;
one run counted 13 differing lines there, which 16 later executions
did not repeat (tb1.out, tb2.out). Explained the same day: std's
objects differed from build to build (ThinLTO module paths in a
randomly named directory), and one build in three gave a traceback of
`?` frames; the steps now run in that directory by base name, and four
builds agree (det2.out;
gotchas/llvm-std-objects-roots-and-named-section.md, symptom 4).

## Stage 3, the second half: the program imports from std (2026-11-21)

std's objects exist before any program, so a call from the program
into std stayed a call whatever its size. On ELF the program's own
module now goes through the same distributed ThinLTO steps with std's
bitcode as the import source:

- std's build keeps `std.N.bc` beside `std.N.o` in the cache entry
  (only when the objects were made by ThinLTO; `mode.txt` says so).
- The program's module is compiled to bitcode, `ld.lld -r
  --thinlto-index-only` indexes it together with std's bitcode, and
  its code generation runs with that index (compiler/xo link.xo
  `std_imports`), in a directory of its own by base names. std's
  objects are linked as they were built: nothing of std is compiled
  again.
- std's private constants (string literals, constant tables) are
  written `linkonce_odr hidden` under their own names in std's build
  (split.xo `split_emit`). An imported function names the constants it
  reads, and a private one is promoted to `NAME.llvm.HASH` in the
  importer's view only, a symbol std's objects never defined.
- A toolchain that cannot do a step (no lld, no index written) compiles
  the module by itself as before. `--phase-times` prints `cache std
  llvm imports on` or `off`.

Measured on linux/arm64 host 2 (shared, so ratios; minimum of 5 runs;
~/xo-lead/notes/xo-lang-1/p607/p2s4.out and imp1.out), run time over
the whole program's: atomic add 1.47 without imports, 0.99 with; text
cut 1.19, 0.95; text cut_prefix 1.24, 0.98; ctx budget 1.32, 1.19;
rsa sign2048 1.10, 1.08; digest sha256_small 1.18, 1.14; ctx defer
1.35, 1.20. The program's clang step with the imports took 0.4 to 1.1
s.

The 71 workloads on host 1 alone (linux/arm64, 64bb87aec, median of 30
alternating ratios a workload, every checksum equal;
~/xo-lead/notes/xo-lang-1/p607/rt-time-64bb87aecf2d06f4d748be37a1c5f151b642120d.out):
geomean 1.042 of the whole program's time (1.062 before the imports,
1.486 with plain objects), 9 rows above 1.10 (13 before). atomic add
1.470 to 1.005, ctx budget 1.326 to 1.160, portable str_hash 1.176 to
1.057. Still slow: ctx defer 1.317, text cut 1.231 (the shared host's
0.95 above did not hold on the quiet one), digest sha256_small 1.183,
fault isolate 1.153. Sizes: .text 5 to 20% larger than the whole
program's (rsa 3.19 MB to 3.81 MB), the file 3 to 6 MB larger, mostly
std's debug sections. Open: those rows, the size, and darwin, which
links plain objects and imports nothing (Apple's clang has no lld to
write the index).

Debug info of std's frames (2026-11-21): a fault under std's frames
names them with their files and lines as the whole program's build
does (gotchas/llvm-std-objects-roots-and-named-section.md, symptom 6:
dsymutil's prepend path on darwin, skeleton unit headers in the
runtime's reader on ELF). The darwin binary's debug map names std's
objects by their paths in the user's backend cache.

## The rows still slow: what was measured (2026-11-22, task #607)

After the program's imports the opt in runs 1.042 of the whole
program's time over 71 workloads, with a few rows at 1.15 to 1.35 (ctx
defer and budget, digest sha256_small, text cut, fault isolate). Four
explanations were measured on linux/arm64 (host 3, a shared container:
ratios to the whole program, minimum of 7 runs; files under
~/xo-lead/notes/xo-lang-1/p607/), and none closes them:

- ThinLTO's import limit (lim1b.out): std's index and the program's at
  `-import-instr-limit` 400 and 2000 against the default. No row moved
  (ctx defer 1.34, 1.32, 1.32), and std took 10 to 40% longer to build.
- All of std visible to one link (orc4.out): std's bitcode linked with
  the program's module in one ThinLTO link by lld, 100 to 124 s a link.
  ctx defer 1.28 (opt in 1.31), ctx budget 1.40 (1.17), digest
  sha256_small 1.19 (1.31), text cut 1.30 (1.23), fault isolate 1.12
  (1.27): mixed, no better on average. It is not a clean bound for
  internalizing std: those binaries held 15 to 19 MB of code, so lld
  dropped nothing.
- Xo's own whole program passes (ps1.out): the whole build with
  specialize_args, devirt_once, devirtualize or inline off, one at a
  time and three together, ran ctx defer in 26.6 to 27.0 ms, as the
  plain whole build does (the opt in: 35.6 ms). None of them is what
  the opt in lacks.
- The number of std's modules (one1.out, one3.out): two ThinLTO modules
  ran as seven do; one module compiled plainly (a scratch build: std
  then takes 113 to 131 s, and the program imports nothing) gave ctx
  defer 1.34, digest sha256_small 1.15, fault isolate 1.20, text cut
  1.39.

So a design that tells LLVM which of std's functions no program can
reach (internalizing the rest) has no measurement behind it, and is not
proposed. What is known of the rows themselves: ctx defer calls
xo_tlocal_get three times where the whole program calls it once (its
shield functions stay calls: profile pf1-full.out), and digest and text
spend their extra time inside std's allocator and scan functions. The
next step is per row, not a general mechanism: why the program's
module does not import `xo_x_ctx_shield_begin` (an exported entry with
no hidden linkage), and a disassembly of scan.find_bytes in both
builds.

Also found on the way and fixed: std's helper functions were local to
its modules, and two programs of eight failed to link (ada388a03).

## Two causes found by disassembly (2026-11-22, task #607)

The same runtime functions were disassembled from both binaries
(bench/perf ctx, linux/arm64, host 2: p607/dz1.out, dz2.out, ir1.out),
and two were not the same code:

- `xo_tlocal_get`, the read of the thread's table that every context
  and fiber lookup goes through, was 11 instructions for the whole
  program's 5. The whole program's one module is compiled and linked as
  a static executable by one clang call, which picks the local exec
  model itself; std's modules are compiled alone, as position
  independent code, and got the descriptor form, which the static link
  relaxes but leaves its frame and two nops. std's own build declares
  the table `thread_local(localexec)` on linux and freebsd
  (compiler/llvm sched.xo): its objects go into static executables only
  (a library builds whole, with the initial exec model).
- The emitter's own inlines were lost across std's modules. The emitter
  marks the calls it wants inlined at the call (`#6`, alwaysinline:
  compiler/llvm ranges.xo inlinable_set, call.xo), and in one module
  the mark wins even over a `noinline` callee, which the runtime's
  functions are when they can raise (decision 0318 stage 4). Split into
  std's modules the callee was a declaration, and ThinLTO's import
  neither reads a call's own attributes nor brings a `noinline`
  function over: `cabi.budget_tick`, which generated code calls at
  every loop head, was 4 instructions that called `budget.tick` in
  another module, where the whole program has one function of both. A
  module of std's own build now holds an `available_externally` copy of
  each function its marked calls reach in another module, and of what
  those reach in turn (compiler/xo split.xo inline_copies); the module
  emits no code for a copy.

Measured on host 2 (ratios to the whole program, minimum of 7 runs,
the host shared and loaded, so the direction and not the digits;
p607/tl1.out, tl2.out, tl3.out):

| Row | Before | Local exec | And the copies |
|---|---|---|---|
| ctx defer | 1.32 | 1.08 to 1.14 | 1.02 to 1.07 |
| ctx budget | 1.16 | 1.16 | 1.00 |
| fault isolate | 1.26 | 1.27 | 1.19 |
| text cut | 1.34 | 1.36 | 1.28 |
| text zone_fmt | 1.21 | 1.18 | 1.09 |
| digest sha256_small | 1.18 | 1.17 | 1.15 to 1.17 |

The opt in's text shrank with the copies (bench/perf ctx 893,007 to
868,587 bytes). TestSepcompLLVM "runtime calls as the whole program's"
reads the two functions' sizes from std's objects.

On host 1 alone at e43aa14699 (`xo bench --ab` per workload, 4 rounds,
checksums compared on all 71 rows; p607/rt-time-e43aa14699....out
against rt-time-64bb87aec....out) the opt in runs 1.029 of the whole
program's time over the 71 workloads, from 1.042: ctx defer 1.325 to
1.030, ctx budget 1.153 to 0.997, text cut 1.258 to 1.167, text
zone_fmt 1.140 to 1.091, fault isolate 1.170 to 1.145, digest
sha256_small 1.193 to 1.180. By group: atomic 1.003, ctx 0.983, digest
1.055, json 1.038, portable 1.009, rsa 1.073, text 1.024, fault 1.147.
Nine rows remain at 1.10 or more: digest sha256_small 1.180, fault
defer 1.177, text cut 1.167, portable str_hash 1.145, fault isolate
1.145, rsa sign2048 1.135, text replace 1.132, fault owned 1.131, json
maps 1.102.

Tried and dropped (tl3.out, branch l607-exports-tried): copying the
runtime's C ABI exports too (`xo_x_obj_str_eq` and the like, whose
calls carry the mark) and marking each export's one call, so the body
behind it inlines as it does in a whole program. The calls of
`cabi.obj_str_eq` in bench/perf text went from 118 to 0, the text grew
9% (1,414,316 to 1,540,192 bytes), and no row moved beyond the noise.

Also tried and dropped (pf2.out, uw1.out, tl5.out, branch
l607-solecall-tried): the fault rows lose in the unwinder. A whole
program's cabi.fault_raise_cur is one function of 5,632 bytes holding
unwind.raise, personality, frame, dframe and cstep, since each is
internal there with one caller and LLVM inlines such a function
whatever its size; in std's objects each is a function of its own (540
to 1,960 bytes). Marking, in std's own build, the one call of every
function that nothing else in std names grew the text 15% (bench/perf
ctx 864,467 to 1,000,651 bytes) and left the unwinder as it was (63
symbols on the fault path to 55), with no row clearly faster on a
loaded host 2: across all of std those functions have more than one
user. "One caller" is a fact about a program, after its unused code is
pruned, and std's own build cannot know it. Closing these rows needs
the program's link to see std's bitcode with what the program reaches,
which this decision's stages do not have.

A first measurement of that change read 1.00 on every row and was void:
std's objects had failed to compile and the whole program was linked
without a word (gotchas/sepcomp-llvm-silent-whole-build.md); the build
warns now.

Still open: the nine rows above. The counts of calls by callee (sz4.out) name nothing more that is
a call in the opt in and inlined in the whole program: the other names
that differ (cabi.fault_continue, obj_rs_push_byte, the drop helpers)
cost the same number of calls in both.

## Consequences

- A warm LLVM build of a program that reaches http or crypto compiles
  its own IR (median 11% of today's lines for the 126) and links.
  Expected from the line counts: the 126 goldens' clang time from 4,989
  s toward 566 s plus the links, and the store's one build per variant.
- Less optimization across the std boundary: one module with
  `internal` linkage lets clang inline, specialize and drop arguments
  across all of std. The native default build shows what that costs
  when nothing is exported for it (task #511: fault defer 1.725 of LLVM
  against 1.018 whole program). Item 4 keeps inlining; interprocedural
  constant propagation and dead arguments across the boundary are lost.
  Release builds are unaffected.
- The output differs from today's whole program build by design; the
  goldens compare behavior, so they hold, and timing baselines of dev
  LLVM builds are taken again.
- More link work: std's objects and their DWARF are read at every
  link. 0388 measured this for the native linker (most of the cost was
  DWARF) and built an indexed store; clang's linker has no such store,
  so stage 0 measures the link first.
- One more cached artifact per variant, hundreds of megabytes with
  DWARF; the backend cache's prune covers it.
- A sanitizer or race golden gains nothing until its variant exists.

## Alternatives

- ThinLTO with a shared cache (`-flto=thin` with the linker's cache):
  keeps cross module inlining, and the cache reuses a module's code
  generation when its imports are the same. Rejected as the first
  step: a hit depends on each program's import decisions, so reuse
  between different programs is not something xo controls. Worth an
  oracle beside stage 0, since it needs no facts and no new linkage.
- Caching clang's output per split module: the test binary has this
  already (xotool EnableTestClangCache). A whole program's IR is
  pruned and specialized per program, so two programs share no
  byte identical module.
- Running fewer goldens on LLVM, or not the ones that reach crypto
  through internal.crypto_op: the owner chose reuse (xw #250); it also
  leaves users' LLVM builds as slow as they are.
- Linking the native std store into an LLVM build: the two backends'
  calling conventions and unwind tables differ.
