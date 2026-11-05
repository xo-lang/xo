# A test that writes to a peer that does not read hangs once the kernel's buffers are full

Status: Observed 2026-11-04 (tier 48, internal/llvm TestGolden net_close_flush on the Mac)

## Symptom

A program that writes many bytes on one end of a loopback TCP pair, and has
the other end read only after the write returns (a channel the writer sends
on after its write and close), sometimes hangs forever. Every worker is idle
in cond_wait or the poller's kevent. netstat shows the reading socket's
Recv-Q and the writing socket's Send-Q full (408,604 + 351,788 of 1 MiB that
time), the connection ESTABLISHED.

## Cause

A write returns only once the kernel has taken every byte. With the reader
not reading, that is the send buffer plus the peer's receive buffer, which
on macOS start at 128 KiB each (net.inet.tcp.sendspace and recvspace) and
grow by autosizing up to 4 MiB depending on timing. A 1 MiB write fits only
when autosizing grew the buffers in time, so the test passed most runs and
hung under load. This is the program's own deadlock, not a lost wakeup.

## Fix

Have the reader read while the writer writes, and send the "done" word
after the close for the reader to take after the end of the stream
(net_close_flush since 2026-11-04, still 1 MiB), or keep what is written
before the peer reads under the default buffers (128 KiB fits the macOS
send buffer alone; Linux's defaults are larger).
