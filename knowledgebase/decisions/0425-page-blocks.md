# 0425: Page blocks: the 16 KiB and larger size classes keep their class and links in a per-chunk table

Status: Proposed (xo-lang-6, 2026-10-30; the owner's go on the plan 2026-10-29)

## Context

The native allocator (std/native/llrt/alloc.xo) puts a block's size class in the 8 bytes
before the block and its free list link in the block's first word, so a free 16 KiB or 32 KiB
block (classes 68 and 69) keeps one resident page for its prefix and links, and heapos.xo
scavenge (decision 0383) gives back only its inner whole pages. The self build's footprint
holds about 0.8 GB of such pages (probe, 2026-10-29).

## Decision

To be filled with the change: blocks of class 68 and up are page aligned, carved from 4 MiB
aligned page chunks whose first page is a table of their classes and list links.

## Consequences

To be filled.

## Alternatives

To be filled.
