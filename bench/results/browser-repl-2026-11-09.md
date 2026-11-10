# Browser REPL, step 1 measurements (decision 0583; Xo Lang 2, 2026-11-09)

The playground's compiler (compiler/wasm built for wasm by itself, 54.1 MB)
at origin/main 5b9691110 plus XO_PLAY_TIMES, run under Node 22.22.1's WASI
(compiler/wasm/run.mjs) on the Mac (M-series, shared), one process per
command as the page's worker runs it. Three tour examples from the site
repo (examples/tour/basics, collections, concurrency), three builds each.
Scripts: the 0583 notes folder (measure-node.sh, phases.sh).

| | basics | collections | concurrency |
|---|---|---|---|
| `--play build`, whole process | 1982-2073 ms | 2182-2188 ms | 1985-2016 ms |
| run of the built module (Node start included) | 109-113 ms | 106-114 ms | 104-111 ms |
| check (XO_PLAY_TIMES) | | 1507 ms | 1488-1505 ms |
| SSA build | | 63 ms (99 functions) | 51-53 ms (18 functions) |
| passes | | 159 ms | 89-102 ms |
| wasm gen | | 92 ms | 15-16 ms |

Load baseline (`--play fmt` of one file, no check): 330-343 ms (Node start
plus compiling the 54 MB module).

The lead's Chrome numbers on the live page (~0.34), 2026-11-09: compiler
load 2.9 s, about 1.95 s per build+run warm, programs 1 ms.

Reading:
- The check is about 1.5 s of the 1.7-1.85 s compile on every example, and
  it is nearly the same for an 18-function and a 99-function program: it
  is std (the native prelude and the runtime, checked lazily, decision
  0247), not the entry.
- SSA, passes and gen together are 150-320 ms, scaling with the program.
- So level 2a of decision 0583 (one long-lived instance keeping the
  checker's std modules) is the lever: if an entry's own check is under
  about 100 ms, an entry would cost about 0.25-0.45 s, within the 0.5 s
  target without caching SSA or generated code (2b, 2c).
- Not measured yet: 2a itself (needs the `--play repl` loop), the page's
  worker (Chrome) rather than Node, and memory held by a kept checker.

Found on the way: `--play build` given absolute paths (`--wd=/abs/dir
--play build /abs/dir out`) answers `no main function in the root
module`, relative paths (`--wd=. --play build . out`, as the page passes
them) build. Not chased; the page is unaffected.

## Level 2a measured (2026-11-10)

`--play rebuild WARM DIR OUT N` (compiler/wasm/play.xo) checks a small
warm module (a three line `main` using a List and string interpolation)
once on a fresh checker, then builds DIR N times, each from a copy of that
checker made a base with `start_member_with(true)` (decision 0513's
workspace member checker: the base's modules are not checked again, its
per-node records are kept for the SSA builder). Same Node 22.22.1, same
xoc.wasm build of the branch, three runs each:

| example | warm check | entry check | entry total (check, ssa, passes, gen) | fresh `--play build` (2026-11-09) |
|---|---|---|---|---|
| basics | 1667 ms | 22 ms | 76-77 ms | 2.0-2.2 s |
| collections | 1678 ms | 21-24 ms | 265-275 ms | 2.0-2.2 s |
| concurrency | 1664 ms | 22-30 ms | 110-119 ms | 2.0-2.2 s |

- The wasm each copy builds is byte-identical to a fresh `--play build`
  of the same example, and runs with the same output.
- A copy reports the same diagnostics as a fresh check (none on the tour;
  the same four on a program with a type error and two unknown names).
- A copy of a checker that checked the same module again (no base marking)
  is not a valid check: it re-declares every loaded module (12,711 XO0603
  diagnostics), which is what start_member_with prevents.
- So level 2a alone puts an entry at 76-275 ms in Node, inside the 0.5 s
  target; 2b and 2c are not needed for the tour's programs. Still to
  measure: the page's worker (Chrome), a 30-entry session (programs grow
  with the session), and the memory a kept base holds.
