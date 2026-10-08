# Decisions

Format: `NNNN-short-title.md` with sections Status (Proposed, Accepted, Superseded), Context, Decision,
Consequences, Alternatives. Never delete a decision; mark it `Superseded by NNNN`.

- [0001-name-xo.md](0001-name-xo.md): language is named Xo, extension `.xo`.
- [0002-transpile-to-go-first.md](0002-transpile-to-go-first.md): P1 compiles to Go source.
- [0003-value-semantics.md](0003-value-semantics.md): no pointers, immutable persistent collections, move only Buf.
- [0004-effects-and-capabilities.md](0004-effects-and-capabilities.md): `uses` clauses plus capability values from `main`.
- [0005-no-build-tags.md](0005-no-build-tags.md): platform code via compile time `match target.os`, all arms checked.
- [0006-directory-is-module.md](0006-directory-is-module.md): no module declaration in source files.
- [0007-structured-concurrency.md](0007-structured-concurrency.md): scopes replace goroutines and ctx.
- [0008-errors-and-faults.md](0008-errors-and-faults.md): typed `! E`, no nil, faults are separate from errors.
- [0009-block-scoped-defer.md](0009-block-scoped-defer.md): `defer` runs at block exit.
- [0010-naming-enforced.md](0010-naming-enforced.md): naming rules are compiler errors; `pub` marks exports.
- [0011-explicit-failure.md](0011-explicit-failure.md): `return Err(e)`, `Never` type.
- [0012-patterns-and-unused.md](0012-patterns-and-unused.md): full patterns, `_`, unused rule relaxed.
- [0013-prelude-variants-uppercase.md](0013-prelude-variants-uppercase.md): `Ok Err Some None`, implicit `Some`.
- [0014-effect-variables.md](0014-effect-variables.md): effect variables, subsumption, `uses (a, b)` in types.
- [0015-scopes-and-cancellation.md](0015-scopes-and-cancellation.md): background tasks, exit rules, observable cancellation.
- [0016-lost-update-check.md](0016-lost-update-check.md): XO0520, `Map.update`, `Mutex.with` semantics.
- [0017-test-os-and-testing.md](0017-test-os-and-testing.md): implicit test `os`, `std/testing`.
- [0018-ambient-logging.md](0018-ambient-logging.md): logging needs no capability.
- [0019-stdlib-surface-and-http-errors.md](0019-stdlib-surface-and-http-errors.md): `spec/std.md`, `StatusError`.
- [0020-slicing-literals-grammar.md](0020-slicing-literals-grammar.md): slicing, `#{}`, closures, trailing commas.
- [0021-dollar-interpolation.md](0021-dollar-interpolation.md): `${x}` interpolation, literal braces, `r#"..."#`.
- [0022-shielded-defer.md](0022-shielded-defer.md): `defer` shielded from cancellation.
- [0023-narrow-lost-update-check.md](0023-narrow-lost-update-check.md): XO0520 only for values copied out of collections.
- [0024-fs-root-and-signals.md](0024-fs-root-and-signals.md): `os.fs` root, lazy `sub`, SIGINT/SIGTERM.
- [0025-module-local-names.md](0025-module-local-names.md): `-` to `_` in module local names.
- [0026-option-question-and-leading-dot.md](0026-option-question-and-leading-dot.md): `?` on Option, leading `.` continuation.
- [0027-spec-0-3-clarifications.md](0027-spec-0-3-clarifications.md): grouped clarifications.
- [0028-grammar-readings.md](0028-grammar-readings.md): grammar readings from the P1 parser.
- [0029-typing-readings.md](0029-typing-readings.md): typing readings from the P1 checker.
- [0030-is-operator-and-small-syntax.md](0030-is-operator-and-small-syntax.md): `x is Pattern`, compound assignment, `t.0`.
- [0031-ignore-err-with-reason.md](0031-ignore-err-with-reason.md): `let _ =` on a Result is XO0206; `ignore_err("reason")`.
- [0032-task-groups.md](0032-task-groups.md): `task.Group`, `os.tasks`, `try_send`, `map_concurrent`.
- [0033-effect-elision.md](0033-effect-elision.md): effect arguments of types may be omitted.
- [0034-widen-lost-update.md](0034-widen-lost-update.md): XO0520 covers or_err copies and post store mutation.
- [0035-std-0-4.md](0035-std-0-4.md): std 0.4 additions.

