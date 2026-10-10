# Spec stability audit, delta (2026-10-10)

Delta over [spec-audit-2026-10-09.md](spec-audit-2026-10-09.md) (its
fourth pass). Scope: the CHANGELOG items of v0.8.0 and v0.9.0 only, not a
re read of the spec. Input for the owner's 1.0 decision (decision 0089);
nothing here declares, tags, or freezes 1.0.

Toolchain: `a5ca90f` (v0.9.0, spec draft 0.10 open), darwin/arm64, Go
1.26.1. Status meanings as in the 2026-10-09 second pass (decision 0089):
**stable** = the Go backend implements it as specified and a test covers
it there; **preview** = implemented without a test, or not on the Go
backend, or a Proposed decision may change it; **gap** = specified, not
built. Native columns: **yes** (LLVM and arm64 `--no-gc`, same output as
the Go backend), **partial**, **XO1201**, **n/a** (toolchain only).

## Known suspect: fault_defer_chain

`internal/llvm/testdata/golden/fault_defer_chain` (deferred code that
faults, decision 0085 amendment, from the native runtime leftovers work
0117 to 0120). It exists only under internal/llvm (no internal/gen copy).

Result: **no disagreement**. The program's standard output equals the
`--- stdout` section of want.txt byte for byte on all three backends, and
each exits 0:

```
cp internal/llvm/testdata/golden/fault_defer_chain/main.xo <tmp>/prog/
xo run <tmp>/prog                    # Go backend: identical to want.txt
xo run --backend=arm64 <tmp>/prog    # identical
xo run --backend=llvm <tmp>/prog     # identical
```

Coverage note: `internal/llvm` `TestGolden` builds each golden on the Go
backend and on LLVM and compares both with want.txt, but it is skipped
under `-short`, so the per commit test run does not check this golden.
`go test ./internal/llvm -run 'TestGolden$/fault_defer_chain'` does.

**Fixed (63755b4, 2026-10-10):** `TestGolden` runs `fault_defer_chain`
(and the new `request_eq`) under `-short` too (`shortGoldens`, about a
second each with a cold build directory).

## CHANGELOG items since v0.7.0

### v0.9.0

