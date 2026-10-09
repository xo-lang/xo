# Spec stability audit (2026-10-09)

Input for the owner's 1.0 decision (decision 0089). Nothing here declares,
tags, or freezes 1.0 or the 2026 edition.

Scope: every section of `spec/core.md` (draft 0.8) and `spec/std.md`
against the implementation (`internal/check`, `internal/parser`,
`internal/gen`, `internal/ssa`, `std/rt`, `std/llrt`, the LLVM and arm64
backends) and the tests. Toolchain at `de24fd9` plus this audit's
commits, darwin/arm64 (M4 Pro), Go 1.26.1.

## Method

- Read both spec files section by section and listed every claim that a
  program or a user can observe.
- Coverage: a grep driven index of every test corpus (Go backend goldens
  `internal/gen/testdata/golden`, native goldens
  `internal/llvm/testdata/golden`, which `internal/arm64` also runs,
  `internal/arm64/testdata/run`, the checker goldens and rule tables,
  parser, lexer, vet, fix, cmd/xo, std/rt tests, examples). Cited as
  `gen:<golden>`, `llvm:<golden>`, `check:<file>`, or a test name.
- Native support: `xo check --no-gc` on all 99 Go backend goldens; every
  `XO1201` names the construct the native backends (llvm, arm64, `--no-gc`,
  which share one SSA builder) cannot build yet, or ever (`use go`).
- Probes: about 45 small programs for claims without a clear test, each run
  with `xo run` (Go backend) and `xo run --backend=arm64`.
- The std surface: a script compared every signature in std.md's code
  blocks with the declarations in `std/*.xo`, and every std enum and struct
  with its spec form.

Status meanings:

- **stable**: implemented as specified on the Go backend, covered by a
  test, no disagreement left. Native gaps the spec itself states (no TLS,
  no frames) do not demote a feature.
- **preview**: implemented, but the spec claim has no test, or a backend
  the spec does not exclude cannot build it (`XO1201` planned), or a
  Proposed decision may still change it.
- **gap**: specified but not implemented, or not usable as specified.

## Summary

| Status | Features |
|---|---:|
| stable | 131 |
| preview | 31 |
| gap | 17 |
| total | 179 |

Backends: the Go backend implements every stable and preview feature.
The native backends (llvm, arm64, `--no-gc`) build the core language, HTTP,
JSON, SQL (in memory), crypto, tasks, and contracts; their largest holes
are interface types (dynamic dispatch), list patterns, newtypes,
`Float32`, `Secret`, `target`, `Buf`, most `Fs` methods, standard input,
`std/path`, `Once`, `Semaphore`, `map_concurrent`, and TLS (all `XO1201`
planned) and `use go` (impossible).

## Gaps ranked by how much they block 1.0

Ranked by: would closing it later force a breaking change (worst), does
a typical backend service need it, and does the spec promise it today.

0. **The four Appendix B questions** are still open; Proposed decisions
   0090 to 0093 (below) await the owner. They fix syntax (methods),
   observable output (Map order), and semantics (error frames, FFI and
   replay) that 1.0 would freeze.
1. **Modules: `require` and `xo.lock`** (core 4.2, 4.3; decision 0086,
   Accepted, not implemented). `require` lines are ignored and nothing
   writes `xo.lock`. Module identity, version selection, and checked
   semver are the substance of a compatibility promise; without them no
   one else can depend on an Xo module.
2. **`std/time`** (core 14). The module is empty; services need zones,
   formatting, and parsing beyond RFC 3339 UTC. A Proposed API is now in
   std.md 13 for the owner to decide (open points listed there).
3. **Release builds and error frame semantics** (core 8, 11.1). There is
   no release mode, so `requires(debug)`, `ensures(debug)`, and
   `--error-args` mean nothing yet; decision 0092 proposes the rule. The
   meaning of debug contracts in release builds must be fixed before 1.0
   because turning checks off later changes behavior.
4. **Newtypes cannot be unwrapped** (core 2.8). `type Email struct(Str)`
   constructs and compares, but neither `e.0` nor a pattern `Email(s)`
   reads the value, so newtypes are unusable beyond equality. Choosing the
   syntax is a language decision (recommendation: `e.0`, matching tuple
   fields, plus the pattern `Email(s)`).
5. **`derive OpenApi` and `derive Proto`** (core 2.9). Promised as schema
   and codec generators; the names are accepted but no builtin type
   implements them and nothing is generated. Either build them or drop
   them from the 1.0 text and keep the names reserved.
6. **FFI and replay of foreign calls** (core 6.1 `ffi`, 11.3; decision
   0093). The `ffi` effect has no meaning; Go calls are re executed in a
   replay, so replays of programs that do I/O through Go diverge.
7. **Native backend parity** for features the spec does not exclude:
   interface types, list patterns, newtypes, `Float32`, `Secret`, `target`,
   `Buf`, Fs, stdin, `std/path`, Once, Semaphore, `map_concurrent`, Set
   algebra, TLS. Not a language blocker (each is a precise `XO1201`), but
   `--no-gc` is not a general purpose target until these land.
8. **`xo why`, `xo debug` (`--json`, `--dap`, `--replay`)** (core 10.1,
   11.3, 11.5; [P2]). Tooling only, additive.
9. **Observability: spans, request ids in log records, OpenTelemetry**
   (core 11.6, std.md 6). Additive, but log record fields are observable
   output.
10. **SBOM and a reproducible build test** (core 12.1). Release
    engineering.
11. **`std/sys/<os>` and `XO1001`** (core 12.2). Additive; the code is
    reserved.
