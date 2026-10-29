# 0401: a separately compiled darwin link carries std's DWARF as a debug map

Status: Proposed (owner approved the design, 2026-10-28)

## Context

Carrying std's ~4 MB of DWARF through a separately compiled HTTP server's image costs
about 11 ms of its 41 ms store link (the copy and the two hashes over its pages; caching
std's relocated bytes saved nothing). Design: ~/xo-lead/notes/xo-lang-4/debugmap-design.md.

## Decision

(to be filled) std's __debug_line stays embedded (Xo's tracebacks, xo dbg and atos read
lines from the image); its other DWARF sections are reached through a debug map (N_OSO
stabs naming std's part objects, kept in the std cache beside the store, N_FUN per
function with its final address); --release and -o outputs keep full embedded DWARF.

## Consequences

## Alternatives
