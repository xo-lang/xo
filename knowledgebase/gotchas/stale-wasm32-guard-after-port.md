# A wasm32 guard outlives the port that made it needless

Status: Observed 2026-11-21 (task #577)

## Symptom

TestWasmCommandGoldens/http_idle_pings failed in tier 919 at 0.55.0
candidate 6 (wasip2 under wasmtime, host 2): the case "a request during an
idle PING" printed `connections 2`, want `connections 1`. arm64 and LLVM
passed, and so did the push's gate.

## Cause

Task #474's fix in std/native/llrt/hclient.xo `pool_take` (a request waits
for a connection that is out for its idle PING) began with
`if pc != 0 || target.arch == TargetArch.Wasm32 { return pc }`. It was
written when the idle pinger did not run on wasm. Task #492 started the
pinger on every target the same day, and #474 was rebased onto it with
the guard kept. So on wasip2 the PING ran but the wait never did, and the
request dialed a second connection every time. The gate's `wasm` step
runs TestSelfhostWasm, which skips network goldens, so only the tier's
TestWasmCommandGoldens on a host with wasmtime ran the new case.

## Fix

Drop the guard, and give the wait a wasm sleep: poll.sleep_until parks
through `rt.fn_addr`, which wasm lacks (a first fix that only dropped the
guard failed 10 of 10 wasip2 runs), so on wasm32 the loop sleeps with
`rt.clock_sleep_or_cancel`, as fsops' retry loop does. Before keeping a `target.arch ==
TargetArch.Wasm32` early return in llrt, check that the feature it skips
is still native only (`git log -S` on the feature's wasm start). A
change to a runtime path that a network golden covers runs that golden
under TestWasmCommandGoldens on host 2 (`XO_TEST_WASM_TOOLS=1
XO_WASM_GOLDENS=NAME go test -run '^TestWasmCommandGoldens$'
./internal/selfhost`, with /work/tools/bin on PATH).
