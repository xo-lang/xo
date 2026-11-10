# FreeBSD: callers of sock.xo and linux.xo get Linux's ABI

Status: Observed (Xo Lang 1, 2026-11-09, FreeBSD stage 2).

## Symptom

On FreeBSD every HTTP golden's client failed with "protocol error ... EOF"
right after sending its request; truss showed the client's `read` return
EAGAIN (FreeBSD errno 35) and the connection closed at once, and the server
then saw ECONNRESET.

## Cause

std/native/llrt/sock.xo gives FreeBSD Linux's socket ABI (fret maps errnos
to Linux's, 35 to 11; fb_sa_out rewrites a sockaddr as Linux's, family a
u16 at offset 0). The callers' per-OS arms (hclient, hconn, tlssrv and
tlsio `would_block`, hconn's peer family read) grouped FreeBSD with Darwin,
whose layer keeps BSD errnos (35) and sockaddrs (family byte at offset 1).
A would-block read looked like an error.

## Fix

Those arms take FreeBSD with Linux. Rule: a caller of sock.xo matches
FreeBSD with Linux; only code that issues FreeBSD system calls itself
(freebsd.xo, poll.xo's kqueue arm, fsops' layer 4) uses FreeBSD's numbers.
## Also: linux.xo called on FreeBSD issues Linux's numbers (Observed 2026-11-09)

Symptom: SIGSYS (rc 140) from `compat.getkerninfo` (Linux x86_64 uname, 63), truss showing
sched_get_priority_max(-100) (Linux statx, 332) and shmat(1, ...) (Linux clock_gettime, 228); "server misbehaving"
resolving localhost (Linux openat 257 on FreeBSD); every fault escaping isolate/defer (auxval returned 0, so the
unwinder found no .eh_frame_hdr).
Cause: code with a Linux arm that FreeBSD falls into (dns.xo os() is 1 on FreeBSD; libcsys/libcthr, sysio, proc)
calls linux.xo, whose wrappers use Linux system call numbers.
Fix: linux.xo's wrappers that callers share (open with flags mapped, read, write, close, fsync, stat, fstat,
clock_gettime with clock ids mapped, auxval below 16) take FreeBSD's calls when target.os is FreeBSD; uname goes
through host.xo's sysctl. A new linux.xo wrapper reachable on FreeBSD needs the same arm.
