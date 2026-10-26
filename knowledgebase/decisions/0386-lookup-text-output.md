# 0386: Lookups print compact text by default, a record a line

Status: Accepted (lead, 2026-10-26, for the owner: the lookups were
burning agents' context). Replaces decision 0289's output rule (text
at a terminal, JSON otherwise); 0289's lookups are otherwise unchanged.

## Context

Decision 0289 made `xo def`, `xo refs`, `xo symbols`, `xo doc NAME` and
`xo type` print text at a terminal and JSON otherwise. Agents run them
through a shell whose standard output is a pipe, so every answer was
JSON, and JSON on one line: grep and head could not cut it, and a single
grep match printed the whole answer. Measured on main's xo (2026-10-26,
stdout a file):

| command | bytes | lines |
|---|---|---|
| `xo symbols --dir=compiler/ssa` | 540,681 | 1 |
| `xo symbols --dir=compiler/check` | 386,541 | 1 |
| `xo refs check.Checker --dir=compiler` | 77,020 | 1 |
| `xo doc Str` | 12,360 | 1 |
| `xo def check.Checker --dir=compiler` | 5,334 | 1 |

## Decision

- Text is the default everywhere; `--json` asks for JSON (`--text` is
  still accepted and does nothing).
- The text is a record a line, compact:
  - symbols: `kind name  file:line  signature` (the signature's first
    line), no doc comments unless `--doc`;
  - refs: the declaration's line, then `file:line:col  context` per
    reference (the source line, trimmed, cut at 120 bytes), and the
    count;
  - def: the place, the doc comment's first line, the signature;
  - doc NAME: the declaration with its whole doc, then a method a line
    (its signature's first line and its doc's first line).
- `--json` keeps the answer's shape, printed a record a line: an object
  or array that holds an object or array is broken over lines, a flat
  one (a reference, a symbol without methods) stays on one line.
- `xo symbols [--dir=D] NAME`: an argument that is not a directory is a
  name filter (a substring, or `*` for any run); `--limit=N` on symbols
  and refs keeps the first N and says how many there are (a note in the
  text, JSON "total").

The same commands after (2026-10-26):

| command | bytes | lines | longest line |
|---|---|---|---|
| `xo symbols --dir=compiler/ssa` | 173,303 | 1,935 | 307 |
| `xo symbols --dir=compiler/check` | 144,038 | 1,608 | 462 |
| `xo refs check.Checker --dir=compiler` | 69,248 | 699 | 158 |
| `xo doc Str` | 4,743 | 52 | 274 |
| `xo def check.Checker --dir=compiler` | 5,012 | 165 | 46 |
| `xo symbols --dir=compiler/check 'check_*'` | 136 | 1 | 135 |
| `xo refs check.Checker --dir=compiler --limit=10` | 1,103 | 14 | 111 |
| `xo symbols --dir=compiler/ssa --json` | 550,364 | 1,939 | 2,497 |

(refs' text grew a context per line, which is what a reader looks at;
`--limit` is the way to keep it short.)

`xo query` already printed text by default, a line a record (api,
callers, deps, effects), so it is unchanged.

## Consequences

- A tool that read the lookups' JSON without asking for it must pass
  `--json` (xo mcp runs `xo query ... --json`, never the lookups).
- The text forms are for reading and grep; scripts use `--json`.

## Alternatives

- JSON lines for every answer (one object per line): changes the shape
  scripts parse; the line-broken form keeps it.
- Keep JSON as the default and add `--text` to agent-tools.md: agents
  forget the flag (half reached for grep anyway, owner 2026-10-21), and
  the cost lands in every context that forgets it.
