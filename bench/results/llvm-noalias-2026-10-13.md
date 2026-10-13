# Alias scopes for lists on LLVM (decision 0211)

Date: 2026-10-13. Branch of the noalias agent; base 5f1c53ac (the ADR
stub). Instruction counts are whole process instructions retired,
minimum of 5 runs (3 for bench/perf/llvm and bench/perf/xo),
`/usr/bin/time -l`, on the development Mac (Apple M-series, clang
21.0.0) with other agents running, so counts, not times, are reported.
Builds use the Go driver (`XO_SELF_DRIVER=0`), whose internal/llvm
emits the same IR as selfhost/llvm (TestSelfhostLLVM: 11 runs, 1,226
functions identical, 11 modules byte identical). "Xo / C" is (workload
- none) of the Xo build over the same for the C build
(`XO_LLRT_C=digest`), as in raw-memory-2026-10-13.md.

## What landed

- selfhost/llvm noalias.xo (internal/llvm noalias.go mirrors it): the
  classes, summaries, and scopes of the ADR; list accesses and Bytes
  reads carry `!alias.scope` and `!noalias`, the entry declares the
  scopes, the nodes follow the functions. SplitIR (and its frozen copy)
  keeps the metadata lines (`Meta`).
- Argon2id and BLAKE2b in Xo are the default again (the code of
  a4168f52, `raw.zero_ints` for `bs` and `t`, argon2_g's second guard
  covering bs and q; the digest_tables.xo generator in
  internal/llvm/digest_test.go writes it); `XO_LLRT_C=digest` builds the
  C, and `xo_crypto_argon2id` is back in IOFlags' list (both drivers).
- internal/llvm/testdata/golden/noalias_lists: one list through two
  parameters (`var` and borrowed, two `var`), a list also in a map,
  captured by a closure, sliced, handed back by calls (its own, another,
  a new one), recursion, a borrowed Bytes, a list of lists, and three
  lists rotated between calls in a loop. Same output on Go, LLVM (and
  split), arm64, and wasm.

## Argon2id (bench/perf/digest, `WORKLOADS="none argon2"`)

The branch as committed (1f147c2c's std, the scopes):

| Build | arm64 argon2 | arm64 none | LLVM argon2 | LLVM none |
|---|---|---|---|---|
| Xo | 198,807,783 | 22,404,444 | 207,133,158 | 19,210,326 |
| C (`XO_LLRT_C=digest`) | 202,316,618 | 22,371,902 | 199,432,388 | 19,244,923 |
| Xo, same std, no scopes | | | 218,649,302 | 19,165,201 |

**Xo / C: arm64 0.980, LLVM 1.043. Decision 0180's bar (both backends
within 5 percent of the C) holds; Argon2id and BLAKE2b in Xo are the
default.** Without the scopes the same std is 1.107 on LLVM.

The steps on LLVM (each its own run with its own C build; Xo / C):

| Variant | LLVM Xo / C |
|---|---|
| no scopes (rawmem2's 1.155) | 1.154 |
| scopes, `t` without a class (its `var` results of blamka_row and blamka_column had none) | 1.155 |
| scopes with the handed back summaries (bs, t, q three classes) | 1.132 |
| the same and argon2_g's second guard covering bs and q (committed) | 1.043 |
| no scopes, with the guard | 1.107 |

Why each step: with three scopes LLVM keeps the length loads and drops
the index checks before the permutation calls, but its SLP vectorizer
still leaves the unrolled `r = bs[a+i] ^ bs[b+i]; q[i] = r; t[i] = r`
as `ldp`, `eor`, `stp` (it does the same for the equivalent C with
`restrict`). After the 16 permutation calls every index of the final
xor was checked again (two instructions a word: LLVM reloads the
lengths of bs and q, since the calls may change any memory). The guard
after the calls gives LLVM one fresh length to prove them against; it
then drops the checks and vectorizes the final xor (`eor3.16b`, the
three way xor). The guard needs the scopes: without them the stores to
bs, q, and t still block it (1.107).

Experiments not kept (Xo / C):

| Variant | LLVM | arm64 |
|---|---|---|
| `!noalias` on calls for the classes whose values never leave the function's list operations, unrolled | 1.117 | |
| the same, with the guard | 1.093 | 0.980 |
| xor parts as plain loops, scopes | 1.129 | 1.400 |
| xor parts as plain loops, scopes and call `!noalias` | 0.869 | 1.400 |
| xor parts as loops unrolled by 8, scopes | 1.374 | 1.766 |
| the same, no scopes | 1.461 | |

The call metadata lets LLVM keep the words stored into q (or loaded
from bs) in registers across the 16 calls instead of reloading them, and
with 128 of them it spills (1.093 against 1.044 with the guard). It is
what makes the plain loops vectorize (0.869), but arm64 runs those
loops at 1.400, and decision 0180 needs both backends.

## Other workloads (LLVM, scopes on over off, instructions)

Same tree and std, the scopes switched off in a scratch copy of the
emitter. No workload moves by more than 1.5 percent (the setup alone of
bench/perf/portable by 2.0); the largest are the
system call heavy suites, which vary run to run as much:

| Suite | Range of on / off | Largest |
|---|---|---|
| bench/perf/atomic | 0.999 to 1.007 | contended 1.007 |
| bench/perf/fsproc | 0.986 to 1.002 | proc 0.986, proc_plain 0.988 |
| bench/perf/gen | 1.000 to 1.001 | |
| bench/perf/glob | 1.002 to 1.010 | |
| bench/perf/httpparse | 0.988 to 1.008 | h1_get 0.988 |
| bench/perf/json | 1.000 to 1.000 | |
| bench/perf/portable | 1.000 to 1.007 | setup (none) 1.020 |
| bench/perf/report | 0.998 to 1.002 | |
| bench/perf/text | 0.997 to 1.002 | |
| bench/perf/unicode | 0.999 to 1.001 | |
| bench/perf/llvm/xo (fib, sieve, nbody, sort, interp, collatz, strings) | 1.000 to 1.000 | |
| bench/perf/xo (18 collection and generic workloads) | 0.999 to 1.001 | branch 1.001 |
| bench/perf/digest (SHA-256, HMAC, PBKDF2) | 1.000 to 1.000 | |

These workloads read and write lists through fields, elements, and
calls far more than through two parameters at once, and most of their
hot loops already keep one list; the scopes matter where a function
writes one list while reading another it was given or made.

## Tests

- TestGolden and TestGoldenSplit (internal/llvm) and TestGoldenRun
  (internal/arm64) on noalias_lists; TestSelfhostWasm passes with it.
- TestDigestTables, TestDigestDiff (Xo and C paths, both backends),
  TestCryptoDiff (4,086 operations agree), TestGolden/native_crypto,
  TestIOFlags.
- TestSelfhostLLVM on noalias_lists, var_alias, rc_aliasing, lists,
  value_semantics, closures, native_crypto, and bench/perf/digest: every
  module byte identical. Then on every internal/llvm golden program and
  bench/perf directory (1f147c2c): 182 runs compared, 25,450 functions
  identical, 182 modules byte identical.
- TestGolden (internal/llvm, all programs, LLVM against the Go backend
  and want.txt) passes on 1f147c2c.
- The recorded LLVM goldens (gogoldens/llvmdump) change with the IR and
  need a new recording (scripts/selfhost-record-goldens.sh with
  `RUN='^TestSelfhostLLVM(Mutations)?$'`).
