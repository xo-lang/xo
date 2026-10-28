# 0388: Std compiled once to objects and export data; an edit compiles only the program's modules

Status: Accepted (owner, 2026-10-27: "Approved, start"). Proposed by Xo Lang 1 the same day for
milestone 0.34.0, "edit rebuilds at Go's speed", criterion 2; first written as the 0.33.0 edit-rebuild criterion's
structural option, from decision 0382's measurements; the oracles and the DWARF decision below
were added the same day)

## Context

A real body edit of llvm-fib, every cache warm, darwin/arm64, quiet machine (2026-10-27,
XO_CHAIN_KEYS=1 with 0382 stage 3): 274 ms of wall. Its phases run in sequence on the build's
thread, so their sum is the critical path: check 54.4, the SSA build 45 (fe.keys 8, fe.read 7.4,
fe.build 22), the middle passes about 75 (prune_unwind, cln, specialize_args, insert_rc, inline,
dce, inline.verify), the back end about 54 (fn_keys 8.4, emit 8, hollow 18, assemble 7.8, link
11.6), and about 20 outside every phase (process start 3, the key 1, the stores after the link 7,
the exit 9). The HTTP server: 564 ms.

Go's twin of the same program, the same kind of one-function edit, warm build cache: 102 ms
(`go build -debug-actiongraph`, n=5): the edited package compiles in 9.4 ms, the link takes 49.5,
every dependency is a cache check (the action graph spans 80.5 ms, the checks run in parallel),
and about 20 ms is the go command's start. The HTTP server's twin: 246 ms.

The difference is structural. Go compiles the edited package against its dependencies' export
data and links their objects. Xo checks the whole program (std's 195 files are parsed, declared
and resolved on every build: about 38 ms of the check's critical path is std's), builds SSA for
every function, and runs every whole-program pass over all of them (1,300 functions for fib);
decisions 0220, 0283, 0380 and 0382 made each pass's per-function work a cache hit, but every pass
still computes keys, reads entries and decodes over the whole program. The oracle runs of
2026-10-27 (stubs that skip a phase, output ignored) bound what more per-pass caching can buy at
about 35 to 50 ms of the 274 (bench notes: o497, the ceiling table): enough for the 0.33.0
criterion (230 ms, owner 2026-10-26) only with decision 0380 stage 3, and not for 200 or for
anything near Go's 99.

## Decision

1. Std (and each library module a program imports) is compiled once per xo identity, target and
   build flags into an object and export data, kept in the back-end cache. The export data holds
   what a module importing it needs to check and lower its own code without std's sources: the
   checked declarations (types by stable names, signatures, effects, constants), the bodies of the
   functions the inliner may take (Go exports its inlinable bodies the same way), and the facts the
   whole-program passes read of a callee: may_fault and cancel bits, catches, borrowed parameters,
   immortal results, handed-unique facts (0322), result bounds, constant results.
2. An edit rebuild checks, lowers and emits only the program's own modules against that export
   data, then links their objects with std's. Generic std functions instantiated with the
   program's types, and specialize_args clones of std callees, are compiled in the program's
   module (as Go stencils its instances per package).
3. Dead code is removed at the link (per-function sections or subsections, the linker's
   dead_strip) instead of by hollow's whole-text pass over std's code.
4. Debug information stays as decision 0346 has it: every build, edit builds included, embeds
   its DWARF in the executable (crash tracebacks, atos and lldb read it there), std's DWARF
   coming from std's object and joined at the link. No debug map and no separate DWARF for edit
   builds: the oracles below show the join's cost was the linker's, not the DWARF's (see "The
   DWARF join").
5. Landing gates (milestone 0.34.0): a fib edit at most 230 ms and an HTTP server edit at most
   500 ms on darwin/arm64, every cache warm, and no runtime regression on the release-vs-go
   workloads (the risk named under "What it costs"). The fixed point, the goldens and the timing
   suites re-baseline once, in the landing change.

Stages, each measured with `--ab` against its parent before the next: (1) the export data format
and std's object, built once per xo identity, target and flags into the back-end cache; (2) the
program's modules checked against the export data instead of std's sources; (3) SSA and the
whole-program passes over the program's functions only, with std's exported facts; (4) the link
of the program's objects with std's, dead code stripped by symbol.

## What it costs (to decide on)

- Cross-module inlining narrows to the bodies std exports, chosen when std is compiled; a std
  function that today is inlined after the program's specialization may not be.
- hollow's whole-program reach becomes the linker's dead stripping by symbol; data reachable only
  through dead functions, and DWARF for them, need the same treatment.
- Borrow inference, uniqueness (0322), may_fault and unwind pruning stop seeing through std:
  std's facts are computed with std alone and exported. A std function that calls a closure the
  program passes keeps conservative facts; today those are inferred per program.
- Whole-program effect inference and the module check cache's interaction
  (gotchas/module-check-cache-skips-whole-program-passes.md) become per-module contracts.
