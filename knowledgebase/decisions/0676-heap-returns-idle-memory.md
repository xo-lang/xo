# 0676: the native heap returns idle memory to the OS

Status: Proposed (Xo Lang 1, 2026-11-16; task #91). Stub: filled in by a
later commit.

## Context

The sql stress program grows from 56 to 88 MB resident over 30 minutes on
linux/arm64 with one live block at the end and 68 of 92 MB on the size
class free lists; Go's twin stays near 25 MB (aws-43, 2026-11-16,
562cbe72e). Free blocks of a class are never returned to the system.

## Decision

To be written.

## Consequences

To be written.

## Alternatives

To be written.
