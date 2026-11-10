# A Bytes slice kept past a reuse of its buffer changes under you

Status: Observed (2026-10-30, decision 0426's Net.interfaces on linux/arm64; 2026-11-10, the resolver's dual stack answers).

## Symptom

Net.interfaces on Linux returned interfaces with index 0 and empty names,
out of order (`sorted false named false`), while the same code's macOS
and Windows paths, which read no reused buffer, were right. The Linux
path had only been compiled on the Mac, never run.

## Cause

std/native/llrt/ifaces.xo read netlink replies into one receive buffer
(raw.bytes_buf) in a loop of recvfrom calls and kept each message as
`buf[off + 16..off + len]`. Since decision 0330 a Bytes (and Str) slice
is a view of its parent, not a copy, so every kept message pointed into
the buffer the next recvfrom overwrote: the link messages of the first
batch read the bytes of later batches.

## Fix

Copy what outlives the buffer: `buf[a..b].clone()`. A slice is only safe
to keep while nothing writes its parent; a raw.bytes_buf reused across
system calls is the usual case where something does. Run a new OS
specific path on that OS before trusting it (the netlink code passed
every Mac gate).

## Second instance: concurrent reads into one buffer (2026-11-10, DNS)

Observed on the owner's OCI host (the decision 0568 beta). The resolver
(std/native/llrt/dns.xo) sends a name's A and AAAA queries at once and
reads every reply into one receive buffer; `answer` kept each address as
`m[d..d + 4]`, a view of it. When both replies arrived before the first
one's addresses were used (0.1 ms apart there), the IPv4 addresses read
the IPv6 reply's bytes: xo dialed 38.6.71.0 (26 06 47 00, the start of
2606:4700::) and 192.12.0.28 (c0 0c 00 1c, a name pointer and TYPE 28),
and `xo mod sumdb serve` timed out uploading to R2. It never reproduced
off that host: the Mac's and Docker's resolvers answered no AAAA or
answered later. Fixed in 90bec2cd1 (`.clone()`); internal/llvm
TestResolverDiff's `pair.test` answers both back to back.

Rule: a parser of a received message copies what it returns. A test of
such a parser makes the next read overwrite the buffer before the result
is used (two replies back to back), since a single slow reply hides it.

`xo vet` reports the loop shape as XO0541 (2026-11-10): a slice of a
`raw.bytes_buf` that the loop reads into again, pushed, stored outside
the loop or returned without `.clone()`, and, one call deep, a call
whose function returns slices of the buffer (or of a slice of it) it is
given, as the resolver's `answer`, when that result is kept.
