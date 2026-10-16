# Code reachable from fault.exit grows every raise's frame

Status: Observed (2026-10-16, XO_TRACEBACK, decision 0239).

Symptom: with fault.xo importing prof.xo (for the traceback), every
golden whose fault is raised inside a task (fault_tasks,
conc_send_closed, conc_reentrant, ...) died with SIGBUS (exit 138), a
stack overflow in unwind.step on the worker thread, even with
XO_TRACEBACK unset; main and every fault outside a task were fine.

Cause: cabi.xo fault_raise_cur calls fault.exit at the end of every
raise. With the traceback's printing (prof.xo trace_text: symbols,
the line table, lists) in exit's direct reach, the raise path's frame
grew, and a task's fiber stack, small by design, no longer held a raise.
Bisecting by file (each std file back to main's) found it; the trace
call itself was not the cause.

Fix: fault.xo does not import prof.xo; exit calls the printer
(cabi.xo fault_traceback) through its address, kept in a static word
at start, only when XO_TRACEBACK is on and frames were recorded. Keep
anything large out of fault.xo's direct calls: it runs on task stacks.

Two more traps of code that runs in fault.exit (same day):

- A test binary's fault test hung: the printer faulted (in a test
  binary its code checks for cancellation), the context still unwinding
  handed the fault on, and exit printed again, forever. exit now takes
  the trace out of the context and clears its unwinding and faulting
  flags before calling the printer, so a fault while printing ends the
  process with its own report.
- dwline.xo read files through sysio, which in a test binary is the test
  world's; it reads with plain system calls (read_raw).
