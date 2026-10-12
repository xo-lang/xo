# Spec stability audit, fifth pass (2026-10-11)

Delta over [spec-audit-2026-10-10.md](spec-audit-2026-10-10.md). Input
for the owner's 1.0 decision (decision 0089); nothing here declares,
tags, or freezes 1.0, and no status marker of the spec was changed.

Toolchain: `2cdbb29` (spec draft 0.10 open, after v0.9.0) plus this
pass's commits, darwin/arm64, Go 1.26.1. Status meanings as in the
2026-10-10 pass: **stable** = the Go backend implements it as specified
and a test covers it there; **preview** = implemented without a test, or
not on the Go backend, or a Proposed decision may change it; **gap** =
specified, not built. Native columns: **yes** (LLVM and arm64 `--no-gc`,
same output as the Go backend), **XO1201** (planned; the native builds
report it), **checker** (a check rule, the same on every backend),
**n/a** (toolchain only).

## Scope

Every spec row added or changed since the 2026-10-10 pass (`318d07e`):
the "Changes in 0.10" list of core.md and the std.md sections it names.
The task named decisions 0155 to 0165; the same range also merged
language changes of decisions 0131, 0133, and 0136 to 0143 (0.10's
leading operators, `while let`, `for` over Bytes, loop bounds, real-time
functions, assignment and call order, constant indexes), which no pass
had scored, so they are scored here too. Decisions of the range with no
spec row (backend ABI 0145 to 0153, self hosting 0127, 0141, 0150, 0152,
0158, prebuilt runtime 0157, crypto 0134, site and playground 0139, 0144,
mascot and domains 0132, 0135) are listed under "Toolchain items" with
their tests, not counted.

Native evidence: `xo check --no-gc` on the 16 internal/gen goldens added
in the range (5 report XO1201, all planned and stated by the spec:
cap_attenuation, formats_archive, formats_regexp, fs_files, proc_cmd);
`xo test --no-gc` on the five new `test_*` goldens (test_cli and
test_term identical to the Go backend; test_fs_files and test_proc_cmd
XO1201 as specified; test_loop_bound's contract fuzzing is reported as
skipped natively, as core 10.1 states for all contract fuzzing).

## New and changed rows

### Language (core.md)

| Row | Status | Native | Tests (package) |
|---|---|---|---|
| 1.3 a line starting with a binary operator is XO0178, with a fix (0136) | stable | checker | internal/check `TestR4HabitsReported`, `TestR4HabitsNotReported`; internal/fix habits goldens codex-leading-or, codex-leading-plus |
| 3.5 `while let` (0137) | stable | yes | internal/check `TestWhileLet`; internal/llvm `TestGolden/while_let` (Go and LLVM); internal/arm64 `TestGoldenRun/while_let` |
| 3.5 `for b in bytes` yields UInt8 (0138) | stable | yes | internal/check `TestBytesFor`; llvm `TestGolden/bytes_for`; arm64 `TestGoldenRun/bytes_for` |
| 3.5 loop bounds `bound N`, XO1402, XO-F006 (0133) | stable | yes (run time check; contract fuzzing of bounded loops skipped natively, as all contract fuzzing, core.md:2235) | internal/gen `TestGolden/test_loop_bound`; internal/check `TestGolden` (realtime_loop); llvm and arm64 `realtime_bound_fault` |
| 5.2 an assignment computes its value before it reads the target (0140) | stable | yes | internal/gen `TestGolden/assign_rhs_mutates`; internal/llvm `TestGenGoldensNative/assign_rhs_mutates` |
| 5.2 `var` receiver and arguments read after every argument (0142) | stable | yes | gen `TestGolden/var_recv_after_args`; llvm `TestGenGoldensNative/var_recv_after_args` |
| 15 constant base `NAMES[i]` is an index, not type arguments (0143) | stable | checker | internal/parser `TestExpressions`; gen `TestGolden/const_index`; llvm `TestGenGoldensNative/const_index` |
| 4.1 import path segments (`PathSeg`, dashes, `.`), XO0158 (changed row) | stable | checker | internal/check `TestResolution`; internal/parser `TestDeclarations`, `TestErrorRecovery` |
| 4.2, 13 `edition preview`, `edition 2026` alias, XO1315, `xo fix --edition=preview` (0155) | stable | n/a | internal/check `TestEditionAliasNote`, `TestResolveEdition`; internal/fix `TestFixEditionAlias`; cmd/xo `TestFixEditionFlag`, `TestInit` (writes `edition preview`) |
| 4.2 vouches, XO1316, `xo get` and `accept-effects` refuse an unvouched escape hatch (0156, f6f9bd6) | stable | n/a | cmd/xo `TestVouches` (`get --accept-effects` refused), `TestVouchesC`; internal/check `TestVouchVendored`; internal/cbind `TestBindMemoUsesNone` |
| 4.3 effect lock, XO1317, `xo mod accept-effects`, `xo get --accept-effects` (0156) | stable | n/a | cmd/xo `TestEffectLock`, `TestModules` |
| 6.1 logging is the effect `log` (0156) | stable | checker | internal/check `TestEffects` ("logging needs the log effect"); cmd/xo `TestSupplyChainAttacks` (logleak) |
| 6.3 attenuation `Net.only`, `Env.only`, `Proc.only`, `Denied` variants (0156) | stable | XO1201 (planned) | gen `TestGolden/cap_attenuation`; cmd/xo `TestAttenuationFakes` |
| 10.1 `xo audit [--json]` (0156) | stable | n/a | cmd/xo `TestAudit` |
| 6.5 real-time functions XO1401 to XO1405, sections and XO-F007 (0133) | stable | yes (sections are native only, as specified) | internal/check `TestRealtimeRules`, `TestGolden` (realtime_alloc, _blocking, _call, _loop, _ok, _recursion); internal/llvm `TestRealtimeNative` (llvm, arm64), `TestRuntimeRealtime`, `TestGolden/realtime_section` |
| 10.1 `xo build --stamp NAME=VALUE`, also `xo run`, XO1501, kept by `xo replay` (0165) | stable | yes | internal/build `TestStamp` (go and native), `TestStampErrors`; cmd/xo `TestRecordReplayStamp` (added in this pass) |
| 10.1 hybrid assembly loops yield to the scheduler and GC (0131) | stable | n/a (Go backend hybrid) | internal/gen `TestHybridPreemption` |

