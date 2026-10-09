# 0096: `xo check --fix` for the agent loop

Status: Accepted (2026-10-09, by the owner; not yet implemented)

## Context

Codex r2 (`bench/results/codex-xo-r2-2026-10-09.md`): of its ten first
runs, 5 pass as written on the current toolchain and 9 pass after
`xo fix`. `xo check` already reports machine fixes, but an agent must
apply them itself, and Codex usually did not (for example XO0162,
`}` newline `else`, which has an exact fix).

## Decision

- Add `xo check --fix`: apply every machine fix that is unambiguous (the
  same set `xo fix` applies, round after round), reformat the changed
  files, then report the remaining diagnostics in the usual JSON with a
  list of the fixes applied. Exit status as `xo check` on what remains.
- Plain `xo check` stays read only.
- The agent run protocol (bench/harness prompts, spec 10, the MCP
  `check` tool as an option) uses `xo check --fix`.
- Benchmarks keep reporting first try as written and first try after
  fix, so the fix step never hides a diagnostic's cost.

## Consequences

- Agents cannot forget the fix step; habits with exact fixes stop
  costing a run.
- A diff the agent did not write appears in its files; the JSON lists
  each applied fix so the agent can see it.

## Alternatives

- Prompt instructions only: agents forget.
- `xo check` always fixing: surprising, breaks the read only guarantee.
