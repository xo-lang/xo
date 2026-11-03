# A wasm golden's node runner outlived its test

Status: Observed (2026-11-02, Xo Lang 2)

## Symptom

A `node --no-warnings .../compiler/wasm/run.mjs .../TestSelfhostWasm.../zzl2p_19.wasm`
process used a full core for 12 h 25 min, its parent PID 1, its worktree
(l2-astress) already deleted. The lead found it in a process listing.

## Cause

internal/selfhost's runWasm started node with a bare `exec.Command` and
`cmd.Run()`. When the test process ended without running its cleanup (a
`go test -timeout` panic, or the run stopped from outside), the runner was
reparented to launchd and kept running a golden that never finished
(the alloc_stress work, whose program then looped under wasm).

## Fix

runWasm wraps the command in `testtmp.Guard`, as the other wasm runners in
internal/selfhost (wasm_http_test.go, the build steps) already did: the
child runs behind the lifeline wrapper and gets SIGTERM, then SIGKILL,
when the test process ends however it ends. Every runner a test starts
(node, wasmtime, a built program) goes through testtmp.Guard or
testtmp.Lifeline.

A runner already orphaned is stopped by its exact PID, after checking its
command line is your own test's; never by name or pattern.
