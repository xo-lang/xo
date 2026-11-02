# /alloc's 0.36 gap: one 4 KiB phase of the fiber stack (decision 0459), 2026-11-02

Question: why did linger plus "narrow B" (slot_group, Xo Lang 1) cost about 38% more CPU
a request than linger alone on bench/perf/jitter's /alloc, on Linux only, when the only
code difference was http.respond's frame (0x240 against 0x170 bytes)? Linger is
43b0a327f, linger plus narrow B is ab1690c50 (one change, compiler/arm64/regalloc.xo).

Host: the first linux/arm64 offload host, a Docker linuxkit VM (kernel 7.0.14-linuxkit) on
an M4 Pro, 4 KiB pages, transparent huge pages "always", no hardware performance counters
(perf_event_paranoid 2, no PMU in the VM). Servers on CPUs 0-5, the loadgen on 6-11, closed
loop /alloc at 32 and 256 connections, 10 s after a 2 s warmup, servers built on the second
host and measured alone on the first (bench/perf/jitter/alloc-split.sh), rounds rotated. CPU
a request is the server's user plus system time over the requests (loadgen -pid).

## What it is not

| check | linger | linger + B | reading |
|---|---|---|---|
| code placement: XO_ARM64_PAD=0..3, XO_ARM64_FALIGN=4 and 6, plain (7 arms each, 3 rounds) | 32.4 to 34.2 us | 45.7 to 47.5 us | every arm keeps the gap: not where code sits |
| .data phase (alloc-shift-2026-11-02.md) | | | a 64 byte .data shift moved CPU 1% |
| allocations a request (XO_PROF=heap totals, XO_HEAPSTATS; second host) | about 867, 94 KB | about 842, 99 KB | the same work; alloc_space shape identical |
| minor faults, major faults, context switches a request (/proc; second host) | 0.0001, 0, 0.01 | 0.0002, 0, 0.01 | no faults, no extra switches |
| system time a request (/proc) | 6.6 to 7.3 us | 6.1 to 7.6 us | the kernel is not it |
| user time a request (/proc) | 28.9 to 30.9 us | 36.2 to 40.0 us | the whole gap is user time |
| darwin on the same M4 (the Mac, ps cputime, 3 rounds) | 45.5 to 50.0 us | 45.8 to 50.2 us | no gap on macOS |

Xo Lang 4's CPU profile (second host, c256) puts the extra time in obj.rs_append_bytes
(20% flat against under 1%), the copy of a field's bytes into the JSON out buffer under
jsontext.append_str: the same appends, about 3x the cost a byte.

## The stack phase

XO_STACK_SHIFT=N puts every fiber's first frame N bytes lower (experiment commits
db3f246f0 on linger and b80dff2e7 on linger + B). The handler runs under http.respond,
whose frame is 208 bytes smaller with B, so every frame of the JSON encoder sits 208 bytes
higher. CPU a request (median of 2 rounds, ranges at or under 0.3%):

| XO_STACK_SHIFT | linger c32 | linger c256 | linger + B c32 | linger + B c256 |
|---|---|---|---|---|
| 0 | 34.7 | 33.5 | **47.7** | **47.0** |
| 208 | 34.7 | 33.6 | 34.6 | 33.6 |
| 1024 | 34.7 | 33.5 | 34.7 | 33.6 |
| 2048 | 35.6 | 33.6 | 34.7 | 33.7 |
| 3888 (-208 mod 4096) | **47.8** | **46.8** | 34.8 | 33.6 |

v0.35.0 in the same job: 35.8 (c32), 34.5 (c256).

The only slow arms put the encoder's frames at the same address mod 4096: linger moved
down 3888 bytes, which is up 208 mod 4 KiB, and linger + B unmoved. Shifting B down by
the 208 bytes it saved restores it. Neither change was slow; the stack phase was.

Darwin on the same M4 (the Mac, linger + B's experiment build 5624bf863, XO_STACK_SHIFT 0 to
3840 in steps of 256, c32, 2 rounds): 45.0 to 48.6 us at every phase, no step.

## Huge pages do not remove it

Experiment commits 57aa4a181 (linger) and 5624bf863 (linger + B) add XO_STACK_THP=1: fiber
stacks advise MADV_HUGEPAGE instead of MADV_NOHUGEPAGE. XO_THP=off (decision 0454) keeps
the heap off huge pages. Median of 2 rounds:

| arm | linger c32 | linger c256 | linger + B c32 | linger + B c256 |
|---|---|---|---|---|
| plain | 35.5 | 34.4 | 48.4 | 47.9 |
| stacks on huge pages | 35.5 | 34.6 | 45.4 (44.5-46.3) | 47.4 |
| heap off huge pages | 35.6 | 34.6 | 46.3 | 46.0 |
| both | 35.7 | 34.5 | 44.1 | 45.5 |

