# macOS bash 3.2: a `case` inside `$( ... )` needs `(pattern)` arms

Status: Observed 2026-11-22 (task #706, scripts/lead/tier-move.sh).

**Symptom:** `syntax error near unexpected token ';;'` from /bin/bash on
the Mac for a script that parses under newer bash; every function after
the error is undefined ("command not found").

**Cause:** bash 3.2 ends a `$( ... )` at the first unbalanced `)`, so a
case arm written `llvm) ...` inside a command substitution closes it.

**Fix:** write the arms with a leading paren, `(llvm) ...`, or move the
body into a function. Run `bash -n` with /bin/bash on the Mac, not only
on a linux host.
