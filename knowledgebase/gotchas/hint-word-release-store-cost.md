# A release store on a hint word cost a hot path 1.5 to 3.4%

Status: Observed 2026-11-16 (#107, xo-lang-1).

## Symptom

Decision 0675's C call marks made std/sqlite point lookups 1.020 and
scans 1.023 of before on host 1. Inlining the marks into the adapter
recovered point lookups but not scans (0.989, 0.994 against 0.985, 0.967
with no marks at all).

## Cause

The marks stored W_CSEQ_I with release ordering (`stlr` on arm64), which
orders every earlier store of the thread before it, around every C call.
The word is a hint the monitor reads on its tick; nothing is published
through it.

## Fix

Relaxed stores (a99b162e9): 0.982 and 0.970. A per-thread word that
another thread only samples as a hint needs no release ordering; use
release only when the reader then reads data the writer published.
