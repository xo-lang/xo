# A raw Winsock code never reaches a caller of sock.xo

Status: Observed (2026-10-30)

## Symptom

udp_echo timed out (exit 124) on windows/amd64 after "dial echo": the
server's `read_from(5)` of an 18 byte datagram returned an error, its loop
ended, and the client waited for an echo that never came. The truncation
branch in std/native/llrt/netconn.xo udp_read, `r == -10040`, was there
and looked right.

## Cause

Every Winsock result goes through std/native/llrt/sock.xo `wret`, which
turns WSAGetLastError into minus an errno with `wsa_errno`. A code missing
from that table becomes EIO (5), so a caller comparing against the raw
Winsock number (10040, WSAEMSGSIZE) can never match.

## Fix

Add the code to `wsa_errno` (10040 => 115, EMSGSIZE) and compare against
the mapped errno at the call site. When a caller needs a new Winsock
condition, check the table first; a golden on the Windows offload host
(`OFFLOAD_ENV=offload-win.env ~/xo-lead/agents/amd64-run.sh REV NAME
GOLDEN`) is the only test that runs the path.
