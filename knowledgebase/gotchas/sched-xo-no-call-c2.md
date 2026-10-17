# sched.xo, rc.xo and alloc.xo cannot call rt.call_c2 or rt.fn_addr

Status: Observed (2026-10-15, decision 0233 step 2)

## Symptom

A std change that replaces an `rt.*` intrinsic call in
std/native/llrt/sched.xo (or rc.xo, alloc.xo) with
`rt.call_c2(rt.fn_addr("export"), ...)` fails the build of xo with:

    error XO1201: ... a call of `rt.call_c2` in a reference count or heap function (decision 0207: no allocation, no counting)

The message names no function or file.

## Cause

compiler/ssa contract.xo check_no_alloc holds every function of
native/llrt/rc, native/llrt/alloc and native/llrt/sched to decision 0207's
rule: scalar values only, and calls only of the intrinsics in
rc_callee_ok (raw memory, slot atomics, rt.heap_*, the scheduler's
rt.stack_switch, rt.sched_hook, rt.sched_run_task, ...). rt.call_c2 is
not in that list.

## Fix

Keep the intrinsic in the restricted module and point it at the export
in compiler/ssa exports.xo rt_export (both emitters call the mapped
xo_x_ symbol). Keep the old C symbol as a forwarder until the next seed,
because the seed's code still calls it. Example:
`rt.sched_run_task` maps to `xo_x_fiber_run_task`, and xo_rt.c keeps
`xo_sched_run_task` as a forwarder for the seed.
