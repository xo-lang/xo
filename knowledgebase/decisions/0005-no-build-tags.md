# 0005: No build tags; platform code is a compile time match

Status: Accepted (2026-10-07)

## Context
Go build tags and `_windows.go` files hide compile errors on platforms the
developer did not build for.

## Decision
`target` is a predeclared compile time constant. Platform code is
`match target.os { ... }`. Only the chosen arm is generated, but every arm is
type checked for every target on every build.

## Consequences
- A broken Windows branch fails on macOS.
- `std/sys/<os>` APIs must have type information available on every host.
