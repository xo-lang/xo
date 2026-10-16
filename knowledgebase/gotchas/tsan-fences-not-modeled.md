# ThreadSanitizer reports a fence and a plain store as a race

Status: Observed (2026-10-16, decision 0241's addendum)

## Symptom

With the runtime built under `--race`, ThreadSanitizer reports a data
race between a store and a load that the code orders correctly in
C11's terms: poll.xo's timer fired flag (fire: `rt.fence(RELEASE)` then
a plain 4 byte store; fired: a plain load then `rt.fence(ACQUIRE)`), and
mutex.xo's reader grant flag (the same shape).

## Cause

ThreadSanitizer does not model standalone fences: it orders accesses
only through atomic operations with acquire or release semantics (and
locks). A plain store after a release fence is, to it, an unordered
plain store, whatever the fence promises on the hardware.

## Fix

Put the ordering on the access: `rt.slot_store(p, i, v, RELEASE)` and
`rt.slot_load(p, i, ACQUIRE)` of the word (an 8 byte slot: a 4 byte
flag either moves to a word of its own or becomes a half of the word
it shares, mutex.xo GRANTED). On arm64 a release store is `stlr` and an
acquire load `ldar`, no slower than the fence pair. Likewise a word one
thread reads without a lock while others store it under one (stdio.xo's
buffer length, the statistics switch) needs relaxed atomic accesses on
both sides; a relaxed slot load or store is the same `ldr` or `str`.
