# 0032: Task groups, os.tasks, non blocking channel ops, map_concurrent

Status: Accepted (2026-10-07). Amends 0007 and 0015.

## Context
A cache could not start a background refresh from `get`: tasks cannot outlive
their scope and a scope handle could not be stored (07). Collecting ordered
worker results needed `Mutex[Map]` in three paper runs (02). Waiting for N
tasks with a deadline was hand built (06). ([planted and harder](../../bench/results/planted-and-harder-2026-10-07.md))

## Decision
- `s.group() -> task.Group`: storable handle; its tasks belong to the creating
  scope. Types containing a `Group` cannot escape that scope (XO0802).
- `os.tasks`: program lifetime group; `TestOs.tasks` lives for the test.
- `try_send`, `try_recv` on channels.
- `task.map_concurrent(items, limit, f) -> List[Result[R, E]]`, ordered.
- Outer task handles may be awaited inside `task.with_timeout`.

## Consequences
Structured concurrency is kept (no task outlives its creating scope) while
long lived services become expressible.