- [0036-spec-0-4-readings.md](0036-spec-0-4-readings.md): readings from implementing 0.4 (effect elision, group failures, run_main).
- [0037-store-back-and-0-5-fixes.md](0037-store-back-and-0-5-fixes.md): XO0520 requires store back; `is` with `&&`; arm return position.
- [0038-map-index-wildcards-split-sections.md](0038-map-index-wildcards-split-sections.md): XO0410 no Map index reads; XO0305 and XO0804 warnings.
- [0039-regressions-in-module.md](0039-regressions-in-module.md): regression tests go in the module's `regressions_test.xo`.
- [0040-path-to-beating-go-performance.md](0040-path-to-beating-go-performance.md): where Xo could beat Go and how to test it.
- [0041-go-library-interop.md](0041-go-library-interop.md): `use go` bindings to Go packages until Xo has its own ecosystem (implemented, core 4.4).

- [0042-two-phase-shutdown.md](0042-two-phase-shutdown.md): first signal requests a stop, cancellation after a grace period.
- [0043-channel-loops-once-deadline-map.md](0043-channel-loops-once-deadline-map.md): `for x in ch`, `task.Once`, `map_concurrent_until`.
- [0044-semantic-edit-rules.md](0044-semantic-edit-rules.md): `xo edit` resolves through the checker, rechecks before writing, add-field backfills literals, change-sig keeps argument order.
- [0045-shared-ssa-ir.md](0045-shared-ssa-ir.md): `internal/ssa`, the typed SSA IR (block parameters, value semantics, RC hooks) native backends lower from.
- [0046-reference-counting-ownership.md](0046-reference-counting-ownership.md): native memory is reference counted with Perceus style ownership per heap leaf on the SSA IR, borrow inference, in place reuse when unique, verified per path.
- [0047-mutex-barging-in-real-runs.md](0047-mutex-barging-in-real-runs.md): the runtime lock barges in real runs, stays FIFO under test and the deterministic scheduler.
- [0048-ssa-closures-boxes-generics.md](0048-ssa-closures-boxes-generics.md): closures as a counted environment leaf, recursive types as boxes, generics monomorphized in the SSA builder; deferred drops free long chains with a worklist.
- [0049-native-arm64-backend.md](0049-native-arm64-backend.md): `internal/arm64` lowers the SSA IR after reference counting straight to AArch64 assembly (word layout, range set allocation with coalescing, AAPCS64, fault stubs), linked with cc; darwin and linux arm64.
- [0050-arm64-closures-boxes-equality.md](0050-arm64-closures-boxes-equality.md): the arm64 backend lowers closures (`blr x0`, deferred environment drops), boxes, structural equality helpers, and values past eight words (stack helper arguments, results in the caller's stack argument area).
- [0051-arm64-match-dispatch.md](0051-arm64-match-dispatch.md): native arm64 lays out `match` compare chains as runs of tests and uses jump tables only from 8 values (tables lose on unpredictable tags on Apple M4).
- [0052-recursive-boxing-from-type-graph.md](0052-recursive-boxing-from-type-graph.md): the SSA builder boxes recursive types from the checker's type definition graph, one box per cycle at the edge out of its least user type by name, independent of the order types are first met (supersedes 0048's boxing rule).
- [0053-float-comparison-total-order.md](0053-float-comparison-total-order.md): Float `==` is IEEE (NaN differs from itself, also in aggregates); `<`, `<=`, `>`, `>=`, sort, min/max/clamp use the Ord total order (NaN last, -0.0 equal to 0.0, receiver kept on a tie); `-0.0` is negative zero; native lowering with `fccmp` / paired `fcmp`.
- [0055-http-request-values-scoped-middleware.md](0055-http-request-values-scoped-middleware.md): std/http typed request values (`http.Key[T]`, keyed by name and type, process local), middleware that may fail (`wrap` takes `! E`, mapped like handler errors), `Router.mount(prefix, sub)` for scoped middleware, `Listener.addr()` / `Server.addr()`, `Request.with_header`.
