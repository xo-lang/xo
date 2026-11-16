# 0670: std/ui page layer: any page, forms and frames without full loads

Status: Proposed (owner session, 2026-11-15). Stub: the decision is being written.

## Context

Decision 0668 follows links between std/ui App pages. Apps also have
plain server pages (search, sign in) and forms.

## Decision

To be written: the runtime takes over every same-origin page that loads
it (GET forms, POST forms with 303/422, frames), as Turbo Drive does.
