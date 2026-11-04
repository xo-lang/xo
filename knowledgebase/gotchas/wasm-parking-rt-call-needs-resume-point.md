# Wasm runtime: a runtime call that parks the task needs a resume point

Status: Observed (2026-11-04, decision 0447 stage 5 step 3, compiler/wasm)

## Symptom

A wasip3/wasm component with rt10.xo's parking calls built and served
simple routes, but a streamed POST /echo answered "len 0", and every
outgoing request trapped in `wasi_http_resp_headers` with "unknown
handle index 0".

## Cause

A runtime function that waits parks the task: it stores the task as the
waiter, calls `$block` (which sets `$sstate` to 1), and returns dummy
results. The caller has to see the suspension and unwind, so that the
call repeats when the task runs again. That needs two things in the
code generator, and rt10's calls had neither:

- a resume point before the call and `suspend_check` after it
  (call.xo's rt3_call does this for rt3_blocking and rt9_blocking
  names; rt5-style calls went through rt_call, which has neither);
- the name in gen.xo's `blocks` test, which decides whether functions
  keep frames at all.

So the caller took the dummy results as real: an empty body read as the
body's end, and a send that had only started as a finished one.

A second trap once that was fixed: every new request task faulted at
wasm address 0xffffff70 (an empty save stack popped for a 144 byte
frame). The scheduler resumes a task with `$sstate` 2 (rewinding), and
the runtime function the rewind reaches must end it (`call $resumed`,
as rt3.xo's waiting functions start); rt10's did not, so the next
function called, a new task's main, took itself for a rewind.

## Fix

Name every call that can park in one list (rt10_blocking), and use that
list in call.xo (resume point, then `suspend_check`), in gen.xo's
`blocks` test, and in its per-function `s` (suspendable) mark, and start
each such runtime function with `call $resumed` (drop its result when
the function does not need it). A new
parking runtime call must be added to the list, or it fails in this way.

## Related: a flag test anded with a Bool

With the tests passing, the first load run stopped at exactly 42,623
requests (about 333 instances x wasmtime's 128 reuses each) with
"maximum concurrent limit of 1000 for core instances reached", at any
concurrency, even one request at a time. A finished request's export
returned WAIT forever: p3_after tested `$p3_has(R, 8192)` `i32.and` "no
kick pending" (1), and $p3_has answered the masked bit (8192), so the
and was 0. wasmtime keeps an instance while a task of its runs, so no
instance was ever torn down. $p3_has now answers 0 or 1. Found by a
debug line printing R's flags on each WAIT: the third line of each
request showed done (0x2000) with nothing outstanding.