### Standard library (std.md)

| Row | Status | Native | Tests (package) |
|---|---|---|---|
| 14 `std/cli`: flags, arguments, subcommands, help, usage errors, `parse_or_exit` (0160) | stable | yes | std/rt `TestCliValidate`, `TestCliSuggest`, `TestCliHelpAlignment`; gen `TestGolden/test_cli`; llvm `TestGolden/cli_parse`, `cli_usage_exit`, `cli_fault`, `cli_decl_fault`; cmd/xo `TestNoGCTests/test_cli` (added in this pass) |
| 14.5 shell completions (0165) | stable | yes | std/rt `TestCliBashCompletion`, `TestCliCompletionSyntax`; llvm and arm64 `cli_completion` |
| 3.8 `Proc.cmd`, `Cmd`, `Child`, `Proc.pipeline`, FakeProc `on_run` (0161) | stable | XO1201 (planned) | gen `TestGolden/proc_cmd`, `TestGolden/test_proc_cmd` |
| 3.6 `Stdio` terminal checks, `width`, `read_secret`, `read_bytes`, `FakeStdio.set_tty` (0162) | stable | yes | gen `TestGolden/test_term`; llvm and arm64 `term_stdio`; cmd/xo `TestNoGCTests/test_term` (added in this pass) |
| 15 `std/term` (0162) | stable | yes | gen `TestGolden/test_term`; llvm and arm64 `term_stdio` |
| 3.1 `File` handles, `append`, `copy`, `mode`, `symlink`, `read_link`, `glob`, `lock`, `try_lock`, MemFs (0163) | stable | XO1201 (planned) | gen `TestGolden/fs_files`, `TestGolden/test_fs_files`; std/rt `TestLinkEscapes` |
| 3.5 `Env.config_dir`, `cache_dir` (0163) | stable | yes | gen `TestGolden/fs_files`, `test_fs_files`; llvm and arm64 `env_dirs` |
| 16 `std/csv` (0164) | stable | yes | llvm `TestGolden/csv_basic` (Go and LLVM); arm64 `TestGoldenRun/csv_basic` |
| 17 `std/regexp` (0164) | stable | XO1201 (planned) | gen `TestGolden/formats_regexp` |
| 18 `std/archive` (0164) | stable | XO1201 (planned) | gen `TestGolden/formats_archive` |
| 19 `std/toml` (0164) | stable | yes | std/rt `TestTomlToJSON`, `TestTomlErrors`; gen `TestGolden/formats_toml`; llvm and arm64 `toml_basic`; llvm `TestGenGoldensNative/formats_toml` (added in this pass) |

### New gap

| Row | Status | Spec line |
|---|---|---|
| 6.1 each log record tagged with the dependency that wrote it (0156) | gap | core.md:1696 **[later]** |

## Summary counts

| Status | 2026-10-10 | 2026-10-11 |
|---|---:|---:|
| stable | 180 | 207 |
| preview | 1 | 1 |
| gap | 10 | 11 |
| total | 191 | 219 |

28 rows added (27 stable, 1 gap) and one changed (4.1 import paths,
stable before and after). No row changed status. Of the 27 new stable
rows, 14 build natively with the Go backend's output, 5 are XO1201
(planned, each stated by the spec), 3 are checker or parser rules, and 5
are toolchain only (n/a).

Preview left (1): 6.1 `use c` (native only, XO1201 on the Go backend by
design).

## Remaining gaps (11)

