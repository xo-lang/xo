# A decision narrowed after measuring, the prototype's code not

Status: Observed (2026-11-02, decision 0455).

Symptom: f5392f175 shipped small first heap chunks on Linux too, while ADR 0455, the
CHANGELOG and the commit message all said darwin only; Linux's conc-spawn edit paid 2 to 3%
wall for no RSS gain until 2d90b65d9 (about 40 minutes on main).

Cause: the prototype was measured on both platforms (the ADR's "first only" Linux rows came
from it), the decision then narrowed it to darwin, and the narrowing went into the prose
only. The push cherry-picked the measured WIP commit; its gate (darwin builds, darwin tests)
could not see a Linux only difference.

Fix: when a decision narrows what was measured (a platform, a mode, a size), make the code
change in the same step as the ADR text, and grep the pushed diff for the narrowed condition
(here `TargetOs.Linux`) before pushq; a platform gated change gates with one build and run on
that platform's target.
