# amd64 at arm64 parity, per target (2026-10-27)

Milestone 0.33.0, exit criterion 2. This table follows
[amd64-parity-audit-2026-10-26.md](amd64-parity-audit-2026-10-26.md),
splitting each row by target and re-checking each gate in the code at
origin/main aaf2541c3 (xo-lang-4; the code gates were read, and
each cited commit was confirmed with git log). The 2026-10-26 audit's
rows for the fn/part cache, lazy inline decode, deferred inline verify
and Float.fma are closed (14a046c55, 0be785fc9).

States: **done** (on by default, with its evidence), **missing** (gated
off or not implemented), **partial**, **untested** (shared code that
should work, with no test or run on that target), **n/a**.

## Edit loop and build pipeline

| feature | arm64 | linux/amd64 | darwin/amd64 | windows/amd64 | evidence |
|---|---|---|---|---|---|
| list packing (pack=1, decision 0369) | on | done | done | done | compiler/xo/main.xo:676 (no OS gate); 59f191adb (2026-10-26) |
| hollow (decision 0371) | darwin, linux | done (ec383109d, 2026-10-25) | done (abfac9234, 2026-10-26) | missing: hollow.xo scans Mach-O and ELF only | main.xo:1497; compiler/arm64/hollow.xo:61-72; internal/arm64/dce_test.go TestHollowUnreachedAmd64 |
| incremental hollow (incr_hollow) | on | done: HTTP edit hollow 196 to 86 ms | done | missing: hollow (above); the COFF objects are cached since e6e5558ec (next row) | main.xo:1504, 1601-1611; 1869a496a; TestBackendCacheEditIdentity |
| object cache of assembled parts | on | done | done | done since 2026-10-27 (COFF objects keyed on the text with its real paths; HTTP body edit assemble 1897 to 162 ms, wall 2783 to 1179 against linux's 921) | main.xo cached_macho_objects (coff) |
| per-function and part cache | on | done: HTTP body edit 1234 to 668 ms (arm64 589) | done: 1248 to 640 ms | done since f309fa7a9 (the entries carry the Win64 thunks) | main.xo:1459; bench/results/amd64-edit-cache-2026-10-26.md; gotchas/fn-cache-hit-skips-emit-side-tables.md |
| first edit after a cold build reuses parts | on | done | untested (shared code) | needs the row above | compiler/xo/bcache.xo PART_PACK_FIRST_BYTES; a69d4a7b7; internal/arm64 TestFirstEditAfterColdParts (host arm64 and linux/amd64) |
| lazy inline decode (0283 stage B) | on | done | done | done | main.xo:1389 (no OS gate) |
| deferred inline verify | on | done | done | done | main.xo:2513-2514 |
| inliner | on | done | done | done | compiler/arm64/inline.xo (no arch gate) |

Native windows/amd64 (Windows CI run 40532204416, Xeon Platinum 8488C,
xo 0.32.0 windows/amd64, a body edit through the cache after a cold
build): llvm-fib cold 3209 ms, edit 811 (emit 63, assemble 108, pe.pdb
68, pe_link 90; 571 of 572 functions and 13 of 14 objects from the
cache); HTTP server cold 8250 ms, edit 1882 (emit 193, assemble 276,
pe.pdb 235, pe_link 318; 1397 of 1398 functions, 36 of 37 objects). Each
edit's .exe and .pdb equal the cache-off build's byte for byte, every
section compared (scripts/ci/l4-pediff on the CI branch). Run
40525427807 reported them different only because it named the two
builds edit and off (gotchas/darwin-binary-identity-output-name.md).

## Code generation

| feature | arm64 | linux/amd64 | darwin/amd64 | windows/amd64 | evidence |
|---|---|---|---|---|---|
| codegen pattern tests (`xo dev codegen`) | 33 `arm64:` patterns | missing | missing | missing | compiler/cli/dev/codegen.xo:58 knows arm64, llvm, llvm/arm64, llvm/amd64 (LLVM IR), no x86 key |
| async preemption (0318) | on | done | done | done (thread suspend, d53742c9a) | compiler/ssa/build.xo:483 |
| shrink wrapping | on | done | done | missing (Win64 unwind: one prologue) | compiler/arm64/isel.xo:157 |
| arg_slots under shrink wrapping | on | off where it shrink wraps | same | same | regalloc.xo:441 |
| reuse_flags, cluster_mem | on | missing | missing | missing | isel.xo:76-133, 135, 147 |
| wide crypto (aes x4, aes_ctr, aes_gcm(8), chacha4, ghash4) | NEON | aes_gcm8 done since 2026-10-27 (seal 1.09, open 1.02, key setup 1.28 times Go, amd64-crypto-vs-go-2026-10-27.md); chacha4, ghash4, aes x4, aes_ctr portable | same | same | compiler/ssa/call.xo:218-232, x86aesgcm8.xo |
| Float.fma in hardware | fmadd | done (FMA3 behind CPUID) | done | done | 0be785fc9 (2026-10-26) |
| rt.ticks | cntvct | missing (mono_ns body) | missing | missing | call.xo:229 |
| str.cmp in line, str_eq_const | on | missing | missing | missing | compiler/arm64/calls.xo:1293 (comment says no cneg; the x86 emitter retargets cneg now), :1312 |
| ext_test, ext_add, chain_products | on | missing | missing | missing | isel.xo:3216, 3256; calls.xo:2946-2949 |
| load memo across the preempt poll | kept | cleared ("until measured") | cleared | cleared | calls.xo:2431-2433 |
| slp_f64 | NEON | missing | missing | missing | compiler/arm64/slp.xo:170 |
| `--inline-str` (experimental) | opt in | refused with a warning | refused | refused | main.xo:678-681 |

## Runtime, tools and debugging

| feature | arm64 | linux/amd64 | darwin/amd64 | windows/amd64 | evidence |
|---|---|---|---|---|---|
| --race | darwin, linux | partial: links, never run (no x86 Linux host; Docker emulation cannot start TSan) | done (Intel Mac, race_sync cases) | done (c0d8bef75; one CI run, 120 of 120) | compiler/xo/ld.xo:125; gotchas/tsan-amd64-not-on-arm64-mac.md |
| DWARF lines and variables | on | done | done | done (DWARF in the PE, 41cbcb183) | 5156c6684 (2026-10-20); internal/arm64/x86debug_test.go TestAmd64DebugInfoSameCode (linux/amd64) |
| XO_TRACEBACK=locals | on | done | done (04e1e851e) | missing (refused) | std/native/llrt/fault.xo:318-322 |
| split debug info / dSYM / PDB | dSYM, split | untested (shared ELF path) | untested (OS-gated dSYM path) | partial: PDB stage 1 (names and publics, 92e966495) | internal/arm64/splitdebug_test.go (arm64 only) |
| CPU profiling (XO_PROF=cpu) | on | untested (shared runtime, no native amd64 run found) | partial: under-samples, 15 ms of samples for about 90 ms of CPU at 1000 Hz (aesgcm seal1 on the i7-9750H, 2026-10-27) | partial (sampler 28681ddae, checked on the LLVM backend under Wine) | std/native/llrt/prof.xo:23, 173 |
| no-libc static build | linux | done | n/a (Xo's Mach-O linker) | done (PE linker, no CRT) | compiler/xo/link.xo:297-311 |
| heap scavenge (0383) | on | done | done | missing (no-op, phys_mem 0) | std/native/llrt/heapos.xo:686-708 |

## Gaps, in proposed order

1. windows/amd64 edit loop: the fn/part cache (bb5392d32, gating), then
   hollow and incr_hollow over COFF (a COFF scan in hollow.xo, the
   object cache on the windows assembly path at main.xo:1601-1611),
   then shrink wrapping with Win64 unwind for split prologues.
2. An amd64 codegen pattern set: add `amd64` to codegen.xo:58's keys and
   port test/codegen's arith and lists patterns, so x86 selection wins
   are checked by a test instead of commit messages.
3. Wide crypto on x86 (VAES or AES-NI x4, PCLMUL x4, SSE2 chacha x4):
   the largest runtime gap (TLS throughput), not yet measured.
4. Small codegen gaps, each with its own `--ab`: reuse_flags,
   cluster_mem, str.cmp in line (check the stale cneg comment first),
   ext_test/ext_add, chain_products, the load memo, slp_f64.
5. Runs that only need a machine: CPU profiling on native darwin/amd64
   records too few samples (15 ms of them for about 90 ms of CPU at
   1000 Hz, 2026-10-27), and linux/amd64's is unrun; --race on a real x86 Linux host (CI, with the owner's
   approval); split debug info tests for amd64.
