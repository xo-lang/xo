# A deterministic run's idle root missed its ready list

Status: Observed (2026-10-23, decision 0350 stage 2)

## Symptom

Under `xo test --seed`, every test that waits on the test world's fake
clock (sleeps, deadlines, grace periods) hit the 60 s real time limit.
A sample of the test binary showed the root thread on a futex in
root_main and no other thread.

## Cause

A det build runs every fiber on one thread and keeps ready fibers in
det.xo's ready list, not in the run rings. The idle path's deadlock
check kicks the virtual clock, which wakes the sleeper onto that list,
then asks pool.xo any_work whether to park; any_work looked only at the
rings and the inject queue, so the root parked with work ready.

## Fix

any_work returns 1 when det.built() and det.ready() > 0. Any new
"is there work" test in pool.xo must count the ready list too
(TestTestSeedFakeClock).
