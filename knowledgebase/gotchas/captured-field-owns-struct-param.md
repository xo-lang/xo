# A captured heap field makes the whole struct parameter owned

Status: Observed (2026-11-07, std/metrics series_of)

## Symptom

Every labelled `Counter.inc` and `Histogram.observe` retained and released
each heap field of the instrument's `Family` (name, help, labels, bounds,
the series Mutex, the dropped Atomic, reads, objectives): the function
began with drop calls of the fields it never read and ended with drops of
the rest, each an atomic count since the Family is shared between threads.
Nothing in the hot path (a lookup under a read lock) used those fields.

## Cause

Borrow inference (decisions 0046 and 0149) makes a struct parameter owned
when any path consumes any of its heap leaves. The lookup's miss path
built a closure for `Mutex.with` that captured `let bounds = f.bounds`, a
List: a capture consumes it, so `f` became owned, its callers retained
every field before the call, and the callee released them, on the hit
path too. The parked part-borrow work (bench/results/part-borrow-2026-11-02.md)
is the general fix that was measured and not adopted.

## Fix

Capture what the cold path needs without a heap leaf of the parameter
(`let nb = f.bounds.len()` instead of the List), or pass the fields the
cold path consumes as their own parameters. Check with `xo asm`: a
borrowed struct parameter's function has no drop calls in its prologue.