The gap stays in every arm, so TLB reach under the VM's nested paging is not the main cost.
A 4 KiB collision between the encoder's stack addresses and the out buffer's would not
change with page size. (Whether the stack's top was backed by a 2 MiB page was not checked:
that needs a 2 MiB aligned range at the mapping's top.)

## The window

Linger + B's experiment build 5624bf863 at fine shifts around its own phase (shift 0 is
linger + B as built; 4080 is 16 bytes higher), median of 2 rounds, CPU us a request:

| shift | -112 | -64 | -32 | -16 | 0 | 16 | 32 | 48 | 64 | 96 | 128 | 160 |
|---|---|---|---|---|---|---|---|---|---|---|---|---|
| c32 | 35.7 | 35.7 | 35.7 | **42.6** | **48.4** | 35.6 | 35.8 | **40.7** | **41.1** | **46.2** | 35.6 | 35.7 |
| c256 | 34.7 | 34.6 | 34.6 | **41.8** | **47.9** | 34.8 | 34.7 | **39.9** | **40.2** | **45.6** | 34.7 | 34.8 |

v0.35.0 in the same job: 35.7 (c32), 34.7 (c256).

Not one window but a comb inside about 128 bytes: several of the encoder's stack slots
collide with the out buffer's page offset at different distances, each costing its own
part. Outside those 128 bytes of 4096 every phase runs at v0.35.0's cost.

## Graviton: the same phases, under 1%

One bare-metal Graviton 4 run (c8g.metal-24xl, Neoverse V2, Amazon Linux 2023, kernel
6.18, 4 KiB pages; owner-approved round, 2026-11-02; 0.30 instance-hours), the same
binaries (srv-linger-arm64, srv-lingerB-arm64, cross-built on the Mac), servers on CPUs
0-5 and the loadgen on 6-11, 10 s after 2 s, one round, perf stat on the server for 8 s
(log: the Lang 2 notes, grav1.log). CPU us a request at c32:

| XO_STACK_SHIFT | linger | linger + B |
|---|---|---|
| 0 | 44.15 | **44.94** |
| 16 | | 44.65 |
| 48 | | 44.69 |
| 96 | | 44.69 |
| 3888 | **44.53** | 44.75 |
| others (16 to 2048) | 44.12 to 44.22 | 44.66 to 44.73 |

The bad phases exist on Graviton too but cost under 1%. The one counter that follows them
is st_align_lat (stores that pay misaligned-access latency): about 664 to 669 M in the
perf window at linger 3888 and linger + B 0 and 48, 890 M at linger + B 96, and 21 to 25 M
at every other phase, at c32 and c256. Instructions, cycles, L1D and TLB refills, and
branch misses are within 1% across phases. Reading: at the bad phases some stores, a few
hundred a request, cross a 4 KiB boundary; on the M4 Pro under the VM (4 KiB guest pages)
a page-crossing store is far more costly, and on darwin (16 KiB pages) the same offsets
cross no page. Linger + B 48's store count is up while its CPU is not (44.69), so
Graviton prices the crossing at almost nothing.

## Which stores: callee saved register pairs

The same Graviton round's second run (0.06 instance-hours) sampled st_align_lat with call
graphs on linger + B at XO_STACK_SHIFT 0 (bad) and 16 (good), c32. At 0 the samples fall in
num.json_layout (31%), obj.rs_append (15%), the handler main.run.fn2 (12%) and num.dbox
(11%); at 16 in the stream copies (memcpy, the kernel's copy_from_user), as at every good
phase. Those functions' prologues store callee saved registers in pairs from save_base,
which compiler/arm64 emit.xo put at spill_base + 8 * slots: json_layout's are
`stp x20, x19, [sp, #0x18]` and `stp x22, x21, [sp, #0x28]`. A 16 byte pair at 8 mod 16
crosses a 4 KiB page at one sp phase in 256. The linger + B server has 674 such pairs (all
callee saved registers) and 2095 aligned ones; the hot leaf callees each run once a field,
hundreds of times a request, each at its own offset: the comb.

Not decision 0395's stack Str buffers: XO_PASSES=-stack_small on linger + B (b80dff2e7,
host 1, 2 rounds) kept the gap, 47.7 us at shift 0 against 34.7 at 16 (c32; c256 47.0
against 33.9).

## The spread (measured, not adopted)

An experiment (e9eaf8dac and its linger builds ef0f87402, 39f8c1f5a; not on main) started each fiber's first frame at one of 64 phases 64
bytes apart from a per-worker counter (37 a step, the worker index times 11 as its start;
its XO_STACK_SPREAD=log on the second host, 2,000 connections: 7,089 fibers, 109 to 112 a
phase; 32 connections: 34 fibers over 26 phases). CPU us a request, host 1, median of 3
rounds:

