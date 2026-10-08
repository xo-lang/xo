# P1 code generation: running the stage 2 Xo solutions (2026-10-07)

First run of the eight stage 2 solutions (`bench/xo/v0.4/`, written by
benchmark subjects against the checker only) through the new code generator
(`internal/gen`) and runtime (`std/rt`): `xo build` and `xo test`. The
solutions were not modified. Spec: core draft 0.5, std draft 0.4.

## Results per task

All runs on darwin/arm64, Go 1.26.1. "Tests" is the final state; the
failures seen on the way are classified below.

| Task | `xo build` | Written tests | Contract fuzz (core 9.1) | `--race` | Cross build (7 targets) |
|---|---|---|---|---|---|
| 01 CRUD API | ok | 8/8 | 2/2 (validate_name, validate_email) | no races | not run |
| 02 Worker pool | ok | 6/6 | none fuzzable (generic) | no races | not run |
| 03 Retry client | library (no `main`) | 14/14 | 1/2: `backoff` faults (see below) | no races | not run |
| 04 File sync | ok | 12/12 | none (no contracts) | no races | not run |
| 05 Rate limiter | ok | 8/8 | 1/1 (Bucket.retry_after_secs) | no races | not run |
| 06 Job queue | ok | 11/11 | 1/1 (new_queue) | no races | not run |
| 07 Cache | ok | 11/11 | none fuzzable (function parameter) | no races | not run |
| 08 Notes service | ok (`server`) | 33/33 with `--real` (32 + 1 skipped without) | 3/3 (search, validate_title, validate_tags) | no races | ok |

Written tests: 103 of 103 pass with `--real` (without it, 102 pass and the
`uses fs.read` test is skipped). The
`--race` and cross build columns were run with the written tests; contract
fuzzing was added afterwards (it runs inside `xo test`).

