# An amd64 gate skips the arm64-only tests

Status: Observed (2026-10-30, b2cf3a516, fixed forward eb8b70dd6)

Symptom: a change gated on the linux/amd64 offload host passes, and main
fails a test it added. Decision 0437's TestWholeBuildDebugEnv (cmd/xo)
checks `xo build` on arm64 only; the gate ran on the amd64 host because
the arm64 host was busy, the test skipped there, and the push landed with
a test that failed on every arm64 machine (XO_DUMP_FN was keyed in the
backend caches, so a repeated dump build reused the function's object
and wrote no dump).

Cause: many tests in cmd/xo and internal/arm64 skip unless the host is
arm64 (`native()`, a backend list from `nativeBackends(t)` whose first
entry is the host's), and a skip prints nothing in a package's `ok` line.

Fix: a CLI, cache or native change gates its arm64 tests on arm64: the
Mac (heavy.sh gate) or the linux/arm64 host (2 slots since 2026-10-30).
The amd64 host is an addition for x86 code and Linux-only behavior, never
a substitute (lead, 2026-10-30). Before pushing, run the change's own new
tests by name on arm64 with `-v` and read their PASS lines: a skipped
test says SKIP.

A second miss of the same push: the gate piped `go test ./cmd/xo` through
`tail -15`, so it showed 2 failures while the run had more
(TestAgentLookupsEager, TestMCPSession, TestQueryStdModuleOutsideModule:
0437's "not inside a module" check refused a root of modules and `query
api std/json`; fixed forward 1e4f0f213). A gate keeps the whole output
in a file and prints every `--- FAIL` line (`grep -E "^(--- FAIL|ok|FAIL)"
OUT`), never a tail of it.
