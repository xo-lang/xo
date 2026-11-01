# The slot atomics' second argument is a word index, not a byte offset

Status: Observed (xo-lang-2, 2026-10-31)

Symptom: a probe counter stayed at 0. A count of list grows from a
capacity under four read 0 in a build where such grows were expected, and
the 0 was reported as a finding before the bug was seen.

Cause: `rt.slot_add(xs, i, d, ord)` (and `slot_load`, `slot_store`)
address word `i` of `xs`, while `rt.mem_load(a, off, 8)` and
`rt.mem_store` take a byte offset. `slot_add(addr, 8, 1, 0)` wrote byte
64 and `mem_load(addr, 8, 8)` read byte 8. Neither call faults, and both
addresses were valid words of the static table.

Fix: pass the address and index 0 (as std/native/llrt/budget.xo does:
`rt.slot_add[Int](b + B_REFS, 0, 1, RELAXED)`), or use the same word index
on both sides with `slot_load`. A counter that reads exactly 0 is a reason
to check the probe before reporting the 0.
