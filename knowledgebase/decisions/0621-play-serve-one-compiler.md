# 0621: One playground compiler a visit (`--play serve`)

Status: Accepted (owner, 2026-11-12, relayed by the lead: "reuse one
compiler instance across requests, with a test that two runs share no
state"); Xo Lang 2

## Context

The /play/ page's worker ran every check, format and build in a new
WebAssembly instance of the compiler (xolang.dev assets/js/play
toolchain.js `xoc`). bench/results/browser-play-tab-2026-11-12.md found
that V8 frees an instance late: up to 8 were alive at once, each with 13 to
20 MB of compiled code and its own linear memory, so the tab's footprint
grew with the number of runs (decision 0615's /play/ row: 483 MB peak over
30 runs). Its first candidate fix was to reuse one instance.

A WASI module's `_start` runs once, so reusing an instance means a
compiler that serves requests, as the REPL already does (`--play repl`,
decision 0583, over JSPI's suspending `fd_read`).

## Decision

- compiler/wasm play.xo gains `--play serve`: each standard input line is
  a JSON list of one command's arguments (`["build", DIR, OUT]`,
  `["check", DIR]`, `["fmt", FILE]`), answered with that command's usual
  JSON line, until standard input ends. serve, repl and rebuild are
  refused inside it.
- A request carries nothing to the next: Xo has no globals, and each
  command builds its own checker, std record and output. TestPlayServe
  (internal/selfhost wasm_play_test.go) holds this: eight requests over one
  process, with the program's file rewritten at the same path between them
  (a definition that disappears, diagnostics then none, a syntax error, a
  format), give the same replies, the same built modules and the same
  formatted text as eight fresh processes.
- The page (toolchain.js `compile`) sends check, format and build to one
  compiler per visit under JSPI, one request at a time, refilling /w
  (std as now, the program, an empty out) before each. It falls back to an
  instance a request when the browser has no JSPI or the compiler answers
  serve with a usage error (an older xoc), and replaces the compiler after
  a request that grew its memory past 256 MB.

## Consequences

- The tab holds one compiler's code and memory instead of up to 8. The
  measured change is in "Measured" below; decision 0615's /play/ baseline
  moves with it.
- The REPL and serve share the worker's `server()`.
- A compiler bug that corrupts its own state would now show in the next
  request of the visit rather than vanish with the instance; a crash ends
  the compiler (its reply throws) and the next request starts a new one.

## Measured

scripts/browser-memory.sh on the development Mac (headless Chrome,
darwin/arm64), 2026-11-12: one xo tree (4d9d451cb, with serve) built into
the site's assets twice, once with the worker that makes an instance a
request (site l2-rel0500) and once with this one (l2-playserve-web), the
/play/ test's 30 builds and runs, 4 runs each (the first alone, then
MEMORY_RUNS=3):

| worker | tab footprint peak (MB) | after the 30th run (MB) |
|---|---|---|
| an instance a request | 472, 474, 479, 446 | 243, 336, 249, 218 |
| one compiler a visit | 419, 417, 421, 417 | 189, 188, 188, 184 |

The peak drops about 12% (median 473 to 418 MB) and is steadier; what
the tab holds after the runs drops about 25% (median 246 to 188 MB). The
REPL's peak is unchanged (36 MB). decision 0615's /play/ baseline (482 MB)
is left as it is; lowering it is a separate commit to the site's
data/memory.json once this worker is deployed. Rows:
~/xo-lead/notes/xo-lang-2/playserve/memory.md (not in the repository).

## Alternatives

- Keep an instance a request and only reset memory: a wasm memory cannot
  shrink, and an instance's `_start` cannot run twice.
- Liftoff only (no TurboFan tier-up) for the compiler: smaller code per
  instance but still one per run, and slower builds; not measured.
- Reuse without JSPI by throwing out of `fd_read`: leaves the compiler's
  stack and reference counts mid-request.
