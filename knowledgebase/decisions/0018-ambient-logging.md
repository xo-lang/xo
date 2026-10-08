# 0018: Logging is ambient and untracked

Status: Accepted (2026-10-07)

## Context
Appendix A logged without a capability, contradicting 6.3. Threading a
logger capability through every function is heavy friction for little safety. (Paper test 2026-10-07, [results](../../bench/results/paper-2026-10-07.md))

## Decision
`std/log` needs no capability and is not an effect. The sink is configured in
`main` with `log.configure(os.stdio, ...)`. "Pure" now means deterministic
except allocation, faults, cancellation, and logging.

## Consequences
- A dependency can write to logs without declaring it. Accepted: logs are
  write only and captured by record and replay.

## Alternatives
- A `log` effect label: would appear on most functions.
- A `Logger` capability: has to be threaded everywhere.
