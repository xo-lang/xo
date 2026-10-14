# regexp_table's wasm module traps now and then under XO_LEAKCHECK=1

Status: Observed (2026-10-14), open.

Symptom: TestSelfhostWasm failed once in the linux/amd64 milestone run at
main f8ebc9ec (node v22.22.1): `regexp_table: leak check: "wasm trap:
memory access out of bounds\nxo leakcheck: 29 live of 3453
allocations"`. The plain run of the same module passed. The linux/arm64
run of that commit passed, and so did the rerun on amd64 at 775d7677
(133 pass, 0 fail of 154).

What is known (EC2 jobs msfix-amd64c and msfix-wasmrepro-arm64,
2026-10-14, node v18.20.8): xo1's `xo build --backend=wasm` module of
internal/llvm/testdata/golden/regexp_table has the same SHA-256 on both
hosts (adbe1b48...). Run 8 times with XO_LEAKCHECK=1, it trapped on the
first run on amd64 (`24 live of 75015 allocations`, exit 134) and passed
the other 7 (`0 live of 333533`). All 8 passed on arm64. So the module
does not depend on the host. Its run is not deterministic: the trap
comes at a different allocation count each time (3453, 75015).

Likely cause, not yet confirmed: something that changes between runs of
one module, such as the hash seeds from WASI random_get or the clock,
picks a path that reads out of bounds. The leak check's bookkeeping is
also only on in this mode.

Next experiment: run the module 200 times with XO_LEAKCHECK=1 and record
which runs trap. Then pin random_get in selfhost/wasm/run.mjs to a fixed
seed per run, so a trapping seed can be replayed and bisected. When it
is fixed, flip this entry to fixed with the cause.
