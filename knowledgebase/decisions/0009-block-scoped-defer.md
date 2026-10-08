# 0009: defer runs at block exit

Status: Accepted (2026-10-08). Used correctly in every run, including temp file cleanup; shielding from cancellation (0022) made it work after timeouts and shutdown.

## Decision
`defer` runs when the enclosing block exits, not the function.

## Consequences
- `defer` inside a loop body closes per iteration (the common intent; a classic
  Go leak).
- Differs from Go. Listed in gotchas.

## Evidence
- Paper test 2026-10-07 ([results](../../bench/results/paper-2026-10-07.md)): 0 violations; used correctly for temp file cleanup.
- Paper test 2026-10-07 spec 0.2 ([results](../../bench/results/paper-2026-10-07-v0.2.md)): 0.2 run: used correctly; exposed that sticky cancellation blocks cleanup in defer.
- Paper test 2026-10-07 spec 0.3 ([results](../../bench/results/paper-2026-10-07-v0.3.md)): 0.3 run: used correctly with a `var done` flag (04).
