# xotool hung forever building the seed on Windows

Status: Observed (2026-10-13, building the windows-amd64 seed)

## Symptom

`go run ./scripts/xobuild` on a fresh Windows Server instance printed
"xotool: building the go-compiler-final seed in C:\...\seed\...\tree"
and then nothing for 47 minutes: a `git archive --format=tar` process
with almost no CPU, no `tar` process, the tree directory empty.

## Cause

internal/xotool piped `git archive` into an external `tar -x -f - -C
DIR`. On Windows the `tar` on PATH is Git Bash's GNU tar, which misreads
the `C:\...` path given to -C ("tar: C\:...: Cannot open") and exits.
The Go process still held the read end of the pipe (exec's StdoutPipe
passed to the second command), so git never saw a closed pipe and
blocked writing forever; the error from tar was never reached.

## Fix

archiveTree (internal/xotool/chain.go) unpacks the stream itself with
archive/tar, used by both the go-compiler-final root and the chain hops,
and drains the pipe before waiting for git, so a failure returns an
error instead of hanging. TestArchiveTree covers it. Never pipe one
child process into another with exec while keeping the pipe in the
parent; and do not rely on the PATH's tar on Windows.
