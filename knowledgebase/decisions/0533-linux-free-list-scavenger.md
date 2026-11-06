# 0533: Return the pages of long free class blocks on Linux (a time based scavenger)

Status: Proposed (Xo Lang 2, 2026-11-06; stub, being filled in; parked for the owner)

## Context

To be filled in: the soak's free list growth (stability/notes, 8 to 10 MB/h of RSS
outside huge pages, leakcheck 0) and the load sweep's per connection growth
(about 90 KiB per extra busy connection, +32.7 MB of free list bytes from 8 to
256 workers, mostly the 16 to 256 KiB classes), both pages of free blocks that
Linux never gets back (decision 0383: scavenge_pages is darwin only).

## Decision

To be filled in.

## Consequences

To be filled in.

## Alternatives

To be filled in.
