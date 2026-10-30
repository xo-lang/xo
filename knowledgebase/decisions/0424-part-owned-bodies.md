# 0424: A split generation's parts own their functions' bodies; the module keeps shells

Status: Proposed (xo-lang-6, 2026-10-30; the lead's go on the plan, 2026-10-29 23:30; stub, filled in with the change)

## Context

The native back end's split generation (arm64/parallel.xo generate_cached) hands the whole
ssa.Module to every emit worker, and the module keeps every function's body (blocks, values,
block parameters, the inliner's copies, cse tables) until the Program returns: 2.3 to 3.5 GB of
the self build's heap are live through emit (heap snapshots past inline.verify and past emit,
2026-10-29), under the build's peak, while a part reads only its own functions' bodies and, of the
other functions, their names, signature facts and flags.

## Decision

To be filled in with the change (each part takes its functions' bodies before the workers start,
the module keeps shells with body facts, the bodies drop as each part is done; the driver's last
use of the module is the generation).

## Consequences

To be filled in.

## Alternatives

To be filled in.
