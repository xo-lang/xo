# A debugger's trap write: flush the caches, stop the other threads

Status: Observed (2026-11-05, decision 0253 stage 2, std/native/llrt/dbgmac.xo)

## Symptom

`xo debug` with a breakpoint in a loop: the first stop works, and after
`continue` the program usually runs to its end (19 of 20 runs) although
reading the address back shows the `brk #0` word. Separately, a
`continue` stops with `stopped by exception 1 (code 2, subcode
0xADDR)`, an EXC_BAD_ACCESS whose address is the pc of a runtime
function (rtstats.rec) on the same 16 KiB page as the breakpoint.

## Cause

1. The word written through mach_vm_write reaches memory, not the
   instruction cache: a trap planted again after the step over it is
   not executed until the line is evicted.
2. mach_vm_protect cannot make a text page writable and executable at
   once (macOS refuses READ|WRITE|EXECUTE), so the write's window has the
   page without EXECUTE; the program's other threads (the scheduler's
   workers, even in a single threaded program) keep running at a stop
   and fault when they execute on that page.

## Fix

After each write, mach_vm_machine_attribute(task, addr, n, MATTR_CACHE,
MATTR_VAL_CACHE_FLUSH), as lldb's debugserver does (20 of 20 runs). At
each stop, task_threads and thread_suspend every thread but the stopped
one, and resume them when the program continues; the step over a trap
runs the stopped thread alone (dbg_resume's `alone`). 100 of 100 runs
clean with both.
