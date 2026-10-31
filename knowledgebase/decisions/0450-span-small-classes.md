# 0450: Small size classes carved from spans, so freed memory serves any class

Status: Proposed (xo-lang-2, 2026-10-31; stub, being written)

## Context

Decision 0448's emit peak is allocator retention, not live data: in the
self build the heap in use during emit stays at 2.4 to 3.1 GB while the
mapped peak is 6.1 to 6.2 GB and max RSS 6.2 to 6.4 GB. To be filled in.

## Decision

To be written: the small classes (16 B to 8 KiB, prefixed blocks today,
decision 0207) carved from spans of one class, a span whose blocks are
all free returned to a pool any class takes from; behind an XO_* switch,
default off, each step measured with `xo bench --ab` against the switch.

## Consequences

To be written.

## Alternatives

To be written.
