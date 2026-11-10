# bash reads a running script from the file as it goes

Status: Observed (2026-10-20, 2026-11-10)

## Symptom

A long script fails partway with an error its text does not have:
`scripts/release-vs-go.sh: line 430: syntax error near unexpected token
'('` (2026-11-10, an HTTP section that had waited an hour for its host),
or it runs a step twice (2026-10-20, a lead gate script reran a gate).
`bash -n` on the file passes.

## Cause

bash does not read a script whole before it runs: it reads and runs it a
command at a time, keeping a byte offset into the file. An edit to the
file while the script runs (here a `git commit --amend` in the worktree
the script came from, which rewrote scripts/release-vs-go.sh with lines
added above the running part) leaves that offset in the middle of a
different line, so bash reads a fragment as the next command. The
script's own long waits (a host's lock, a load wait) make the window wide.

## Fix

Never edit, amend, check out or rebase a file a running script is
reading, including a script started from a worktree you keep working in.
Copy the script to a new name and edit the copy (the lead's rule for its
own scripts), or work in a second worktree until the run ends. To rerun
after it happened: the file on disk is right; start it again.
