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
Chrome probe of the transport is drafted but has not run to the end yet.
