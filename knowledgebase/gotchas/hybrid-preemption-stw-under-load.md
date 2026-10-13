# TestHybridPreemption's stop-the-world bound fails under full-suite load

Status: Observed (2026-10-13). Fixed (2026-10-13); second failure mode (no GC ran) below, fixed 2026-10-13.

Symptom: internal/gen TestHybridPreemption fails in a full run with
`a stop the world took 22.000 ms (bound 10)` on all three attempts at
GOMAXPROCS=4 (batch 4, main d4a8b548, r8g.8xlarge, every package
running at once, 117 GB in use).

Cause: load, not a code change. Go's stop-the-world pauses grow when
the machine is saturated by the other packages' tests: a thread the OS
has not scheduled cannot reach a poll or take a preemption signal, so
plain Go code pauses as long. Rerun alone on an on-demand r8g.4xlarge at
the same commit, `-count=5` passed 5 of 5. Reproduced on the 12-core Mac
on 2026-10-13 with 36 `yes` busy loops (started and stopped by PID):
the hybrid build's longest stop the world was 27 to 78 ms, the same
program with spin in Go (xo_pure) 15 to 118 ms, and the build without
polls 6.4 to 6.8 s; idle, the hybrid build's was 0.05 to 0.07 ms. The
old test failed 2 of 2 under that load (stop the world 53 to 128 ms);
main's gap at GOMAXPROCS=1 also missed its 100 ms bound once (189 ms).

Fix: the test keeps its fixed bounds (10 ms stop the world, 100 or
250 ms gap) and, only when a run misses them, runs the pure control
right after it under the same load and widens each bound to twice the
control's figure plus a margin (20 ms stop the world, 50 ms gap), capped
at a quarter of the hybrid run's loop. Without polls the first stop the
world waits for the rest of the loop and main's gap is the whole loop, so
the cap always catches it. Under the same 36 busy loops the new test
passed 5 of 5 (one attempt retried: 185 ms against a widened 128 ms);
with the polls disabled (HybridNoPoll on the hybrid build) it failed in
all three configurations (main stalled 8.8 to 9.1 s against widened
bounds of 368 to 538 ms). If it still fails in a full run, read the
logged control figures: a hybrid figure far above the control's is a
real regression.

## "no GC ran during the loop" (Observed 2026-10-13, fixed 2026-10-13)

Symptom: on an on-demand r8g.8xlarge (the gocache agent's stress runs,
main 24982a6a and its branch), the -short test failed every attempt at
GOMAXPROCS=4 with `no GC ran during the loop`: the hybrid run and the
pure control both logged 0 GCs (spin about 500 ms, 300 to 450 main
iterations).

Cause: not load and not preemption. goldenEnv drops GOGC, so the
program ran with the Xo heap floor (decision 0065: 16 MiB at
GOMAXPROCS=4). On Linux a 100us sleep takes about 1 ms, so a 0.5 s
-short loop made only about 400 iterations of a 2000 element List,
about 12 MB in all, and never reached the goal. On the Mac (sleeps near
their length) the same loop makes about 2300 iterations and 5 GCs.
Reproduced on the Mac with the hybrid binary at 300000 rounds (306
iterations, about 300 on EC2's -short): 0 GCs with the floor, 13 at
GOGC=10.

A second trap showed up once GCs were forced: without polls, the GC
whose stop the world waits for the loop ends only after it, and the
program exited before that GC printed its gctrace line, so the no-poll
control logged 1 GC with a 0.007 ms stop the world and its check
(both bounds missed) failed 9 of 10 full runs.

Fix: preemptRun sets GOGC=10 (heap goal near 400 KB, a GC every few
dozen iterations, independent of sleep granularity), and the program
sleeps 200 ms after its loop (outside total_ms) so a GC stopped behind
the loop finishes and prints. Mac, 2026-10-13, under 24 busy loops plus
a loop of `go test -short` over every package: polls on, 10 of 10
-short and 10 of 10 full passes (36 to 249 GCs per GOMAXPROCS=4 run,
longest stop the world 46 ms, one attempt retried); the in-test no-poll
control saw both bounds missed in all 10 (stop the world 5116 to 7264
ms); with the hybrid build itself without polls (HybridNoPoll), 0 of 3
-short and 0 of 3 full passes (stop the world 695 to 5179 ms). The EC2
rerun was not done: the worktree isolation check refused
`scripts/remote-full.sh --cmd`.
