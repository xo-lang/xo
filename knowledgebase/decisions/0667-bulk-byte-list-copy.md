# 0667: BytesBuilder.write_list and Bytes.from_list, a List[UInt8] copied in bulk

Status: Proposed (Xo Lang 3, 2026-11-15; the lead asked for the primitive
for std/brotli's ring buffer writes). Stub: filled in with the change.

## Context

A `List[UInt8]` reaches a `BytesBuilder` or `Bytes` only one
`write_byte` at a time; std/brotli's write_ring_buffer spends 23% of a
decode there.

## Decision

To be filled in.

## Consequences

To be filled in.

## Alternatives

To be filled in.
