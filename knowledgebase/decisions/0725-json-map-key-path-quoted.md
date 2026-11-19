# 0725: a Map key in a json error path is quoted, ["KEY"]

Status: Accepted (xo-lang-3, 2026-11-19, task #241 item b42)

## Context

A `json.DecodeErr` path named a Map key (and a `json.Value` object's key)
as `.KEY`, the key's text copied raw. A peer controls those keys, so
`{"m":{"a\nERROR forged":"x"}}` put a newline and a forged line into
`at $.m.a\nERROR forged: expected integer`, and into `decode_go`'s
message, which built "Go struct field" from the same path:
`GoDecMap.m.a\nERROR forged`. Go 1.26's encoding/json leaves Map keys and
slice indices out of that field name (`T.m`, `Inner.mi.n`), so the key
was also a difference from Go (the owner's v0.51.1 evaluation, b42).

## Decision

- A Map key's (and a `json.Value` object key's) path segment is
  `["KEY"]`, the key written by `Str.quote` (Go's strconv.Quote on the Go
  backend): `$.m["a\nERROR forged"]`, `$["a"]["x"][0]`. A struct field
  stays `.name`; an index stays `[2]`.
- `decode_go`'s "Go struct field" name leaves out every `[...]` segment,
  reading a quoted key's escapes so a key holding `]` or `"` does not end
  the segment early: Go's text.
- Both backends: compiler/ssa/json.xo through `llrt_json_key_seg`
  (std/native/prelude.xo), std/native/json.xo `path_text` and `go_final`;
  std/rt `JsonKeySeg` and `jsonGoFinal`.

## Consequences

- A path is no longer something a peer can write into a log line.
- A program that matched on `$.m.KEY` text sees `$.m["KEY"]`
  (CHANGELOG, Breaking).
- `decode_go` errors under a Map name the field as Go does.

## Alternatives

- Escape only control characters in `.KEY`: still ambiguous (a key
  holding `.` or `[` reads as more segments) and still not Go's field name.
- Leave keys out of the path entirely, as Go: loses where the bad value
  is in a document with many keys; the strict decoder's path is Xo's own.