| Item | Go backend | Native | Tests |
|---|---|---|---|
| Module proxies, `XO_PROXY`, `XO_NOPROXY`, `xo mod proxy serve` (0126) | stable | n/a | internal/mod `TestProxy`, `TestProxyUntrusted`, `TestProxyServe`; cmd/xo `TestModProxy` |
| Checksum log `XO_SUMDB`, XO1313, XO1314 (0124) | stable (default `off`, no public log) | n/a | internal/sumdb `TestProofs`, `TestKeys`, `TestClientServer`, `TestFork`, `TestEscape`; internal/mod `TestChecksumLog`, `TestChecksumLogThroughProxy`; cmd/xo `TestChecksumLogCommand` |
| Definition hashes, `xo query hash`, `xo publish --check` listing (0125) | stable | n/a | internal/defhash `TestStable`, `TestSensitive`, `TestRecursion`; cmd/xo `TestQueryHashAndPublish` |
| HTTP/2 over TLS by ALPN (0121) | stable | yes | std/rt `TestArenaHTTP2`, `TestServeNoH2C`; internal/llvm `TestH2Diff`, `TestH2Sanitizers` |
| Native test binaries: property tests, `testing.gen`, `Set[Int8]`, `--real` (0120) | stable (already) | yes | cmd/xo `TestNoGCTests` (testdata/nogc/prop, nogc/real), `TestNoGCTestLeaks` |
| Deferred code that faults runs the rest of cleanup; `task.isolate` sees cancellation again (0085 amendment) | stable (already) | yes | llvm:fault_defer_chain (`TestGolden`, full mode only) |
| LLVM CPU features (LSE, outline atomics) (0119) | n/a | yes | internal/build `TestParseTargetAttrs`, `TestLLVMTargetAttrs` |
| Native `Mutex` handoff to readers, scopes drop finished tasks, cross thread frees (0118) | n/a | yes | std/llrt/testdata/tasks.c (`TestRuntimeTasks`): `handoff`, `prune`, `magazines` (6e65cc5; each fails with its mechanism disabled) |
| Native HTTPS speed, session tickets and resumption (0117) | n/a | yes | internal/llvm `TestTLSDiff` (resumption on TLS 1.3 and 1.2 by `DidResume`, 6e65cc5), `TestTLSLeaks`, internal/build `TestTLSAccel` |
| `xo lsp` (0114) | stable | n/a | internal/lsp `TestDiagnosticsAndQuickFix`, `TestHover`, `TestDefinition`, `TestReferencesAndRename`, `TestFormatAndSymbols`, `TestSemanticTokens`, `TestCorpus`, `TestStdCopy` |
| VS Code grammar (0113) | n/a (not in the spec) | n/a | editors/vscode `TestKeywordsMatchLexer`, `TestManifestPointsAtGrammar` |
| `use c`, `xo bind --c`, `ffi` effect (0093 stage 2, 0116) | XO1201 by design | yes, calls not recorded | internal/llvm `TestUseC`, `TestUseCZlib`; cmd/xo `TestBindC`; internal/check `TestUseC` |
| Overlapping `var` arguments are XO0503 everywhere (0111) | stable | yes (same diagnostic) | internal/check `TestVarArgsOverlap`; internal/gen list_model_test; llvm:var_alias |
| Handles and function values not comparable, XO0404 (0112) | stable | yes (same diagnostic) | internal/check `TestHandlesNotComparable` |
| Native `Proc.run`, `os.tasks`, method values, `map_concurrent_until`, base64, `Range.to_list`, `Rand.shuffle`, `target`, MemFs, FakeProc (0107 to 0109) | stable (already) | yes | scored in the 2026-10-09 fourth pass (llvm:method_values, llvm:map_until, gen:os_tasks, gen:test_fakes, `TestNoGCTests`) |
| TestCompileProperty (0110) | test | test | internal/gen `TestCompileProperty` |
| Codex r3 fixes (select arm without binding, `?` fix placement, XO0205 notes) | stable | n/a | internal/gen goldens and fix tests (not traced one by one; see "Items with no test traced") |
| Native interfaces as values, newtypes, list patterns, Float32, `Secret`, std/path, Once, Semaphore, Fs, error frames, std/time, stdin, TLS 1.2/1.3 (0094, 0100 to 0106) | stable (already) | yes | scored in the 2026-10-09 third pass (`TestGenGoldensNative`, `TestErrorTracesNative`, `TestStdinNative`, `TestTimeDiff`, `TestTLSDiff`) |
| HTTPS on every backend, `tls_cert`, `with_tls`, `with_roots`, `TlsErr` (0105, 0106) | stable | yes | internal/llvm `TestTLSDiff`; internal/build `TestTLSLibrary` |
| `bench/perf/http` `SCHEME`, `NEWCONN`, `-resume` | n/a | n/a | bench scripts, no test |

### v0.8.0