The skipped test in 08 (`archtest`, "project modules respect the dependency
rules") declares `uses fs.read`; core 9.2 gives such a test real
capabilities only under `xo test --real`, so without the flag it is skipped
and reported as such.

Smoke test of a real server: the 01 binary served `POST /users`, `GET
/users/1`, a 400 for `/users/abc` (`{id:Int}` segment), a 404 for an
unknown path, and stopped with status 0 on SIGINT (core 7.5).

## Failures on the way, classified

| Task | Symptom (first run) | Class | Resolution |
|---|---|---|---|
| 02 | generated Go did not build: `expect(a) == b` as the tail of a test block was used as a value | codegen bug | a `()` tail in a `()` frame is evaluated for effect, then `()` is returned |
| 06 | 9 of 11 tests failed: leases never expired, acks did nothing | codegen bug | the receiver of a `var` method was spilled into a temporary when a later argument needed statements, so `st.jobs.put(...)` mutated a copy; places are never spilled now (gotcha `codegen-var-receiver-spill.md`, golden `var_receiver_order`) |
| 08 | export test saw an empty body | runtime gap | `Router.call` did not collect a body produced by `http.stream`; in process dispatch now collects it |
| 08 | archtest failed: "xo.mod not found" | runtime gap (test semantics) | the test declares `uses fs.read`; real capabilities in tests were not implemented. Now: skipped without `--real`, real read only filesystem with `--real`; passes |

| 03 | contract fuzz of `backoff` faults, shrunk to `policy = RetryPolicy{max_attempts: 1, base_delay: -1ms, max_delay: 3ms, jitter: 0.0}`, `attempt = 63`: `integer overflow: -8796093022208000000 * 2 does not fit Duration at retry.xo:139:9` | real bug in the solution (latent) | `backoff` doubles `d` until `d >= max_delay`; with negative delays that never happens and the doubling overflows. `RetryPolicy`'s invariant checks `max_attempts` and `jitter` but admits negative delays, so the function faults on inputs its contract allows. Fix in the solution: add `base_delay >= 0ms && max_delay >= 0ms` to the invariant. Not changed (bench/xo is read only) |

The written tests found no bug in a solution; contract fuzzing found one
latent bug (above).

## Extra corpus: the blind paper solutions (bench/paper/v0.3)

The eight spec 0.3 paper solutions (written from the spec alone, never
checked) were also run, unmodified where they check under spec 0.5:

| Task | Under spec 0.5 | `xo test` |
|---|---|---|
| 01 CRUD API | checks | 5 passed |
| 02 Worker pool | checks | 4 passed |
| 03 Retry client | checks | 11 passed |
| 04 File sync | XO0206, XO0601 | 8 passed after the checker's own fixes on a scratch copy |
| 05 Rate limiter | checks | 7 passed |
| 06 Job queue | XO0206 | 9 passed after the checker's fix on a scratch copy |
| 07 Cache | checks | 10 passed |
| 08 Notes service | XO0206 | 26 passed after the checker's fix on a scratch copy |

They found two more codegen bugs (fixed, with golden tests): a closure
expected to return `()` whose tail is a value (07: `fn() { hits.add(1) }`;
golden `unit_discard`), and `?` on an optional inside a closure returning
an optional (06; golden `closure_option_try`). Their archtest assumed tests
run in the module's directory, so the runner now changes into each test's
module directory, as `go test` does per package (the v0.4 archtest accepts
either).

## Do the tests catch what the checker misses?

`bench/xo/v0.4/mutations.jsonl` lists 381 single line mutations of the
solutions; `xo check` catches 341. The 40 it misses were applied again and
run through `xo test --real`:

| Mutation | Checker misses | Caught by the solution's tests | Survived |
|---|---|---|---|
| drop_store_back (lost update) | 13 | 12 | 1 |
| flip_compare (`<` to `<=`) | 14 | 5 | 9 |
| drop_match_arm | 5 | 3 | 2 |
| some_to_raw (`None` to `0`) | 5 | 1 | 4 |
| drop_uses | 3 | 0 | 3 (no behavior change by construction) |
| total | 40 | 21 | 19 |

Lost updates, the class the checker is weakest on (XO0520), are almost all
caught at run time. Survivors are boundary flips the tests do not probe
(TTL `<` vs `<=`, priority ties), `lease_token: 0` where the tests never
read the token after an ack, a deleted arm whose case the tests do not
reach, and three deleted `uses` clauses (effects have no run time
behavior; they were legal because the functions are not pub).

Survivors: 01 `main.xo:23`; 03 `retry.xo:55`, `retry.xo:57`, `retry.xo:123`;
04 `main.xo:108` (uses); 06 `queue_test.xo:150`, `worker.xo:76` and `:90`
(uses), `queue.xo:203`, `:254`, `:274`, `:188`, `:218`; 07 `cache.xo:181`,
`:186`, `:196`; 08 `api/api.xo:149`, `domain/domain_test.xo:151`,
`store/store.xo:78`.

## What was built

- `internal/gen`: lowering from the checker's typed AST to Go with `//line
  file.xo:L:C` directives; `internal/build`: check, generate, write the
  build directory, `go build -trimpath` per target with `CGO_ENABLED=0`;
  `cmd/xo`: `xo build [--targets=...]`, `xo run`, `xo test`.
- `std/rt` (moved out of `wip/rt`, which is deleted): checked arithmetic,
  persistent List/Map/Set, Option/Result with error frames, faults and
  crash bundles, scopes and tasks with sticky cancellation, channels and
  `select`, Mutex/Atomic/Semaphore, `task.*`, real capabilities, http, json,
  log, the test runner and fakes, property test generators.
- Tests: 50 golden programs in `internal/gen/testdata/golden` (expected exit
  status, stdout, faults, error frames), the race detector over the
  concurrent ones, a cross compile test (linux/amd64, darwin/arm64,
  windows/amd64, wasip1/wasm), a stack trace test (fault frames name
  `main.xo:4`), a SIGINT test, a holes test, and `TestStdTypes`.

The lowering conventions are in `knowledgebase/infrastructure/toolchain.md`
("Code generation", "Runtime"). New gotchas: `codegen-var-receiver-spill`,
`go-instantiation-cycle`, `go-enotempty-is-errexist`,
`virtual-clock-wake-before-block`, `canceled-sleep-unwinds`,
`spawn-background-never-result`, `go-constant-float-division`,
`recursive-struct-option`.

## Lowering choices worth knowing

1. Function values always return `rt.Result[R, E]` (E is `rt.Never` when
   they cannot fail). The Go form of a value never depends on whether a
   generic error parameter became Never, which removes most coercions at
   generic boundaries; the cost is a Result per call of a closure.
2. Xo generics are Go generics with `any` constraints; Eq, Hash, Ord,
   Display, and Debug dispatch on the dynamic type (generated `Xo*`
   methods). Monomorphization was not needed.
3. Enums are `struct{ Tag int; P any }` with a payload struct per variant:
   recursive enums work, equality and hashing are generated per enum.
4. Structs that contain themselves by value (through Option, Result,
   tuples) store that field in `rt.Box[T]`, an immutable shared pointer.
5. Blocks with `defer` become `func() int { ... }()` so Go's defer is
   block scoped; control flow crosses it as codes. `scope` is the same with
   `Close` deferred, which applies the core 7.1 exit rules.
6. Persistent List: a shared backing array with a claimed length;
   `push` appends in place when the list ends at the claimed length
   (compare and swap), else copies; older values never see new elements.
7. The test clock is a world per test: it advances when every task is
   blocked, and every waker counts the woken task as running before it
   signals. Channels, mutexes, semaphores, and `select` therefore have their
   own wait queues instead of Go channel operations.
8. Interface conversion: direct when the concrete Go methods match the
   interface's Go signatures; otherwise a generated adapter type (for
   example a method that cannot fail behind an interface method that can).
9. Integer operations and conversions call rt functions with the source
   position; faults inside rt functions without one use the innermost
   `.xo` frame (line only).

## Spec ambiguities and the behavior chosen

| # | Question | Choice |
|---|---|---|
| 1 | Float division by zero (core 8 lists faults only for integers) | IEEE 754: infinity or NaN, no fault |
| 2 | Sign of `%` | follows the dividend (truncated division, as Go) |
| 3 | Duration display ("1.5s", "250ms" in std.md 2.6) | Go's format: `1.5s`, `250ms`, `2m0s` |
| 4 | How `main`'s error is printed (core 11.1 example shows the Debug form) | `error: <Display>` then the frames, innermost first; capability parameters (os, clock, ...) are left out of frames |
| 5 | Where crash bundles go (core 11.2) | `$XO_CRASH_DIR`, else the system temporary directory; the path is printed |
| 6 | Does a failed `expect` stop the test | yes, the first failure ends the test and prints both values |
| 7 | A test with `uses` run without `--real` | skipped, with the reason |
| 8 | `os.proc.exit` in a test outside `run_main` ("the test block continues", std.md 8.4) | unwinds to the runner; the test ends and counts as passed (continuing would need a recovery point per statement) |
| 9 | Property tests (core 9.2) | 100 cases (`-cases`), each in a fresh test world; the failing input is shrunk and printed; no regression files yet |
| 10 | Route precedence when several patterns match | most literal segments, then registration order (router and FakeNet) |
| 11 | Bodies of 404, 405, and bad `{x:Int}` responses | JSON `{"error": "not found"}`, `"method not allowed"`, "path segment `x` must be an Int" (400) |
| 12 | `http.new_request` with a path only URL | allowed (for `Router.call`); the client requires an absolute URL and returns `InvalidUrl` otherwise |
| 13 | Key order of a decoded `Map[Str, V]` | the order in the JSON text |
| 14 | JSON form of a `from` variant | `{"Variant": value}`; NaN and infinities encode as null |
| 15 | `Mutex.with` and `Semaphore.with` as cancellation points | only while waiting (core 7.2 says "Mutex.with while waiting") |
| 16 | `t.await` on a task that was canceled (not failed) | unwinds if the awaiting task is canceled, else faults "awaited a task that was canceled" |
| 17 | XO-F005 (shielded defer over 5s) | checked when the deferred code returns, real time only |
| 18 | Every task of a test blocked, no timer pending | the test fails with "deadlock" instead of hanging (XO-F001 is P2) |
| 19 | Log record format | text: `<RFC 3339> LEVEL msg k=v`; JSON: one object; `log.configure(out)` writes to `out`'s standard error |
| 20 | `match target.os` "resolved at compile time" | evaluated at run time; only the build target's value can occur, all arms are compiled (there is no `std/sys` yet) |
| 21 | Shifts | a shift amount outside the type's width faults, and so does a left shift that loses bits |
| 22 | `--seed` (core 7.4 is P2) | seeds `os.rand` and the property test generators; scheduling is Go's |
| 23 | A generic interface method (`Rand.shuffle[T]`) | not a Go interface method; called as `rt.Rand_shuffle[T](tk, r, xs)`. User interfaces with generic methods are not supported |
| 24 | Where an invariant fault points | the construction site, or the `var` method that broke it |
| 25 | `Fs.case_sensitive()` of the real filesystem | false on darwin and windows, true elsewhere (not probed) |
| 26 | Working directory of tests (core 9.2 is silent) | each test runs in its module's directory, as `go test` does per package |

## Contract fuzzing

`xo test` runs every function with `requires` or `ensures` in the root
modules on 100 generated inputs (core 9.1): inputs that fail a `requires`
(or a `requires ... else`) are skipped, capability parameters get the
test's fakes, and any fault (a failed `ensures`, an overflow, an index out
of range) fails the case, reported with the generated input shrunk to a
small one. Numbers are kept small (|n| <= 1000) so loops bound by an input
end.

Shrinking (property tests and contract fuzzing): every random choice of
the generators is recorded; a failing case is shrunk by deleting runs of
choices and binary searching each choice toward 0 (choice 0 is the
simplest value: 0, the empty string, the first variant, None), keeping an
edit only when the test still fails and the choices got smaller in
shortlex order. Functions with
type parameters, function values, interfaces, handles, or std types among
their inputs are not fuzzed. Golden test: `test_contracts`.

## Performance (first look)

Micro benchmark, darwin/arm64 (M4 Pro), generated code against the same
loops written in Go by hand:

| Workload | Go | Xo (generated) | Ratio |
|---|---|---|---|
| 30M iterations of `acc = (acc + i * 3) % 1000003` | 90 ms | 124 to 138 ms | 1.4 to 1.5x |
| 3M `List.push`, then a sum | 7.6 ms | 14 ms | 1.8x |
| 3M `Map.put` (50k keys) then 3M `get` | 50 ms | 3.59 s, then 2.24 s after the fixes below | 72x, then 45x |
| 300k `map` + `fold` over a 10 element list with closures | 2.2 ms | 18 ms | 8x |

Map profile: `put` cost 1.1 µs, 4.2 KB, and 10 allocations; `get` 55 ns
with 2 allocations. Fixed during the run: `Eq`, `Hash`, and `Cmp` boxed
their arguments into `any` (now type switched without allocation: `get`
22 ns, 0 allocations), and trie slots held keys and values inline (now
two pointers per slot: `put` 720 ns, 1.7 KB). The remaining cost is the
path copying of a persistent map under an update heavy loop; making it
cheap needs transient (owned) nodes, which needs to know when a map value
is shared (next step 2).

## Known gaps

- No regression files (`testdata/regressions/<test>.xo`, core 9.2) for
  shrunk failures.
- An interface conversion inside generic code whose concrete type needs an
  adapter faults at run time (adapters are generated only for concrete
  types).
- User interface methods with their own type parameters.
- Deterministic scheduling, record and replay (P2).
- No performance measurement of checked arithmetic or persistent
  collections yet.
- `xo build --json` reports binaries and diagnostics but not the Go
  compiler's output when generation is wrong.

## Next steps

1. Regression files (`testdata/regressions/<test>.xo`, core 9.2): render
   shrunk inputs as Xo literals and load them in `xo test`.
2. Transient Map nodes: let `put` mutate trie nodes in place while the map
   value is not shared (the generator marks the points where a `var`
   binding's value is copied out: assignment, argument, capture, store), so
   an update heavy loop stops copying paths. Then benchmark against the Go
   baseline solutions in `bench/go/`.
3. Generic adapters (dictionary passing) so interface conversions inside
   generic code work for every implementing type.
