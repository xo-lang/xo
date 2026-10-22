# Where a cold build of xo spends its copies, and a per-site fix that did not pay (2026-10-22)

Question: the `xo vet --cost` sweep (owner poll, 2026-10-22) was to fix
cost notes inside measured hot loops of the compiler and std. Do any of
them sit in hot loops?

## Profile

`XO_PROF=cpu` of a cache-off `xo build compiler/xo` (main 6a6f9f1e8,
darwin/arm64, other agents running): 16,210 samples, 162.1 s CPU, 113.2
s wall.

- `alloc.alloc` 10.6% flat and `platform_memmove` 7.8% flat are the top
  two functions.
- `obj.hmove` (std/native/llrt obj.xo: a heap block's resize, which
  allocates, copies, and frees) calls 10.2 s of the alloc time and 4.8 s
  of the memmove time: about 15 s, or 9% of the build.
- hmove's callers:

  | caller | time |
  |---|---|
  | arm64.solve_live | 2.46 s |
  | obj.reserve_slow | 1.83 s |
  | ssa.renumber | 1.50 s |
  | arm64.G.dv_plan | 0.75 s |
  | arm64.push_rng | 0.74 s |
  | prelude.merge_sort[Int] | 0.68 s |
  | arm64.G.slp_f64 | 0.59 s |
  | the ssa.Rc passes | about 2 s together |
  | more callers below 0.5 s each | |

The compiler's `xo vet --cost` notes are not among them:

- In compiler/arm64, ssa, and macho, the 8 cost notes (XO0910) are all
  cold.
- The two closures in ssa/rc.xo that copy the 72-word Rc are made once
  per worker thread.
- Inline costing totals 3% cumulative.
- Call-argument mapping does not show at all.

vet has no check for a List grown by push in a loop, which is where the
copies are.

## The per-site fix

arm64 solve_live (regalloc.xo) pushed into thousands of per-block lists
of unknown final size. The change (branch live-bucket, d8ea0b5e9, not
merged) collected (block, register) pairs into two flat lists and
bucketed them into lists of exact size.

`xo bench --ab` compared old and new xo, each building compiler/xo with
the cache off: 5 rounds, 0 loaded, machine load 18 to 30.

| metric | B / A | 95% interval | verdict |
|---|---|---|---|
| instructions | 1.009 | 1.003 to 1.011 | worse |
| cpu_ms | 1.035 | 0.899 to 1.06 | within noise |
| wall_ms | 1.186 | 0.752 to 1.792 | within noise |
| max_rss_kb | 0.807 | 0.592 to 1.435 | within noise |

Dropped. The site's ceiling was 1.5% of the CPU, and the flat lists
grow too. The output-identity check was not established: the two built
binaries differed, and no determinism control was run.

## Conclusion and next experiment

The 9% is spread over dozens of sites, at about 1% each, so per-site
rewrites cannot pay. ssa.renumber is not tried, for the same reason
(0.9% ceiling).

The next experiment is the growth itself: obj.hresize or hmove growing
in place when the block's size class has room or the next block is
free, and the growth schedule of small lists. It will be measured with
`xo bench --ab` on the same cold build and on the http and json
benches. No vet note is proposed, since the experiment did not pay.