| Item | Go backend | Native | Tests |
|---|---|---|---|
| Modules: require, MVS, fetch, xo.lock, `xo get`, `tidy`, `vendor`, `clean`, `--offline`, xo.work (0086 steps 1, 2) | stable | n/a | cmd/xo `TestModules` (mod_test.go), internal/mod tests |
| Effect ceilings, `xo publish [--check]`, `xo init`, `/vN` (0086 steps 3, 4) | stable | n/a | internal/check ceiling_test; cmd/xo release_test (`TestPublish`), `TestEffectCeilings` |
| Modules in repository subdirectories | stable | n/a | cmd/xo `TestSubdirModules` |
| `deny` import rules, XO1312, `xo query imports`/`deps` (0095) | stable | checker (all) | internal/check rules_test; internal/query imports_test; internal/mod mod_test |
| std/time on the Go backend, XO0420, XO0177 (0094) | stable | yes (v0.9.0) | gen:time_civil, gen:time_zones, gen:time_format; internal/check timelayout_test, habits_test; internal/llvm `TestTimeDiff` |
| `ZoneErr`, `CivilErr` display text | stable | yes | gen:time_civil want.txt ("does not exist (skipped") |
| Newtypes `e.0`, `Email(s)` (0097) | stable | yes | gen:newtype_value, llvm:newtypes |
| `xo check --fix` (0096) | stable | n/a | cmd/xo `TestCheckFix` |
| Go backend output size, no DWARF, net/http only when used (0098) | stable | n/a | internal/build gocache tests (cache trimming), `TestGoBinaryTrimming` (no DWARF unless `XO_GO_DWARF=1`, no net/http without std/http; 153f721) |
| Codex r2: XO0175 split fix, `.display()` on every Display type, closure that never completes infers `Never` | stable | n/a | internal/parser parser_test (XO0175); internal/check rules_test |
| `x.debug()` on every value; second pass audit tests | stable | yes | gen:audit_second_pass |
| Go calls in record and replay (0093 stage 1) | stable | `use go` impossible | internal/gen `TestReplayGoCalls`; gobind `TestReplayModes` |
| Release builds, `requires(debug)` (0099) | stable | yes | internal/gen release_test; internal/build `TestReleaseNative`; cmd/xo ci_test |
| Lazy error frames, `--error-args=false` (0092) | stable | frames yes in main's report | std/rt frames_test; internal/gen release_test; cmd/xo ci_test; internal/llvm `TestErrorTracesNative` |

### Items with no test traced

1. **Fixed (6e65cc5).** Native session tickets and TLS resumption
   (0117): `TestTLSDiff` dials twice with a `ClientSessionCache` on TLS
   1.3 and 1.2 and prints `DidResume` (`resumed=false resumed=true` on
   every backend).
2. **Fixed (6e65cc5).** 0118 in std/llrt/testdata/tasks.c: `handoff`
   (readers queued behind a writer read before a writer queued after
   them), `prune` (a scope that starts 16N tasks keeps a list of about
   64), `magazines` (40 rounds of 50,000 blocks allocated on one thread
   and freed on another carve 1 chunk after warm up, 31 without
   magazines; counted by the new `xo_chunks`). Each check was seen to
   fail with its mechanism disabled. RSS of a real server is still
   bench only.
3. **Fixed (153f721).** 0098: internal/build `TestGoBinaryTrimming`
   builds hello world (no DWARF section, no `net/http.` name in the
   binary, whose pclntab keeps function names), an std/http program
   (the probe finds `net/http.`), and hello world with `XO_GO_DWARF=1`
   (DWARF present).
4. Codex r3 fixes were not traced to a test name in this pass (time box);
   grep `internal/gen/testdata/golden` and `internal/fix` for the select
   arm without a binding to confirm.

## Summary counts

Start: 2026-10-09 fourth pass = second pass counts (the third and fourth
passes scored native rows only): stable 169, preview 2, gap 14, total 185.

Deltas:

| Row | Change | Evidence |
|---|---|---|
| 11.1 error frames with parameters | preview to stable | 0092 landed (v0.8.0): std/rt frames_test, gen release_test |
| 11.2 crash bundles | preview to stable | 0092 landed; cmd/xo replay_test, internal/gen golden_test |
| 4.3 per definition hashes | gap to stable | 0125: internal/defhash, cmd/xo `TestQueryHashAndPublish` |
| 8 release builds, `requires(debug)` | gap to stable | 0099: gen release_test, build `TestReleaseNative` |
| 11.1 `--error-args` | gap to stable | 0092: gen release_test, cmd/xo ci_test |
| 6.1 `use c`, `ffi` effect | gap to preview | 0116: native only (Go backend XO1201 by design), calls not recorded |
| 4.2 module proxies (new row) | stable | 0126 tests above |
| 4.3 checksum log (new row) | stable | 0124 tests above |
| std 4.x HTTP/2 over TLS (new row) | stable | 0121 tests above |
| 10.x `xo lsp` (new row) | stable | internal/lsp |
| 3.x overlapping `var` arguments XO0503 (new row) | stable | `TestVarArgsOverlap` |
| 2.x handles not comparable XO0404 (new row) | stable | `TestHandlesNotComparable` |

| Status | 2026-10-09 (fourth pass) | 2026-10-10 |
|---|---:|---:|
| stable | 169 | 180 |
| preview | 2 | 1 |
| gap | 14 | 10 |
| total | 185 | 191 |

Gaps left (10): 1.2 doc comment examples under `xo test`; 2.9 `OpenApi`
and `Proto` derives (after 1.0, owner); 5.5 user written `arena scope`;
10.1 `xo why`, `xo debug`; 11.3 `xo debug --replay`; 11.5 `xo debug
--json`, `--dap`; 11.6 spans and OpenTelemetry; 12.1 SBOM; 12.2
`std/sys/<os>` and `XO1001`; std 10.1 real `std/sql` drivers. Spec 0.10
still marks all of them **[later]** or **[P2]** (core.md lines 75, 346,
1479, 1906, 1916, 2374, 2402, 2438); none was touched by v0.8.0 or v0.9.0.

Preview left (1): 6.1 `use c` (native only).

Native check over the Go backend goldens (`xo check --no-gc` on all 115
internal/gen goldens; was 99): 13 report XO1201. 12 are `use go`
(impossible by design; three also hit the planned `go.Error.go_type`).
One was planned: **http_scoped**, `==` on `http.Request` (`with ==
plain`, line 100), because the native `Request` carries request values
the equality helpers do not compare. **Fixed (63755b4):** `Request` holds
no handle, so under decision 0112 it stays comparable on every backend
(the Go backend and the golden were right); the native lowering compares
its six spec fields and ignores the typed values, as std/rt's `XoEq`.
`http_scoped` now runs with `--no-gc` (same output but the log
timestamps); internal/llvm golden `request_eq` covers `==`, `!=`, and
`contains`. So 12 goldens report XO1201, all `use go`.

