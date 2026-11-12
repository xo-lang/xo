# 0583: A REPL in the browser playground

Status: Accepted (owner, 2026-11-11; implemented on main). Was: Proposed (owner request via the lead, 2026-11-09; measurements first, no page work until the owner reads them)

## Context
- The owner asked (2026-11-09, via the lead) for a REPL on /play/, after
  the Mac REPL (decision 0572 dropped its XO_REPL gate). Target: under
  0.5 s per entry in the tab.
- The playground's compiler (decision 0242) is compiler/wasm built for
  wasm, run in the page's worker; compiler/wasm/play.xo runs one command
  per instance (`--play check|fmt|build`) and prints one JSON line.
  play_build checks the program and all of std it reaches (native mode,
  lazy bodies), builds SSA, runs passes, and generates one wasm module.
- Live page (~0.34), lead's Chrome measurement 2026-11-09: compiler load
  2.9 s, ~1.95 s per build+run warm on the tour's basics, collections and
  concurrency; the programs run in 1 ms. So the whole entry cost is the
  compile, and most of that is std (checking and building what the entry
  reaches), the same split the Mac REPL had before separate compilation.
- The Mac REPL reaches 72-300 ms per entry with std's prebuilt objects
  (separate compilation). The wasm backend has no separate compilation:
  it emits one module.

## Measurements to take first (step 1)
1. Today's compiler/wasm in a tab (Chrome, worker as the page runs it),
   per entry, on a 30-entry session (the Mac REPL's step 7 evidence
   session) and the three tour examples: check ms, SSA
   build ms, passes ms, wasm gen ms, instantiate+run ms. Node's WASI under
   compiler/wasm/run.mjs as a proxy first, then the page's worker.
2. The same entries with std's work kept in the tab:
   a. one long-lived compiler instance (a `--play repl` loop reading
      entries from stdin, as WASI preview1 allows), keeping the checker's
      std modules between entries (check only the entry's module);
   b. plus std's SSA functions kept after passes, so an entry builds and
      optimizes only its own functions and copies std's reached ones;
   c. plus std's generated wasm function bodies kept (code section bytes
      per function), so gen emits only the entry's functions and links.
   Each cache level reported separately, so the ADR picks the cheapest
   that meets 0.5 s.
3. test-playground.mjs against https://xolang.dev/ fails before the
   page's script runs (lead): fix the test first, since the measurements
   in a real tab go through it.

## Step 1 results (2026-11-09, Node proxy)
bench/results/browser-repl-2026-11-09.md: under Node's WASI the
playground compiler builds each tour example in 2.0-2.2 s (load 0.34 s);
the check is about 1.5 s of it whatever the program's size (std), SSA,
passes and gen 150-320 ms. Level 2a (keep the checker's std modules in
one instance) is the lever: about 0.25-0.45 s per entry if an entry's own
check stays under about 100 ms. Next: the `--play repl` loop with 2a, then
the same in Chrome's worker.

## Decision (proposed, pending the numbers)
- `--play repl` in compiler/wasm/play.xo: a loop over JSON entries on
  stdin, one JSON reply per entry, reusing compiler/cli/repl's Sess
  (session.xo has no Os dependency: classify, program, after, pins,
  :type/:doc/:defs/:undo/:reset), so the browser and the terminal REPL
  give the same answers.
