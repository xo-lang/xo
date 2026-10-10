# Faults inside native deferred code skipped the cleanup and kept the shield

Status: Observed (2026-10-10, decision 0085 amendment)

Symptom 1: on the LLVM and arm64 backends, deferred code that faulted
ended the function's cleanup there: the deferred statements registered
before it and the outer blocks' deferred code did not run, and with two
faulting deferred statements the first fault was reported instead of
the last (the Go backend runs them all and reports the last).

Cause 1: internal/ssa places no landings inside deferred code
(`s.shield > 0`), so `addLandings` gave its faulting operations the
function's landing, which only releases references.

Symptom 2: after `task.isolate` caught such a fault, the task never saw
cancellation again: a sibling's fault left it waiting on a channel and
the program ended with XO-F001 (deadlock).

Cause 2: `xo_shield_begin` raised the context's shield count and only
`xo_shield_end`, after the deferred code, lowered it.

Fix: landings in top level deferred code end the shield
(`rt.shield_abort`) and jump to a per frame continuation chain
(build_defer.go `faultCont`); golden `fault_defer_chain`.

Trap met while building it: the builder places an operation's landing
when the next instruction is added to the same block (`s.pending`,
`flushLanding`). Without concurrency no `shield_end` follows a deferred
call, so the pending call was still open when `faultCont` started
building another block, and `flushLanding` silently dropped it (`w.Block
!= s.cur`). Any code that switches `s.cur` must flush the pending
landing first.
