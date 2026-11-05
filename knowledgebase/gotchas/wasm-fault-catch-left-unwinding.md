# A caught child fault left a wasm task unwinding

Observed 2026-11-05.

Symptom: a wasm program (WASI, Node) stopped part way and exited 0, with
no error: after `task.isolate` caught the fault of a scope's foreground
child, the next `clock.sleep` (any suspension) ended main. Only when the
scope's body was blocked (in a sleep or an await) when the child faulted;
a body that had already returned was fine. Found by
conc_scope_bg_outlive's child fault path; every native backend and the Go
backend ran it.

Cause: the child's fault cancels the scope, so the blocked body resumes
unwinding ($unw = 1) and reaches scope_close in mode 2, which returns 3
(a child faulted) and raises the fault with $unw still set. $fault_take
(task.isolate's catch, compiler/wasm rt.xo) cleared $faulting and $prop
but not $unw, so the catching task went on "unwinding": its next
suspension returned through main_entry as if main had ended, and the
scheduler ended the program.

Fix: $fault_take clears $unw, as the native fault.take clears
C_UNWINDING; $unw moved from the task globals (rt3) to the base runtime's
globals, since every program has $fault_take. A new runtime flag that a
native catch clears must be cleared by the wasm catch too.
