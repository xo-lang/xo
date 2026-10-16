# Raw writes into a List bypass its copy on write

Status: Observed (2026-10-16, cpu.sha256_block in sha256.xo)

## Symptom

With SHA-256's compression moved to raw memory (cpu.sha256_block
writing the state words at the list's address), one shot digests and
every compression were right, but `hash.split256` (a Hasher's sum, then
more writes, and a clone) and every TLS 1.2 handshake were wrong: 206 of
848 TestSHA256HWDiff cases, ten crypto and TLS goldens, 1186
TestTLSServerReplayDiff differences. TLS 1.3 passed.

## Cause

Lists are values with copy on write: `Hasher.sum` and `clone` share the
state list's words with the original, and the first index store into
either copies them. digest_tables.sha256_block writes `st[i] = ...`, so
it copied; a raw store through `rt.obj_addr(st)` does not, so it changed
the words both values share (the sum's compression advanced the
running hash too). TLS 1.2's transcript takes sums mid handshake; TLS
1.3's path happened not to share.

## Fix

Before raw writes into a list that may be shared, make an index store
(`st[0] = st[0]` in sha256.xo block256): it copies the words when they
are shared and costs one store when they are not. bigmod.xo and cpu.xo's
own checks write only into lists they just made, which no one shares.
Test raw writers with a value that is shared: a clone or a sum before
more input.
