# Windows: TCP bind on a port UDP got from :0 can fail with permission denied

Status: Observed (2026-10-30, the dns_records golden on the owner's windows/amd64 box).

## Symptom

A program that binds UDP on 127.0.0.1:0 and then TCP on the same port
(a DNS fixture answering both) faults now and then on Windows with
`listen tcp 127.0.0.1:55220: bind: permission denied` (WSAEACCES); the
same binary passes on the next run.

## Cause

Windows reserves TCP port ranges for Hyper-V, WSL and WinNAT
(`netsh int ipv4 show excludedportrange protocol=tcp`). The reservation
is per protocol: UDP may hand out a port from :0 that is inside a TCP
excluded range, and binding TCP there is refused with WSAEACCES, which
Xo reports as permission denied.

## Fix

Bind the second protocol and, on failure, close the first socket and try
another :0 port (the dns_records golden's `servers` tries 20 times). Do
not assume a port free for one protocol is free for the other on
Windows.
