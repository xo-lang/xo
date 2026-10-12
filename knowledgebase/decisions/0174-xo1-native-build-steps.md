# 0174: What xo1 must do to replace the Go driver for native builds

Status: Proposed (2026-10-11). Being implemented in `selfhost/xo`.

## Context

The switch-over experiment of decision 0158 reached the fixed point on
the assembly: xo1 (`selfhost/xo`, the lexer through the arm64 emitter in
Xo, then `cc`) compiles itself byte identically and passes 132 of the
138 arm64 goldens. What internal/build adds beyond the C runtime was not
ported: the TLS library of std/http (the 6 failures), the zone data of
std/time, the prebuilt runtime packs and the runtime object cache
(decision 0157), `use c` sources (decision 0152), cross targets, and
test binaries.

## Decision

To be filled in with the steps as they land.

## Consequences

To be filled in.

## Alternatives

To be filled in.