| arm | c32 | c256 |
|---|---|---|
| linger + spread (ef0f87402) | 34.4 | 33.6 |
| linger, spread off | 34.5 | 33.5 |
| linger + B + spread (39f8c1f5a) | 35.1 | 34.3 |
| linger + B, spread off | 47.9 | 46.9 |
| main (02e6ffa1a) | 43.7 | 42.7 |
| main + spread (e9eaf8dac) | 35.5 | 34.9 |
| v0.35.0 | 35.8 | 34.6 |

The spread takes the bad phase from every connection to about 3% of fibers (phases 0 and 1
of 64 at most); main itself sat on a bad phase that day.

## The fix: callee saved registers from a 16 byte aligned offset

compiler/arm64 emit.xo now starts the save area at a multiple of 16 in both frame shapes and
pairs two registers only at a 16 byte aligned offset (decision 0459 item 5), so no prologue
stp straddles a page. The jitter server built with it has 2,550 sp relative x pairs, none at
8 mod 16 (674 before). Host 1, median of 2 rounds, CPU us a request, the fix cherry-picked
onto the shift experiment commits (f22eac717 on linger + B, bf9b620ec on linger):

| arm | XO_STACK_SHIFT | c32 | c256 |
|---|---|---|---|
| linger + B, no fix (b80dff2e7) | 0 | **47.8** | **47.0** |
| linger + B + fix | 0 | 35.3 | 34.2 |
| linger + B + fix | 16 | 35.3 | 34.2 |
| linger + B + fix | 48 | 35.3 | 34.3 |
| linger + B + fix | 64 | 35.2 | 34.2 |
| linger + B + fix | 96 | 35.2 | 34.3 |
| linger + B + fix | 4080 | 35.2 | 34.2 |
| linger + fix | 3888 | 34.7 | 33.6 |
| v0.35.0 | | 35.8 | 34.5 |

Every former bad phase now runs at the good phases' cost and at or below v0.35.0's. xo's
__text grows 216 bytes (32,698,876 to 32,699,092); its gate passed internal/arm64's full
tests, the self rebuild fixed point, the amd64 cross builds and test/codegen's 262
patterns.

## amd64: two word stack copies peeled at 8 mod 16

amd64 saves callee saved registers with pushq, one 8 byte store, which cannot straddle a
page. The one 16 byte store into the stack is x86_copy_runs (compiler/arm64/x86emit.xo):
it copies two words at a time through %xmm15 with movdqu, and into %rsp's words at 8 mod
16 such a store straddles a page at one stack phase in 256, as the arm64 pairs did. The
fix copies the first word of such a run alone, so every 16 byte store into the stack is
16 byte aligned; the emitter knows %rsp's phase from the frame (x86_rsp_phase: a leaf
frame is entered at 8 mod 16, a frame with a red zone sits at its pushes' parity).

Misaligned 16 byte stores into %rsp (static count of movdqu to `N(%rsp)` with
(N + phase) at 8 mod 16, linux/amd64), and .text:

| program | stores before | after | .text before | after |
|---|---|---|---|---|
| jitter server | 563 | 0 | 912,640 | 913,376 (+736, +0.08%) |
| xo (linux/amd64) | 82,849 | 0 | 97,974,400 | 98,053,408 (+79,008, +0.08%) |
| bench/perf/headphase | 119 | 0 | | |

Timing (AMD Ryzen 9 9955HX under WSL2, the linux/amd64 offload host): XO_STACK_SHIFT over
all 256 phases showed no per phase structure, with or without the peel (the correlation
between two rounds of the sweep was 0.11 before and -0.05 after, the noise about 7%), so
on this x86 a page straddling store costs too little to see in a parse loop; the peel is
kept for the CPUs where it does cost (it is the same pattern that cost 35% on arm64)
at 0.08% of .text.

The peel's own cost, `xo bench --ab` on the same host (ab-host, 3 rounds): headphase `run
200000 0 1 ... 255` (200,000 head calls under each of 256 pad depths in one process; pad's
frame is 112 bytes, 16 mod 32, so the depths reach every 16 byte phase of a page), A the
bench alone (1ad9a5c65), B with the peel (ea063a667):

| metric | A | B | B / A | 95% interval |
|---|---|---|---|---|
| wall ms | 199.0 | 199.7 | 1.004 | 0.976 to 1.01 (noise) |
| CPU ms | 197.7 | 197.6 | 1.000 | 0.972 to 1.024 (noise) |

Level: the peel costs nothing measurable here, and neither did the crossings it removes.
