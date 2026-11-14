# The memory budget's stop re-entered itself

Status: Observed (2026-11-14, cmd/xo TestMemBudget on linux/arm64 host 1,
intermittent).

## Symptom

`xo build` under `XO_MAX_MEM=16M` exited -1 (a signal) instead of 70. The
report repeated with a growing count, each cut short after the budget's
digits ("fatal error: memory budget exceeded: 18 MB mapped, more than
XO_MAX_MEM (16fatal error: memory budget exceeded: 22 MB mapped ... 26 ...
90 MB"), then a SIGSEGV whose frames repeated a cycle of five: a stack
overflow.

## Cause

std/native/llrt/membudget.xo over_budget printed its last line with
`stdio.err(p, n, true)`. With a newline, stdio.err copies the line into a
buffer it allocates (one write for text and newline). When that
allocation found its heap chunk full, the heap mapped another chunk,
membudget.add saw the count still past the budget and called over_budget
again, recursively, until the stack ran out. Decision 0633's region
made it likely: region_chunk counted a new 4 MiB chunk before the region
record held it, so a stop raised by that count allocated from the old,
full chunk, carved another, and was stopped by its count (the report's
count grew by 4 MB each time). It failed only when the build's first
mapping past the budget was a region chunk, so now and then.

## Fix

over_budget prints with nl false only (the newline inside the constant),
so it allocates nothing, and the first mapping past the budget takes the
budget with a compare-and-swap (sets it to 0) before stopping, so a
mapping the stop makes, or one on another thread, cannot stop it again.
region_chunk and the in-place grow count a mapping only once the record
holds it. Anything on the stop's path writes to stderr without
stdio.err's newline flag, and a mapping is counted after the heap can
allocate from it.
