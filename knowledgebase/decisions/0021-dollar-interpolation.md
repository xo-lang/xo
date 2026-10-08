# 0021: Interpolation is ${expr}; braces are literal; r#"..."# raw strings

Status: Accepted (2026-10-07)

## Context
`{x}` interpolation broke Appendix A's route pattern in spec 0.1 and blocked
JSON fixtures in 0.2 (raw strings could not contain quotes). Two runs in a row. (Paper test 2026-10-07 spec 0.2, [results](../../bench/results/paper-2026-10-07-v0.2.md))

## Decision
- `"${expr}"` interpolates; `{` and `}` are literal; `\$` is a literal `$`.
- `r"..."` raw; `r#"..."#` raw and may contain `"`.

## Consequences
- Route patterns and JSON are plain strings. Matches JS, Kotlin, Dart priors.
- `{{` escaping is gone.
