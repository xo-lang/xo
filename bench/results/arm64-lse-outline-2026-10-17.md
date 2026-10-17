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

The LLVM backend's no-libc Linux builds inline their atomics as load and
store exclusive loops (-mno-outline-atomics, decision 0206); a cc link
uses libgcc's helpers, which already dispatch. Neither changes here.

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