- Std object builds: a cold build after an xo change compiles std once (about the cold cost of
  today's caches); the cache holds one std object per target and flag set.
- Output identity with today's whole-program build is lost by design: the binaries differ (less
  inlining across the std boundary, different layout). The fixed point, the goldens and the
  timing suites must re-baseline once, and runtime performance must be measured against today's
  (the risk is std calls the program's specialization made cheap).

## Expected effect (an estimate from measured phases, not an oracle)

A fib edit would be the process start (3 ms), the key (1), the check of the program's module
against export data (a few ms: check_module is 4.4 ms of CPU today including std's lazy bodies),
its SSA and passes for two functions (a few ms), its emit and one object's assembly (a few ms),
the link of that object with std's (11.6 ms today, more with dead stripping in place of hollow),
and the exit (9, less with 0382's deferred exit work): about 35 to 45 ms. The HTTP server scales
with its own code, not std's. Measuring this needs a prototype: the first step would be an oracle
that links fib's own part against a std object made from today's parts.

## Oracle measurements (2026-10-27)

Oracle 1, n=8 interleaved, darwin/arm64, quiet machine, at aa91ea517 (a throwaway xo that replays a build's
link inputs through macho_link alone: process start, the link, the exit):

| Row | Median (ms) | How |
|---|---|---|
| xo start and exit (`xo version`) | 9.0 | measured |
| fib's link alone (13 hollowed objects, 2.1M) | 12 (21 with start and exit) | measured |
| the HTTP server's link alone (38 objects, 7.9M), standing in for a full std object | 42.5 (51.5 with start and exit) | measured |
| the program's own check, SSA, passes, emit and assembly for fib's two functions | 5 to 10 | bounded from phase times (check_module 4.4 ms of CPU; assemble 2.2 for 13 objects today) |
| a fib edit today (`xo build`, every cache warm) | 261.5 (mean 261.9) | measured |

A fib edit under this decision: 9 + 42.5 + 5 to 10, about 57 to 62 ms, above the 35 to 45 estimated
above: the link against a full std set costs about 42 ms, not the 12 of today's hollowed objects.
That is still about 4x faster than today's 261 and under Go's 102.

Oracle 2, the same harness and machine, n=8 interleaved, medians with xo's start and exit
(8.0 alone): the HTTP link (38 objects, 7.9M) 51.5 ms; the same without the embedded DWARF
(decision 0346) 21.5; its objects merged into one by `ld -r` (which drops DWARF; the throwaway
linker also dropped the merged object's DWARF-mode unwind entries) 17.0. The system `ld` on the
same inputs, as a reference linker: 36.0, with `-dead_strip` 35.5 (the objects are already
hollowed), merged 32.0.

So about 30 of oracle 1's 42.5 ms is embedding std's DWARF, not linking its code: without it a
full std set links in about 13.5 ms (9 when std is one prelinked object). A fib edit under this
decision would be 8 to 9 + 9 to 13.5 + 5 to 10, about 23 to 33 ms, below the 35 to 45 estimated
above, if the edit build does not relink std's DWARF: std's DWARF kept in std's object and
referenced from a dSYM-style debug map (as ld64's N_OSO entries do), or written once per std
object and copied, are the options to weigh. With today's embedded DWARF the floor stays at 57
to 62 ms.

### The DWARF join

A CPU profile of the HTTP link (40 links in one process, 1000 Hz) put about 13 of its 18 ms of
CPU in the linker's DWARF join (compiler/macho dsym.xo dwarf_data): every DWARF byte copied into
a List[Int], the joined lists copied once per object, and the result written a byte at a time,
with the memory mapping and unmapping that came with them. f59cab86a joins the sections as
Bytes, copied in slices around their patches, with the output the same byte for byte (the
__DWARF sections of llvm-fib and the HTTP server, and their --dsym bundles). Quiet `--ab`, n=10
interleaved, A built at its parent: llvm-fib's edit 245.0 to 242.1 ms mean, its link phase 11.9
to 7.0; the HTTP server's edit 603.5 to 587.8, its link phase 43.7 to 27.1. The HTTP link
without DWARF is about 13.5 ms, so about 14 ms of DWARF remains (writing the larger file is
part of it); the floor of a fib edit under this decision with embedded DWARF is about 9 + 27 +
5 to 10, 41 to 46 ms, and 23 to 33 with no DWARF to join. The rest is the next profile's to
place before stage 4; if it cannot be brought down, std's DWARF prejoined once per std object
(its offsets moved by the program's pieces at the link) is the fallback, still embedded.

## Progress

- Stage 1a-i (d65aa66ad, 2026-10-27): the std tree's modules are declared and resolved before the
  program's, so std's objects, types and effect sets are one block of the checker's tables; no
  output change (identity 200 of 200, `xo check --json` of every errorcheck directory the same).
- Stage 1a-ii (aa7cb9da2 and the opt in XO_STD_EXPORT, 2026-10-27): that block as records with
  no syntax tree, renumbered for another program by one offset per table and by path for modules
  and files, installed in place of the std phases with the trees joined from the parses. A fib
  edit's check (33 ms) was profiled first: std's parses on the prefetch tasks (39 ms of CPU) and
  the main thread's 16.6 ms (declare 4.3, resolve_sigs 4.7). So the records alone buy nothing
  (measured: check 29.9 and 30.7 ms off and on); the gain is stage 3's, when an edit neither parses
  nor lowers std (generic instances in fib's emitted code: 44 of 1,373; the HTTP server's 415 of
  3,391).
- Split (Lead, 2026-10-27): xo-lang-4 builds std's object (1b), its cache kinds and xo dev stdobj,
  and stage 4's dead stripping; Xo Lang 1 stages 1a, 2 and 3 and the facts std exports
  (ssa.std_facts; an emitter section per function from 1b's builder).
- Stage 1b prototype (xo-lang-4, 2026-10-27, unpushed branch l4-stdobj): XO_STD_ROOTS=1 builds
  std alone as one program: every std module loaded, every body checked, and every function with a
  body in std or its implementations (native/NAME) a root, but not the runtime's own modules
  (native/llrt: some exist for one build mode, their C exports declared only in it) and not the
  test world's (std/testing, rttest, llrt_tw_*) outside a test build; the dump's and std's roots
  are declared before the C ABI exports. 1,991 roots, 6,594 functions, __text 5.05 MB (21 MB of
  part objects with DWARF), about 4 s cold and 1.3 GB, verifying clean with DCE and hollow off.
  The builder's own flags of 2,380 builds in this Mac's cache came in 5 combinations, 71%
  conc=false and 27% conc=true: the object is built per combination, lazily (conc=true adds
  shield calls to deferred code and keeps cancellation checks).

### The shape of std's object: an indexed atom store (Lead, 2026-10-27)

The link ceiling, Xo Lang 1's oracle harness (macho_link of a build's dumped inputs alone, n=8
interleaved, medians, xo's 8.5 ms start subtracted):

| Link inputs | With DWARF (ms) | Without (ms) |
|---|---|---|
| fib today: 13 hollowed objects, 2.1 MB | 7.5 | 3.5 |
| std's unhollowed parts, every function: 43 objects, 21 MB | 71 | 34.5 |

So a link that reads std's whole object is over the edit budget before any strip: even without
DWARF, reading and placing 21 MB is 34.5 ms, and dead stripping only shrinks what is written.
Std's object is therefore an indexed store: per function, its code, relocations, unwind entry and
DWARF ranges; a symbol index; and each global symbol's reachability closure, computed once when
the store is built. A link unions the closures of the std symbols the program references and
copies those atoms only, as Go's linker has loaded only reachable symbols from its indexed object
format since Go 1.15.

The emulation (a scratch reader of the same 43 objects: atoms at symbols, every relocation an
edge, so calls, function values and tables all root atoms; fib's 13 objects' symbols as the
roots, which include its generic instances' calls into std):

| | |
|---|---|
| std's atoms, global symbols | 13,133, 8,401 |
| store build: read and graph, closures (once per variant) | 53 ms, 56 ms (21.6 MB of bitsets) |
| fib's roots into std, the union of their closures | 823, 0.2 ms |
| live atoms | 926, 0.48 MB (0.41 MB of text) |
| fib's hollowed objects today | 0.35 MB of text |

The closure is a superset of hollow's reach by about 17% of text. DWARF: std's abbreviations and
strings, shared and not per function, are 0.21 MB in all (__debug_abbrev 0.01, __debug_str 0.08,
__debug_line_str 0.12) and are copied once per link; __debug_info (2.49 MB), __debug_line (2.47)
and __debug_loclists (4.93) split by function with the code, except their shared type DIEs, to
be measured when the store's DWARF is split. A fib link from the store is then about today's
7.5 ms plus the union and the shared DWARF: about 8 to 10 ms. Walking the edge list at the link
(122,507 edges, 0.49 MB as 4 byte indexes) from fib's roots takes 17 us and gives the same live
set as the closures, so the store keeps the edges and no bitsets (Lead's check).

### Groundwork landed (xo-lang-4, 2026-10-27)

- compiler/macho's assembler has an atoms mode (`.subsections_via_symbols`): a branch to another
  atom of its section is relocated, not patched, and the object says MH_SUBSECTIONS_VIA_SYMBOLS.
- The linker splits such an object at its symbols and strips by atom (strip.xo): roots are the
  entry and every section of a whole object; every relocation is an edge; a function's compact
  unwind entry, FDE and unsafe range record stay with it and root what they name (the LSDA, the
  personality, the table). __eh_frame stays whole and is never walked (its FDEs' SUBTRACTOR
  addends are offsets from the section's start, which ld64 handles by writing the section
  again): a dead function's FDE points to 0, and __unwind_info names only the live functions'.
  DWARF of a stripped atom resolves to 0 for now (the per atom DWARF is next).
  section$start$SEG$SECT and section$end$SEG$SECT are defined as ld64 defines them.
- Async preemption's unsafe range tables were one per part, offsets from the part's first text
  label resolved by the assembler (40 tables, 29,162 entries in std): stripping or reordering
  atoms would have pointed them at the wrong code with no error. A store build writes one table
  per function instead, in an atom of its own, its ranges offsets from the function's label, and
  a record (function, table) in __DATA,xounsafe, which the linker gathers for live functions; the
  runtime (llrt pool.xo async_pc_ok) scans that index when bit 40 of xo_unsafe_parts' count is
  set, after the per part tables. Normal builds set no bit and write the same tables as before.
- XO_STD_ROOTS=1 (with XO_DCE=off XO_PASSES=-hollow) builds std whole that way. The std-all
  program: __text 5.05 MB unstripped, 0.31 MB stripped (hollow's size), and it runs;
  async_preempt_alloc and async_preempt_regs built so match their want.txt in 5 of 5 runs each
  (402 live functions in the index, 6,432 bytes, scanned linearly per SIGURG).

### The store link (xo-lang-4, 2026-10-28)

- compiler/macho store.xo: `store_encode` packs objects made with `.subsections_via_symbols` into
  one file. It holds every atom (strip.xo atomize) with its bytes and relocations in a data
  region, the symbols, each split section's atoms, and each atom's edges as store atom numbers:
  what its relocations reach (a reference by section expanded to every atom of it), plus what its
  FDE, compact unwind entry and unsafe range record name (strip.xo unwind_refs). `link_store`
  walks the edges from what the program's objects (kept whole) leave undefined, the entry, and
  the sections nothing may strip. It reads only the live atoms' bytes and relocations; a dead
  atom stays an empty section, so every ordinal the relocations and DWARF name holds. It then
  runs link_debug's tail (link_rest). The result is the binary link_debug writes for the store's
  objects and the program's, byte for byte.
- DWARF by function (dwtrim.xo), in the strip and the store alike. The arm64 emitter writes each
  function's subprogram last among its unit's entries, its line sequence on its own, and each
  variable's location list in __debug_loclists starting with the function's address. So a dead
  function's subprogram, sequence and lists are cut. Only the unit length is patched, plus the
  list offsets table, whose entries move with their lists; nothing else refers into those runs.
  The store keeps each run's bytes and relocations apart, so a link copies only live runs.
- XO_STD_STORE=PATH on an XO_STD_ROOTS=1 build (darwin): the first build writes the store of its
  parts (prog-darwin.N.o); later builds link through it without reading the parts.
  XO_STD_STORE_CHECK=1 also links the parts and fails on any difference; XO_STD_STORE_REPEAT=N
  times N more links.
- llvm-fib through the store, 2026-10-28 (the program's own functions are in the store too,
  the stand-in until stage 3 compiles the program apart; the store is 29.2 MB, the parts' link
  is the same binary):

  | store link                                | min ms | median ms | binary   |
  |-------------------------------------------|-------:|----------:|---------:|
  | DWARF whole                               | 50.7   | (1 run)   | 11.42 MB |
  | dead subprograms and line sequences cut   | 26.7   | 27.1      | 7.26 MB  |
  | and their location lists                  | 22.6   | 24.9      | 2.97 MB  |

  n=21 per row (20 repeats plus the first) on a loaded Mac (load average 15 to 26). The last
  row's CPU profile, 101 links: 21 ms of CPU each. Of that, link_rest is 9.7 ms (the output's
  copies and growth about 3, the signature's and UUID's SHA-256 2.3, DWARF 1.5, relocation 1.4),
  store_cut 2.9, the run tables' decoding 2, resolve 1.8. __DWARF left: 1.88 MB (info 635 KB,
  mostly each part's own types; line 262 KB; line_str 115 KB). The target is 8 to 12 ms.
- Second round (2026-10-28): the store's header is read in place (it was sliced, a copy, each
  link), a cut section's run table is read only when its section is used, and an object whose
  xo unit describes only dead functions (and no variable: xo_static stays) loses all its DWARF,
  in the strip and the store alike (dw_droppable). The executable is assembled in one buffer:
  the body is written after the header's place, the UUID hashes the header's zeros and the body
  with a streaming SHA-256 (the same bytes as before), and code_signature takes the header and
  the body apart, so the file is one copy, not three fresh 3 MB buffers (link.xo write
  4.6 ms of the 21 at line 1746). llvm-fib: 17.2 ms of CPU a link (1.74 s over 101; write 7.1 to
  3.3 ms), wall min 19.5 ms and median 20.9 at a load average of 25 to 59, binary 2.71 MB
  (__DWARF 1.62 MB); the store check passes.
- Third round (2026-10-28): the store's sections are fixed 52 byte records (STORE_REC: a name
  index into one table of the section names, the alignment, flags, data and relocation places,
  the cut runs' place, the address and size) read in place, each atom's edges read in place
  (no list per atom), the lists built at their final size, and relocate's patches list sized
  for every relocation of the placed sections (it was copied as it grew: 2.5 ms). llvm-fib: min
  17.2 ms, median 17.9 (n=21, load average about 50; the second round's 19.5 and 20.9 at a
  similar load); the store check passes.
- Fourth round (2026-10-28): a section's patches are put in address order by merging their
  runs (out.xo patches_in_order: a section's relocations, which compiler/macho's assembler writes
  in descending order, then a unit's or line table's header fields, one per file name), not by
  sorting the list (2.5 ms of a merge sort's copies per link); the store keeps each cut run's
  relocations in address order. link_store is three steps a caller can time (store_prepare,
  StoreLink.resolve, finish), and XO_STD_STORE_REPEAT prints them. llvm-fib: prepare 5.8 ms,
  resolve 3.1, finish 8.7 (4.9 with no DWARF: the DWARF is about 4 ms; the binary 2.72 MB, 1.07
  without). Prepare and resolve read every symbol and section of std's 37 parts; the next round
  compacts each store object to its live atoms and the symbols they name.
- Fifth round (2026-10-28): each store object is compacted as it is read. Its kept sections (live
  atoms, whole sections, cut DWARF) come in order, then one empty dead section stands for every
  dead atom (a DWARF or FDE reference into one is 0). Its kept symbols, renumbered in order, are:
  those in kept sections; those a kept relocation names; a dead atom's external definition
  when a kept relocation anywhere names it (a DWARF address of a dead constant is 0, not an
  undefined symbol); and an undefined one only when named or defined nowhere in the store (an
  import, as the parts' link binds it). Symbols are fixed 28 byte records with names in the
  data region, and the global definitions a table sorted by name for a binary search (no map
  built per link). llvm-fib: min 13.8 ms, median 14.6 (n=21; round four 16.0 and 16.3); the
  store check passes. The check caught two wrong first rules: an undefined symbol kept with
  its definition dropped, and a dead definition another part's DWARF names dropped.
- Sixth round, DWARF types (2026-10-28): a unit's types (base, structure, pointer, array,
  enumeration, union, typedef) and abstract subprograms are kept only when a kept entry refers
  to them (DW_FORM_ref4, closed over: types refer to types; dwtrim.xo dw_keep_with), and every
  kept reference is moved by its target's shift as the runs are written (dw_write_runs), in the
  strip and the store alike. The store's run records carry each run's references (STORE_RUN 28
  bytes; the file is XOSTORE2), read only for kept runs. llvm-fib: __debug_info 635 to 248 KB,
  __DWARF 1.64 to 1.36 MB, binary 2.72 to 2.44 MB; dwarfdump --verify clean; TestStdStoreUnwind
  (lldb's backtrace) passes; the store check passes. Interleaved with main's xo on one loaded
  Mac (n=40 each): median 15.8 ms against 16.5 (min 15.6 and 15.3). Reading every run's
  references first cost more than the cut saved (17.0 against 16.6).

### The store's format-neutral core (xo-lang-4, 2026-10-28)

- compiler/macho store_core.xo holds what does not depend on the object format, so Lang 2's COFF
  adapter (pelink) shares it. The writer, store_write, takes CoreObj values: atoms with
  their bytes, opaque fixed-size relocation records (12 bytes: field 0 the address in the
  atom, field 1 a symbol index or section ordinal), DWARF runs and edges as store atom numbers;
  symbols; the split map; the droppable unit's functions; and the global definitions. The
  reader is store_view (the header, read in place), def (binary search), walk (from roots and
  the sections the adapter says nothing may strip), dw_dropped, cut (a cut DWARF section's bytes
  and its kept relocation records' places with their new addresses) and keep_syms (the
  compaction's symbol rule, the adapter giving each symbol type's kind: in a section,
  undefined, a debugger entry, other). store.xo's store_encode and store_prepare are the
  Mach-O adapter; the file and the link are unchanged (the store check passes).

### std's object (xo-lang-4, 2026-10-28)

- XO_STD_OBJ=DIR (with XO_DCE=off XO_PASSES=-hollow; it implies XO_STD_ROOTS=1), built from an
  empty program (`fn main(os: Os) {}`), writes std's object to DIR instead of linking: the part
  objects (prog-darwin.N.o) and their store (std.store). Only std's functions are emitted
  (ssa.std_func), plus the generic instances and the generated functions with no file (a std
  global's value, a zero function); the program module's are left to the program's object. The
  last part keeps the runtime's glue, the static data, std's exports (`_main` among them,
  cabi.c.main's C wrapper) and the helpers; `_xo_entry` (g.entry: main's signature, conc,
  budgets) is the program's, which defines it too under separate compilation. The object
  leaves `_xo_f_main` and `_xo_entry` undefined, plus what the link's own inputs define
  (`_xo_buildinfo_line`, `_xo_tz_table`, `_xo_tsan_runtime`) and the section bounds.
- Helpers (`xo_h` + base62(FNV-1a(key)), the key a kind and a type or function name) and generic
  instances are weak definitions (N_WEAK_DEF: `.weak_definition` on Mach-O, `.weak` on ELF) when
  Module.weak_shared is set: std's object, and the program's under separate compilation, may
  each define one, and the link keeps the first (add_globals). A std function defined twice
  stays an error. weak=1 is in the function and part cache keys. Normal builds set neither flag
  and write the same objects as before.
- macho.defined_symbols(path, bytes) lists an object's external definitions, for the driver's
  undefined-symbol check under separate compilation.
- The conc0 std object, 2026-10-28: 41 parts, 8,275 global definitions, 1,825 of them weak
  (1,211 helpers, 614 instances), no name defined twice.

### The program's side (xo-lang-4, 2026-10-28)

- ssa Module.sepcomp_program (the driver sets it under XO_SEPCOMP, Lang 1's stage 3 (c)): the
  program's entry file keeps `_xo_entry` (g.entry), the program's own exports, its helpers
  (weak), literals, LSDAs and DWARF, and leaves out what std's object defines: the system call
  trampolines, the task switch code (shared_code), the unsafe range list, the end data
  (`_xo_empty_list`, `_xo_zero_handle`, `_xo_sys_words`, `_xo_static`) and xo_static's DWARF
  variable. It implies std_store (`.subsections_via_symbols`, per-function unsafe tables
  gathered into the one index std's `_xo_unsafe_parts` names) and weak_shared. The part cache
  key gains " store=1" (a part's per-function unsafe tables), so a store mode part is never
  taken for a normal one. fib so built (whole program, the flag forced): the entry file defines
  `_xo_entry` and none of std's runtime symbols, 544 index records, 66 weak definitions.

### std's object by default: the cache (xo-lang-4, 2026-10-28)

- compiler/xo stdcache.xo std_variant(os, ctx, tgoos, tgoarch, conc, phases) gives the paths of
  std's store and facts for a target and concurrency variant, from the back-end cache:
  std/<key>.store and std/<key>.stdf. The key hashes this xo's code generation identity
  (without a separately compiled build's facts), the target, the variant, STD_CACHE_VERSION
  and every std source's path, size and modification time, so a body edit in a development std
  makes a new one. A miss builds it with `xo dev stdobj --target --conc --facts` into a
  directory of its own under the cache and renames each file into place. Concurrent builds
  each build it and the last rename wins: the same bytes. The prune treats the two files as
  any entry, and a pair with one file gone is built again. None (the cache off, xo's identity
  or std unreadable, the build failed) leaves the build whole-program.
  `--phase-times` says `cache std KEY conc=C reused|built|failed`.
- macho.store_defs(store) lists the store's global definitions (its sorted table, no atoms
  read), so the undefined-symbol check needs no part objects.
- Lang 1's step (d) calls std_variant once the build knows its conc. The 0.34 release ships
  the host target's conc0 and conc1 stores beside std (Lead, 2026-10-28) so a first build
  after install skips the build; cross targets and other variants build lazily.
- The release (scripts/release.sh) builds them for darwin/arm64, both variants, with the
  release's xo from the staged std (`xo dev stdcache`) into lib/xo/stdstore/<os>-<arch>-
  conc<N>.store and .stdf, with a .id file holding the code generation key. std_variant takes
  those (std_released) when std is a release's (<prefix>/lib/xo/std) and the .id names this xo,
  before the cache: a release's std never changes, so no file times are needed. Other targets
  follow their store adapters (ELF, COFF).

- A link through the store (XO_SEPCOMP_STORE) keeps the program's objects whole, so a weak
  helper or instance the program and std both define stays in the program's text after the
  link took std's copy. Its compact unwind entry names the program's copy by address, while
  its FDE names the symbol, std's copy, so unwind_info found no FDE for it (`no FDE for the
  DWARF mode function`, Lang 1's TestSepcompLinks, 2026-10-28). StoreLink.resolve drops a
  program object's compact unwind entries for weak definitions the link took from another
  object (uncoalesced_unwind); nothing reaches those copies.

### std's variants: conc (xo-lang-4, 2026-10-28)

- Every XO_STD_ROOTS build was conc=true: rooting every std function builds bodies that need
  concurrency (proc_run, the waiting natives, tasks), which set need_conc, and build_program_fe
  builds again with unwinding checks. So the first conc0 files were conc1 (the fe-hint of a
  fib roots build says conc=true; a normal fib build's says conc=false).
- XO_STD_CONC=0|1 fixes the builder's conc (FeCache.conc_fixed) with no second build. With 0, a
  body that needs concurrency faults where it would (Builder.conc_off, a fault then dead code,
  where a build that will be made again keeps undef), so every root verifies; a program without
  concurrency never calls one.
- conc changes the facts: true conc0 (6,041 functions, 2,402 with a borrowed parameter) against
  conc1 (6,198, 2,451): 441 errors, most `is not canceled, but the whole program's may be`. The
  conc0 file against a whole-program fib dump (conc0): 0 errors, 9 functions' body facts
  differ (pool and sched). conc0's std object: 37 parts; conc1's 41.

- Stage 2 (2026-10-27, parked behind stage 3): an oracle that parses std only to its imports and
  installs the records cut a fib edit's check from 31.0 to 16.0 ms (median, n=8); the rest is
  reading every std file (3.2 ms of CPU), relocation (2.6), decoding (1.8), the import parse (1.75),
  walks (1.45): a floor of about 8 to 10 ms with the module closure from the record and a
  stat based fingerprint (Go's rule: size and mtime trusted only when the mtime is more than 2 s
  old when recorded). It needs the program check's reads of std trees served on demand (defaults,
  constants' values); its wall gain arrives with stage 3, which stops parsing std for the passes.
- Stage 3 oracles (2026-10-28): (a) runtime, one xo with std's callees opaque to program code
  (no inlining of std into it, no specialize_args clones, may fault and be canceled, no result
  bounds; std's parameters owned): llvm-fib, sort, strings, collatz within noise to +2%, interp
  15% faster (logged as an inliner item), the HTTP server 145,658 and 145,771 requests a second,
  CPU 26.28 and 26.82 us a request: std's facts are needed for correctness, not speed. Making the
  runtime's (native/llrt) parameters owned made the HTTP server fault on its first request, which
  no verifier saw: borrowed parameters (and var parameters, by-reference passing) are the calling
  convention and must be exact. (b) compile time, std's bodies not built: a warm llvm-fib edit
  238 to 95 ms, the HTTP server's 542 to 168 (ssa 108 to 25, insert_rc 55 to 10, inline 57 to 12,
  hollow 34 to 4), before the link with std's object and the facts' decoding.
- Stage 3 steps landed: std externs, body-less shells std's facts answer (a47b8b476); the facts'
  record and codec (e4338ea59), XO_STD_FACTS_DUMP (5c07f468b), xo dev stdfacts (0e5b9ab0f),
  XO_STD_FACTS_VERIFY with TestStdFactsVerify (479e8bab1, 02e7139c9); XO_SEPCOMP=1 with
  XO_SEPCOMP_FACTS, std's functions built as externs (825392145). Std's object (xo-lang-4):
  XO_STD_OBJ (std functions only, xo_h helpers and generic instances weak, the program's
  `_xo_entry`), conc=0 and conc=1 variants (XO_STD_CONC; conc changes 441 facts, so each program
  takes its variant's), macho.defined_symbols, the indexed store (fib links in 14.0 ms).
- Findings on the way: an undecoded cache hit's shell had no var parameters (6bf1b9158: it keeps
  them; an audit found no wrong code in 0.33.0, the empty list could only have faulted the build);
  a warm build folds target constants as a cold one does (the binaries are byte for byte the same;
  a Windows LLVM split build links cold and warm, ThinLTO and none).
### COFF: atoms and pelink's strip (Xo Lang 2, 2026-10-28)

- Step 1 (c8872ec8b): `.subsections_via_symbols` in the COFF assembler (no
  header flag: an empty `.xoatoms` LNK_REMOVE section marks the object). A
  branch or rip relative reference resolves only within its own atom; a
  temporary label in another atom is relocated to that atom's symbol with
  its offset, in code and in data words; `.Lseh_end` belongs to its
  function's atom. Atoms are spans at symbols in one `.text`, as Mach-O's,
  not a COMDAT section per function (about 6,000 section headers an object
  saved, and the store core's adapter has Mach-O's shape).
- Step 2: pelink splits an atoms object (compiler/macho pestrip.xo
  coff_split): sections at their symbols, `.pdata` per 12 byte record,
  `.xdata` where a record's unwind information starts, a section symbol's
  reference moved to the piece its in-place offset names (a `.pdata` end
  field by the byte before it). coff_live walks relocations from the
  entry and from every section of a whole object, `.tls`, `.CRT`,
  `.drectve`; a `.pdata` record and a `.xounsafe$m` index record follow the
  function their first field names. A dead piece is unplaced; DWARF's
  references to it are 0, any other is an error.
- The unsafe range index on COFF: a windows store build (std_store) writes
  apreempt's per-function tables, the index records in `.xounsafe$m`, its
  bounds labels in `.xounsafe$a` and `$z`, which pelink places after the
  other data sorted by name (one run, no padding: 16 byte records, 8
  aligned). unsafe_fn_tables skips numeric local labels (`8:`, the x86
  emitter's), which it took for functions (darwin/amd64's store build
  failed on `8$ut`).
- A std-roots llvm-fib for windows/amd64 (XO_STD_ROOTS=1 XO_DCE=off
  XO_PASSES=-hollow): 8,553 text atoms, 579 live, .text 346 KB (the normal
  hollow-free windows build 505 KB); it runs under Wine (Box64) with the
  normal build's output. Two leaks found on the way: the per-part unsafe
  tables rooting every part's first function (gone with the index), and
  each `.pdata` end field resolving into the next function (2.1 MB of
  text kept before). Async preemption and the debugger on the stripped
  build wait for the batched Windows CI run (Wine does not model thread
  suspension).

### COFF: weak definitions and the store (Xo Lang 2, 2026-10-28)

- Weak definitions (10766a60e): COFF has no defined weak symbol, so the
  assembler lists an object's `.weak_definition` names in an `.xoweak`
  LNK_REMOVE section, and pelink and the strip keep the first definition,
  a strong one over a weak one (as add_globals).
- The store's COFF adapter (compiler/macho coffstore.xo) on xo-lang-4's
  core (store_core.xo, 2cd461a10): coff_store_encode splits each part
  (coff_split), drops the auxiliary symbol records, and gives each piece
  as a core section whose edges are its relocations' targets (an undefined
  name through the store's definitions, strong over weak) and, for a
  function, its `.pdata` and `.xounsafe$m` records; the symbol record's
  type is the storage class with bits for undefined and absolute.
  coff_store_objs walks from the program's undefined names and the entry
  (never stripped: DWARF, `.tls`, `.CRT`, `.drectve`, removed sections,
  the unsafe index's `$a` and `$z`), compacts each store object to its
  live pieces and one removed dead stub, and renumbers its symbols
  (keep_syms); pelink takes the result whole. XO_STD_STORE=PATH and
  XO_STD_STORE_CHECK=1 work for windows/amd64 as for darwin (the check:
  the store's image is the strip's of the parts, byte for byte), and
  XO_STD_OBJ writes windows/amd64's std object and store.
- A std-roots windows/amd64 llvm-fib (the program's own functions in the
  store too, as darwin's stand-in): the store 29.9 MB; the store check
  passes; through the store, warm: the store objects 6.8 to 7.3 ms, pe.read
  21 to 23, pe.link_pe 9; it runs under Wine. XO_STD_OBJ (an empty
  program, conc0): 39 parts and a 28.5 MB store. pe.pdb_parts is 360 ms:
  the compacted objects have no part keys, so every link parses each
  store object's whole DWARF; the PDB's parts per atom, made when the
  store is written and cut with it, are next.
- The PDB through the store: each part's PDB part (cvobj.xo obj_pdb of
  its split object: functions' rows, inline sites, finished locals, its
  variables' types by function) is written beside the store
  (std.store.pdb, coffstore.xo coff_store_pdb) when it is made; a link
  through the store reads them, maps each split section to where pelink
  placed its compacted one (StoreMap, obj_map_store), and leaves out the
  stripped functions and their variables' types (the image's DWARF has
  only the live functions': debug.read skips a subprogram at 0).
  XO_STD_STORE_CHECK=1 also requires the PDB from the store's parts to
  equal the one made from the image's DWARF. A std-roots llvm-fib: the
  store link's pe.pdb_parts 360 ms to 17 to 23 (the parts read and decoded
  in the store objects step, 7 to 28 ms: decoding only the live functions'
  records is next), the link 416 to 85 to 96 ms; the check passes.

## Alternatives

- More per-pass caching (0382 stage 3 and its successors, 0380 stage 3): bounded by the oracle
  runs at about 35 to 50 ms more of fib's 274; reaches the 0.33.0 criterion with 0380 stage 3,
  not Go's range.
- The compile server (0283): keeps the whole program's state in memory between builds, so an
  edit re-runs only what changed without reading caches; complementary, costs resident memory and
  a daemon, and is not a one-shot `xo build`.
- A debug map for edit builds (the objects keep their DWARF, the executable names them, as
  ld64's N_OSO entries do): not needed once the join was fixed, and it would reverse part of
  0346 (a traceback or lldb session would depend on objects in a cache a prune can remove).
- Keep the whole-program build for `--release` and use separate compilation only for edit
  rebuilds (Go has one mode; two modes double what is tested): a variant of this decision to weigh.

### ELF (xo-lang-4, 2026-10-28)

- Atoms mode on ELF (compiler/macho elf.xo elf_split): `.subsections_via_symbols` gives each
  symbol defined in an allocated section that already holds something a section of its own, of
  the same name and flags (as -ffunction-sections and -fdata-sections), and an empty
  `.note.xo.atoms` marks the object. A `.section NAME` returns to the newest of the name.
- linux store builds (std_store on linux): the per-function unsafe range records go in a
  section named `xounsafe` (an identifier), whose bounds `__start_xounsafe`/`__stop_xounsafe`
  elflink now defines as GNU ld does (the sections of that name placed together at the end of
  their output section; ELinker.bounds).
- elflink strips by section (elfstrip.xo gc_sections, ld's --gc-sections): roots are a whole
  object's allocated sections, notes, the init and fini arrays, the entry's section and what
  the CIEs name (the personality routine's data word, DW.ref.*: missing it, a fault's unwinding
  called address 0); edges are the RELA relocations, a function's FDE references (its LSDA)
  and its unsafe range record. The dead functions' FDEs are dropped from .eh_frame (each CIE
  pointer recomputed), so .eh_frame_hdr names none. The imports and the static link's
  undefined check come after the strip and count only what kept sections name (a std root
  calling qsort no longer fails a no-libc link). The DWARF is cut as on Mach-O (dwtrim.xo
  dw_runs, the run finder now format-neutral).
- The ELF store (elfstore.xo store_encode_elf, link_store_elf over store_core.xo): atoms are
  sections; 20 byte relocation records (with the addend; the store header now holds the record
  size: XOSTORE3); .eh_frame is a whole root whose edges are what its CIEs name. The link reads
  the live sections, cuts the DWARF and drops the dead FDEs as elflink's strip, and is the
  parts' link byte for byte (XO_STD_STORE_CHECK on linux/arm64).
- Proof (Docker, linux/arm64): async_preempt_alloc and _regs 5 of 5 in store and normal builds,
  the caught fault and the traceback the same as a normal build's.
- A link through the ELF store applies its dead marks after the symbols resolve
  (ESect.store_dead, applied in gc_sections), and counts a stored object's undefined
  symbols as only its kept sections' after the strip, so the outline atomics elflink
  synthesizes before it come out in the parts' link's order. XO_STD_OBJ, XO_SEPCOMP_STORE,
  `xo dev stdobj --target=linux/arm64` and `xo dev stdcache --target=linux/arm64` work on
  linux; release.sh ships the linux/arm64 stores beside darwin's.

- The run cache's recorder (compiler/xo recorder.xo rec_skip) leaves a release's
  lib/xo/stdstore/ out of a build's recorded reads, as it does the caches: a separately
  compiled build reads the whole store (about 52 MB), and hashing it for the run cache's
  entry costs about as much as the link. The run key holds the xo's identity, which fixes
  the store it shipped.
- The cache's windows store keeps its PDB parts beside it (compiler/xo stdcache.xo:
  obj/std.store.pdb renamed to <key>.store.pdb before the store, kept used with it), which
  pe_link reads next to XO_SEPCOMP_STORE; without them the link reads std's DWARF instead.
