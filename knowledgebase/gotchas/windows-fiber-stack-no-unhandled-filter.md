# Windows: a fault on a fiber stack never reaches the unhandled filter

Status: Observed (2026-10-22, decision 0343 stage 2, Wine under Box64)

Symptom: a windows/amd64 program with SetUnhandledExceptionFilter
installed (std/native/llrt/crash.xo on_crash_win) wrote nothing on an
access violation in Xo code: not the report, not even standard output
the filter would have flushed. Exit code 0xC0000005 as before.
`WINEDEBUG=+seh` showed the vectored handlers (entry.xo on_overflow)
and one SEH handler (xo_x_personality, which returns
ExceptionContinueSearch) called, then "NtRaiseException Unhandled
exception" with no filter call.

Cause: the unhandled exception filter is not called by the exception
dispatcher. It is called from the `__except` filter of
RtlUserThreadStart (BaseThreadInitThunk), the bottom frame of every
thread's own stack. Xo code runs on fiber stacks (main runs on a fiber
since main-on-fiber, tasks on the pool's fibers); their frames end at
the fiber's entry, so the dispatch finds no handler, and Windows ends
the process without the filter (on a real system: the second chance
raise, then termination).

Fix: a vectored exception handler registered last (crash.xo
on_crash_veh) that reports only when no handler could take the
exception: the faulting pc and every frame to the end of the stack in
the program's image, none with a handler but Xo's personality
(unwind.xo win_unhandled: RtlVirtualUnwind with UNW_FLAG_EHANDLER over
the image's own .pdata). Anything else (a DLL's fault, a C `__try`, a
thread start in kernel32) is left to its handlers and the filter. Any
other "last chance" hook on Windows (a crash dump writer, a logger) has
the same trap.
