# A helper taking a wide value by value copies it twice per call

Status: Observed (2026-11-07)

## Symptom

One `==` of a 4,096 word struct grew native __text from 423,752 to
801,600 bytes and took clang 188.8 s against 12.1 s without it.
Comparing runs of plain words with one byte compare (the first fix)
left native __text at 670,568 bytes and clang at 86 s.

## Cause

The compare was not the cost. The equality helper took both values by
value: the caller loaded every word and stored it to the outgoing stack
arguments at each `==` (24,736 instructions in main for two), and the
helper's prologue copied every stack argument back to its frame
(36,917 instructions), a load and a store, plus a movz/movk pair past
arm64's 32760 byte offset, per word. On LLVM the same values went
through first class aggregates of 4,096 fields at the call and a store
of each into an alloca.

## Fix

A helper of a type of BY_REF_MIN (256) bytes or more takes pointers,
as decision 0145's calls and LLVM's count helpers (rc_by_ptr) already
did: arm64 `eq_by_ref`/`eq_addr` (compiler/arm64/closures.xo), x86
`x86_eq_body`, LLVM `eq_by_ptr`/`val_ptr`. The caller passes the
memory already holding the value (mem_of, g.addr, the held memories)
or stores it to a frame slot once. Before tuning a helper's body for a
wide type, count the instructions of its prologue and of its call
sites in an asm dump (`--asm=FILE`): per function sizes are the
fastest way to see which side grew.
