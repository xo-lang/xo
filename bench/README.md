# Xo benchmark

Measures whether Xo's design actually helps AI agents, and which proposed rules
cost more than they return. Decisions marked `Proposed` in
`knowledgebase/decisions/` are kept, changed, or dropped based on these results.

## Nightly

`bench/nightly.sh` runs the full test suite with high random seeds, `-race`
on the runtime, and vet; one summary line per run goes to
`results/nightly/summary.txt`. Not scheduled by default.


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
- [2026-10-08 Monomorphization](results/perf-mono-2026-10-08.md): `xo build --mono` vs idiomatic Go generics: generic workloads 1.6x to 17.8x slower -> 0.49x to 2.26x; beats Go on max_by and map/filter/fold.
- [2026-10-08 Request scoped arenas](results/perf-arenas-2026-10-08.md): task 01 under loopback load (`bench/perf/http/`): `xo build --arenas` cuts GCs per request 3.6x and raises throughput 40 percent, but Xo still allocates 2x Go per request (Go 123k req/s, Xo 52k, Xo --arenas 73k).
- [2026-10-08 Request throughput](results/perf-throughput-2026-10-08.md): the gap was a lock convoy on the store mutex, not Xo CPU (5 percent of samples); barging plus a Gosched after wake takes Xo --arenas from 64k to 88k req/s (Go 124k). Also direct `derive Json` codecs and a lazy cancellation channel.
- [2026-10-08 native arm64 backend](results/arm64-native-2026-10-08.md): `xo build --backend=arm64` lowers the SSA IR straight to AArch64 assembly (no LLVM); all bench programs and SSA goldens identical, 0 live allocations; 0.93x to 1.25x of the LLVM backend (fib 1.02x, sieve 1.10x, nbody 0.93x, collatz 0.98x) and 0.43x to 1.07x of Go, same peak memory.
- [2026-10-08 native arm64 vs LLVM gap](results/arm64-perf-2026-10-08.md): match chains laid out as runs of tests plus jump tables from 8 values (decision 0051), branchless compare select, retain/release cancellation; interp 1.23x to 0.70x of LLVM, sort 1.15x to 1.09x, strings 1.10x to 0.99x, nothing slower.
- [2026-10-08 reference counts shared between threads](results/rc-shared-2026-10-08.md): thread local counts until a value is marked shared (decision 0058); every bench/perf/llvm workload within 2% on both native backends (process cycles), ThreadSanitizer clean C stress test and four thread whole programs; tasks do not lower natively yet.
- [2026-10-08 LLVM backend with reference counting](results/llvm-rc-2026-10-08.md): the LLVM backend lowers from internal/ssa with Perceus style RC and in place reuse; every allocation freed; peak RSS at or below Go (sieve 54 MB vs 49, sort 78 vs 78, string churn 16 vs 28) from up to 10 GB; times equal or better than the prototype (0.40x to 1.15x of Go; string churn 0.40x).
- [2026-10-08 LLVM backend prototype](results/llvm-prototype-2026-10-08.md): `xo build --backend=llvm` on a core subset, checked arithmetic kept: 0.47x to 1.13x of hand written Go on six compute workloads (fib 1.03x, sieve 1.02x, nbody 1.13x, sort 0.87x, interp 0.47x, collatz 1.00x); against Xo's Go backend the LLVM build ties fib, is 1.2x to 102x faster elsewhere, and finishes sieve and sort, which the Go backend does not in 60 s (O(n) list writes); never freeing memory costs up to 2.9x Go's RSS.
- [2026-10-08 stage 3, tasks 09 and 10](results/stage3-2026-10-08.md): first run success Xo 2/2, plain Go 1/2, strict Go 0/2; Xo about 55% of Go lines; graceful drain gap found.
- [2026-10-08 Go library interop prototype](results/go-interop-2026-10-08.md): `use go` and `xo bind` (decision 0041); crypto/sha256, encoding/hex, regexp, strings, strconv, net/url bind 123/125 functions and 92/95 methods with goldens for each; github.com/google/uuid binds and runs (29/31 functions, 20/28 methods); gaps: interface conversion between handles, methods of named basic types, `any` parameters.
- [2026-10-08 real application: notes-service](results/real-app-2026-10-08.md): HTTP, std/sql, bearer auth, purge job, and two phase shutdown in `examples/notes-service/`; check, test (9/9), and all targets pass with no compiler change; top gaps: no request scoped values, global non failing middleware, `Listener` has no `addr()`, no std crypto, test logs go to real stderr.
- [2026-10-08 self hosted lexer](results/selfhost-lexer-2026-10-08.md): `selfhost/lexer`, the lexer ported to Xo, matches internal/lexer byte for byte (tokens, positions, values, diagnostics) on all 416 repo `.xo` files, 93 spec blocks, every rune, and 93,000 mutated inputs; 1394 Xo vs 1170 Go code lines; 2.33x slower than Go on the Go backend, native backends cannot build it; top gaps: no byte string builder, no numeric character literals, no Bytes in the SSA builder.

## Other models: `bench/harness/`

`bench/harness/` packages stage 3 and the planted bug benchmark so they can
be rerun with any model (GPT, Gemini, local) outside Claude Code, and scored
the same way: one prompt per task and arm (Xo, plain Go, strict Go) and per
planted bug author set, an adapter for any OpenAI compatible endpoint with
the same write then iterate loop, and `score.py`, which reruns every cell
with the current toolchain and prints the tables of the three arm and
planted results. A full run is about 0.9M input and 0.4M output tokens.
See [harness/README.md](harness/README.md).

- [2026-10-08 harness validation](results/harness-validation-2026-10-08.md): scoring the stored Claude outputs reproduces every published runs to green, line, and test count and every planted bug result (Go 9 and 13 of 30, Xo 29 and 23 of 30); with today's toolchain Xo 03 fails its contract fuzz (the known `backoff` overflow) and Xo 06 fails XO0410 (a spec 0.6 rule).