| Gap | Spec line |
|---|---|
| 1.2 doc comment examples run by `xo test` | core.md:412 ([P2]); 10.1 table core.md:2178 |
| 2.9 `OpenApi` and `Proto` derives | core.md:832 (**[after 1.0]**) |
| 5.5 user written `arena scope` | core.md:1660 (**[P2]**) |
| 6.1 log records tagged with the dependency (new) | core.md:1696 (**[later]**) |
| 10.1 `xo why`, `xo debug` | core.md:2188 ([P2]) |
| 11.3 `xo debug --replay` (stepping backward) | core.md:2558 (11.3 [P2]), 2598 |
| 11.5 `xo debug --json`, `--dap` | core.md:2188 ([P2]), 2630 to 2634 |
| 11.6 spans, request ids in records, OpenTelemetry | core.md:2639 (**[later]**); std.md:1149 (**[later]**) |
| 12.1 SBOM | core.md:2661 (**[later]**) |
| 12.2 `std/sys/<os>` and `XO1001` | core.md:2688 (**[later]**) |
| std 10 real `std/sql` drivers | std.md:1384 ("in memory driver only", "none ships yet") |

Markers folded into stable rows by earlier passes, not counted as gaps:
the task line of error traces (core.md:2544, inside 11.1), the deadlock
wait cycle (core.md:2608, inside 11.4), and the [P2] headings of 7.4 and
11.3 whose implemented parts are stable.

## Gaps to close or cut (owner decides)

- **std 10 real `std/sql` drivers**: close before 1.0; the driver
  interface may change once one real driver is written against it.
- **12.1 SBOM**: close before 1.0; its JSON format is a compatibility
  surface people script against, and the build list already has the data.
- **1.2 doc comment examples under `xo test`**: close before 1.0; landing
  later changes what `xo test` runs, so 1.0 projects could start failing.
- **5.5 `arena scope`**: cut from the 1.0 text (keep the word reserved);
  automatic arenas already cover the allocation case invisibly.
- **6.1 log records tagged with the dependency**: keep after 1.0, but fix
  the record field's name first, since record fields are observable output.
- **11.6 spans and OpenTelemetry**: keep after 1.0, with an ADR first on
  `std/log` extension or `std/trace` so the 1.0 log surface does not move.
- **12.2 `std/sys/<os>`, `XO1001`**: keep after 1.0; additive and the
  code is reserved, nothing can depend on it yet.
- **10.1, 11.3, 11.5 debugger (`xo why`, `xo debug`, `--replay`,
  `--json`, `--dap`)**: keep after 1.0; tooling, no language surface.
- **2.9 `OpenApi`, `Proto`**: keep after 1.0, as the owner decided on
  2026-10-09.

## Disagreements fixed in this pass

| Where | Spec said | Code did | Fix |
|---|---|---|---|
| core 10.1 command table | `xo build` row had no `--stamp`, though 0.10's change list and std.md 14.5 point at 10.1 for it | `xo build` and `xo run` take `--stamp` (cmd/xo/build.go) | spec: `--stamp NAME=VALUE` added to the `xo build` row with XO1501 and replay |
| core 14 module table | `std/time` "Go backend only so far" | native builds build all of std/time since 2026-10-09 (std.md 13.5, internal/llvm `TestTimeDiff`) | spec: the stale note removed |
| core 10.1, decision 0165 | stamps are recorded so `xo replay` rebuilds the same program | implemented (cmd/xo/replay.go passes `h.Build.Stamps`), no test | test: cmd/xo `TestRecordReplayStamp`; fails with the stamps dropped from replay.go |
| std 14, 3.6, 15 native claims | std/cli and terminals on every backend | the native test binaries of test_cli and test_term were never compared with the Go backend | test: added to cmd/xo `TestNoGCTests` |
| std 19 | std/toml on every backend | formats_toml builds natively (`xo check --no-gc` clean) but was not in internal/llvm/testdata/gen_native.txt | test: added to `TestGenGoldensNative` |

## Inconsistencies listed, not fixed

1. `TestSelfhostArm64/llvm` fails in the full tier (frames 256 bytes
   smaller than the Go emitter's), recorded in
   [selfhost-arm64-2026-10-11.md](selfhost-arm64-2026-10-11.md); the
   merge tier result of this pass is under "Test status". Not a spec row.
2. Contract fuzzing of loops with `bound N` is skipped natively (like all
   contract fuzzing, core 10.1), so XO-F006 found by fuzzing is a Go
   backend result only; the run time XO-F006 is native too
   (`realtime_bound_fault`). Next: native contract fuzzing, which would
   let test_loop_bound join `TestNoGCTests`.

## Toolchain items (no spec row)

| Item | Tests (package) |
|---|---|
| Prebuilt runtime packs, `XO_RTPACK`, `xo rtpack` (0157) | internal/build `TestRTPackVariant`, `TestRTPackKey`, `TestRTPackDeterministic`, `TestRTPackUse`, `TestClangErrorRuntimeHint`; cmd/xo `TestRTPackCommand` |
| arm64 emitter in Xo, `xo asmdump`, switch-over driver (0158) | internal/selfhost `TestSelfhostArm64`, `TestSelfhostArm64Mutations` |
| Release archives (scripts/release.sh) | scripts/disttar `TestArchiveDeterministic` |

## Test status

TBD
