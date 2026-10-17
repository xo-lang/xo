# arm64 Linux: LSE in elflink's outline atomics (2026-10-17, xo-fa)

Xo lang 4's finding: every runtime read modify write on Linux arm64
calls one of the `__aarch64_{cas,swp,ldadd}{4,8}_{model}` helpers that
selfhost/macho elflink synthesizes for a link without libgcc (decision
0182), and they were load and store exclusive loops; libgcc's test
`__aarch64_have_lse_atomics` and use one ARMv8.1 instruction. Now:

- `xo_lse`, a word every no-libc Linux program defines (ssa.nolibc_data,
  both emitters), is set to 1 at the start (cabi_nolibc.xo nl_start,
  before any thread) when AT_HWCAP has HWCAP_ATOMICS (bit 8), unless the
  environment has XO_LSE=0;
- elflink's cas, swp and ldadd helpers test it (`cbz`) and run `cas`,
  `swp` or `ldadd` with the model's suffix when set, the loop otherwise
  (ARMv8.0 machines, XO_LSE=0, and a link of a program that does not
  define xo_lse, which gets the loop only). ldclr, ldeor and ldset keep
  the loop (no emitter calls them).

Follow-up (the same day): the LLVM backend's no-libc Linux builds
compile with -moutline-atomics (they inlined ARMv8.0's load and store
exclusive loops, -mno-outline-atomics), and the helpers every cc no-libc
link gets (selfhost/xo link.xo nolibc_atomics, xo_atomics.s) are the
dispatching bodies above, sizes 4 and 8, under `.arch_extension lse`;
so an arm64 backend program linked by cc dispatches too. A libc link
uses libgcc's helpers, which dispatch already.

CPU time (user + sys) of 20 runs, bench/perf/conc, linux/arm64 in Docker
on the Mac (Apple M-series, `atomics` in /proc/cpuinfo), two rounds:

| workload | threads | main | LSE | XO_LSE=0 |
|---|---|---|---|---|
| spawn | 1 | 768, 737 | 542, 567 | 689, 773 |
| spawn | all | 1197, 1245 | 943, 964 | 1189, 1091 |
| fanout | 1 | 481, 482 | 501, 412 | 544, 503 |
| fanout | all | 858, 813 | 614, 546 | 722, 717 |
| pingpong | 1 | 776, 800 | 679, 738 | 861, 858 |
| pingpong | all | 844, 827 | 751, 708 | 849, 797 |
| mutex | 1 | 220, 258 | 168, 174 | 179, 209 |
| mutex | all | 292, 328 | 255, 283 | 286, 309 |

(ms; XO_LSE=0 is the loop path, about main's.)

LLVM backend, no-libc linux/arm64 (Docker, 20 runs, user + sys ms, two
rounds; the inline loops before):

| workload | threads | inline loops | outline, LSE | XO_LSE=0 |
|---|---|---|---|---|
| spawn | 1 | 504, 514 | 339, 338 | 493, 487 |
| fanout | 1 | 323, 350 | 219, 263 | 310, 399 |
| pingpong | 1 | 622, 693 | 491, 604 | 622, 723 |
| mutex | 1 | 99, 96 | 66, 66 | 98, 99 |
| fanout | all | 3078, 1920 | 1270, 1454 | 3300, 3117 |
| mutex | all | 382, 610 | 219, 167 | 351, 341 |

(spawn and pingpong with all threads vary more than the change.)
