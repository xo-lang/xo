# 0025: Module local names from directory names

Status: Accepted (2026-10-07)

## Context
With no module declaration (0006), a directory named `03-retry-client` had no
defined local name. First negative signal for 0006. (Paper test 2026-10-07 spec 0.2, [results](../../bench/results/paper-2026-10-07-v0.2.md))

## Decision
The local name is the last path segment with `-` replaced by `_`. If that is
not a valid identifier or is a keyword, `use ... as` is required (XO0602).

## Consequences
0006 stays Proposed; this patches its main weakness without a module clause.
