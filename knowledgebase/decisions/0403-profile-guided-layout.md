# 0403: Profile-guided code layout

Status: Proposed (stub, 2026-10-29; xo-lang-6; the owner's 0.35 item 9).

## Context

A function's placement moves interp's time by 15% (74 to 63 ms across
XO_ARM64_PAD placements, bench/results) with its code unchanged; decision
0398's padding and the pad knob measure the effect but do not remove it.

## Decision

To come: the profile, what the layout reorders, how the profile is stored
and keyed, reproducible builds, separate compilation and the std store.

## Consequences

To come.

## Alternatives

To come.
