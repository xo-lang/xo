# 0452: A List grows by Go's slice curve, not by doubling

Status: Proposed (xo-lang-2, 2026-10-31; stub, being written; needs the owner's acceptance)

## Context

A native List doubles its capacity when a push finds it full
(std/native/llrt/obj.xo grow), so a large list holds up to half its
capacity unused. In xo's self build that waste is part of decision
0448's emit peak. To be filled in.

## Decision

To be filled in: double up to 256 elements, then grow by a quarter plus
192 elements (Go's growslice), toward 1.25x.

## Consequences

To be filled in.

## Alternatives

To be filled in.