- The std cache at the level the measurements pick (2a, 2b or 2c), held
  in the long-lived instance's memory; nothing persisted across page
  loads in step 1 (IndexedDB of std's cached output is a later step).
- A REPL panel on /play/ (website change, through the owner).

## Precedents
- Pyodide / JupyterLite: CPython compiled to wasm, one long-lived
  interpreter per tab, state kept between cells.
- ruby.wasm irb (TryRuby): a long-lived VM in the tab, entries evaluated
  in one session.
- OCaml.org's toplevel (js_of_ocaml toplevel): the compiler's toplevel
  in the page, the stdlib's compiled form preloaded once, each phrase
  compiled alone.
- Klipse: in-page evaluation of snippets for several languages, each
  with a long-lived evaluator.
Xo differs: it compiles to a new wasm module per entry, so the session
keeps compiler state (2a-2c), not a running program's heap; values carry
over by replaying the session's bindings, as the Mac REPL does.

## Alternatives
- Re-run the whole program per entry with no cache (today's ~1.95 s): misses the target.
- Interpret SSA in the tab instead of generating wasm: a second
  execution engine to keep in step with the backends.
- Ship std prebuilt as a wasm object and link in the tab: needs a wasm
  linker in compiler/wasm (relocations, a linking section); larger than 2c.

## Consequences
- The playground's worker keeps one compiler instance alive (memory per
  tab rises by the cached std state; to be measured).
- The repl session code gets a second caller; its tests cover both.

## Step 2 result (2026-11-10)
Level 2a measured in Node (bench/results/browser-repl-2026-11-09.md, "Level
2a measured"): one warm check (about 1.67 s), then each entry checked on a
copy made a base with decision 0513's `start_member_with(true)`: 21-30 ms
per entry check, 76-275 ms per whole entry on the tour's examples, output
byte-identical to a fresh build. 2a is the level the decision takes; 2b
and 2c stay unbuilt unless the session measurements below miss 0.5 s.
Next: `--play repl` (the compiler answers an entry with its module; the
page runs it and returns the run's output, from which the session keeps
the entry, as repl.xo's step does with parse_run and Sess.after), a
30-entry session, the page's worker in Chrome, and the base's memory.

## Step 3 result (2026-11-10)
`--play repl WARM DIR` (compiler/wasm/playrepl.xo) keeps one compiler
instance for a session. Requests, one JSON object a line: `{"in": SRC}`
is answered with `{"run": PATH}` (the entry's module; the page runs it)
and then `{"ran": STDOUT, "code": N}` with xo repl --json's reply; an
entry that does not check or build is answered at once (ok false,
diagnostics at "entry" lines or "session"). `{"cmd": C, "arg": A}` takes
type, defs, undo, reset and quit; doc, save and load stay terminal only
(they need `xo doc` or a file system). The session is compiler/cli/repl's
Sess, with xo repl's rules (an unused use kept, a redefined type's
bindings run again or dropped with a note, binding and value types).

The terminal REPL's 30-entry evidence session
(bench/browserrepl/session.txt, its values in want.txt) on linux/arm64
host 3, Node 20 (bench/results/browser-repl-2026-11-09.md, "Step 3: the
30-entry session"): every value equal to xo repl's (14 of 14); the first
entry 3185 ms (the warm check included), later entries median 294 ms, at
most 418 ms, each including about 120 to 170 ms of starting node for the
run that the page's in-worker instantiate does not pay. So level 2a meets
the 0.5 s target for a session, not only for the tour's single programs.

Open (step 4): the page's transport. browser_wasi_shim reads stdin
synchronously, so a long-lived instance waits for the next entry through
JSPI (fd_read as `WebAssembly.Suspending`, `_start` through
`WebAssembly.promising`: Chrome 137 and later; Firefox announced it as
default from 153; Safari 27 beta lists it), with one instance per entry
(today's cost) where `"Suspending" in WebAssembly` is false. A headless
Chrome probe of the transport ran (below).

## Step 4 probe (2026-11-10, headless Chrome 157 on the Mac Studio)
One xoc instance running `--play repl` for the whole page, stdin's
fd_read wrapped in `WebAssembly.Suspending` and `_start` called through
`WebAssembly.promising` (the instance is made with the async
`WebAssembly.instantiate`: the synchronous constructor is refused on the
main thread above 8 MB), each built entry run in its own WASI instance as
the playground's runner does. Ten entries: the first 1042 ms (the warm
check in the tab), the others 16 to 137 ms each, run included, values
right. The compiler instance's memory reached 1216 MB, which is the next
question: a phone's tab limit, so a smaller warm base or less garbage
kept between entries before the page offers this by default.

## Memory target before the page offers it (owner, 2026-11-10)
No REPL panel on /play/ until the compiler instance's memory is lower:
at most 300 MB at peak in the tab over the 30-entry session, flat after
the warm-up (no growth per entry), and the first entry still about 1 s
or better. Baseline (headless Chrome 157, the Mac Studio, 2026-11-10):
today's /play/ compiler (the website's xoc-20e5db5939ae) reaches 1920 MB
of wasm memory on one build of a hello program in a fresh instance, and
the step 4 probe's long-lived instance 1216 MB after its warm check,
flat over the session's entries, with 88 MB of it on the allocator's
free lists (so about 1.1 GB is held, not size class garbage). The work
is the compiler's own check and build peak, which /play/ pays today too.
Levers: a smaller warm base (what the warm check keeps), freeing each
entry's garbage, memory.grow headroom, and levels 2b/2c.

## Step 5 result: memory (2026-11-10)
Two causes, both fixed; the target is met in Node and in headless Chrome.
- The playground's check (compiler/wasm play.xo `play_check`) set
  native and lazy, as `xo build` does, but not `lazy_parse` (decision
  0351 stage 2) or `skip_tests`, so it parsed every function body of the
  runtime and the std modules it loads. Native measurement of the same
  hello on a cold cache: `xo build`'s check holds 59 MB of heap with
  lazy parsing and 225 MB without (XO_LAZY_PARSE=off), and `xo check`,
  which parses everything, peaks at 800 MB RSS. play_check now sets both
  (XO_LAZY_PARSE=off restores the old path for a comparison).
- The wasm runtime grew memory by doubling (`memory.grow` by
  memory.size + 1 pages), so the memory a tab holds was up to twice the
  heap's high water. It now grows by a quarter of the memory (at least
  a page): a constant number of grows per doubling, and at most 25% over
  the high water.

The 30-entry session (bench/browserrepl), today's compiler (main
2c05aef20 plus this change):

| compiler | wasm memory | first entry | later median |
|---|---|---|---|
| main (XO_LAZY_PARSE=off) | 608 MB | 1874 ms | 195 ms |
| lazy_parse | 304 MB | 999 to 1544 ms | 199 to 272 ms |
| lazy_parse and 25% growth | 265 MB | 805 to 1169 ms | 155 to 185 ms |

Node 22 on the Mac Studio through session.mjs (each later entry includes
about 100 ms of starting node for the run); every value equal to xo
repl's, 14 of 14. Headless Chrome 157 (the step 4 probe page with this
compiler and std, 0583/jspi2): 608, 304 and 265 MB, flat over the 30
entries after the first (212 MB after the first entry, 265 after the
second), entries 44 to 116 ms each, run included. The tour programs
build to byte identical modules with and without lazy parsing.

