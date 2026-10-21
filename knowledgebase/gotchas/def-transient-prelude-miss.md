# xo def missed a prelude method it then suggested (transient)

Status: Observed, cause open (2026-10-20, decision 0289's resolver); second case 2026-10-21, likely a silently skipped directory listing

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

Second case (2026-10-21, Xo Publish, from a worktree's root during the
memory reaper's pressure, load average about 87): `xo def --dir=.
Checker.check_dirs`, `xo def check.Checker.check_dirs` and `xo doc
check.new_checker`, run back to back, all printed a bare "no declaration
named ..." (no "did you mean", so neither the resolver nor the
declaration scan saw the compiler's modules at all). Minutes later the
same commands hit, and 8 more runs at the root all hit. This is a
different miss from the first: the program loaded without compiler/'s
modules, not a found declaration dropped by found_at.

Likely cause (not confirmed by a reproduction): the root walk,
find_module_dirs and load_dir in compiler/check/load.xo, listed each
directory with `read_dir(dir).ok() ?? []`, so a listing that failed for
any reason (EMFILE or ENOMEM under load) dropped that directory's whole
subtree from the check without an error. Fix (local branch
check-walk-err, for the morning): Checker.list_dir skips only NotFound,
Denied, and NotDir; any other failure sets walk_err and check_dirs fails
with it ("read DIR: ..."), so a tool answers with the error instead of a
wrong "no declaration". Test: compiler/check/check_test.xo "a directory
that cannot be listed fails the check" (FlakyFs). If a bare miss recurs
after it lands, the cause is elsewhere: capture as above.
