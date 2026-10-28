# 0395: small Bytes and Str that do not escape live on the stack

Status: Proposed (the owner approved the direction, 2026-10-28, through the
lead; draft by xo-lang-5; measurements pending, below)

## Context

std/uuid's `bytes()` row is 1.97x Go's time (bench/perf/uuid, measured
2026-10-28): 39% of the run is allocating and freeing the 16 byte Bytes
each call returns, where Go's `u[:]` is a slice of the value's own array
and its escape analysis keeps that array on the stack. Every native Bytes
and Str is a reference counted heap object (decision 0046), however small
and however briefly it lives.

## Decision (draft)

A Bytes or Str value of a known size up to a limit (64 bytes proposed)
whose object provably does not outlive its function is built in the
function's frame instead of on the heap:

- Scope: the value's every use is a read (length, index, slice view,
  comparison, hashing, formatting into other text), a pass to a borrowed
  parameter (decision 0046's borrow inference), or a copy of its bytes out.
  A use that would retain it (a store into a struct, list, map, closure or
  box, a return, a pass to an owned parameter, a send) sends it to the
  heap, as today. Views of it (decision 0330) are uses of it.
- Reference counting: a stack object carries a count the runtime never
  frees (an immortal mark, as constants have), so a retain or release that
  reaches it is harmless; insert_rc emits none for it, and the verifier
  rejects a retain of it that can outlive the frame.
- Verifier: a stack object never reaches a heap store, a return, an owned
  call argument or a closure environment on any path; a function whose
  analysis cannot prove that keeps the heap path.

## Measurements (pending)

1. The ceiling: uuid bytes with its allocation replaced by a hand written
   stack buffer (a throwaway).
2. How many allocations in the compiler's self build and in the
   release-vs-go rows are small Bytes or Str that do not escape.

## Consequences, Alternatives

To follow with the measurements.
