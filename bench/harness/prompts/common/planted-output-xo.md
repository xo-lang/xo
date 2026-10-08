# Output format (required)

Return files only as blocks of exactly this shape:

    === FILE: <path> ===
    <the whole file content>
    === END FILE ===

(The marker lines start at column 0; the indentation above is only for this
description.)

- For each planted variant, every changed file in full at
  `variants/<Id>/<path in the base>`, for example `variants/X1/store.xo`.
- One `manifest.json`: a JSON array with one object per class you were asked
  for, planted or not:

      {"id": "X1", "class": "ignored error",
       "xo_change": "what was changed, quoting the old and new code",
       "status": "variant",
       "reason": ""}

  `status` is `variant` (files sent) or `inexpressible` (no files; `reason`
  says why the mistake cannot be written in Xo).
- Send whole files, never diffs. Paths are relative and never contain `..`.
- Text outside the blocks is ignored.
