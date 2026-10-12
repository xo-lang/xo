# 0168: LLVM builds split into modules compiled in parallel

Status: Proposed (2026-10-11, llvm-split agent, on the lead's task).

## Context

An LLVM build emits one IR module and clang compiles it at -O2 with
full LTO on one core; selfhost/ssa takes over half an hour.

## Decision

To be filled in with the measurements.

## Consequences

## Alternatives
