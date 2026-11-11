# 0583: A REPL in the browser playground

Status: Proposed (owner request via the lead, 2026-11-09; measurements first, no page work until the owner reads them)

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
