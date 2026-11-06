# A Go ticker under a millisecond does not deliver its rate

Status: Observed (2026-11-05, stability/harness load on linux/arm64)

## Symptom

`harness load -rates 4000` (an open loop pacing one request per
`time.NewTicker(time.Second / rate)` tick) reached about 1,200 requests a
second for both the Xo app and its Go twin, with no drops and no errors, so
the run looked healthy while offering less than a third of its rate. 1,000/s
reached about 850.

## Cause

A Go ticker's channel holds one tick and drops the ticks its reader misses;
at a 250 us period the timer's wake and the reader's scheduling lose most of
them. The lost ticks are never counted anywhere.

## Fix

Pace by schedule, not by tick: a 1 ms ticker, and each tick sends what the
rate owes since the start (`rate * elapsed - sent`), counting a send no
worker takes as a drop (stability/harness load.go `driver.open`, test
TestOpenLoopRate). Check any Go load generator's achieved rate against its
offered rate before trusting its latencies.