The 1216 MB of step 4 was an older compiler (2026-11-10 01:31); the
same probe with main's compiler reads 608 MB. So: 265 MB meets the 300 MB
target, flat after the warm-up, first entry about 1 s. The REPL panel on
/play/ is the next step (a website change, through the owner); a
one-shot /play/ build gains the same (one build of a tour program: 608 to
304 MB in Node, check 750 to 550 ms).

## Memory below 50 MB: study (a) (2026-11-10, Node, compiler/wasm built at main)

The owner asked whether a REPL entry can peak under 50 MB. Entry 1 (`use std/adler32`, a checksum
and `1 + 2`) reached 212 MB of wasm memory by the end of check, and SSA, the passes and code
generation added nothing. Natively, the same play build path peaks at 81 MB with 55 MB in use after
check, so wasm held about 3x native's live data. That gap was allocation count, not word size:
wasm made 2.84M allocations (587 MB requested) against native's 0.9M (241 MB).

- `[]` took room for 4 elements at once (compiler/wasm call.xo list.new), where native's `[]` is a
  shared empty list. Each token's `parts` (4 StrParts: 304 bytes, a 512 byte block) did this for
  about 430K std tokens. With no room until the first push, wasm memory is 136 MB after check and
  169 MB at the entry's end (212 before); live small blocks peak at 125 MB (196 before) and requests
  total 354 MB (587 before).
