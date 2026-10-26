# amd64 against arm64: what the native backend does differently (2026-10-26)

Milestone 0.33.0, exit criterion 2 ("amd64 at arm64 parity"). A code
audit of compiler/ at origin/main fe1a63473 (the darwin/amd64 hollow
default below was then in review, xo-lang-4): every optional pass in compiler/arm64 passes.xo
and every architecture gate found in compiler/xo, compiler/arm64 and
compiler/ssa, read at its call site. No build or measurement went into
this table; the impact column is an estimate from the code comments and
the decisions, and the gaps are closed (and measured) in that order.

compiler/arm64 holds both native backends. The x86 path leaves the shared
pipeline in isel.xo `function()` at the `if g.x86` branch (isel.xo:76):
it runs place_latches, mark_tails, the x86_* passes and x86_emit, and
returns before reuse_flags, shrink_split (except as noted) and
cluster_mem (isel.xo:135-149).

## Gaps, by estimated user impact

| feature | arm64 | amd64 | evidence | impact |
|---|---|---|---|---|
| per-function and part backend cache (fn/, the pack) | on by default | off on every OS ("no per-function cache for amd64 yet (its canonical text is arm64's)") | compiler/xo/main.xo:1451-1455; decisions/0254-amd64-backend.md:559-561 | edit rebuild: every amd64 edit regenerates every function and part (the arm64 self build went 30.6 s to about 11.3 s with these caches, toolchain.md) |
| lazy inline-cache decode (decision 0283 stage B) | on | off (`march == "arm64"`) | compiler/xo/main.xo:1389 | edit rebuild time and memory |
| deferred inline verification | on | off (`march == "arm64"`, shares the fn cache's keys) | compiler/xo/main.xo:2486-2488 | edit rebuild: keys computed twice per edit (0.2 to 0.5 s for xo per the comment) |
| wide crypto intrinsics (chacha blocks4/stream4, ghash4, aes x4, aes_ctr, aes_gcm, aes_gcm8) | NEON, AES, PMULL | portable Xo bodies | compiler/ssa/call.xo:218-232; decisions 0329, 0339 | TLS and crypto throughput (single-block AES-NI, PCLMULQDQ, SHA-NI and ML-KEM SSE2 exist on amd64: compiler/arm64/calls.xo:1613-1665, 1823, 1865) |
| hollow on windows/amd64 | darwin, linux | darwin and linux amd64 on; windows off (hollow.xo scans Mach-O and ELF only) | compiler/xo/main.xo:1493; compiler/arm64/hollow.xo:61-72 | Windows binary size (hello -27% on the other amd64 targets, decision 0371) |
| incremental hollow and the object cache on windows/amd64 | darwin, linux | windows: no incr_hollow; COFF parts assembled one after another, uncached | compiler/xo/main.xo:1500, 1601-1611 | Windows edit rebuild |
| Float.fma in hardware | one fmadd | software body (about 10.9 ns against 1.62 ns a call) | compiler/ssa/call.xo:164, 2066; decision 0385 (stage 3, vfmadd231sd behind CPUID, planned) | numeric code that calls fma |
| reuse_flags (MIR) | on | not run: each branch keeps its compare | compiler/arm64/isel.xo:76-79, 135-137 | speed and size: one compare a branch |
| cluster_mem (MIR) | on | not run | compiler/arm64/isel.xo:147-149 | memory operation grouping |
| str.cmp in line | memcmp plus csel/cneg | export call xo_x_obj_str_cmp ("x86_64 retargets no cneg yet") | compiler/arm64/calls.xo:1293-1296; the x86 emitter does retarget cneg now (x86.xo:346, 682, 1231), so the gate may be stale (to check) | Str sorting and comparison |
| `s == "lit"` in line (str_eq_const) | in line | call to xo_x_obj_str_eq | compiler/arm64/calls.xo:1312 | Str compares against literals in loops |
| sized-int range test as one compare (ext_test, ext_add) | on | off | compiler/arm64/isel.xo:3216-3218, 3256; bench/results/amd64-native-vs-llvm-2026-10-22.md (gap 2) | Int8/16/32 arithmetic checks |
| carry-chain products (chain_products) | on | off (imul writes flags) | compiler/arm64/calls.xo:2933-2938 | bignum code (RSA, P-256) |
| load memo across a loop's preempt poll | kept | cleared ("amd64 keeps it until measured there") | compiler/arm64/calls.xo:2412-2423 | loads redone after each back edge poll |
| slp_f64 (paired Float operations) | NEON | not implemented (returns on g.x86) | compiler/arm64/slp.xo:170-172; decision 0323 (SSE2 addpd is its own decision) | Float-heavy loops |
| `--inline-str` (decision 0370, experimental, off by default) | opt in | refused with a warning | compiler/xo/main.xo:678-681; compiler/arm64/calls.xo:86-87 | memory when asked for |
| shrink_split on windows/amd64 | on | darwin and linux on, windows off (Win64 unwind describes one prologue) | compiler/arm64/isel.xo:157-161 | Windows functions with cold paths |
| arg_slots with shrink wrapping | on | off for x86 functions that shrink wrap | compiler/arm64/regalloc.xo:434-441 | small |
| rt.ticks | mrs cntvct_el0 | portable body (mono_ns) | compiler/ssa/call.xo:224-226 | profiler and timing overhead (estimate: small) |
| heap scavenge (decision 0383) on windows | darwin, linux | windows: a no-op; phys_mem() is 0 (no default budget) | std/native/llrt/heapos.xo:686-708, 189 | Windows memory (an OS gap, not an architecture one) |
| locals in tracebacks, PDB on windows | DWARF lines and variables | darwin and linux amd64 the same; windows: no locals in XO_TRACEBACK=locals, PDB stage 1 (functions and publics) | std/native/llrt/fault.xo:318-322; compiler/macho/pdb.xo:1-18 | Windows debugging |
| `soft_cpu` in the fe cache key | not set | set for `--backend=amd64` only; nothing else reads it | compiler/xo/main.xo:1957-1958; compiler/xo/bcache.xo:431 | extra fe cache misses between `--backend=amd64` and a default build on an x86 host |
| XO_POISON_FRAMES, XO_OP_VERIFY | available | ignored on x86 | compiler/arm64/emit.xo:431-485, 819 | developer tooling only |

## Different by design (not gaps)

- Post-emit text peepholes: arm64 has copy_runs, borrow_flag_peep,
  reload_reuse, pair_mem, far_slots, far_tests, local_empty; x86 has its
  own (x86_copy_runs, x86_imul3, x86_r11_reuse, x86_not_reuse, x86_rmw,
  x86_reload_peep), emit.xo:430-513 and x86emit.xo:1704-2099. pair_mem
  and the far_* passes are arm64 encodings.
- logic_ror, xor_ones, addr_fold: arm64 operand forms x86 lacks or does
  differently (isel.xo:1176-1180, 1630-1631; calls.xo:292).
- x86_red_zone is off on windows by the ABI (x86emit.xo:195).

## At parity (checked)

- Every module and SSA pass from map_core through inplace_elems, and the
  MIR passes mir_dce, thread*, jump_tables (x86 lowering at
  x86emit.xo:716), live_dce, select_compares, hoist_consts,
  hoist_invariants, copy_prop, reuse_loads, place_latches, mark_tails,
  cold_saves (regalloc.xo:340-347).
- List packing (decision 0369) on amd64: 59f191adb (main.xo:676,
  gen.xo:620, x86.xo:1347, x86rt.xo:336).
- Hollow: linux/amd64 by default since 2026-10-25; darwin/amd64 is
  `XO_PASSES=+hollow` until its default change lands (decision 0371); incremental hollow on darwin
  and linux amd64 (1869a496a; asmparts.xo:264, 356, 414 use ud2).
- `--race` (isel.xo:69-74), async preemption (ssa/build.xo:476), the
  profiler (std/native/llrt/prof.xo), DWARF lines and variables on darwin
  and linux, and the intrinsics clz, ctz, popcount, bytes_eq16,
  case_fold16 and prefetch.

## Not determined yet

- The edit-rebuild cost the missing amd64 fn and part caches have: no
  amd64 measurement exists; the first gap's work measures it first.
- Whether the str.cmp gate is stale (x86 retargets cneg now).
- Whether x86 needs analogues of the arm64-only peepholes (for example
  borrow_flag_peep); needs a codegen comparison.
- The size of a COFF hollow scan for windows/amd64.

## Order of work

1. Edit loop (criterion 3 also): the per-function and part cache on
   amd64, then lazy inline decode and deferred verification, measured on
   llvm-fib and the HTTP server with `xo bench`'s edit rebuild on
   darwin/amd64 and linux/amd64 (Docker).
2. TLS throughput: the wide crypto intrinsics on amd64 (AES-NI x4,
   PCLMULQDQ ghash4, SSE2 ChaCha x4), with the Go differential corpora.
3. Small codegen gaps, each with `--ab` on linux/amd64 in Docker and the
   Intel laptop: the str.cmp gate, str_eq_const, reuse_flags, ext_test,
   cluster_mem, the preempt-poll load memo, chain_products.
4. Windows: hollow on COFF, the object cache, shrink_split.
5. Float: fma (decision 0385 stage 3) and slp_f64 on SSE2 (own decision).
