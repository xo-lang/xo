# /alloc regression since v0.35.0: bisect and fixes (2026-11-02)

Xo Lang 4, with Xo Lang 1 (regression B), Xo Lang 2 (memory review, data
shift run) and Xo Lang 3 (the Response field). bench/perf/jitter's Xo
server answering `GET /alloc` (a JSON page of ITEMS records, 14,683 bytes),
loaded by its Go loadgen, closed loop at 32 and 256 connections, 10 s
after a 2 s warm-up, `-awake`. First linux/arm64 offload host (12 CPUs),
exclusive lease, servers pinned to CPUs 0-5 and the loadgen to 6-11.
Every job measures each revision and v0.35.0's server (built by the
v0.35.0 seed from the same jitter source) round-robin; tables give the
median of 3 rounds, req/s and the ratio to
v0.35.0 in the same job, and the server's CPU time a request as a ratio
to v0.35.0. Ratios are comparable across jobs; absolute req/s are not
(the host's load before a job differed until the quiet wait of
2026-11-02, see "Method notes").

## Findings

1. **Regression A: the HTTP/1.1 idle park (decision 0449, 3fdfa3fd3).**
   A connection whose read buffer was empty and whose last request was
   not "quick" (under 2 ms since the previous one) parked: its fiber
   ended and a new one was spawned when bytes arrived. On Linux under
   load the next request's readiness arrives one poll later, so busy
   connections looked idle: runtime counters (l4prof2, second host)
   showed a spawn for 57% of requests at 32 connections and 91% at 256.
   3fdfa3fd3 alone took /alloc to 0.61 / 0.57; its follow-ups recovered
   part of it (pass 2).
2. **Regression B: slot_group (3b0ccbb29),** a register allocation pass
   whose unused stack slots pushed frames past the single store pair
   limit (Xo Lang 1, bench/results/slot-group-frames-2026-11-02.md):
   0.87 / 0.73 against its parent's 1.01 / 0.85 (pass 3).
