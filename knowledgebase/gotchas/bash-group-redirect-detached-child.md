# A detached child keeps a `{ } >/dev/null` group's caller waiting

Status: Observed (2026-11-15)

## Symptom

scripts/lead/xw-cc-hook.sh, written as `{ ...; ( xw ... ) & } >/dev/null
2>&1; exit 0`, returned at once but `out=$(... | xw-cc-hook.sh)` waited
20 s for a hanging stand-in xw: the caller's pipe stayed open.

## Cause

For a redirection on a `{ }` group, bash saves the original descriptor
(the caller's pipe) in a high descriptor for the group's duration. A
background subshell forked inside the group inherits that saved copy;
it is close-on-exec, so the exec'd xw drops it, but the subshell itself
(a bash waiting on xw) keeps it, and the pipe has a writer until it ends.

## Fix

Redirect with `exec >/dev/null 2>&1` at the top of the script instead
of a group, so no saved copy exists, and give the detached subshell
`</dev/null`. Test a hook-like script with a hanging stand-in and a
command substitution (scripts/lead/xw-cc-hook_test.sh).
