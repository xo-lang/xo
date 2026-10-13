# TestHybridPreemption's stop-the-world bound fails under full-suite load

Status: Observed (2026-10-13).

Symptom: internal/gen TestHybridPreemption fails in a full run with
`a stop the world took 22.000 ms (bound 10)` on all three attempts at
GOMAXPROCS=4 (batch 4, main d4a8b548, r8g.8xlarge, every package
running at once, 117 GB in use).

Cause: load, not a code change. Go's stop-the-world pauses grow when
the machine is saturated by the other packages' tests. Rerun alone on an
on-demand r8g.4xlarge at the same commit, `-count=5` passed 5 of 5.

Fix: rerun it alone before bisecting. A next step to remove the flake:
measure the bound relative to a control run on the same machine (the
test already has one without polls), or skip the stop-the-world bound
when GOMAXPROCS exceeds the free cores.
