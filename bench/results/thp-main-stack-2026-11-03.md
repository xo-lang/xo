# THP under XO_THP=dense: main stack, TLS and large blocks (2026-11-03)

Question (lead, 2026-11-03): would a huge-page threshold close the gap between XO_THP=dense
and XO_THP=off in max RSS (coll putget 8,696 KB against 5,604 after decision 0469), and what
would it cost the self build? Host: the first linux/arm64 offload host, THP enabled=always,
defrag=madvise. Base 8e23ffa9c. The knobs below lived on a measurement branch only:

- XO_THP_MIN=MB: no MADV_HUGEPAGE advice until the heap holds MB of chunks;
- XO_THP_LARGE=small: large blocks (heapos.map_os) MADV_NOHUGEPAGE under dense;
- XO_THP_STACKS=small: the main thread's 1 GiB stack and its TLS block MADV_NOHUGEPAGE under
  dense (they were advised only under off).

## Max RSS (KB, median of 5)

Sweep 2 ([host-thpmin2.txt](thp-main-stack-2026-11-03/host-thpmin2.txt)):

| workload | dense | +STACKS | +STACKS+LARGE | +all+MIN16 | off |
|---|---|---|---|---|---|
| xo putget | 8,696 | 6,612 | 5,604 | 5,584 | 5,604 |
| xo push | 31,748 | 29,680 | 28,564 | 28,564 | 28,564 |
| xo hof | 33,172 | 31,204 | 26,536 | 26,536 | 26,536 |
| json small | 8,964 | 7,012 | 6,912 | 5,328 | 5,328 |
| csv parse | 10,196 | 8,152 | 6,964 | 6,844 | 6,844 |
| csv reader | 5,908 | 4,068 | 2,876 | 2,876 | 2,876 |

Sweep 1 ([host-thpmin1.txt](thp-main-stack-2026-11-03/host-thpmin1.txt)) tried XO_THP_MIN at
8, 16 and 64 alone: coll unchanged, json small 9,052 to 7,524 at 16.

The whole dense-to-off gap has three sources, and the chunk threshold the idea was about is the
smallest: the main stack and TLS, one 2 MiB huge page in every program (about 2 MB); large
blocks (1 to 4.6 MB); a chunk threshold, only json small (1.6 MB).

## Time (ab-host against 8e23ffa9c)

| candidate | program | rounds | wall B/A | max RSS B/A |
|---|---|---|---|---|
| B bc119f40f: stacks, TLS and large blocks NOHUGEPAGE | xo push | 6 quiet (stopped early) | 2.308 (1.987 to 3.248) | 0.899 |
| B | self build | 5 (3 loaded) | 1.028 (1.007 to 1.031) | 0.977 |
| C b866c387f: stacks and TLS only | xo push | 9 quiet | 1.002 (0.968 to 1.049) | 0.935 |
| C | self build | 5 (4 loaded) | 0.992 (0.973 to 0.998) | 1.002 |
| C | deeprec | 5 quiet | 0.995 (0.942 to 1.005) | 0.788 |

Large blocks without huge pages fault 4 KiB at a time: push, which grows a large list, takes
6.8 ms against 15.7 ms. B is rejected. C ships as decision 0477 (accepted by the owner, 2026-11-03). deeprec is a measurement-only
program, not in the repository: 40 non-tail recursions 150,000 calls deep with a wrapping
checksum (several MB of the main stack, 9,756 to 7,692 KB max RSS), the case where a stack
without huge pages costs most (lead, 2026-11-03); within noise.

Gate of the change (second linux/arm64 host, THP always,
[gate-thpstk.txt](thp-main-stack-2026-11-03/gate-thpstk.txt)): xobuild, a self build with the
new xo (38.3 s, 3.87 GB max RSS), the arm64 heap, allocation and recursion goldens, two of them
again under XO_THP=kernel: all pass.

Files: [thp-main-stack-2026-11-03/](thp-main-stack-2026-11-03/) (ab-thpself and ab-thppush are
B, ab-thpstk-* are C, gate-thpstk the change's gate).
