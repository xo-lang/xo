# ConPTY: null standard handles, and a pipe that never ends

Status: Observed (2026-11-22, task #704, Windows amd64 host)

## Symptom

1. A child started with `PROC_THREAD_ATTRIBUTE_PSEUDOCONSOLE` wrote its
   output on the parent's own stdout, not into the pseudo console.
2. Reading the ConPTY's output pipe blocked forever after the child had
   exited and its output was read.

## Cause

1. Without `STARTF_USESTDHANDLES` the child inherits the parent's
   (redirected) standard handles and writes to them; the console is used
   only for handles that are null.
2. The output pipe belongs to conhost, which lives until
   `ClosePseudoConsole`; the child's exit does not end it.

## Fix

std/native/llrt (windows.xo `spawn` with an hpcon, ptyos.xo's W section):
set `STARTF_USESTDHANDLES` with null handles for the streams the ConPTY
serves (proc.xo `FD_NONE`). A read that finds the pipe empty and the child
exited waits for 100 ms of quiet output, then calls `ClosePseudoConsole`;
its last bytes follow, then `ERROR_BROKEN_PIPE`, read as the end.

Probe trap: `cmd /c "set /p L=& echo %L%"` prints `%L%` unexpanded
(cmd expands the line before `set /p` runs); use powershell `Read-Host`
to check that a written line reaches the child. Also a check that greps
one line of a multi-line result hides the rest of the output.
