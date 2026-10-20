# xo def missed a prelude method it then suggested (transient)

Status: Observed, cause open (2026-10-20, decision 0289's resolver)

Symptom: from a checkout's root with XO_STD set to that checkout's std,
`xo def --text Str.to_upper` printed "no declaration named Str.to_upper;
did you mean Str.to_upper (std/prelude.xo:265)". Bytes.to_hex did the
same. `xo query def` of the same name resolved. Lib 1 saw it on 6 of 6
runs in one worktree; Xo Publish saw it once in about 10 in another.
Minutes later neither could reproduce it, with the same binaries,
directories, and environment (PWD equal to `pwd -P`, no XO_WORK,
XO_MODCACHE, or XO_CHK_CACHE). Only shared state had changed: the user
cache and other sessions' builds.

Cause: not found. The resolver (semantic.Session.def) found the
declaration; agent.xo found_at then found no identifier starting at its
position, so the hit was dropped and only the suggestion scan (a text
scan of std) named it. Suspects: a shared cache entry from another
worktree's std (Lib 1's guess; the agent check uses no module check
cache, so which one is not known), or a position mismatch between the
object's recorded line and column and the walked identifier.

Fix (symptom): found_at's miss falls back to the declaration's object
(agent.xo found_obj, Res.obj_target), so a resolved name is always
reported. If it recurs, capture `xo def --json NAME` and `xo query def
--json NAME` from the failing shell before anything else runs, and list
<user cache>/xo with modification times.
