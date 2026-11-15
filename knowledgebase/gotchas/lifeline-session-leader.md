# The lifeline wrapper failed for a session leader

Status: Observed (2026-11-15)

## Symptom

`xo repl` processes from cmd/xo TestReplLineEditing stayed on the Mac
after their tests (three, 2026-11-15). The test kills script(1) on a
timeout, and `t.Fatalf` in its prompt wait killed nothing.

## Cause

script(1) runs its program as the leader of a new session on the pseudo
terminal, so killing script leaves the program running. The test did not
put xo repl behind the lifeline wrapper (internal/testtmp/lifeline). The
wrapper could not have run it anyway: its `setpgid(0, 0)` fails with
EPERM for a session leader, and it exited 127.

## Fix

The wrapper accepts a failed setpgid when the process already leads its
group (a session leader does). TestReplLineEditing builds the wrapped
command with `testtmp.Lifeline` and passes its file descriptor 3 through
script. On Linux the shell line starts with `exec`, so the wrapper is the
session leader and its group stays the terminal's foreground group. A
cleanup kills script itself. TestReplWarmup and TestReplJSON run behind
`testtmp.Lifeline` too. Test: internal/testtmp TestLifelineSessionLeader
(the old wrapper fails it).
