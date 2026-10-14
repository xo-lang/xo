# regexp_table's wasm module traps on linux/amd64 hosts, depending on timing

Status: Observed (2026-10-14), open, parked. Owner UPDATE 20 (2026-10-14):
the gate is arm64 only, and x86-only failures are recorded, not chased.

Symptom: TestSelfhostWasm failed once in the linux/amd64 milestone run at
main f8ebc9ec (node v22.22.1): `regexp_table: leak check: "wasm trap:
memory access out of bounds\nxo leakcheck: 29 live of 3453
allocations"`. The plain run of the same module passed. The linux/arm64
run of that commit passed.

Measurements (EC2, node v18.20.8 unless noted; module = xo1's `xo build
--backend=wasm` of internal/llvm/testdata/golden/regexp_table, SHA-256
adbe1b48... on both architectures):

| host | runs | at once | mode | traps |
|---|---|---|---|---|
| amd64 c7i.8xlarge | 8 | 1 | XO_LEAKCHECK=1 | 1 (msfix-amd64c) |
| amd64 | 200 | 1 | XO_LEAKCHECK=1 | 1 (msfix-wasm200-amd64) |
| amd64 | 200 | 1 | plain | 0 |
| amd64 | 400 | 16 | XO_LEAKCHECK=1 | 18; again 24 (msfix-wasmtier, msfix-wasmmx) |
| amd64 | 400 | 16 | `--liftoff --no-wasm-tier-up` | 23 |
| amd64 | 400 | 16 | plain (no leak check) | 24 |
| amd64 | 400 | 16 | no worker (main thread, --stack-size=7000) | 43 |
| amd64 | 400 | 16 | without run.mjs's three V8 flags | 34 |
| amd64 | 400 | 16 | worker stack 64 MB | 31 |
| amd64 | 400 | 16 | XO_WASM_MEMTRACE (MD5 of memory in JS before every WASI call) | 400 (msfix-wasmdiv-amd64) |
| arm64 (EC2 r8g) | 8, 200, 5 x 400 | 1 and 16 | every mode above | 0 |
| arm64 Mac, node v22.22.1 | 40 and 160 | 1 and 10 | XO_LEAKCHECK=1, and MEMTRACE | 0 |

The "leak-notraphandler: 200 bad of 200" line of msfix-wasm200-amd64 and
-arm64 measured nothing: node v18 rejects `--no-wasm-trap-handler` as a
command line option (`node: bad option`, exit 9), so the module never
ran. Pass V8 flags through XO_WASM_V8_FLAGS instead.

What the data says:
- Not the seed. run.mjs now seeds random_get (XO_WASM_SEED, printed on
  the `xo wasm seed:` line), but the module makes no random_get and no
  clock_time_get calls. Its only WASI call is fd_write (1144 in a good
  run), which never returned an error. So there is no seed to replay. The
  program's inputs are identical in every run; only timing differs.
- Not V8's tier-up (Liftoff only traps as often), not the worker or its
  stack, and not run.mjs's V8 flags.
- Every trap happens while memory is still 2 pages (131072 bytes), early
  in the run (fd_write calls 16 to 196 of 1144), at a few fixed sites:
  rc_dec <- drop10 <- f28 (most), or idx_chk <- f78 <- f28/f78. A good
  run grows to 5 pages at call 661.
- More JS work inside the host calls makes it worse: 16 runs at once
  raise the rate from about 0.5% to about 6%, and hashing memory in each
  call made all 400 runs trap at fd_write call 20 to 26. On arm64 the same
  traced runs are clean and their traces are identical run to run (1145
  hashes). The program is deterministic, so a memory difference between
  two amd64 runs means the engine or the host-call path changed wasm
  state. The suspects are the x64 wasm-to-JS import call (or a GC during
  it) in node 18 and 22, or a value the module keeps that does not
  survive an import call on x64.

Next experiments (amd64, when x86 is picked up again; run.mjs has the
switches):
1. Traced runs trap every time, so diff memory without a good reference:
   dump with XO_WASM_MEMDUMP_AT=19 under MEMTRACE on amd64, compare with
   the arm64 dump at call 19, and list the differing bytes. Script:
   scratchpad msfix/wasm/diverge.sh. Its first run failed because the
   reference run itself trapped.
2. Separate the GC from the JS work: in the fd_write wrapper, call
   `gc()` (node --expose-gc) instead of hashing, then do pure arithmetic
   without allocating, and compare the trap rates.
3. Run one module on node 22 and node 20 on amd64 to see whether this is
   specific to V8 x64 (with step 2, the size of a V8 bug report).
4. Disassemble f28 around 0xd75d/0xd786 (wasm-objdump or `xo dev dump`)
   and list what drop10 and idx_chk read across the fd_write calls just
   before.
