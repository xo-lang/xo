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
- [2026-10-10 self hosted parser](results/selfhost-parser-2026-10-10.md): `selfhost/parser` covers the whole grammar; identical tree dumps to internal/parser on all 736 accepted repo and std files, 150 generated programs, and every accepted mutation, same rejections, on the go, arm64, and LLVM backends; parsing 1.3x faster than Go natively (LLVM), 1.13x slower on arm64, 2.56x slower on the Go backend; found and fixed two arm64 limits (frames past 4000 bytes, helper frame records past stp reach).
- [2026-10-10 self hosted checker, stage 1 (names)](results/selfhost-checker-2026-10-10.md): `selfhost/check` resolves names like internal/check: identical names sections (49,346 lines) on all 347 runs the Go checker accepts and the same diagnostics on all 56 rejected only for names, on the go, arm64, and LLVM backends, plus 1,000 mutations; 403 of 597 runs compared; names stage 0.63x the Go checker's full time on LLVM, 1.31x on the Go backend; found and fixed three compiler bugs (nested optional `??`, same named types of two modules natively, assignment order natively).
- [2026-10-10 self hosted SSA builder, stage 1](results/selfhost-ssa-2026-10-10.md): `selfhost/ssa` lowers the core subset exactly as internal/ssa's builder: 0 differences on every function it completes over the 300 repository runs both build and 400 mutations, go, arm64, and LLVM builds; 5,844 of 10,990 functions identical (maps, interfaces, defer, concurrency are later stages); LLVM build about 1.45x the Go builder, arm64 about 10x (large `var` receivers copied in and out); found an arm64 `add` immediate limit and an enum equality cost of the Go backend.
- [2026-10-11 self hosted Go code generator](results/selfhost-gen-2026-10-11.md): `selfhost/gen` writes the Go of internal/gen byte for byte on every run both checkers check (131 internal/gen goldens, 538 other repository runs; programs, test binaries, release, with and without monomorphization and the range analysis), its own last step included; `xo1 build --backend=go` compiles selfhost/check to identical Go and itself to a fixed point (byte identical tree and binary).
- [2026-10-11 self hosted LLVM emitter](results/selfhost-llvm-2026-10-11.md): `selfhost/llvm` lowers the self hosted SSA module to the LLVM IR internal/llvm prints (decision 0166); stages 1 to 3: all 13,683 functions of the 318 repository runs both lower identical, all 318 modules and 46 test binaries byte identical; the self hosted pipeline lowers selfhost/ssa to 399 MB of IR byte identical to internal/llvm's and clang builds a working binary from it; the go build of the Xo emitter about 0.53x the Go emitter's time; found the Xo SSA builder making its generated functions in another order.
- [2026-10-11 self hosted arm64 emitter](results/selfhost-arm64-2026-10-11.md): `selfhost/arm64` emits the same assembly as internal/arm64 (four stages, compared through `xo asmdump`): all 10,975 functions and all 306 runs both SSA builders build completely identical; the switch-over: the Xo compiler built with the arm64 backend compiles itself to byte identical assembly (also the Go emitter's) and its self compiled build passes 132 of 138 arm64 goldens (std/http needs TLS linking).
- [2026-10-10 self hosted checker, stage 2 (types)](results/selfhost-checker-2026-10-10.md#stage-2-types-and-inference-2026-10-10): `selfhost/check` types every expression like internal/check: identical names and types sections (91,173 and 116,419 lines) on all 348 accepted runs and the same diagnostics on all 76 rejected only for names and types, on the go, arm64, and LLVM backends, plus name and type mutations; 1.24x the Go checker's full time on LLVM, 2.0x on the Go backend; found and fixed a native backend bug (`var` receiver read before the arguments) and a parser trap (`NAMES[i]`).
- [2026-10-09 Linux validation](results/linux-2026-10-09.md): `scripts/linux-test.sh` (Docker, `bench/linux/Dockerfile`); the whole suite passes on linux/arm64 natively (ELF goldens, epoll, futex, signals, TCP, HTTP, TSan, ASan) and linux/amd64 under Rosetta (TSan cannot start there); fixes: glibc port truncation in listen and dial, UBSan nonnull and null offset reports in std/llrt.
- [2026-10-09 spec stability audit](results/spec-audit-2026-10-09.md): every core.md and std.md feature sorted for the owner's 1.0 decision (131 stable, 31 preview, 17 gap) with backends and the test that covers it; top gaps: modules and xo.lock (0086), std/time (Proposed API in std.md 13), release build semantics, newtype unwrap, OpenApi and Proto derives; `xo build --ci` added, core 10.6 code registry under test, Proposed decisions 0090 to 0093 for Appendix B. Second pass (same day, section at the end): 169 stable, 2 preview, 14 gap of 185 under decision 0089 (native gaps no longer demote), new tests for the untested claims, `x.debug()` fixed.
- [2026-10-10 spec audit delta](results/spec-audit-2026-10-10.md): the v0.8.0 and v0.9.0 CHANGELOG items scored with their tests (180 stable, 1 preview, 10 gap of 191); fault_defer_chain matches want.txt on Go, LLVM, and arm64; 1.0 readiness ranked (real std/sql drivers, SBOM first) and what native lacks to leave preview (test_logs, test_once, test_shutdown report parity, frames outside main, crash bundles, record and replay).
- [2026-10-11 spec audit, fifth pass](results/spec-audit-2026-10-11.md): the 0.10 rows (decisions 0131, 0133, 0136 to 0143, 0155, 0156, 0160 to 0165) scored with their tests and native column (207 stable, 1 preview, 11 gap of 219; new gap: log records tagged with the dependency); gaps to close or cut for the owner; `--stamp` added to the 10.1 table, the stale std/time note removed, `TestRecordReplayStamp` and wider native test lists.
- [2026-10-09 hybrid builds](results/go-asm-hybrid-2026-10-09.md): `xo build --hybrid` emits eligible loops (no allocation, scalars and Lists of flat structs) as Go assembler from the arm64 backend inside Go backend builds; nbody 0.69x, collatz 0.74x, interp_flat 0.78x, sort 0.85x of the Go backend, within 3 to 8 percent of native; nothing in the HTTP benchmark qualifies, recursive fib is 1.49x slower; kept as an option (decision 0084).
- [2026-10-10 hybrid preemption](results/hybrid-preemption-2026-10-10.md): a hybrid loop held a GC stop the world for its whole run (11.6 s behind an 11.5 s loop); back edge polls of g.stackguard0 bring it to 0.058 ms at 0 to 7 percent cost (median 2); chunking from Go is cheaper on sieve (1.006x) but bounds nothing in general (decision 0131).
- [2026-10-09 binary size and build cache](results/binary-size-2026-10-09.md): hello world on the Go backend 7.9 to 3.6 MB, Go cache growth per program 34 to 2.9 MB (notes-service 54 to 6.9 MB), compile RSS 286 to 123 MB; Go compiled every method of every runtime generic instantiation in each package; runtime generics now keep protocol methods only, binaries drop DWARF, net/http only with std/http (decision 0098); `bench/size/measure.sh`.
- [2026-10-10 native runtime](results/native-runtime-2026-10-10.md): native HTTPS full handshakes 9.7k to 15k req/s (Go backend 17k), CPU per request 763 to 389 us, resumed 18.3k (Go 18.8k) with server session tickets (decision 0117); P-256 signing is the remaining gap.
- [2026-10-11 prebuilt runtime](results/prebuilt-runtime-2026-10-11.md): the first arm64 build with cold caches takes 0.13 s instead of 1.94 s (darwin) and 0.07 s instead of 3.21 s (linux/arm64) with a release's prebuilt C runtime, an HTTPS server 0.28 s instead of 4.27 s (decision 0157); the linux pack cross compiled with zig cc links with gcc 10 on glibc 2.31; packs and release archives reproducible byte for byte.
- [2026-10-10 HTTP/2](results/http2-2026-10-10.md): HTTP/2 over TLS on every backend (decision 0121), 32 workers as streams of one connection: native 87.8k req/s at 37 us CPU each, Go backend 58.2k at 51 us; over HTTP/1.1 with 32 TLS connections the same session measured 108k native and 133k Go.

## Other models: `bench/harness/`

`bench/harness/` packages stage 3 and the planted bug benchmark so they can
be rerun with any model (GPT, Gemini, local) outside Claude Code, and scored
the same way: one prompt per task and arm (Xo, plain Go, strict Go) and per
planted bug author set, an adapter for any OpenAI compatible endpoint with
the same write then iterate loop, and `score.py`, which reruns every cell
with the current toolchain and prints the tables of the three arm and
planted results. A full run is about 0.9M input and 0.4M output tokens.
See [harness/README.md](harness/README.md).

Results: [2026-10-09 Codex](results/codex-2026-10-09.md): Xo still prevents or catches 22 to 27 of 30 planted safety bugs (strict Go 17, plain Go 12); Codex needed 20 Xo runs vs 10 Go, all first try failures at the checker from other-language habits.
Rerun [2026-10-09 Codex Xo r2](results/codex-xo-r2-2026-10-09.md): Codex Xo arm rerun on v0.7.0: 4/10 first try (target 7), the remaining failures traced to XO0175 recovery, an unapplied XO0162 fix, an XO0601 bug on std error Display, XO0402, and no way to test import direction. After the fixes (same section): the first drafts pass 5/10 as is and 9/10 after `xo fix` (were 4/10 and 6/10); decision 0095 proposes import direction rules.
Third run [2026-10-09 Codex Xo r3](results/codex-xo-r3-2026-10-09.md): v0.8.0, first try 6/10 (low effort) and 7/10, 8/10 after fix (high); remaining failures were a `select` code generator bug, an unsound `?` fix in non-failing closures, and two missing help notes, all fixed.

- [2026-10-09 habit diagnostics](results/habit-diagnostics-2026-10-09.md): the checker names habits from Go, Rust, TypeScript, Python, Swift, and Kotlin with the Xo form and a fix when unambiguous (decision 0087); on Codex's 7 reconstructed failing first runs `xo fix` now makes 5 green (0 before), and 139 of 149 habit cases go green; the Codex rerun is the next experiment. Tools in [habits/](habits/).
- [2026-10-08 harness validation](results/harness-validation-2026-10-08.md): scoring the stored Claude outputs reproduces every published runs to green, line, and test count and every planted bug result (Go 9 and 13 of 30, Xo 29 and 23 of 30); with today's toolchain Xo 03 fails its contract fuzz (the known `backoff` overflow) and Xo 06 fails XO0410 (a spec 0.6 rule).
- [results/test-speed-2026-10-09.md](results/test-speed-2026-10-09.md): build reuse across test modes, native C runtime object cache, parallel tests: slow packages 1.6x to 3.1x faster (arm64 177 to 57 s).
Fourth run [2026-10-10 Codex Xo r4](results/codex-xo-r4-2026-10-10.md): fresh answers at fixed settings: 6/10 first try (7 after fix) at both efforts, against 8 and 9 for rescored r3; run to run variance plus four new habits (while let, Bytes iteration, leading ||, if without else) and one XO0177 misfire.
- [results/realtime-2026-10-10.md](results/realtime-2026-10-10.md): real-time mode (decision 0133) on examples/realtime-control: swap tick output latency 0.17 to 0.21 us real-time against 30 to 114 us unmarked (frees deferred past the output), worst output over 200,000 ticks 1.2 to 7.8 us against 39 to 114 us; the allocating version's median tick is 60 times slower.
- [2026-10-11 supply-chain hardening](results/supply-chain-2026-10-11.md): Backstabber's Knife Collection attack classes replicated as Xo dependencies (decision 0156); 10 of 15 classes blocked (fetch 1, check 7, lock 1, runtime 1), the 5 not blocked being out-of-scope logic/DoS (2), default-posture choices a ceiling or attenuation fixes (2), and a wrong human vouch (1); caveats on comparability with Flix 48/51.
