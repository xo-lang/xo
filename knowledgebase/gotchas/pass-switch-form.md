# A new pass switch in a form TestPassesListed does not read fails it

Status: Observed (2026-10-25).

Symptom: main went red on internal/repotest TestPassesListed ("passes in
passes() that nothing checks (turning them off does nothing): hollow")
after the hollow pass landed (cc467b993). The pass worked, and
XO_PASSES=-hollow turned it off.

Cause: the test finds a pass's switch only as `g.on("NAME")`,
`pass_on(X, "NAME")` or `passes_off.contains("NAME")`. compiler/xo's
gate read `passes_off(os).contains("hollow")`, so the test saw no
switch. The fast repo checks a push runs (TestOutsideBytesValidated and
the rest) do not include TestPassesListed, so only the 2-hourly
`-short` run found it.

Fix: compiler/xo's `pass_on(os, name)` (fa17f5486). Write every new
switch in one of the three forms, and run `go test ./internal/repotest
-run TestPassesListed` in the gate of any change that adds a name to
compiler/arm64 passes.xo.