12. **Real `std/sql` drivers** (std.md 10). Ecosystem, depends on 1.
13. **Doc comment examples run by `xo test`** (core 1.2, [P2]). Additive.
14. **User written `arena scope`** (core 5.5, [P2]). Additive.

Claims that were true but untested (preview until a test exists):
`Float32` (test added), single field positional variant construction,
`T ! Never`, user `-> Never` functions, `XO-F005`, `xo test --real`,
`Fs.cwd`, `Fs.case_sensitive`, real `Proc.run`, `Stdio.write`, raw TCP on
the Go backend, `Secret` redaction beyond interpolation and replay,
`target` at run time, reproducible builds.

## Disagreements fixed in this audit

| Where | Spec said | Code did | Fix |
|---|---|---|---|
| core 10.1, 10.5, 11.4 | `xo build --ci` fails on unformatted files and rejects `dbg` | no `--ci` flag (vet's message even pointed to it) | code: `--ci` added (`cmd/xo/build.go`), test `TestBuildCI` |
| core 10.1 | every command accepts `--json`; JSON is the default off a terminal for check, build, test, query | `run`, `dump`, `replay`, `serve`, `version` have no `--json`; only `query` defaults to JSON | spec: lists the commands that take `--json` |
| core 10.1 | `xo test --fuzz=10s` | the flag is `--cases=N` | spec |
| core 10.1 | `xo why`, `xo debug` listed as commands | do not exist | spec: marked [P2], not implemented |
| core 10.2 | 31 codes the toolchain emits were nowhere in the spec | | spec: 10.6 lists all 77 codes and 3 fault codes; `TestSpecListsEveryCode`, `TestSpecListsEveryFaultCode` keep it so |
| core 11.1 | the trace ends with `in task ... (scope ...)` | no task line is printed | spec: example fixed, task line [later] |
| core 11.1, 8 | release builds, `--error-args` | no release mode | spec: [later] |
| core 11.6, std.md 6 | every log record carries the span and request id | records carry the time only | spec: [later] |
| core 12.1 | each build writes an SBOM | none written | spec: [later] |
| core 12.2 | `XO1001` for `std/sys` misuse | no `std/sys`, code never emitted | spec: reserved |
| core 2.9 | OpenApi and Proto derives generate schemas and codecs | nothing generated | spec: [later] |
| core 2.8 | newtype construction only | no way to read the value | spec: gap stated |
| core 3.5 | shielded defer over 5 s faults | real runs only, checked at return | spec: clarified |
| core 4.2, 4.3 | `require` and `xo.lock` (P1) | ignored, never written | spec: [P1, not implemented yet] |
| core 14 | module table | `std/path` and `std/go` missing; `std/time` empty | spec: rows added, std/time marked |
| std.md header | draft 0.6 | core is draft 0.8 | spec: draft 0.8 (one number, decision 0089) |
| std.md 2.7 | | `Buf.len` exists | spec: added |
| std.md | no `std/go` section | `go.Error` with `display`, `message`, `go_type` | spec: section 12 |

## Diagnostic codes (task 4)

- Registry `internal/diag/codes.go`: 77 codes. Before this audit the spec
  mentioned 46 of them; 31 (most lexer and parser codes, XO0202,
  XO0302 to XO0304, XO0405 to XO0407, XO0603 to XO0607, XO0702, XO0803)
  were undocumented. Now core 10.6 lists all 77 with severity and the
  component that reports them, and two tests fail if the table and the
  registry (or the runtimes' `XO-F` codes) drift.
- Documented but dead: `XO1001` only (no `std/sys` module); now marked
  reserved. Every other code is referenced by the emitting code
  (`internal/lexer`, `internal/parser`, `internal/check`, `internal/vet`,
  `internal/ssa`).
- Fault codes: the runtimes raise exactly `XO-F001`, `XO-F004`, `XO-F005`,
  all now listed.
- `xo explain`: covers every code (`internal/diag` `TestExplanationsComplete`
  requires a title, detail, example, and fix for each constant and no
  extras).

## Proposed decisions (Appendix B 1, 2, 3, 5)

All are Status: Proposed, awaiting the owner.

- [0090](../../knowledgebase/decisions/0090-method-syntax-receivers.md):
  keep Go style receivers; `impl` blocks carry no meaning with structural
  interfaces, and the `XO0177` habit fix already rewrites them.
- [0091](../../knowledgebase/decisions/0091-map-iteration-order.md): keep
  insertion order. Measured (10k Str keys): insertion ordered iteration
  18 us vs Go's unordered map 48 us; after heavy churn 45 us; sorted
  iteration 797 us; put cost equal to Go's; the order field costs 0 bytes
  (padding).
- [0092](../../knowledgebase/decisions/0092-error-frame-capture-cost.md):
  frames on in every build, captured lazily. Measured: an error through 5
  `?` frames costs 842 to 874 ns end to end (24 ns without frames), 2.6 us
  with a 90 byte parameter; eager `Debug` formatting is 90 to 99 percent of
  it; lazy capture 214 ns regardless of size; the success path is free.
- [0093](../../knowledgebase/decisions/0093-ffi-and-record-replay.md):
  `use go` on the Go backend, `use c` (effect `ffi`) on native backends;
  foreign calls recorded at the boundary and replayed by value, handles re
  run and flagged.

Benchmarks: `go test ./std/rt -run '^$' -bench 'Order|Frames' -benchmem`
(`std/rt/audit_bench_test.go`).

## std/time (task 5)

std.md 13 now holds a Proposed API, not implemented: `Zone` values (IANA
names from embedded tzdata, pure; the machine's zone only through `Env`),
civil types `Date`, `ClockTime`, `Civil`, `Weekday` with explicit
daylight saving rules (`Ambiguity`), and layouts with named fields
(`"{yyyy}-{MM}-{dd}"`) for `format` and `parse` returning `ParseErr` with a
position. Open points for the owner are listed in std.md 13.4.

## Next steps

1. Owner: accept, change, or reject 0090 to 0093; then remove the questions
   from Appendix B in the same change.
2. Newtype unwrap: write an ADR choosing `e.0` and the `Email(s)` pattern,
   implement in `internal/check/expr.go` (selector on `KNewtype`) and
   `internal/check/pattern.go`, then `internal/gen` and `internal/ssa`;
   golden `consts_newtypes` reads the value back.
3. std/time: owner decides std.md 13.4; then implement on the Go backend
   over Go's `time` with `time/tzdata`, goldens for DST gaps and overlaps.
4. Modules (0086): start with `require` resolution from a local cache and
   `xo.lock` writing, test with two modules in `cmd/xo` testdata.
5. Tests for the untested claims above, starting with `xo test --real`
   (`cmd/xo`), `XO-F005` (a std/rt test with a fake monotonic clock), and
   a reproducible build test (`internal/build`: build twice, compare
   bytes).
6. Release builds: after 0092, add `--release` to `xo build` with lazy
   frames.

## Feature table

Backends: `go` is the default Go backend; `llvm, arm64, no-gc` are the
native backends (one SSA builder; llvm goldens also run on arm64);
"native: XO1201 planned" means `--no-gc`, llvm, and arm64 reject it with a
precise diagnostic; "checker (all)" is a compile time rule that holds on
every backend; "toolchain" is a command.

| Section | Feature | Status | Backends | Tests | Notes |
|---|---|---|---|---|---|
| 1.1 | Module = directory, no module declaration, per file `use` | stable | checker (all) | check:rules_test, gen goldens with lib/ modules (mono_generics, iface_adapters) |  |
| 1.1 | `_test.xo` files hold only tests and helpers | stable | go, llvm, arm64, no-gc | gen:test_* goldens, cmd/xo nogc tests, examples/notes-service |  |
| 1.2 | Line and doc comments; no block comments (habit XO0177 for `/* */`) | stable | checker (all) | lexer tests, check:habits_test, fix:habits |  |
| 1.2 | Doc comment examples compiled and run by `xo test` | gap | none | none | [P2]; `xo doc` renders them only |
| 1.3 | Newline termination, Go rule; trailing commas; `} else` on one line (XO0161, XO0162) | stable | checker (all) | parser tests, spec_test (every spec block parses) |  |
| 1.3 | Leading `.` and declaration clauses continue the previous line | stable | checker (all) | check:accept_test, parser tests | no runtime golden splits a chain |
| 1.4 | 28 keywords, predeclared names, contextual words | stable | checker (all) | token/lexer tests, check:rules_test (XO0603 redefinition) | verified: 28 in internal/token |
| 1.4 | Unparenthesized struct literal in a condition is XO0163 | stable | checker (all) | parser tests |  |
| 1.5 | Enforced naming (XO0101), `pub` exports | stable | checker (all) | check golden naming, rules_test |  |
| 1.6 | Int literals (hex, octal, binary, `_`), contextual integer typing | stable | go, llvm, arm64, no-gc | lexer tests, gen:int_ops, llvm:native_sized_ints | probe: all forms print on go and arm64 |
| 1.6 | Rune literal as integer code point (decision 0079) | stable | go, llvm, arm64, no-gc | gen:rune_numbers, llvm:native_bytes |  |
| 1.6 | Str interpolation `${}`, literal braces, `\$` | stable | go, llvm, arm64, no-gc | gen:interpolation, llvm:interp, llvm:strings |  |
| 1.6 | Raw strings `r""`, `r#""#` | stable | go, llvm, arm64, no-gc | lexer tests, gen:json_codec, llvm:native_json |  |
| 1.6 | Triple quoted strings (indent removal) | stable | go, llvm, arm64, no-gc | lexer tests, gen:interpolation | probe ran on arm64; no native golden |
| 1.6 | Bytes literals `b"..."` | stable | go, llvm, arm64, no-gc | gen:bytes_builder, llvm:native_bytes |  |
| 1.6 | Duration literals; `1.5s` is XO0155 | stable | go, llvm, arm64, no-gc | lexer tests, gen:duration_time, llvm:clock_timers |  |
| 1.6 | List, Map, Set literals, `{}` is a Map | stable | go, llvm, arm64, no-gc | gen:collections, llvm:maps, llvm:lists |  |
| 1.6 | Escapes; `\xHH` above `\x7f` in Str is XO0153 | stable | checker (all) | lexer tests | probe confirmed |
| 2.1 | Bool, Int, sized ints with overflow faults | stable | go, llvm, arm64, no-gc | gen:arith_overflow, gen:int_ops, llvm:overflow_*, llvm:sized_overflow |  |
| 2.1 | Float and its total order for `<`, IEEE `==` (decision 0053) | stable | go, llvm, arm64, no-gc | gen:float_order, llvm:float_order |  |
| 2.1 | Float32 | preview | go; native: XO1201 planned | gen:conversions (added by this audit) | was untested; native: the type is XO1201 |
| 2.1 | Numeric conversions `Float(i)`, `Int(f)` (NaN faults), `Int32(x)`, `.try`, `parse` | stable | go, llvm, arm64, no-gc | gen:conversions, llvm:float_int_fault, llvm:native_sized_ints | native: `Int32.try`, `UInt8.try` are XO1201 planned |
| 2.1 | Rune and integer conversions, `Rune.try` (decision 0079) | stable | go, llvm, arm64, no-gc | gen:rune_numbers, llvm:rune_fault |  |
| 2.1 | Duration and Time operators | stable | go, llvm, arm64, no-gc | gen:duration_time, gen:test_fakes, llvm:clock_timers | probe: every operator in the table on go and arm64 |
| 2.1 | `Never` type: `fault`, `return`, `while {}`, user `-> Never` functions | stable | go, llvm, arm64, no-gc | gen:loops, gen:main_noos, llvm:user_fault | user `-> Never` function: probe only |
| 2.1 | `Secret[T]` redacted in interpolation | preview | go; native: XO1201 planned | gen:interpolation, gen:replay/effects | spec says redacted in every output path; only interpolation and replay recording are tested; native: the type is XO1201 |
| 2.1 | Tuples and unit | stable | go, llvm, arm64, no-gc | gen:patterns, llvm:tuples_consts |  |
| 2.2 | Structs: named construction, defaults, generic inference, `with` | stable | go, llvm, arm64, no-gc | gen:structs, gen:calls_defaults, llvm:structs, llvm:contract_with |  |
| 2.3 | Enums, qualified and bare variants, exhaustive match (XO0301) | stable | go, llvm, arm64, no-gc | gen:enums, llvm:enums, check golden match |  |
| 2.3 | Single field variant constructed positionally | preview | go, llvm, arm64, no-gc | check:rules_test (negative case only) | works in a probe; no positive test |
| 2.3 | Variants with a `Never` field may be omitted from a match | stable | checker (all) | check:rules07_test, gen:test_once |  |
| 2.4 | Interfaces, structural, as types (dynamic dispatch) and constraints | preview | go; native: XO1201 planned | gen:interfaces, gen:iface_adapters | native: interface types are XO1201 planned (llvm unsupported/interface) |
| 2.4 | `Never` satisfies every interface; derivable names as constraints | stable | checker (all) | check:accept_test | probe: `fn f() -> Display { fault(...) }` builds |
| 2.5 | Optionals, auto `Some`, `??`, `if let`, `if let e = opt` shorthand | stable | go, llvm, arm64, no-gc | gen:option_result, gen:patterns, llvm:option |  |
| 2.5 | `?` on Option in a function returning Option; XO0204 with fix | stable | go, llvm, arm64, no-gc | gen:closure_option_try, check:rules_test, llvm:closure_try |  |
| 2.6 | `!` error types, `! Error`, `T ! Never` is `T` | stable | go, llvm, arm64, no-gc | gen:error_conversion, gen:main_custom_err, llvm:result | `T ! Never`: probe only |
| 2.6 | Returning `T`, `Err(e)`, or a Result; XO0203 | stable | go, llvm, arm64, no-gc | gen:error_conversion, check golden errors |  |
| 2.6 | `?` conversions (same type, to Error via Display, `from` variants); XO0202 | stable | go, llvm, arm64, no-gc | gen:error_conversion, gen:enums, llvm:rc_failing | native: some error conversions XO1201 |
| 2.6 | `map_err`, mandatory handling XO0201, `let _ =` is XO0206, `ignore_err` | stable | go, llvm, arm64, no-gc | gen:option_result, check:rules04_test, check:planted_test |  |
| 2.7 | Generics, inference, explicit type args, XO0402 | stable | go, llvm, arm64, no-gc | gen:generics, gen:mono_generics, llvm:generics, check:rules_test |  |
| 2.8 | Type aliases | stable | go, llvm, arm64, no-gc | gen:consts_newtypes |  |
| 2.8 | Newtypes `type Email struct(Str)` | gap | go; native: XO1201 planned | gen:consts_newtypes | construction and `==` only: the underlying value cannot be read back (no `e.0`, no pattern); native: XO1201 |
| 2.9 | Automatic Debug, Eq, Hash; `derive Display`, `Ord`, `Json` | stable | go, llvm, arm64, no-gc | gen:enums, gen:json_codec, llvm:native_display, llvm:native_json |  |
| 2.9 | `derive SqlRow` | stable | go, llvm, arm64, no-gc | gen:sql_decode, llvm:native_sql, check golden derive_sqlrow |  |
| 2.9 | `derive OpenApi`, `derive Proto` | gap | none | none | accepted names, but no builtin type implements them and nothing is generated; marked [later] |
| 2.10 | Exactly three implicit conversions; `opt == v` | stable | checker (all) | check:accept_test, check:rules_test (XO0401) |  |
| 3.1 | `let`, `var`, `const` (top level and local), compile time initializers XO0408 | stable | go, llvm, arm64, no-gc | gen:consts_newtypes, llvm:tuples_consts |  |
| 3.1 | Unused bindings XO0110, `xo fix` removes them | stable | checker (all) | check:rules04_test, fix:fix_test |  |
| 3.1 | Shadowing in one block is XO0603 | stable | checker (all) | check golden naming |  |
| 3.2 | Block values, `while {}` as `Never` | stable | go, llvm, arm64, no-gc | gen:loops, llvm:loops |  |
| 3.2 | Named arguments, defaults, no variadics | stable | go, llvm, arm64, no-gc | gen:calls_defaults, llvm:enums | native: some calls with missing arguments XO1201 |
| 3.3 | Closures, inference, capture by value, XO0502 | stable | go, llvm, arm64, no-gc | gen:closures, llvm:closures, llvm:rc_closures, check:rules_test |  |
| 3.4 | Methods, `var` receivers (XO0501), same module only (XO0606) | stable | go, llvm, arm64, no-gc | gen:var_receiver_order, llvm:misc, check:rules_test | Appendix B 1 Proposed: decision 0090 |
| 3.5 | if, for over List, Map, ranges, channels, while, labeled loops | stable | go, llvm, arm64, no-gc | gen:loops, gen:chan_loops, llvm:loops, llvm:conc_channels |  |
| 3.5 | `is` patterns with `&&` and `\|\|` | stable | go, llvm, arm64, no-gc | gen:patterns, llvm:option |  |
| 3.5 | Compound assignment `+= -= *= /= %=` | stable | go, llvm, arm64, no-gc | llvm:arith, check:rules04_test |  |
| 3.5 | Block scoped `defer`, reverse order, on errors, faults, cancellation | stable | go, llvm, arm64, no-gc | gen:defer_blocks, llvm:defer_blocks, llvm:conc_defer |  |
| 3.5 | Shielded defer; over 5 s faults XO-F005 | preview | go | gen:cancellation (shielding); XO-F005: none | XO-F005 untested; checked at return, real runs only (spec clarified) |
| 3.5 | Patterns: literals, ranges, or, tuples, variants, struct, nested | stable | go, llvm, arm64, no-gc | gen:patterns, llvm:switch, llvm:enums |  |
| 3.5 | List patterns | preview | go; native: XO1201 planned | gen:patterns | native: XO1201 planned (llvm unsupported/list_pattern) |
| 3.5 | XO0305 `_` hiding own variants (decision 0080) | stable | checker (all) | check golden match, rules06_test |  |
| 3.6 | List and Bytes indexing (faults), `get` | stable | go, llvm, arm64, no-gc | gen:index_oob, llvm:index_oob, llvm:bytes_oob |  |
| 3.6 | Map index read is XO0410; `m[k] = v`; `get_or_fault` | stable | go, llvm, arm64, no-gc | check:rules06_test, gen:map_key_fault, llvm:map_key_fault | native: get_or_fault only for Str and Int keys |
| 3.6 | Str not indexable (XO0406); slicing List, Bytes, Str; mid UTF-8 faults | stable | go, llvm, arm64, no-gc | gen:strings, llvm:str_slice_fault |  |
| 3.7 | Holes `???`, XO0900, `xo build` rejects, `--holes` | stable | go | check golden holes, gen:holes, gen TestHoles |  |
| 4.1 | Imports, `as`, `-` to `_`, XO0602, cycles XO0604 | stable | checker (all) | check:rules_test | no golden imports `as` |
| 4.2 | `xo.mod` module and edition lines | stable | toolchain | check:edition_test |  |
| 4.2 | `gc` lines (decision 0065) | stable | go | gen:gc_config, check:gcconf_test | native has no GC |
| 4.2 | `require` of Xo modules (decision 0086) | gap | none | none | lines are ignored; no fetch, MVS, vendoring |
| 4.3 | `xo.lock` content hashes | gap | none | none | nothing writes it (decision 0086); per definition hashes are [P3] |
| 4.4 | `use go` packages, bindings, `go` effect, XO1101, XO1102 | stable | go; native: XO1201 impossible | gen:go_* goldens, gen:test_go_interop, check:rules_go_test | Appendix B 5 Proposed: decision 0093 |
| 4.4 | Go callbacks on foreign goroutines as tasks (decision 0067, 0073) | stable | go; native: XO1201 impossible | gen:test_go_callback_task, gen:replay_gocallback_test |  |
| 5.1 | Value semantics everywhere, no pointers | stable | go, llvm, arm64, no-gc | gen:value_semantics, gen:list_value_semantics, gen TestListModel, llvm:value_semantics, llvm:rc_aliasing |  |
| 5.2 | Mutation rules, nested paths, `update` | stable | go, llvm, arm64, no-gc | gen:places_boxes, gen:map_value_semantics, llvm:maps |  |
| 5.2 | Lost update check XO0520 | stable | checker (all) | check:rules_test, rules04_test, rules05_test, planted_test |  |
| 5.3 | `Buf`, `StrBuilder`, `BytesBuilder`, moves XO0510 | preview | go; native: XO1201 planned | gen:collections, gen:bytes_builder, llvm:native_bytes, check golden mutation | native: StrBuilder and BytesBuilder yes, `Buf` XO1201 planned |
| 5.4 | `Mutex.with`, write back, keep and fail pattern | stable | go, llvm, arm64, no-gc | gen:mutex_atomic, llvm:conc_mutex, llvm:conc_mutex_rollback |  |
| 5.4 | `with_read` (decision 0063) | stable | go, llvm, arm64, no-gc | gen:mutex_read, gen:test_mutex_read, llvm:conc_reentrant_read, std/rt rwlock_test |  |
| 5.4 | XO0804 split critical section | stable | checker (all) | check:rules06_test, planted_test |  |
| 5.4 | Re entrant `with` faults XO-F004 | stable | go, llvm, arm64, no-gc | gen:mutex_atomic, llvm:conc_reentrant |  |
| 5.4 | `Atomic[Int]`, `Atomic[Bool]` | stable | go, llvm, arm64, no-gc | gen:mutex_atomic, llvm:conc_mutex |  |
| 5.5 | Tracing GC; request scoped arenas by default (decision 0066) | stable | go | gen:arena_escape, gen:arena_holes, gen arena_diff_test | native backends use reference counting instead (core 10.1) |
| 5.5 | User written `arena scope` | gap | none | none | [P2] |
| 6.1 | Effect labels and sub effects; XO0702 | stable | checker (all) | check:rules_test |  |
| 6.1 | `ffi` effect | gap | checker (all) | none | accepted by the checker, nothing performs it ([P3]); decision 0093 Proposed |
| 6.1 | Pure functions; logging ambient | stable | checker (all) | check:rules07_test, gen:logging |  |
| 6.2 | `uses`, `pub` not inferred, XO0701 with call chain | stable | checker (all) | check:rules04_test, rules07_test, habits_test |  |
| 6.2 | Function type effects, subsumption | stable | checker (all) | check:rules_test |  |
| 6.2 | Effect variables, struct effect parameters, omitted effect arguments | stable | checker (all) | check:rules04_test (elision cases), gen:mono_generics |  |
| 6.3 | Capabilities only from `main`; `Os` fields | stable | go, llvm, arm64, no-gc | gen:main_noos, llvm goldens | native: `os.tasks` XO1201 planned |
| 6.3 | Fs attenuation: `sub`, `read_only`, `write_only`, `Denied` | preview | go; native: XO1201 planned | gen:fs_rooting, gen:test_fakes | native: every Fs method except read is XO1201 planned |
| 6.4 | Test fakes satisfy capability interfaces | stable | go, llvm, arm64, no-gc | gen:test_fakes, cmd/xo nogc tests | native: MemFs, set_input, FakeProc.run are XO1201 |
| 7.1 | Scopes, spawn, spawn_background, await, exit and failure rules, XO0801 | stable | go, llvm, arm64, no-gc | gen:scope_tasks, gen:scope_failure, llvm:conc_scopes, check golden concurrency |  |
| 7.1 | Task groups, escape XO0802, XO0803, `os.tasks` | preview | go; native: XO1201 planned | gen:scope_tasks, check:rules04_test, planted_test | native: groups yes (llvm:conc_scopes), `os.tasks` XO1201 |
| 7.1 | `task.with_timeout` | stable | go, llvm, arm64, no-gc | gen:task_std, gen:cancellation, llvm:clock_timers |  |
| 7.2 | Sticky cancellation, Canceled variants, unwinding, `is_canceled` | stable | go, llvm, arm64, no-gc | gen:cancellation, llvm:conc_await_canceled, llvm:conc_unwind |  |
| 7.2 | `sleep_or_cancel`, stdin as a cancellation point (decision 0088) | preview | go; native: XO1201 planned | gen:codex_gaps, gen:test_stdin_stop, llvm:codex_gaps | native: stdin reads XO1201 planned |
| 7.3 | Channels, `try_*`, `select` with `after`, `for x in ch` | stable | go, llvm, arm64, no-gc | gen:channels, gen:chan_loops, llvm:conc_select, llvm:conc_channels |  |
| 7.4 | Deterministic scheduler for `xo test`, `--seed`, `--explore`, `seed N` pins | stable | go | gen:test_explore, gen TestRunSeedReproduces, gen TestTestSeedReproduces | [P2] section but implemented on the Go backend; native has none by design |
| 7.5 | Two phase shutdown, `stop_requested`, `stopping`, grace | stable | go, llvm, arm64, no-gc | gen TestSignal*, gen:test_shutdown, llvm TestNativeSignal |  |
| 8 | Faults: overflow, bounds, contracts, conversion, holes, `fault`; exit 70 | stable | go, llvm, arm64, no-gc | gen:index_oob, gen:div_zero, llvm:fault_* |  |
| 8 | `task.isolate`, HTTP 500 isolation | stable | go, llvm, arm64, no-gc | gen:task_std, llvm:fault_tasks, llvm:http_fault | native: Fault.frames empty (spec says so) |
| 8 | Release builds and `requires(debug)` / `ensures(debug)` disabling | gap | none | parser tests (syntax only) | no release mode; debug contracts always checked (marked [later]) |
| 9.1 | `requires`, `requires ... else`, `ensures` with `old` and `result`, `invariant` | stable | go, llvm, arm64, no-gc | gen:contracts, gen:invariant_*, llvm:contract_* |  |
| 9.1 | Contract failure prints sub expression values | stable | go, llvm, arm64, no-gc | gen:contracts, llvm:contract_requires |  |
| 9.1 | Contract fuzzing in `xo test` | stable | go | gen:test_contracts, cmd/xo TestRegressionWritten | native: reported as skipped (spec says so) |
| 9.2 | Test blocks, `expect`, `testing`, test body `() ! Error` | stable | go, llvm, arm64, no-gc | gen:test_runner, cmd/xo nogc tests |  |
| 9.2 | Property tests `test ... for`, shrinking | preview | go; native: XO1201 planned | gen:test_property | native: XO1201 planned |
| 9.2 | Regression file for shrunk contract failures | stable | go | cmd/xo TestRegressionWritten |  |
| 9.2 | Real capabilities: `test ... uses net` with `--real` | preview | go | none | works in a probe (skipped without --real, run with it); no test |
| 10.1 | `check`, `build`, `run`, `test`, `fmt`, `fix`, `vet`, `query`, `edit`, `doc`, `explain`, `dump`, `mcp`, `serve`, `replay`, `bind`, `version` | stable | toolchain | cmd/xo tests, internal/* tests |  |
| 10.1 | `xo build --ci` | stable | toolchain | cmd/xo TestBuildCI (added) | was missing although core 10.5, 11.4 and vet promised it; implemented by this audit |
| 10.1 | `xo build --targets=all` | stable | go | gen TestCrossCompile (4 targets, full suite) | native: arm64 and host llvm only |
| 10.1 | `--json` on reporting commands | stable | toolchain | cmd/xo TestCheckJSON, agent_test | spec claimed every command; narrowed to the real list |
| 10.1 | `xo test --fuzz=10s` | stable | toolchain | - | spec fixed to the real flag `--cases=N` |
| 10.1 | `xo why`, `xo debug` | gap | none | none | [P2]; commands do not exist |
| 10.1 | `--no-gc` builds and XO1201 (decision 0083) | stable | llvm, arm64, no-gc | build TestNoGCCheckAgrees, cmd/xo TestNoGC* |  |
| 10.1 | `--hybrid` (decision 0084) | stable | go + arm64 asm | gen hybrid_test |  |
| 10.2 | JSON diagnostics with spans, notes, fixes; no cascades | stable | toolchain | check golden want.json, cmd/xo TestCheckJSON |  |
| 10.2 | Habit diagnostics XO0177 (decision 0087) | stable | checker (all) | check:habits_test, fix TestHabitGolden |  |
| 10.3 | `xo query` type, def, callers, effects, holes, api | stable | toolchain | query tests, cmd/xo tools_test |  |
| 10.4 | `xo edit` rename, add-field, change-sig | stable | toolchain | edit golden tests, cmd/xo edit_test |  |
| 10.5 | `xo fmt`, no options | stable | toolchain | format golden tests, TestSpecExamples |  |
| 10.6 | Diagnostic code registry | stable | toolchain | diag TestSpecListsEveryCode, TestSpecListsEveryFaultCode (added), TestExplanationsComplete | added by this audit; XO1001 reserved |
| 11.1 | Error frames with parameters (Debug, 64 bytes) | preview | go | gen:error_frames | native records no frames; task line missing ([later]); Appendix B 3 Proposed: decision 0092 |
| 11.1 | `--error-args` in release builds | gap | none | none | no release mode |
| 11.2 | Crash bundles | preview | go | gen replay_test, cmd/xo replay_test | native writes none (spec says so) |
| 11.3 | `--record`, `XO_RECORD`, `XO_RING`, `xo replay` | stable | go | gen replay_test, gen replay_gocallback_test, cmd/xo replay_test | [P2] section, implemented on the Go backend |
| 11.3 | `xo debug --replay`, `xo why` | gap | none | none | [P2] |
| 11.4 | `xo dump`, SIGQUIT task tree | stable | go | cmd/xo dump_test, std/rt sched_test |  |
| 11.4 | Deadlock detection XO-F001 | stable | go, llvm, arm64, no-gc | gen:deadlock_real, gen:test_deadlock, llvm TestDeadlock | native: first line only (spec says so) |
| 11.4 | `dbg(expr)` | preview | go; native: XO1201 planned | gen:holes, vet tests | native: XO1201 planned |
| 11.5 | `xo debug --json`, `--dap` | gap | none | none |  |
| 11.5 | `//line` directives to .xo sources | stable | go | gen TestFaultStackPointsAtXo |  |
| 11.6 | Spans, request ids in logs, OpenTelemetry | gap | none | none | marked [later] |
| 12.1 | Cross compilation targets incl. windows and wasi | stable | go | gen TestCrossCompile |  |
| 12.1 | Reproducible byte identical builds | preview | go | none | claimed, not tested; marked [later] |
| 12.1 | SBOM per build | gap | none | none | marked [later] |
| 12.2 | `target` constant, compile time match, every arm checked | preview | go; native: XO1201 planned | check:rules_test | no runtime golden; native: XO1201 planned |
| 12.2 | `std/sys/<os>`, XO1001 | gap | none | none | no module; XO1001 never emitted (reserved) |
| 13 | Editions in xo.mod, `xo fix --edition`, `--from=draft-0.N` | stable | toolchain | check:edition_test, cmd/xo fix_test | only edition 2026 exists |
| 14 | std module list | stable | toolchain | - | std/path and std/go added to the table |
| 15 | Grammar summary matches the parser | stable | checker (all) | parser spec_test (every core.md and std.md block), FuzzParse |  |
| std 0 | Conventions: Display on every error, Canceled variants | stable | go, llvm, arm64, no-gc | check std modules |  |
| std 1 | Prelude interfaces Display, Error, Debug, Eq, Hash, Ord; min, max, clamp | stable | go, llvm, arm64, no-gc | gen:float_order, gen:int_ops, llvm:float_order |  |
| std 2.1 | Int methods abs, pow, wrapping, saturating, checked; Float floor..sqrt, is_nan | preview | go; native: XO1201 planned | gen:int_ops, llvm:arith | native: `pow` XO1201; wrapping/saturating/checked have no native golden |
| std 2.2 | Str methods | stable | go, llvm, arm64, no-gc | gen:strings, gen:text_helpers, llvm:native_prelude |  |
| std 2.2 | Bytes hex and base64, decode_rune, quote, Rune classes (decision 0079) | preview | go; native: XO1201 planned | gen:std06_helpers, gen:text_helpers, llvm:native_bytes, llvm unicode_test | native: base64 XO1201 |
| std 2.3 | List methods | preview | go; native: XO1201 planned | gen:collections, gen:list_value_semantics, llvm:native_values_hof, llvm:lists | native: `indexed`, `insert`, `remove_at` XO1201; `Range.to_list` XO1201 |
| std 2.4 | Map methods, insertion order | stable | go, llvm, arm64, no-gc | gen:map_value_semantics, llvm:maps, std/rt map tests | native: `Map.filter` XO1201; Appendix B 2 Proposed: decision 0091 |
| std 2.4 | Set methods | preview | go; native: XO1201 planned | gen:collections | native: union, intersect, difference XO1201 |
| std 2.5 | Option and Result methods | stable | go, llvm, arm64, no-gc | gen:option_result, llvm:option, llvm:result |  |
| std 2.6 | Duration constructors, parse, scale, ceil_seconds; Time Unix and RFC 3339 | preview | go; native: XO1201 planned | gen:duration_time, gen:codex_gaps, llvm:clock_timers | native: from_secs, scale, ceil_seconds, parse_rfc3339 XO1201 |
| std 2.7 | Chan, Mutex, Atomic, Task, Buf, StrBuilder, BytesBuilder, Secret | preview | go; native: XO1201 planned | gen:channels, gen:mutex_atomic, gen:bytes_builder | native: Buf, Secret XO1201; `Buf.len` was undocumented (added) |
| std 3.1 | Fs | preview | go; native: XO1201 planned | gen:fs_rooting, gen:test_fakes | `cwd`, `case_sensitive` untested (probe works); native: all but read XO1201 |
| std 3.2 | Net TCP listen, dial, accept, read, write | stable | go, llvm, arm64, no-gc | llvm:net_echo, llvm:http_wire | go backend: probe only, no golden |
| std 3.3 | Clock now, sleep, sleep_or_cancel, since | stable | go, llvm, arm64, no-gc | gen:codex_gaps, llvm:clock_timers |  |
| std 3.4 | Rand | preview | go; native: XO1201 planned | gen:test_fakes, gen:replay/effects, llvm:native_crypto | native: shuffle XO1201 (spec says so) |
| std 3.5 | Env | stable | go, llvm, arm64, no-gc | gen:test_fakes, cmd/xo main_test |  |
| std 3.6 | Stdio print, write, read_line, read_all, lines | preview | go; native: XO1201 planned | gen:test_fakes, gen:test_stdin_stop, llvm:strings | native: reads XO1201 (spec says so); `write` no Go golden |
| std 3.7 | Proc exit, set_grace, run | preview | go | gen:test_fakes, gen:test_shutdown | real `run` untested (probe works); native: run XO1201 |
| std 4.1 | Router handle, wrap, mount, patterns, 404, 405 | stable | go, llvm, arm64, no-gc | gen:http_router, gen:http_scoped, llvm:http_server |  |
| std 4.2 | StatusError mapping, fault isolation 500 | stable | go, llvm, arm64, no-gc | gen:http_router, llvm:http_fault |  |
| std 4.3 | Request and Response, typed values, stream, decode_json | stable | go, llvm, arm64, no-gc | gen:http_scoped, gen:http_router, llvm:http_server, llvm:http_stream |  |
| std 4.4 | Server serve, Content-Length, graceful stop | stable | go, llvm, arm64, no-gc | gen:http_real, llvm:http_wire, gen TestSignalHTTPInFlight |  |
| std 4.5 | Client get, send, post_json, redirects, parse_url | preview | go; native: no TLS | gen:http_real, gen:http_router, llvm:http_client (redirects) | native: https is XO1201 (no TLS, spec says so) |
| std 5 | std/json encode, decode, Patch, derive rules | stable | go, llvm, arm64, no-gc | gen:json_codec, gen:json_fast, llvm:native_json, llvm TestJSONParseDiff |  |
| std 6 | std/log levels, configure, test capture (decision 0057) | stable | go, llvm, arm64, no-gc | gen:logging, gen:test_logs | span and request id were claimed: marked [later] |
| std 7 | std/task isolate, with_timeout, stop_*, groups | stable | go, llvm, arm64, no-gc | gen:task_std, llvm:fault_tasks |  |
| std 7 | map_concurrent, map_concurrent_until, Once, Semaphore | preview | go; native: XO1201 planned | gen:task_std, gen:test_map_until, gen:test_once | native: all XO1201 planned |
| std 8 | std/testing fakes: FakeNet, FakeClock, MemFs, FakeEnv, FakeStdio, FakeProc, run_main | stable | go | gen:test_fakes, gen:test_signal_after, gen:test_run_main_void | native: partial (spec lists the gaps) |
| std 8.5 | testing.gen and generators | preview | go; native: XO1201 planned | gen:test_property |  |
| std 8.6 | testing.logs | stable | go, llvm, arm64, no-gc | gen:test_logs, cmd/xo nogc tests |  |
| std 9 | std/path | preview | go; native: XO1201 planned | gen:std06_path | native: XO1201 (llvm unsupported/std_module) |
| std 10 | std/sql in memory driver, query_as, transactions, RETURNING | stable | go, llvm, arm64, no-gc | gen:sql_basic, gen:sql_decode, gen:sql_tx, llvm:native_sql | draft per std.md; no real drivers |
| std 10.1 | `sql.open` with real drivers | gap | none | gen:sql_basic (unknown driver error) | no driver ships (std.md says so) |
| std 11 | std/crypto | stable | go, llvm, arm64, no-gc | gen:crypto_basic, gen:test_crypto, llvm:native_crypto, llvm TestCryptoDiff |  |
| std 12 | std/go `go.Error` | stable | go; native: XO1201 impossible | gen:go_strconv, gen:go_url | was undocumented in std.md; section added |
| std 13 | std/time zones, formatting, parsing | gap | none | none | module empty; Proposed API written into std.md 13 |
