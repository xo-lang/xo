# Windows unwinding: compare a frame's own stack pointer with the boundary, not its caller's

Status: Observed (2026-10-16, xo-fa, the first run of the Windows fault
path: windows/amd64 goldens under Wine with Box64 in Docker).

## Symptom

On Windows a fault ran no cleanups and was caught by nothing: every
fault ended the program with status 70 at once, `defer` blocks of main
and of task bodies did not print (fault_unwind, fault_tasks,
fault_task_exit, fault_defer_chain, fault_exit, task_native and four
more of 24 fault and task goldens).

## Cause

std/native/llrt unwind.xo win_raise unwinds a frame with
RtlVirtualUnwind, which turns the context into the caller's, and then
compared that context's stack pointer with the boundary (the frame
address of main's or the task's entry). The frame just inside the
boundary (main, a task's body) has a caller past it, so the walk ended
before its landing pads were looked up. raise (Linux, macOS) compares the
frame's own stack pointer.

## Fix

win_raise compares the stack pointer of `here`, the frame's context
before the unwind. Box64 runs Wine's exception dispatch where Rosetta
cannot ("invalid gdt selector"): scripts/docker-wine.sh box64.
