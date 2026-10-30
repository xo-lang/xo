# 0446: `use c` sources are compiled with debug information

Status: Proposed (stub, 2026-10-30; xo-lang-6 fills it in)

## Context

A `use c` function that LTO inlines into its Xo caller (Linux LLVM
builds, gotchas/linux-lto-inlines-c-frames.md) is not a frame of a
crash report or traceback, and the C sources carry no debug
information, so the binary's DWARF has no inlined subroutine entry for
it either.

## Decision

(to fill: the C adapters and link sources compiled to objects with -g
before the link, on both native backends.)

## Consequences

(to fill)

## Alternatives

(to fill)
