# TestSelfhostWasm with -run on one golden still builds every golden

Status: Observed (Xo Lang 2, 2026-10-26).

## Symptom

A gate ran `go test -run 'TestSelfhostWasm/var_patterns$' ./internal/selfhost`
to check one new golden on wasm. It failed after 600 s with
`panic: test timed out after 10m0s`, in internal/selfhost/wasm_test.go
around line 95 (`cmd.Output()` of compiler/wasm), before any subtest ran.

## Cause

TestSelfhostWasm compiles every directory of
internal/llvm/testdata/golden with one `compiler/wasm --each` run, then
runs the subtests over the results. A `-run` filter on a subtest
narrows only the second part, so the build still takes the full time:
about 14 minutes on the Mac on 2026-10-26 (834 s for the whole test).
That is longer than `go test`'s default 10 minute timeout.

## Fix

Give the run a timeout: `go test -timeout 60m -run 'TestSelfhostWasm/NAME$'
./internal/selfhost`, through heavy.sh in the bulk class (it is not a
short gate step). Leave it out of a gate unless the change touches what
wasm builds (compiler/wasm, the shared front end, a new rt intrinsic):
the full tier runs it.
