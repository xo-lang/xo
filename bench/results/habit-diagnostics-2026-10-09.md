# Habit diagnostics (2026-10-09)

Codex passed `xo check` on the first try in 3 of 10 Xo tasks, and its own
notes blame habits from other languages for every failure
([codex-2026-10-09.md](codex-2026-10-09.md)). This change makes the
checker name each habit and the Xo form, with a machine fix when the
rewrite is unambiguous, and recover so one habit gives one diagnostic
(decision 0087, spec 10.2, `xo explain XO0177`). Toolchain before:
`6f771ce`; after: this branch.

## What Codex hit

`runs/codex/xo/` holds Codex's final code, `NOTES.md`, and `runs.jsonl`
(green or not, and the stage) for each task, but no transcripts or
diagnostic output. The failing first runs are reconstructed from the final
code and the cause Codex wrote in its notes
([bench/habits/reconstruct.py](../habits/reconstruct.py)); where the notes
are vague the reconstruction is a best guess, marked below.

| Task, run | Habit (Codex's note) | Codex wrote (reconstructed) | Xo form | Before (`6f771ce`) | Now |
|---|---|---|---|---|---|
| 01, run 1 | not recorded | unknown: the notes explain run 2 (sandbox cache write) only | | | not reconstructable |
| 04, run 1 | comma separated match arms (Go and Rust) | `Linux => p,` | `Linux => p` | 3 x XO0169, no fix | 3 x XO0169 with a note and a fix |
| 05, run 1 | unused import | `use std/task` | remove it | XO0110 with fix | same |
| 05, run 1 | `get_or_fault` reason argument | `buckets.get_or_fault(key)` | `buckets.get_or_fault(key, "why")` | XO0403 | XO0403 with a note; no fix (the reason is the program's to state) |
| 06, run 1 | `var` at a call site | `expire(var d, now, q.max_attempts)` (4 calls) | `expire(d, ...)` | 13 errors: XO0160, XO0502, XO0403 per call, XO0110 | 4 x XO0177 with fix |
| 06, run 1 | `Map.update` with an optional transform | not in the final code (Codex replaced it) | `m.update(k, init, fn(var v) { ... })` | | not reconstructable |
| 07, run 1 | Rust `Hash + Eq` bounds | `struct Cache[K: Hash + Eq, ...]`, `fn cache[K: Hash + Eq, ...]` | `K: Hash` | 96 errors (XO0160, then `Cache` lost its parameters: XO0403, XO0402, XO0601 with wrong fixes such as `fx` to `Fs`) | 2 x XO0177 with fix |
| 07, run 2 | omitted effect argument stayed abstract `Cache.fx` | `fn routes(c: Cache[Int, Item])`, handler `fn(req) uses (clock, net.dial)` (best guess) | `Cache[Int, Item, net.dial]`, or no `uses` on the handler | XO0701 "performs `Cache.fx`" | XO0701 with a note on where the argument was left out and a fix removing the closure's `uses` |
| 08, run 1 | `use` after declarations | `struct Page ...` then `use std/json` | imports first | XO0173, no fix | XO0173 with a note and a fix moving the line |
| 10, run 1 | JSON for tuples | `top: List[(Str, Int)]` in a `derive Json` struct | a struct with named fields | XO0404 | XO0404 with a note (tuples have no JSON form) |
| 10, run 1 | `Duration.parse` | `Duration.parse(args[i + 1])` | not in std (a gap) | XO0601 | XO0601 with a note (no `parse`; parse the number) |

## Results

Each case is checked in its own `xo check` process
([bench/habits/measure.py](../habits/measure.py)). "Fixable" means every
error diagnostic carries a machine fix; "green" means `xo check` reports no
error after `xo fix`.

Codex's failing first runs, reconstructed (7 snapshots; 01 and the
`Map.update` habit of 06 could not be reconstructed):

| Toolchain | Errors | With a fix | All errors fixable | Green after `xo fix` |
|---|---:|---:|---:|---:|
| before (`6f771ce`) | 118 | 6 | 0/7 | 0/7 |
| after | 15 | 12 | 5/7 | 5/7 |
| after decision 0088 (std gaps) | 14 | 13 | 6/7 | 6/7 |

The two that stay red need a decision or a std change: 05 needs the
`get_or_fault` reason, 10 needs JSON for a struct instead of a tuple and a
way to parse a duration (both std gaps Codex reported).

Update 2026-10-09 (decision 0088): with JSON for tuples and
`Duration.parse`, 10's only error is `Duration.parse(...).or_err(...)`
(Codex assumed an Option, like `Int.parse`); `or_err` on a Result now has
a habit fix to `map_err`, and 10 is green after `xo fix`, its 5 tests
passing. Only 05 stays red (it needs a reason string).

Synthetic habits: 149 golden cases in
[internal/fix/testdata/habits](../../internal/fix/testdata/habits), one
program per habit form (about 130 distinct habits from Go, Rust,
TypeScript, Python, Swift, and Kotlin, and Codex's five):

| Toolchain | Errors | With a fix | Green after `xo fix` |
|---|---:|---:|---:|
| before (`6f771ce`) | 250 | 47 | 10/149 |
| after | 157 | 146 | 139/149 |

The 10 without a fix are the policy's no-fix cases: `x!!`, `x!`,
`.unwrap()` (a reason is needed), `try`/`catch`, bare `print` with no
capability in scope, `go f()`, `make(...)`, `class`, `any`, and an `impl`
with a function that takes no `self`.

Whole first drafts, one per language
([bench/habits/drafts](../habits/drafts)), written the way a programmer of
that language writes a first Xo file:

| Draft | Errors before | Errors after | With a fix | Errors left after `xo fix` |
|---|---:|---:|---:|---:|
| Go | 2 | 41 | 41 | 0 |
| Python | 6 | 41 | 41 | 0 |
| TypeScript | 9 | 44 | 43 | 1 (`import { encode } from "json"`) |
| Swift | 4 | 29 | 28 | 1 (`xs.first` without `()`, reported downstream) |
| Rust | 10 | 30 | 27 | 2 (`.unwrap()` reason; `derive Json` missing) |
| Kotlin | 3 | 30 | 28 | 2 (`data class`, `!!`) |

Before, the first hard syntax error stopped the declaration and hid the
rest (2 errors for the whole Go draft); now every habit is reported once,
and what remains after `xo fix` is a decision the code has to make.

No accepted program changed: `xo check` over all 361 directories with
`.xo` files in the repository reports the same codes at the same places
before and after.

Robustness: `FuzzParse` (seeded with every habit input and draft) ran
clean for 400 s after two lookahead bounds fixes it found, and `xo check`
on every 3 byte prefix of every habit input and draft (4359 programs)
does not panic.

## Next experiments (ranked)

1. Rerun the Codex Xo arm with this toolchain through `bench/harness`
   (same prompts, `xo check` then `xo test`), and measure first try passes
   and runs to green against 3/10 and 20 runs. Target from the Codex
   report: 7/10 or better. Also run Claude's arm to confirm no regression.
2. Close the std gaps behind task 10: `Duration.parse` (or a documented
   parse path) and either JSON for tuples or a clearer note; then rerun
   task 10 alone. Done 2026-10-09 (decision 0088); the reconstruction is
   green after `xo fix`, the live Codex rerun of task 10 is still to do.
3. Kotlin and Swift property habits that pass the checker as method
   values (`xs.first`, `xs.count`): report them where the value is used,
   in internal/check/habits.go `habitField`, with a negative test for real
   method values.
4. Longer optional chains, `u?.a.b?.c`, in a function that does not return
   an optional: the one step chain `u?.name` already gets
   `u.map(fn(u) { u.name })` (`safeNavFix` in internal/check/habits.go);
   extend it to a chain of `map` and `and_then` calls.
5. Kotlin trailing lambdas, `xs.map { it * 2 }`, in parsePostfix (a `{`
   after a call or selector on the same line outside a condition).
6. `xo check a b` with a repeated module path: make it a usage error
   (knowledgebase/gotchas/xo-check-same-module-path.md).