- Str has no views in wasm, so every slice is a new record (1.55M records of 8 to 15 bytes, mostly
  the lexer's token text). They are short lived: 8 MB at the peak.
- Every leaf is 8 bytes in memory (gen.xo size): Bool and enum tags take a word.

Live at the new peak (the end of the entry): two 16 MB blocks (Checker.new_obj's object list at
11 MB, and check_lazy's 11 MB list with its 6 MB copy), Lexer.emit's 7 MB token list, and 76 MB of
smaller blocks. Next, in order: std from its declaration record, so std's tokens, trees and objects
are never built; list growth that does not hold the old and new copies at once; 1-byte Bools and
packed enum tags in memory. Notes and probes: ~/xo-lead/notes/xo-lang-2/repl50/.

## Memory below 50 MB: (b) std's declaration record and (c) size classes (2026-11-10)

(b) `--play` loads std from its declaration record (decision 0448's records, as `xo build` reads
them from its cache) when `XO_PLAY_RECORD=FILE` names one, and `XO_PLAY_RECORD_OUT=FILE` writes it
after a check, so the page's own compiler can write it once for the std it ships. With the record,
std's declarations come from the record and a std body is parsed only where the SSA builder lowers
it: `Checker.join_in_build` (set only by the playground, whose builder runs on one task) makes
compiler/ssa contract.xo `body` join the pending tree itself (`need_tree`), where a multi-task build
marks `trees_missing` and builds again with every tree joined. The record is 1,215,403 bytes
(325,281 gzip -9). Entry 1 in Node: output byte identical, check 490 to 153 ms, live small blocks at
the peak 125 to 57 MB, wasm memory 169 to 87 MB (native footprint of the same path 81 to 33 MB).

(c) The wasm allocator gains a three-quarter size class (3 << (k - 2) bytes) for blocks of 3 KiB
to 1.5 GiB, so a large block is at most a third unused, not half, and a list that grows takes the
capacity its new block holds: a doubled list of 2^k elements asked 24 bytes over a power of two,
so half of each large list block was unused.

The 30-entry session in headless Chrome 157 (0583/jspi2, the same compiler with and without a
record its page writes at load), every value equal across the four runs:

| compiler | wasm memory | later median | max entry |
|---|---|---|---|
| main (power of two classes, no record) | 212 MB | 23 ms | 232 ms |
| record | 108 MB | 23 ms | 204 ms |
| three-quarter classes, no record | 169 MB | 23 ms | 230 ms |
| record and three-quarter classes | 87 MB (69 MB through entry 1) | 21 ms | 208 ms |

Still above 50 MB. Next: free the decoded record's lists before the relocated ones are built
(decode, relocate and install each hold every std object, up to three at once), a growth step
that does not hold a large list's old and new blocks together, and then 1-byte Bools and enum tags.


## Memory below 50 MB: (d) the growth step and large free blocks (2026-11-11)

A probe of every block (the runtime reporting each allocation and release to Node, scratch only)
showed the record path's live blocks peaking at 48.4 MB with 30 MB in seven large blocks: std's object
list three times (install_std's, a copy, and the copy grown), and the record file read into a 3 MB
block. Three runtime changes:
- `fs_read` sizes its buffer from the size `path_filestat_get` gave (it doubled from 4 KiB, so the
  1.2 MB record took a 3 MB block for the session);
- a list of 1 MiB or more grows by a quarter, not double;
- a request of 1 MiB or more takes a free block of one of the next two size classes (kept whole) before
  memory grows: the free lists neither split nor join blocks, and large lists freed in one class were
  asked again in another.

The 30-entry session in headless Chrome 157 (values equal to the terminal REPL's): without the record
169 to 136 MB; with the record 87 MB as before (69 MB through entry 3). The record path's peak is
entry 4 (`xs.sort()`, the first entry that lowers a std body): `need_tree` writes the joined tree into
`c.objs[o]`, and an entry's checker is a copy of the warm one (`var c = base`), so the first write copies
std's whole object list (about 6 MB, then grown) in every entry that lowers a std body. Next: keep the
joined trees where the entry's write does not copy the list (decision 0583 (e)).

## Memory below 50 MB: (e) packed leaves in memory (2026-11-11)

The wasm backend kept every leaf of a value in memory in 8 bytes, its i32 leaves (Bool, the address of
a List, Box or handle, both halves of a Str) as well: std's `Obj` (about 50 i32 leaves and 29 i64 ones)
took 632 bytes, not 432. Values in memory (list elements, boxes, homes, closure environments, wide
results, the blocks the retain and release helpers read, constant data) now pack their leaves: 4 bytes
an i32 leaf, 8 an i64 or f64 one, with no padding (wasm loads and stores need no alignment, and no atomic
operation touches a struct's field: atomics act on a handle's cell). An enum's payload stays in 8 byte
words (its leaves are all i64), so a release of an enum in memory loads its fields a word each. Locals
spilled to the frames' stack keep 8 byte slots. The runtime's lists of Str (str_split, str_lines,
env_*, fs_names, fs_msg) take 8 byte elements, the length at 4. With it, `rl_new` fits a list of 1 MiB
or more to its block, so the copy a write makes takes the pushes after it without a grow.

The 30-entry session in headless Chrome 157, values equal to the terminal REPL's: with std's record
87 to 55 MB (44 MB through entry 3), without it 136 to 108 MB. Live blocks peak at 40.6 MB with the
record; the other 14 MB are free blocks (8 MB of them over 256 KiB, freed by the record's decode and
by entry 4's lowering) that the size classes do not join. Still above 50 MB. Next: std's object list
twice at entry 4 (the warm checker's 6 MB and the entry's copy), and Bool leaves in one byte.

## Memory below 50 MB: (e) big blocks and the grow step (2026-11-11)

A probe of where the heap's end grew, and of the large blocks left free at the end, showed 14 MB of
blocks of 256 KiB or more free in classes no later request asked for. A request whose class is 256 KiB
or more now takes its size rounded up to 64 KiB from a list of free big blocks in address order (the
first that holds it, split from its end when 64 KiB or more is left), else from the top of the heap; a
freed big block joins its free neighbors, and one that ends at the top lowers it. `rl_new` and
`rl_grow` fit a list to what its block holds (`$room`). Memory grows to the pages the heap's end needs
plus 1 MiB instead of by a quarter of itself: the session's 55 MB was 44 MB grown by one quarter step.
The session with std's record: 55 to 53 MB (49 MB from entry 4, then 51 and 53 by its end); without
it 108 to 105 MB; values unchanged. Next: the growth after entry 4 (the heap's end moves while live
blocks stay near 40 MB), and the record's load, which holds std's objects three times (decoded,
relocated, installed).


## Memory below 50 MB: (f) the copy that grows, and the record's bytes (2026-11-11)

A probe of the live blocks at the session's peak by allocation site (1 in 61 sampled, big blocks
exact) showed two whole copies of the checker's object list (3.9 MB each: `new_module` and the lazy
check copy `c.objs`, shared with the base checker, and push to the copy at once, so the copy was made
full and grown again while both lived) and the record file itself (1.2 MB) live through every check.
A copy-on-write list copy of 1 MiB or more now has an eighth more room, and the play path no longer
keeps the record's bytes in `std_export_in` (only `check_dirs_bound`'s restart and
`XO_STD_EXPORT=verify` read them again). The session with std's record: 53 to 49 MB; without it 105
to 101 MB; values unchanged. Below 50 MB, as the owner asked. Left: the record's decode still holds
std's objects twice while it installs them (about 7 MB), and on wasm a slice of the record's bytes is
a copy (1.6 MB).

## Memory: std's record relocated in place (2026-11-12)

Measured on the /play/ page itself (bench/results/browser-repl-page-2026-11-12.md: the 30-entry
session typed into the repl tab in headless Chrome, `data-repl-mem`), main 955a6c8a7 peaks at 49 MB.
A scratch probe of the runtime's live bytes showed the session holding 21.8 MB between entries, live
bytes peaking at 36.5 MB inside entries (entry 4 and later), and the heap's top 12.5 MB above that:
free big blocks, 9.4 MB of them (1.2, 2.6 and 5.7 MB) left by entry 1's record load below the data
the session keeps, where later entries' 4 MB requests did not land. `relocate_export` now renumbers
the decoded record's object and type lists in place (the caller hands it the record's last
reference) instead of building relocated copies (the object list's with half again its length free).
Entry 1 then leaves one 6.9 MB block and a 1.2 MB one; the session peaks at 48 MB (33 MB after entry
1, was 35), values unchanged. Left: install_std's new object list beside the relocated one (the 6.9
MB block), and the 4.3 to 4.4 MB blocks entries 2 and 20 leave free.

## Memory: the SSA builder owns the entry's checker (2026-11-12)

A trace of the big block requests (256 KiB and up, each with its wasm stack, scratch probe) through
the 30-entry session showed every entry copying the checker's object list (4.3 MB) twice: once when
the entry's check adds its module to `var c = base` (base keeps its own; by design), and again when
the SSA builder joins a std function's trees (`need_tree`) or checks a lazy body, because two holders
were still sharing the entry's list: play_repl, which asked `semantic.Session.type_at` for the
entry's types after the build, and `ssa.build_program_fe`, which keeps its checker to build again
from it when a body needs the unwinding checks the first build left out. Entries 4 and later peaked
with three object lists live (36.4 MB). Now play_repl takes the entry's types before the build, and
builds with `ssa.build_program_handed`: one build, no second one, the checker's last reference
handed to the builder, which then writes in place. A program that needs the other setting
(`need_conc != conc`) is checked again from base and built with it; the setting the last build used
is the next entry's first guess, so a session holding a Mutex builds twice once, not in every entry.
The session's live peak is 31.4 MB and the page's compiler memory peaks at 39 MB (48 before; 33 MB
after entry 1, unchanged), the later entries' median 31 ms (35); every reply, types included, is
the same as main's compiler's for the session, a Mutex and Chan session, and `:reset` and `:undo`
back to no concurrency (TestPlayReplConc). Left: entry 1's 6.9 MB free block (install_std's object
list beside the decoded one).

