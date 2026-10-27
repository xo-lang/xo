# 0389: `xo build` and `xo run` skip inline test blocks

Status: Accepted (owner, 2026-10-27, via the lead; Xo Lang 1 to build)

## Context

To be filled in with the change (measured gains, precedent, spec 9.2).

## Decision

`xo build` and `xo run` neither load nor type-check inline `test` blocks, as decision 0348 does for
`_test.xo` files. `xo test`, `xo check`, `xo vet`, `xo fix` and the language server still check them.
