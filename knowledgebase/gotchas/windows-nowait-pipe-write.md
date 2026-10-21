# A big write to a PIPE_NOWAIT pipe on Windows never starts

Status: Observed (2026-10-21, probe run 39693155106)

**Symptom.** On windows/amd64 the golden proc_cmd timed out after a minute
at `p.cmd("wc", ["-c"]).stdin(big.bytes()).output()` with 200000 bytes of
stdin (nothing printed: the program's output was buffered). The same
command with a short stdin, and every other statement of the golden,
passed.

**Cause.** The stdin write end is a PIPE_NOWAIT anonymous pipe (64 KiB
buffer). In that mode WriteFile does not write part of a buffer: when the
pipe has less free space than the call's size it returns success with 0
bytes written. proc.xo's collect loop passed the whole remainder (200000
bytes) every time, read the 0 as "try again later" (-EAGAIN), and never
wrote a byte, while the child waited for input.

**Fix.** windows.xo pipe_write writes at most 4096 bytes a call, so it
succeeds whenever the pipe has a page free, as the child drains it.

**Avoid.** A nonblocking Windows pipe write needs a size the pipe can take
whole; the Unix write(2) takes what fits.
