# 0418: Pack segments compress and stream per entry, and a build stores each part's entries as it finishes

Status: Proposed (xo-lang-6, 2026-10-29; the lead's go on the plan at 20:45; stub, filled in with the change)

## Context

The backend cache's packs (decision 0351, compressed by 0365) keep a target's entries in segment
files: one header (count, keys, lengths) and one body of every entry's bytes, LZ4 compressed as a
whole. Writing a segment builds the whole uncompressed body in a builder and then its compressed
copy; reading one decompresses the whole of it to serve any entry. The function and part entries of
a build (the function's text as Bytes plus its wants; the part's asm) are gathered in Program and
written after emit, so a cold build holds every function's text twice and every part's asm twice
from gen_part until fn_store. Measured 2026-10-29 (main's xo, the self build, same tree): footprint
cache off 8284 MB, cold cache on 9705, warm 6059; the warm build's fn_store alone adds 0.9 GB of peak.

## Decision

To be filled in with the change (format v2 with one LZ4 block per entry and a clean miss on old
segments; each emit worker spills its part's entries to the build's private directory as the part
finishes; segments and loose entries are written one entry at a time from the spill).

## Consequences

To be filled in.

## Alternatives

To be filled in.