## The playground page: a repl tab and std's record (2026-11-11)

The lead asked (2026-11-11) for the owner's 50 MB bar on the real /play/ page, not the harness. The
website branch `l2-play-repl` (15c342fa8, unpushed, for the owner's review) adds a repl tab to /play/:
the toolchain worker keeps one compiler running `--play repl`, its stdin read through JSPI, and each
entry that builds runs in the page's runner worker under a program's limits. `scripts/play-record.mjs`
writes std's declaration record at site build time (in Node, over the page's own paths), and builds
and the REPL load it. Without JSPI (Firefox, Safari) the tab says why and its input is off; run,
format and problems work as before (checked by deleting `WebAssembly.Suspending` and `promising`
before the page's scripts: no Chrome flag turns shipped JSPI off).

Building the page from main showed every non-REPL build overflowing the Chrome worker's stack (the
tour's basics included): `Checker` had grown to about 1,143 leaves, and V8's baseline compiler sizes
a frame by its deepest operand stack. 42160d539 fixes it (gotchas/wasm-browser-stack-locals.md):
a build now runs in a 200 KB Node stack, where 0.35.0's compiler (the live page's) needed 350 KB.

Measured with the page built from 42160d539 (headless Chrome, macOS, 2026-11-11; the session of
the earlier steps, 30 entries, typed into the tab):

