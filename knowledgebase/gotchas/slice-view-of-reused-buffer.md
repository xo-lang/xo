# A Bytes slice kept past a reuse of its buffer changes under you

Status: Observed (2026-10-30, decision 0426's Net.interfaces on linux/arm64).

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
