# Decisions

Format: `NNNN-short-title.md` with sections Status (Proposed, Accepted, Superseded), Context, Decision,
Consequences, Alternatives. Never delete a decision; mark it `Superseded by NNNN`.

- [0001-name-xo.md](0001-name-xo.md): language is named Xo, extension `.xo`.
- [0002-transpile-to-go-first.md](0002-transpile-to-go-first.md): P1 compiles to Go source.
- [0003-value-semantics.md](0003-value-semantics.md): no pointers, immutable persistent collections, move only Buf. **Proposed.**
- [0004-effects-and-capabilities.md](0004-effects-and-capabilities.md): `uses` clauses plus capability values from `main`. **Proposed.**
- [0005-no-build-tags.md](0005-no-build-tags.md): platform code via compile time `match target.os`, all arms checked.
- [0006-directory-is-module.md](0006-directory-is-module.md): no module declaration in source files. **Proposed.**
- [0007-structured-concurrency.md](0007-structured-concurrency.md): scopes replace goroutines and ctx.
- [0008-errors-and-faults.md](0008-errors-and-faults.md): typed `! E`, no nil, faults are separate from errors.
- [0009-block-scoped-defer.md](0009-block-scoped-defer.md): `defer` runs at block exit. **Proposed.**
- [0010-naming-enforced.md](0010-naming-enforced.md): naming rules are compiler errors; `pub` marks exports. **Proposed.**
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
- [0040-path-to-beating-go-performance.md](0040-path-to-beating-go-performance.md): where Xo could beat Go and how to test it. **Proposed.**
- [0041-go-library-interop.md](0041-go-library-interop.md): `use go` bindings to Go packages until Xo has its own ecosystem (implemented, core 4.4).

- [0042-two-phase-shutdown.md](0042-two-phase-shutdown.md): first signal requests a stop, cancellation after a grace period.
