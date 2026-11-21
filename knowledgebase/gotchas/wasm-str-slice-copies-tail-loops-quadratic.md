# A loop that re-slices a Str's rest is quadratic on wasm

Status: Observed (2026-11-20, task #559)

Symptom: a std test that is linear natively times out on wasm only: std/mime's
"decode_header on many invalid encoded words is linear (GO-2026-5038)" took
0.03 s natively and over 60 s under `xo test --backend=wasm` ("the test did not
finish within 1m0s of real time", the lead's tier 918 stdwasm step). A Node
`--cpu-prof` of the wasm module put 95% of the time in `rs_append` under
`str_slice`.

Cause: decision 0330 makes `s[a..b]` a view on arm64, x86 and LLVM; wasm (and
no-prelude builds) still copy (compiler/wasm/rt.xo `$str_slice`). A loop of the
form `h = h[end..]` per token copies the remaining text each step: O(n^2) on
wasm, O(n) natively, so native timings and tests do not show it.

Fix: walk the text with an offset and slice only the pieces kept
(std/mime.xo `decode_header_by` and `find_from`). To check a std loop on wasm,
time its module with `XO_TEST_EXEC="node --no-warnings $PWD/compiler/wasm/run.mjs"
bin/xo test --backend=wasm --std-module=NAME`, and profile a probe with
`node --cpu-prof compiler/wasm/run.mjs prog.wasm` (the worker's profile is the
second file). The lasting fix is wasm Str views, the open question in 0330.
