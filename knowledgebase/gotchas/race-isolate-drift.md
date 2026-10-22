# A fault caught by task.isolate loses its frames' TSan exits

Status: Observed (2026-10-22, Xo Lang 2)

Symptom: with --race stacks (decision 0273's stacks addendum), after a
loop of 200 `task.isolate` calls that each caught a fault, a race report
showed main's frame repeated 15 and more times (`xo_f_main main.xo:18`,
the isolate call's line).

Cause: each instrumented function calls the entry hook after its entry
and the exit hook before its returns. A fault unwinds past the returns,
so the frames between the fault and the catch never call exit. A fault
that ends its task does not matter (the task's TSan context is dropped),
but task.isolate catches in place, in the calling function, and keeps
running on the same context, whose shadow stack keeps the dead frames.

Fix: the hooks count the context's depth (ctx.xo C_RACE_DEPTH); in a
function that catches (calls xo_x_fault_take), the entry's returned depth
is kept and race_unwind_to(depth) runs after the catch, calling the
missing exits. internal/llvm TestRaceStacksArm64 checks it.
