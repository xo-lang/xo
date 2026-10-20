# 0275: Backend cache keys by code generation inputs, not xo's identity

Status: Proposed (2026-10-20, Xo Lang 3; stub, being filled)

## Context

Every backend cache key holds xo's own identity (self_id), so each new
xo starts cold. Measured ceiling: internal/arm64 767 s cold, 349 s warm
with the same xo (2026-10-20).
