# 0552: Map slots carry the key's hash bits

Status: Proposed (Xo Lang 4, 2026-11-07; the lead approved this ADR and a
prototype on a branch; the change waits for the owner).

## Context

To be filled in with the prototype's measurements.

## Decision

Each slot of a Map's hash table holds the low 32 bits of its key's hash
beside the entry index (`slot = h << 32 | pos + 1`): a lookup compares
the tag before the key, and a table rebuild places each live entry by
its slot's tag instead of hashing its key again.

## Consequences

To be filled in.

## Alternatives

To be filled in.