3. **The amplifier.** With the park rule in place, small slowdowns turned
   into many more parks: main in the afternoon (1ef022848, with A, B and
   decision 0447's wasm-only Response field from d71e1b442) measured 0.78
   / 0.65. Turning the park off (869990f26) or lingering before it
   (43b0a327f) brought the same tree to 1.01 to 1.03: both erase A, and
   the B and Response field costs with it (pts6, pts7).
4. **The fix that shipped: linger** (17ded84bb): an idle plain HTTP/1.1
   connection waits 20 ms on its descriptor before parking, and parks only
   if nothing came. Spawns fell to one a connection (34 at c32, 258 at
   c256, l4prof3). Adaptive linger (075705865) later stopped slow
   keep-alive connections from lingering on every gap (Xo Lang 2's memory
   review, decision 0449).
5. **Linger with Lang 1's narrow B fix measured 0.78 / 0.74** (pts7),
   with parks unchanged (l4prof3) and CPU a request 1.33 of v0.35.0's against linger's 0.97. It is not
   .data placement: neither of Lang 1's layout probes (pts11, .data mod
   128 at 0x68 and 0x18) moved it, and Xo Lang 2's XO_DATA_SHIFT run
   (0 against 64 bytes, and 128-byte lines for the runtime's static words)
   stayed within 1% ([alloc-shift-2026-11-02.md](alloc-shift-2026-11-02.md),
   decision 0459's "Finding: refuted on main"). Code placement is refuted
   too: Xo Lang 2's l2pad2 job built both with every padding and alignment
   arm of the XO_ARM64_PAD sweep (decision 0075), and every arm kept them
   apart (linger 32.4 to 34.2 µs a request, linger + narrow B 45.7 to 47.5).
   So is buffer copying (Xo Lang 2's heapcmp). The open lead is 4K stack
   aliasing (narrow B changes frame sizes; Xo Lang 2's l2sshift job
   shifts the stack to test it).
6. **The wasm-only Response field** (d71e1b442: `ws: rt.Any?` on every
   native Response) costs about 9% CPU a request at both data shifts
   (ec2398ed8 removes it: pts8, and Xo Lang 2's alloc-shift run). It is
   folded into the existing stream field (3f708df63): req/s 1.116 and
   1.118 against main in one job, CPU a request 0.891 and 0.894, the same
   as removing the field (l4pts13).
7. **a1 P (25d23d18e)** cost /alloc 0.756 / 0.895 against its parent in
   one job (pts12) and was reverted (b0d12f3f9); decision 0324.
8. **Reverting narrow B on main** (9edfbf82c, a local commit that takes
   compiler/arm64/regalloc.xo back from 10396987a's parent; not pushed)
   brings /alloc back to v0.35.0 in one job: req/s 1.006 / 1.015 at c32 /
   c256, CPU a request 0.993 / 0.986, against main's 0.839 / 0.823 (l4pts14,
   built on the second host, measured on the first). It is not the 0.36
   fix: Xo Lang 2's l2sshift job found the slow cases sit on one 4K phase
   of the fiber stack (linger slow only at shift 3888, linger + narrow B
   only at shift 0), so the revert only moves /alloc off that phase. The
   fix is Xo Lang 2's stack phase change.
9. **Main with the stack phase fix** (7afa6e957, measured at ada68b353;
   decision 0459, bench/results/stack-phase-2026-11-02.md:154) in two jobs
   on the first host, 3 rounds each, against v0.35.0 in the same job:

   | job | arm | c32 req/s | c256 req/s | c32 CPU us | c256 CPU us |
   |---|---|---|---|---|---|
   | l4pts15 | main 3f708df63 (no fix) | 0.833 | 0.801 | 41.75 (1.239) | 40.96 (1.251) |
   | l4pts15 | main ada68b353 | 0.966 | 0.953 | 35.22 (1.045) | 34.34 (1.049) |
   | l4pts15 | v0.35.0 | 164,909 | 182,149 | 33.70 | 32.74 |
   | l4pts16 | main ada68b353 | 0.965 | 0.954 | 35.25 (1.048) | 34.25 (1.047) |
   | l4pts16 | main + a1 P (73a081b7b, local) | 0.702 | 0.807 | 35.99 (1.070) | 35.59 (1.088) |
   | l4pts16 | v0.35.0 | 164,780 | 182,413 | 33.64 | 32.70 |

   The fix takes main from 0.83 to 0.97 of v0.35.0; about 4.5% CPU a
   request is left on this host, with every arm's range (fresh process each
   round) within 0.1 to 0.6 us, so it is not a per start address effect.
   a1 P on top still loses (stays parked). On the Mac (darwin/arm64, real M4 Pro, 16 KiB
   pages), one job of the same three servers built natively (l4macm, 5
   rounds, fresh process each, server CPU from ps around a 10 s run after a
   2 s warm run; the Mac was shared, load 3 to 21) has main level with
   v0.35.0: req/s 0.980 / 1.033 at c32 / c256, CPU a request 1.004 / 1.014,
   every range overlapping v0.35.0's; main without the fix (3f708df63) is
   level too (0.978 / 1.043, CPU 1.009 / 1.034), as the stack phase finding
   predicts for 16 KiB pages. A second Mac job in a quiet window the lead
   opened (l4macq3, 23:09 to 23:24, 7 rounds, the same method; the only
   other load one core of an unrelated node process; the 1-minute load
   before a point, 19 to 22, is mostly the job's own previous point) agrees
   on req/s and puts CPU a request 1 to 4% over v0.35.0:

   | arm | c32 req/s | c256 req/s | c32 CPU | c256 CPU |
   |---|---|---|---|---|
   | main 3f708df63 (no fix) | 1.002 | 1.003 | 1.012 | 1.023 |
   | main ada68b353 | 0.995 | 1.015 | 1.011 | 1.037 |
   | v0.35.0 | 127,732 | 152,862 | 48.37 us | 48.39 us |

   (medians; ranges: v0.35.0's c256 req/s 130,621 to 159,392, main ada68b353's
   c256 CPU up to 50.69 us, its lowest sample a zero, a ps read that
   missed; the
   arms' ranges overlap v0.35.0's at c32 and on c256 req/s.) With Graviton's under 1%
   (stack-phase-2026-11-02.md:89-114), the first host's 4.5% is the 4 KiB
   page VM's; owner, 2026-11-02: it ships in 0.36 as a known VM-only cost.

## Method notes

- Each job builds every revision's compiler (xobuild from the host's
  seed) and its jitter server; pts1 to pts10 and pts12 build every
  server from the last revision's jitter source, pts11 from each
  revision's own (the layout probes change the server source).
- pts13 built on the second host and only measured on the first
  (bench/perf/jitter/alloc-split.sh, Xo Lang 3).
- pts1's second point (2e667a3d2) did not build (an effect error of the
  time) and is left out.
- The 1 minute load average at a job's start was high when it followed
  another job (8 on 12 CPUs for l4pab); jobs alternate their servers each
  round, so ratios within a job hold. From pts10 on, every job waits for
  the load to fall under 1 before its first round.
- l4prof2 ran on the second host while it was loaded (load 26 to 52), so
  only its counts are used, not its rates.

## Runtime counts (XO_RTSTATS, second host, /alloc, 10 s)

### l4prof2 (host loaded: counts only)

| revision | what | c32 requests | c32 spawns | c256 requests | c256 spawns |
|---|---|---|---|---|---|
| 1ef022848 | main, afternoon | 107,568 | 61,316 (57%) | 360,350 | 328,661 (91%) |
| 29e51e0d4 | fix A: poller readiness stamp | 185,419 | 41,179 (22%) | 562,011 | 142,128 (25%) |
| 869990f26 | park off | 126,769 | 34 | 523,100 | 258 |

### l4prof3

| revision | what | c32 spawns | c32 CPU a request (µs) | c256 spawns | c256 CPU a request (µs) |
|---|---|---|---|---|---|
| 43b0a327f | linger | 34 | 33.9 | 258 | 35.9 |
| ab1690c50 | linger + narrow B | 34 | 43.0 | 258 | 43.4 |
| a144a3115 | main (linger + narrow B) | 34 | 41.5 | 258 | 42.2 |

## Jobs

### l4pts1

| revision | what | c32 req/s | ratio | CPU a request | c256 req/s | ratio | CPU a request |
|---|---|---|---|---|---|---|---|
| 2505f6c36 | docs: gotcha recorder-machine-state (the Linux run cache miss, ... | 168,099 | 0.975 | 1.036 | 179,165 | 0.955 | 1.048 |
| 4578e6197 | TLS 1.3 reference client checks a CertificateRequest's extensions ... | 169,362 | 0.982 | 1.029 | 180,922 | 0.964 | 1.037 |
| 45465dbd8 | gotcha: a Linux child's ru_maxrss counts the memory of the process ... | 170,275 | 0.987 | 1.023 | 180,139 | 0.960 | 1.042 |
| e8799439a | 0269 step 1 (WIP): value(), E.from(v) and E.all() of a valued ... | 172,469 | 1.000 | 1.004 | 184,188 | 0.981 | 1.019 |
| daaa2eec7 | decision 0449: CRUD gate passes with the quick-connection flag | 170,314 | 0.988 | 1.019 | 149,607 | 0.797 | 1.255 |
| eff43904e | docs: the emit start label; the earlier exact-capacity finding ... | 166,373 | 0.965 | 1.063 | 147,451 | 0.786 | 1.273 |
| 786032245 | heap profile: the sample table holds 262,144 samples (it filled at ... | 144,190 | 0.836 | 1.244 | 128,061 | 0.682 | 1.466 |
| f58b4116f | main, morning 2026-11-02 | 144,567 | 0.838 | 1.239 | 128,033 | 0.682 | 1.467 |
| v0.35.0 | the 0.35 release (baseline) | 172,460 | 1 | 1 | 187,690 | 1 | 1 |

### l4pts2

| revision | what | c32 req/s | ratio | CPU a request | c256 req/s | ratio | CPU a request |
|---|---|---|---|---|---|---|---|
| 561b0d39d | arm64, ssa, macho: lists grown in place (decision 0448 stage 4): ... | 170,565 | 0.985 | 1.021 | 179,613 | 0.959 | 1.041 |
| bd0bd05a5 | parent of 3fdfa3fd3 | 173,314 | 1.001 | 1.005 | 185,866 | 0.993 | 1.007 |
| 3fdfa3fd3 | decision 0449 HTTP/1.1 idle park (regression A, first bad) | 105,244 | 0.608 | 1.721 | 106,732 | 0.570 | 1.752 |
| 8d1dfc1d6 | 0449 follow-up | 143,245 | 0.827 | 1.215 | 152,224 | 0.813 | 1.220 |
| 60373db40 | 0449 follow-up | 161,869 | 0.935 | 1.076 | 164,986 | 0.881 | 1.104 |
| 1710b7a11 | hserve: srv_free frees the continuation records no resumer took (1 ... | 168,088 | 0.971 | 1.045 | 146,849 | 0.784 | 1.275 |
| 8fe1f704b | List grow rounds its capacity up to the block (Go's roundupsize; ... | 164,768 | 0.951 | 1.068 | 145,904 | 0.779 | 1.284 |
| c101874ea | native (arm64): rc_thunk_loops: a retain in a large loop calls the ... | 144,550 | 0.835 | 1.243 | 127,830 | 0.683 | 1.464 |
| 08b888706 | runtime: an idle main thread no longer locks the park of a task ... | 143,858 | 0.831 | 1.249 | 127,365 | 0.680 | 1.470 |
| 6956acf48 | lists: the floor capacity is 1 for elements over 256 bytes, 4 ... | 144,608 | 0.835 | 1.244 | 127,743 | 0.682 | 1.466 |
| f58b4116f | main, morning 2026-11-02 | 144,120 | 0.832 | 1.245 | 127,060 | 0.679 | 1.474 |
| v0.35.0 | the 0.35 release (baseline) | 173,174 | 1 | 1 | 187,250 | 1 | 1 |

### l4pts3

| revision | what | c32 req/s | ratio | CPU a request | c256 req/s | ratio | CPU a request |
|---|---|---|---|---|---|---|---|
| 38006f0b3 | parent of 3b0ccbb29 | 164,746 | 1.005 | 0.989 | 145,709 | 0.845 | 1.183 |
| 3b0ccbb29 | slot_group (regression B, first bad) | 142,904 | 0.872 | 1.166 | 125,553 | 0.728 | 1.373 |
| 489e2f331 | self build: drop_weak_all frees each part's text as it copies it ... | 143,204 | 0.874 | 1.164 | 126,435 | 0.733 | 1.364 |
| c101874ea | native (arm64): rc_thunk_loops: a retain in a large loop calls the ... | 144,312 | 0.881 | 1.155 | 128,283 | 0.744 | 1.344 |
| 1ef022848 | main, afternoon 2026-11-02 | 128,245 | 0.783 | 1.307 | 114,597 | 0.665 | 1.505 |
| 869990f26 | park off (experiment) | 166,132 | 1.014 | 0.978 | 176,012 | 1.021 | 0.980 |
| v0.35.0 | the 0.35 release (baseline) | 163,846 | 1 | 1 | 172,408 | 1 | 1 |

### l4pts4

| revision | what | c32 req/s | ratio | CPU a request | c256 req/s | ratio | CPU a request |
|---|---|---|---|---|---|---|---|
| f58b4116f | main, morning 2026-11-02 | 144,737 | 0.881 | 1.158 | 128,069 | 0.743 | 1.345 |
| 1ef022848 | main, afternoon 2026-11-02 | 128,023 | 0.780 | 1.319 | 114,188 | 0.663 | 1.509 |
| 29e51e0d4 | fix A: poller readiness stamp (experiment) | 129,622 | 0.789 | 1.299 | 117,679 | 0.683 | 1.464 |
| v0.35.0 | the 0.35 release (baseline) | 164,225 | 1 | 1 | 172,260 | 1 | 1 |

### l4pts5

| revision | what | c32 req/s | ratio | CPU a request | c256 req/s | ratio | CPU a request |
|---|---|---|---|---|---|---|---|
| 1ef022848 | main, afternoon 2026-11-02 | 127,775 | 0.775 | 1.327 | 113,932 | 0.655 | 1.525 |
| e6508f5db | park call out of the loop (experiment) | 128,277 | 0.778 | 1.322 | 114,097 | 0.656 | 1.524 |
| 869990f26 | park off (experiment) | 165,950 | 1.007 | 0.989 | 176,856 | 1.017 | 0.983 |
| v0.35.0 | the 0.35 release (baseline) | 164,848 | 1 | 1 | 173,881 | 1 | 1 |

### l4pts6

| revision | what | c32 req/s | ratio | CPU a request | c256 req/s | ratio | CPU a request |
|---|---|---|---|---|---|---|---|
| 1ef022848 | main, afternoon 2026-11-02 | 128,128 | 0.775 | 1.327 | 113,941 | 0.651 | 1.536 |
| 6f7689527 | 50 ms quick window (experiment) | 129,759 | 0.785 | 1.308 | 131,527 | 0.752 | 1.330 |
| 43b0a327f | linger 20 ms (experiment) | 169,079 | 1.023 | 0.972 | 179,713 | 1.027 | 0.973 |
| 869990f26 | park off (experiment) | 166,613 | 1.008 | 0.989 | 177,511 | 1.015 | 0.986 |
| v0.35.0 | the 0.35 release (baseline) | 165,313 | 1 | 1 | 174,913 | 1 | 1 |

### l4pts7

| revision | what | c32 req/s | ratio | CPU a request | c256 req/s | ratio | CPU a request |
|---|---|---|---|---|---|---|---|
| 43b0a327f | linger 20 ms (experiment) | 169,058 | 1.026 | 0.967 | 179,372 | 1.030 | 0.970 |
| ab1690c50 | linger + narrow B | 128,076 | 0.778 | 1.325 | 129,556 | 0.744 | 1.344 |
| 869990f26 | park off (experiment) | 165,347 | 1.004 | 0.987 | 176,659 | 1.015 | 0.984 |
| v0.35.0 | the 0.35 release (baseline) | 164,709 | 1 | 1 | 174,106 | 1 | 1 |

### l4pts8

| revision | what | c32 req/s | ratio | CPU a request | c256 req/s | ratio | CPU a request |
|---|---|---|---|---|---|---|---|
| 17ded84bb | main: linger + narrow B as shipped | 132,691 | 0.810 | 1.274 | 134,629 | 0.790 | 1.266 |
| 19c56259c | + 4 lingerers a CPU cap | 132,612 | 0.810 | 1.274 | 129,262 | 0.758 | 1.319 |
| ec2398ed8 | + Response without wasm-only ws field | 147,095 | 0.898 | 1.145 | 146,571 | 0.860 | 1.163 |
| v0.35.0 | the 0.35 release (baseline) | 163,747 | 1 | 1 | 170,471 | 1 | 1 |

### l4pts10

| revision | what | c32 req/s | ratio | CPU a request | c256 req/s | ratio | CPU a request |
|---|---|---|---|---|---|---|---|
| 25d23d18e | main with a1 P | 97,931 | 0.595 | 1.334 | 119,979 | 0.686 | 1.380 |
| 0df457730 | + adaptive linger | 105,029 | 0.638 | 1.288 | 124,447 | 0.712 | 1.327 |
| v0.35.0 | the 0.35 release (baseline) | 164,497 | 1 | 1 | 174,804 | 1 | 1 |

### l4pts11

| revision | what | c32 req/s | ratio | CPU a request | c256 req/s | ratio | CPU a request |
|---|---|---|---|---|---|---|---|
| 43b0a327f | linger 20 ms (experiment) | 168,589 | 1.024 | 0.970 | 179,620 | 1.043 | 0.959 |
| ab1690c50 | linger + narrow B | 127,290 | 0.773 | 1.327 | 129,543 | 0.752 | 1.329 |
| b1930fb1b | linger + narrow B, layout probe A | 130,058 | 0.790 | 1.302 | 131,439 | 0.763 | 1.311 |
| b2b53aac2 | linger + narrow B, layout probe B | 127,367 | 0.774 | 1.327 | 128,839 | 0.748 | 1.337 |
| v0.35.0 | the 0.35 release (baseline) | 164,608 | 1 | 1 | 172,220 | 1 | 1 |

### l4pts12

| revision | what | c32 req/s | ratio | CPU a request | c256 req/s | ratio | CPU a request |
|---|---|---|---|---|---|---|---|
| a144a3115 | main (h2 trailers fix; parent of P) | 132,426 | 0.803 | 1.282 | 134,645 | 0.774 | 1.293 |
| 25d23d18e | main with a1 P | 100,127 | 0.607 | 1.330 | 120,487 | 0.693 | 1.375 |
| v0.35.0 | the 0.35 release (baseline) | 164,891 | 1 | 1 | 173,939 | 1 | 1 |

### l4pts13

| revision | what | c32 req/s | ratio | CPU a request | c256 req/s | ratio | CPU a request |
|---|---|---|---|---|---|---|---|
| 45b3157ec | main (P reverted, adaptive linger) | 131,181 | 0.796 | 1.285 | 133,049 | 0.769 | 1.301 |
| 7a4adabfe | + Response wasm function in the stream field (landed as 3f708df63) | 146,450 | 0.888 | 1.146 | 148,761 | 0.859 | 1.163 |
| ec2398ed8 | Response without wasm-only ws field (experiment, on a144a3115) | 143,480 | 0.870 | 1.172 | 149,392 | 0.863 | 1.158 |
| v0.35.0 | the 0.35 release (baseline) | 164,877 | 1 | 1 | 173,112 | 1 | 1 |

### l4pts14

| revision | what | c32 req/s | ratio | CPU a request | c256 req/s | ratio | CPU a request |
|---|---|---|---|---|---|---|---|
| 3f708df63 | main (ws holder landed) | 132,385 | 0.839 | 1.224 | 140,882 | 0.823 | 1.218 |
| 9edfbf82c | main with narrow B reverted (local, not pushed) | 158,639 | 1.006 | 0.993 | 173,675 | 1.015 | 0.986 |
| v0.35.0 | the 0.35 release (baseline) | 157,713 | 1 | 1 | 171,119 | 1 | 1 |
