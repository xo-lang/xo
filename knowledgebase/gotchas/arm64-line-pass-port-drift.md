# A ported arm64 line pass drifts from its text pass

Status: Observed (2026-10-21, Xo Lang 2, decision 0274 step 3).

Symptom: an XO_OP_VERIFY=1 build fails with `XO_OP_VERIFY: FUNC:
reload_reuse_ops differs from reload_reuse_l` (or copy_runs_ops and
copy_runs_l), though neither port changed.

Cause: during step 3 each ported line pass exists twice: the text pass in
compiler/arm64 lsr.xo, which still makes the build's output, and its port
on items in lines.xo, which XO_OP_VERIFY compares with it function by
function. A change to the text pass alone (411089044 taught
reload_reuse_l to forget only the scratch register a line writes) makes
the port disagree.

Fix: change the port in the same commit (lines.xo; for a port derived
mechanically, as reload_reuse_ops is, derive it again from the new text
pass), and run an XO_OP_VERIFY=1 build of a golden or two with
XO_BACKEND_CACHE=off. The note above each ported pass in lsr.xo says so.
