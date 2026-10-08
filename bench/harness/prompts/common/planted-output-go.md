# Output format (required)

Return files only as blocks of exactly this shape:

    === FILE: <path> ===
    <the whole file content>
    === END FILE ===

(The marker lines start at column 0; the indentation above is only for this
description.)

- For each variant, every changed file in full at `variants/<Id>/<path in the
  base>`, for example `variants/X1/handlers.go`.
- One `manifest.json`: a JSON array with one object per variant:

      {"id": "X1", "class": "ignored error", "xo_prevents": "yes",
       "files": ["handlers.go"],
       "description": "what was changed, quoting the old and new code",
       "trigger": "the input or sequence that exposes the bug",
       "symptom": "what a user would observe"}

  `xo_prevents` is `yes` for X1 to X5, `partly` for X6, `no` for N1 to N4.
- Send whole files, never diffs. Paths are relative and never contain `..`.
- Text outside the blocks is ignored.
