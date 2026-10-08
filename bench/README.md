# Xo benchmark

Measures whether Xo's design actually helps AI agents, and which proposed rules
cost more than they return. Decisions marked `Proposed` in
`knowledgebase/decisions/` are kept, changed, or dropped based on these results.

## Tasks

| # | Task | Main risks exercised |
|---|---|---|
| 01 | [CRUD API](tasks/01-crud-api.md) | shared state (`Mutex`), typed errors, HTTP, derive |
| 02 | [Worker pool](tasks/02-worker-pool.md) | `scope`/`spawn`, channels, `select`, accumulation under value semantics |
| 03 | [Retry client](tasks/03-retry-client.md) | effects (`net clock rand`), `from` error variants, fake capabilities in tests |
| 04 | [File sync](tasks/04-file-sync.md) | `fs` capabilities and attenuation, `defer`, platform `match target.os` |
| 05 | [Rate limiter](tasks/05-rate-limiter.md) | `Mutex[Map]`, `var` receivers, background task, clock |

## Stage 1: paper test (no compiler)

A fresh agent gets only `spec/core.md` and one task file, writes the solution in
`bench/paper/<spec-version>/<task>/`, and writes `NOTES.md` listing ambiguities and moments it
reached for Go or Rust constructs. A grader then checks the code against the
spec and classifies every deviation:

| Category | Meaning |
|---|---|
| `GO-HABIT` | Wrote Go syntax or idiom the spec forbids (`func`, `:=`, `nil`, `err != nil`, camelCase, `go f()`) |
| `RUST-HABIT` | Wrote Rust syntax or idiom (`impl`, `&mut`, `Some(x)`, `Vec`, `::`) |
| `MUTATION` | Violated value semantics or move rules (section 5) |
| `EFFECTS` | Missing or wrong `uses`, capability conjured instead of passed (section 6) |
| `ERRORS` | Unhandled result, wrong `!`/`?`/`from` usage, nil like thinking (section 2.6) |
| `CONCURRENCY` | Spawn outside a scope, leaked task, ctx passing (section 7) |
| `NAMING` | Violated section 1.5 |
| `DEFER` | Relied on function scoped defer (section 3.4) |
| `INVENTED` | Used an API or syntax the spec does not define |
| `SPEC-GAP` | The spec is ambiguous or silent; the agent had to guess (a spec bug, not an agent bug) |

Results go in `results/paper-YYYY-MM-DD.md`.

Runs:
- [2026-10-07 spec 0.1](results/paper-2026-10-07.md): 0 rule violations, 116 guesses, 7 spec bugs.
- [2026-10-07 spec 0.2](results/paper-2026-10-07-v0.2.md): 0 rule violations, 22 guesses, 4 spec bugs.
- [2026-10-07 spec 0.3](results/paper-2026-10-07-v0.3.md): 0 rule violations, 11 guesses, 2 spec bugs.

## Stage 2: POC compiler

Same tasks, real `xo check` feedback. Metrics: first try compile rate,
iterations to green, diagnostics by code.

Checker runs over the v0.3 corpus:
- [2026-10-07 spec 0.3](results/check-corpus-v0.3.md): 1 diagnostic (a real violation); 30 checker bugs found and fixed on the way.
- [2026-10-07 spec 0.4](results/check-corpus-v0.4-spec.md): 4 diagnostics, all real violations under 0.4 (3 new XO0206 `let _ =` discards, 1 XO0601); 14 readings chosen for 0.4.

## Stage 3: three arm comparison (plain Go, strict Go, Xo)

Same tasks written by fresh agents in three arms:

| Arm | Feedback loop |
|---|---|
| plain Go | `go build`, `go vet`, `go test` |
| strict Go | `bench/go/strict-check.sh`: vet, staticcheck, errcheck, nilaway, exhaustive, `go test -race` |
| Xo | `xo check`, `xo test` (once the compiler exists) |

Agents write the whole solution before running any command, then iterate.
Each logs every run in `BUILD_LOG.md`. Go solutions live in
`bench/go/<arm>/<task>/`, standard library only. Analyzers are installed in
the project local `.tools/bin` (see `knowledgebase/infrastructure/`).

Metrics: first try pass rate, runs until green, problems by category, and
afterwards: strict findings in plain arm code (escaped bugs), planted bug
detection, time to root cause.

Stage 3 runs:
- [2026-10-07 Go baseline](results/go-baseline-2026-10-07.md): plain Go 5/5 first run pass; strict Go 1/5 first run, 9 runs total; 2 real bugs caught, 3 false positives.
- [2026-10-07 planted bugs and harder tasks](results/planted-and-harder-2026-10-07.md): tests catch 85% of logic bugs but 30% of safety bugs; strict Go 43%; leaks escape all Go tools; Xo estimate 17/30 at compile time.
- [2026-10-07 stage 2 Xo checker loop](results/stage2-xo-check-2026-10-07.md): 7/8 clean on the first check; mutation probe 100% on `?` and `var`, 55% on lost updates (5 real misses).
- [2026-10-07 P1 code generation run](results/gen-run-v0.4.md): all 8 stage 2 solutions build and pass their written tests (103/103, one needs `--real`), also under `-race`; 2 codegen bugs and 2 runtime gaps found and fixed; contract fuzzing finds 1 latent bug (03 `backoff`); the tests catch 21 of the 40 mutations the checker misses.
- [2026-10-07 planted bugs in Xo (measured)](results/planted-xo-2026-10-07.md): Xo prevents or catches 21 to 24 of 30 safety bugs across two independent authors (spec 0.6: 23 to 29 caught, 25 to 29 caught or warned); strict Go 13, plain Go 9; logic bugs 16/20 vs Go 17/20.
- [2026-10-08 three arm comparison](results/three-arm-2026-10-08.md): writing is even (first try passes); safety bugs Xo 23 to 29 of 30 vs strict Go 13; Xo code about half the size.
- [2026-10-08 Map performance](results/perf-map-2026-10-08.md): update heavy Map loops 45x slower than Go -> 3.2 to 4.4x; reads, iteration, copies faster.
