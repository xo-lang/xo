# Output format (required)

Return files only as blocks of exactly this shape, one block per file, with
paths relative to the solution root:

    === FILE: path/relative/to/root.ext ===
    <the whole file content>
    === END FILE ===

(The marker lines start at column 0; the indentation above is only for this
description.)

- Send whole files, never diffs or excerpts.
- Paths are relative, use `/`, and never contain `..`.
- Text outside the blocks is ignored by the harness; keep it short.
- Do not put a Markdown code fence inside a block.