- The REPL's compiler: 35 MB after `use std/adler32`, 43 to 46 MB through entry 28, peak 49 MB
  (entries 29 and 30); the first entry 543 ms (with the instance's start), later entries 127 to
  135 ms each. On a phone viewport (390 by 844) the first 8 entries: peak 46 MB, nothing wider
  than the screen.
- The renderer process's RSS (all of the tab: the page, the toolchain and runner workers, V8's code
  for the 58 MB compiler): 522 MB with the page loaded, 1,151 MB after consent and the first check,
  1,302 MB after one run, 1,677 MB after the REPL's first entry, peak 1,612 to 1,899 MB through the
  sessions. The live page (0.35.0's compiler, no record): 516, 1,010, and 2,004 MB after one run.
  The 50 MB bar is the compiler's own memory; the tab around it is far larger on either page.
- Download (brotli): the compiler 4,454,796 bytes, std 1,680,097, std's record 240,883: 6.4 MB on
  first use, against 3.6 MB for the live page (compiler 2,402,324 and std 1,174,238). The record adds
  0.24 MB; the rest is the compiler grown since 0.35.0 (22.5 to 57.9 MB raw; 0.5 MB brotli of it
  from 42160d539's per-leaf loads and sret calls) and std grown.

Next: the compiler's download size (the wasm module grew 2.6 times since 0.35.0), and the record
could be written by the site build from the released seed's xo instead of the page's compiler.