Native test binaries (`xo test --no-gc`) differed from the Go backend
on three internal/gen goldens (the fourth pass's leftovers). **Fixed
(e9b0f00):** all three are byte identical on llvm and arm64 with `-v`
and in `TestNoGCTests`. Causes: the native `Once` did not store a fault
(f now runs under `task.isolate`, the fault is raised again with
`rt.fault_raise`); a configured log output in a test still went to the
test's buffer (now to the fake standard error, records still kept);
`set_grace` before `run_main` set the grace of nothing, and a third
signal called `_exit(130)` in the test binary (now `run_main` returns
130 and `exit_code()` is `Some(130)`, as std/rt). Before:

```
xo test --no-gc internal/gen/testdata/golden/test_logs      # 2 failed, Go 1: "configured output still captures records"
xo test --no-gc internal/gen/testdata/golden/test_once      # "a fault in f is shared" fails (Go: 8 passed)
xo test --no-gc internal/gen/testdata/golden/test_shutdown  # grace period 32s vs 12s; third signal exits 130 (Go: 9 passed)
```

## 1.0 readiness

### Go backend (the 1.0 path, decision 0089), ranked by how much each blocks

Ranking: could closing it after 1.0 force a breaking change (worst), then
does a user meet it in ordinary backend work, then size.

1. **std 10.1 real `std/sql` drivers.** A backend language whose SQL is
   in memory only is the largest user visible hole; the driver interface
   may change once a real driver (Postgres, SQLite) is written against
   it, which is a breaking risk if 1.0 freezes the current surface. Next:
   write a Postgres driver behind the existing `std/sql` API on the Go
   backend (`use go` of `pgx` inside std) and a golden against a local
   server, noting any API change it forces.
2. **12.1 SBOM.** The spec states every build writes `<bin>.sbom.json`;
   it is supply chain metadata people script against, so its format is a
   compatibility surface. Next: emit it from the build list (`mod.Load`
   modules, `require go` pins, Go module versions) in internal/build,
   tested beside `TestReproducibleBuild`.
3. **12.2 `std/sys/<os>` and `XO1001`.** Reserved code, no module: adding
   platform modules after 1.0 is additive, so low break risk, but the
   `target` arm rule should be exercised once before freezing. Next: a
   minimal `std/sys/linux` (`getrlimit`) and a check test for XO1001.
4. **1.2 doc comment examples under `xo test`.** Additive, but it changes
   what `xo test` runs, so a 1.0 project could start failing when it
   lands. Next: extract fenced blocks of `///` comments in internal/doc
   and run them as tests.
5. **5.5 `arena scope` blocks.** Syntax reserved; additive. Next: decide
   whether 1.0 keeps the keyword reserved (an ADR), then build it.
6. **10.1 / 11.3 / 11.5 debugger (`xo why`, `xo debug`, `--replay`,
   `--json`, `--dap`).** Tooling, additive, no language break. Next:
   `xo debug --json` over the existing replay engine (internal/gen
   replay), breakpoints by `//line` locations.
7. **11.6 spans and OpenTelemetry.** Additive std surface. Next: an ADR on
   whether spans are a `log` extension or a `std/trace` package.
8. **2.9 `OpenApi`/`Proto` derives.** Owner placed after 1.0.

Open decisions: Appendix B has no open question left (1 to 6 decided;
0090 to 0093). The only Proposed ADR is 0040 (performance path), which
does not affect the language surface. The test coverage holes above
(session tickets, 0118 runtime behaviour, 0098 binary trimming) have
tests since 6e65cc5 and 153f721.

### What native lacks to leave preview

1. ~~**Native test binaries match the Go backend's reports**~~ (fixed,
   e9b0f00): test_logs, test_once, test_shutdown.
2. **Error frames outside main's report**: test failure reports, handler
   logs (spec 11.1 says so explicitly), and fault frames.
3. **Crash bundles (11.2)** are not written natively.
4. **Record and replay** (`--record`, `xo replay`) do not exist natively,
   so `use c` calls are not recorded either (Appendix B 5 note).
5. ~~**`==` on `http.Request`**~~ (fixed, 63755b4).
6. **`TestGenGoldensNative`, `TestNoGCTests` run only in full mode**;
   `TestGolden` runs two goldens under `-short` since 63755b4. Next: a
   short subset of `TestNoGCTests` (test_once is about 0.3 s warm).
7. `use go` stays impossible natively by design (Go code needs Go's
   runtime); it is a documented XO1201, not a preview blocker.

Next steps, ranked by likelihood of closing a native preview item fast:
error frames in native test reports (reuse decision 0104's frame list in
xo_testing.c's failure printer), then a `-short` subset of
`TestNoGCTests`, then crash bundles (11.2) natively.

## Test status

Fixes pass (2026-10-10, after 153f721): `go test -short ./...` and the
full runs of the changed packages (cmd/xo, internal/build, internal/llvm,
internal/ssa), `go vet` of them, and the std/llrt C tests under
ThreadSanitizer (`TestRuntimeTasks`, the other threads_test.go tests).

Original pass:

`go test -short ./...` at this commit: all 30 packages with tests pass
(2026-10-10). No Go package changed in this pass (documents only), so no
full package run was needed; the fault_defer_chain check above ran the
program by hand on all three backends.
