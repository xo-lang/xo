# A native program's stdin from a FIFO never ends

Status: Observed (2026-10-16, darwin/arm64, both native backends); fixed 2026-10-16 (xo-4)

## Symptom

A native Xo program reading `os.stdio.read_line()` until it returns
None, with stdin redirected from a named pipe (`mkfifo f; prog < f &`,
then `exec 3> f`, a line written, `exec 3>&-`), never returns: no
writer holds the FIFO (lsof shows none), yet `x.main` stays parked in
`poll.pd_wait_ctx` and the program never exits. The same program with
stdin from an anonymous pipe (`producer | prog`) or `/dev/null` sees EOF
and exits at once.

Hit by bench/perf/tls's server workloads, whose first measure.sh held
the Xo server's stdin open through a FIFO; the run hung for 10 minutes.

## Cause

stdin.xo sysread asks poll(2) whether standard input is readable before
each read (a read that would block must wait on the poller instead). On
macOS neither poll(2) nor kqueue reports anything for a FIFO once its
last writer has closed (no POLLIN, no POLLHUP, no EV_EOF; checked with
Python's select on the same FIFO), though read(2) returns 0 at once. So
the reader waited on the poller 50 ms at a time, forever. An anonymous
pipe reports POLLHUP and works; Linux's poll reports POLLHUP on a FIFO
too (checked in Docker, both backends).

## Fix

On macOS, stdin.xo open marks a FIFO standard input (R_FIFO), and when
poll says it is not readable sysread probes it with a read that does
not block (O_NONBLOCK set with fcntl for the one read, the flags
restored): data, 0 at the end, or EAGAIN (a writer is still open: wait
on the poller as before). Test: internal/llvm TestStdinFIFO (a FIFO's
one writer writes and closes; the output must equal an anonymous
pipe's within 10 s, on llvm and arm64; behind testtmp.Lifeline).
