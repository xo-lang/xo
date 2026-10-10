# 0114: `xo lsp` rechecks open modules in full on each change

Status: Accepted (2026-10-10, by the owner)

## Context

Decision 0113 shipped TextMate highlighting and planned a language server.
The checker already had what a server needs: an overlay for unsaved text
(`check.CheckOpts` with `Options.Overlay`), parse error recovery, spans
with end positions, fixes on diagnostics, and resolution records
(`check.Info`: `VarOf`, `Ref`, `CallOf`, `Ctor`, `PatVariant`, `TypeOf`).
A cold check of a module takes about 10 ms (examples/notes-service,
bench/paper on 2026-10-10).

## Decision

- `xo lsp` (package `internal/lsp`) speaks LSP 3.17 over stdio with
  Content-Length framing, standard library only.
- State is the text of open documents. After a change (debounced 150 ms)
  and before any request on stale state, the server checks the
  directories of all open documents (plus workspace modules) with the
  texts as an overlay, and answers everything from that one result.
  There is no incremental or per-declaration checking.
- Features: diagnostics (notes appended to the message, fixes offered
  as quick fixes), hover, definition, references, rename (names
  declared in user files only), document symbols (from the parser, so
  they work while the file has type errors), semantic tokens (full),
  and formatting (`internal/format`, so it equals `xo fmt`).
- Positions are UTF-16 on the wire, converted from the checker's byte
  columns per line.
- Definitions in std are served from copies of the embedded std
  sources written to `<user cache dir>/xo/lsp-std/std/<name>.xo`.
- The semantic token legend is wire format: new types or modifiers are
  appended, never reordered.
- It does not use `xo serve`: the overlay must change on every keystroke,
  which a cache keyed by file modification times cannot serve.

## Consequences

- Latency grows with the size of the open modules; fine at today's
  module sizes, and the first thing to revisit if a module gets slow
  (cache std and unchanged imported modules across checks).
- Resolution is as good as the checker's records: a name the checker
  does not record (for example a field of an untyped expression in a file
  with errors) has no definition or semantic token, and the TextMate
  grammar colors it instead.
- No completion or signature help yet.

## Alternatives

- Reuse `xo serve` as the backend: rejected (see above).
- Incremental checking per declaration: more complex; not needed at
  current speeds.
- gopls style snapshot with per-file parse caching: possible later
  optimization inside the same design.
