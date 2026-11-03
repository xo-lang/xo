# 0477: XO_THP=dense keeps the main thread's stack and TLS off huge pages

Status: Accepted (owner, 2026-11-03; amends decisions 0456 and 0469; proposed by Xo Lang 2, 2026-11-03)

## Context
Under XO_THP=dense, Linux's default (decision 0456), the heap maps its chunks
MADV_NOHUGEPAGE and advises MADV_HUGEPAGE once a chunk is used up; thread stacks (libcthr
advise_nohuge) and fiber stacks (pool.xo) are MADV_NOHUGEPAGE. The main thread's 1 GiB stack
and its TLS block (cabi_nolibc.xo nl_main_stack, nl_main_tp) were advised only under XO_THP=off
(decision 0454). On a THP=always host the first touched 2 MiB of that stack becomes a huge page,
so every program carries about 2 MB of max RSS that off does not.

The lead asked (2026-11-03) whether a huge-page threshold would close the remaining gap between
dense and off (coll putget 8,696 KB against 5,604 after decision 0469). A sweep with measurement
knobs on the first linux/arm64 host (THP always, defrag madvise), 5 runs, max RSS KB, dense to
dense with the main stack and TLS NOHUGEPAGE: putget 8,696 to 6,612, push 31,748 to 29,680, hof
33,172 to 31,204, json small 8,964 to 7,012, csv parse 10,196 to 8,152, csv reader 5,908 to
4,068 (bench/results/thp-main-stack-2026-11-03.md). The rest of the gap is large blocks (1 to
4.6 MB) and, for json small only, the chunk threshold (1.6 MB).

## Decision
Under dense, nl_main_stack and nl_main_tp advise MADV_NOHUGEPAGE as they do under off
(cabi_nolibc.xo thp_env_off now reads "not XO_THP=kernel"). XO_THP=kernel gives no advice
for them, as before. Large blocks keep the kernel's policy, and there is no chunk threshold.

## Consequences
- About 2 MB less max RSS in every Linux native program under THP=always; nothing changes under
  THP=madvise or never, where the stack had no huge page.
- ab-host against 8e23ffa9c on the first host: bench/perf/xo push 9 quiet rounds wall 1.002
  (0.968 to 1.049), max RSS 0.935; self build 5 rounds (4 loaded) wall 0.992 (0.973 to 0.998),
  max RSS 1.002 (2 MB is noise in a 4.3 GB build); a deep recursion program (40 recursions
  150,000 calls deep, several MB of stack) 5 quiet rounds wall 0.995 (0.942 to 1.005), max RSS
  0.788.
- A program whose main thread recurses through many MB of stack pays 4 KiB page faults there
  instead of 2 MiB ones; the deep recursion run shows no wall cost.

## Alternatives
- Large blocks NOHUGEPAGE as well (1 to 4.6 MB more): push wall 2.308 (1.987 to 3.248), since a
  growing large block faults 4 KiB at a time; self build 1.028. Rejected.
- A chunk threshold (no MADV_HUGEPAGE advice until the heap holds N MB of chunks): 16 MB saves
  1.6 MB for json small and nothing for coll. Not taken.
- Keep the main stack's huge page: 2 MB in every program for no measured speed.
