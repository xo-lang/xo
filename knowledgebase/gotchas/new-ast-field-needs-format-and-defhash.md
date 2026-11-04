# A new AST field needs the formatter and defhash too

Status: Observed (Xo Lang 2, 2026-11-03).

## Symptom

`xo fmt` deleted `derive Json(case: ...)`, `tag:` and `content:` from
enums and `as "name"` from variants (decision 0478 stages 1 and 2,
8d663890f and b8e388f11): the program still built, and its JSON changed.
defhash, which keys the incremental caches, did not hash them either, so
an edit of only a `tag:` could reuse a stale definition.

## Cause

The fields were added to the AST, the parser, the AST codec, the checker
and the backends, and their goldens passed; no golden runs `xo fmt`,
and the formatter (compiler/cli/core/format) and defhash
(compiler/cli/semantic/defhash.xo) read the AST by hand, with no check
that every field is printed or hashed. The struct versions existed
(decision 0277), so the enum code looked complete.

## Fix

Every new AST field also gets: the formatter's printing (with a
cmd/xo TestFmt case that round trips it), defhash's hashing (encoded
only when present, so existing hashes stay), astjson, and semantic/edit
when it applies. Grep for an existing sibling field (`derive_args`,
`wire_span`) across compiler/cli to find each place.
