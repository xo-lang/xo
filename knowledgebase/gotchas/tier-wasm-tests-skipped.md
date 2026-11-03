# The tier's wasip2 command tests passed by skipping

Status: Observed (2026-11-03, checked when TestWasmCommandImports landed)

Symptom: the lead's tier ran internal/selfhost on the second linux/arm64
host and reported it ok, but TestWasmCommandGoldens,
TestWasmComponentServe and TestWasmCommandImports never ran there: a
job's PATH on that host is /opt/llvm/bin, Go and the system directories,
and wasmtime and wasm-tools live in /work/tools/bin (offload.md). Each
test skips when its tool is missing (so the Mac, which lacks them, can
run the package), and the tier's step runs without -v, so nothing
printed SKIP.

Cause: the tools were installed for agents' scripts, which export
PATH=/work/tools/bin:$PATH themselves; scripts/lead/offload-tier.sh
never did.

Fix: offload-tier.sh puts /work/tools/bin first on PATH and sets
XO_TEST_WASM_TOOLS=1, under which needWasmTools (wasm_cmd_test.go)
fails a test whose tool is missing instead of skipping it.

How to avoid: a test that skips without a tool gets a variable that
makes the skip a failure, and the environment that is meant to run it
sets that variable; a pass of a package says nothing about a test that
can skip.
