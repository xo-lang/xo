# selfhost/ssa: made up locals are negative, so -1 is not "none" for them

Status: Observed (2026-10-11, SSA stage 4)

Symptom: the Xo SSA builder lowered `while x > 1 bound 64` without the
bound's counter: the Go builder's loop head had one more block parameter
and the XO-F006 check; the counter's `const 0` was there, its uses were
not.

Cause: `loop_bound` returned the counter's made up variable, or -1 for
a loop without a bound, and `loop_tick` returned early on a negative
value. Made up variables (`Builder.local`, the Go builder's
`local("name")`) are negative ids (-1000001 down), so every real
counter looked like "no bound".

Fix: `loop_bound` returns 0 for none and `loop_tick` asks the bound
table (`S.bounds`). In selfhost/ssa an Int that may hold a variable uses
a table or 0, not a sign test; value, block, type, and object indexes
are the ones where -1 means none.
