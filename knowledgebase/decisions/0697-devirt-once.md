# 0697: closures made and called once become direct calls (devirt_once)

Status: Proposed (xo-lang-1, 2026-11-17)

## Context

Task #211: std/trace Ring push runs 1.21 of Go on host 1; its
Mutex.with closure retains each captured value when it reads it from
the environment and releases it again when the environment is dropped.
Stub; filled in with the change that builds it.

## Decision

To be written.

## Consequences

To be written.

## Alternatives

To be written.
