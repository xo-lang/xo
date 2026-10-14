# 0219: Completion and signature help in xo lsp

Status: Accepted (2026-10-13, by the owner: the work order for `xo lsp`)

## Context

`xo lsp` answered no `textDocument/completion` (decision 0114: "No
completion or signature help yet"), so editors fell back to word
matching. The checker already records what completion needs: the type
of every checked expression (`type_in`, `nat_in`), the binding each
name declares (`decl_at`), module members, the fields, variants,
methods, and statics of each type object, and the resolution that hover
reads (selfhost/cli/servers/resolve.xo).

What it lacks is text that parses at the cursor. The lead's experiment
(2026-10-13, bin/xo 0.9.0): in a file with
`let r = http.new_request("GET", "/")`, a line `r.`, then
`let q = p.x`, the parser recovers and the checker reports XO0601
"`http.Request` has no field or method ``", so the receiver's type at a
dangling `.` is computed; but the recovery swallows the next line (`p`
is reported unused), so the statement after a dangling `.` is lost. An
unclosed call `area(1, ` loses its call the same way.

gopls answers the same problem with source fixing
(gopls/internal/cache/parsego): before type checking for a completion
request it patches the text near the cursor (a phantom selector after a
trailing `.`, a closing bracket for an unclosed call) and checks the
patched copy.

## Decision

- Capabilities: `completionProvider` with trigger character `.`;
  `signatureHelpProvider` with `(` and `,`.
- Text patching on the server side, not a parser change. For completion
  with no name typed at the cursor (after `.` or at an empty position),
  the server inserts a placeholder name (`xo_complete_`) at the cursor,
  so `r.` followed by a newline parses as the selector
  `r.xo_complete_` and the next statement survives. For signature help
  inside a call whose `(` is never closed, it inserts `)` at the
  cursor. The patched text is checked privately for the request (the
  open documents as the overlay, as the debounced recheck does); the
  session's published diagnostics and resolution stay those of the real
  text. With a name already typed (`r.pa`, `ar`) the text parses as is
  and the request reads the session's current check.
- Member completion after `x.` (selfhost/cli/servers/lsp_complete.xo):
  - `x` a `use`d package name: its public top level names;
  - `x` a type name (`Color.`, `http.Method.`): its variants and static
    functions and constants;
  - otherwise the natural type of `x` from the check: a struct's fields
    (public ones only outside its module, std's always) and methods, an
    interface's methods, a type parameter's constraint's methods, an
    alias's target's, a tuple's `0`, `1`, ... . Prelude methods
    (`Str.len`, `List.push`) come from the prelude's type objects, so
    they need no list of their own.
- Bare name completion: the local bindings in scope at the cursor (the
  checker's `decl_at` filtered by the scope ranges of the resolve walk:
  blocks, functions, closures, loops, `if`, match and select arms; a
  `let` or `var` name is not in scope in its own value; a test block's
  `os` and `for` parameters), the module's top level names, the file's
  imported package names, the prelude's public names (builtin types and
  functions), and the keywords (selfhost/lexer's list plus `true`,
  `false`, `self`). Items are filtered by the typed prefix
  (case insensitive); the client filters again.
- Each item has an LSP kind (Field, Method, Function, Struct, Enum,
  Interface, EnumMember, Constant, Variable, Module, Keyword), `detail`
  = the declaration line hover prints, and `documentation` = the doc
  comment (markdown), both from hover's `describe`.
- Signature help: the innermost `(` open before the cursor (strings,
  runes, and comments skipped; a `{` in between ends the search) is a
  call; the name before it is resolved as hover resolves it. A function
  or method shows `name(p: T, ...) -> R ! E` (a method as
  `Type.name`), a variant constructor `Enum.Variant(fields)`; the
  parameters carry label offsets, and the active parameter is the
  number of commas at that depth (clamped to the last parameter).
- Ranking (`sortText`): items whose type equals the type expected at
  the cursor sort first (group 0), then locals and members (1), the
  module's names (2), imports (3), the prelude (4), keywords (5), each
  group by label. The expected type is known for an argument (the
  callee's parameter at the comma count), a `return` (the enclosing
  function's result), and a `let` or `var` with a declared type on the
  cursor's line. A function item's type is its result type.

## Consequences

- A completion with an empty name costs one extra full check of the open
  modules (the patched text), like the debounced recheck (decision
  0114's cost model); with a typed prefix it reuses the session's check.
- No parser change, so diagnostics and the front end recordings are
  untouched (decision 0217). The parser's recovery at a dangling `.`
  (losing the next statement, and an empty name in XO0407 "method ``
  mutates its receiver") is unchanged for `xo check`; see
  gotchas/dangling-dot-swallows-next-line.md.
- cmd/xo/lsp_test.go is the first Go test that drives `xo lsp` since
  decision 0215: initialize, didOpen and didChange, completion after
  `p.`, `http.`, and `r.`, a bare prefix, an empty name, the expected
  type ranking, signature help in an open call, and hover and definition
  into std.

Left for later:

- Expected types elsewhere: struct literal fields, assignments to an
  existing binding, match arms, a `let` whose type is on another line,
  and comparisons. Type compatibility is by the shown type text, not
  assignability (an `Int` item does not rank for an `Int?` slot).
- Snippets (inserting `()` and parameter placeholders), `textEdit`
  ranges, completion of struct literal field names, named arguments,
  `use` paths, and methods that only derive or interface implementation
  provides without a declaration.
- Signature help for named arguments (by name rather than position) and
  for function values held in locals.
- Caching the patched check across keystrokes (gopls caches per file
  parse): today each empty name completion rechecks.

## Alternatives

- Improve parser recovery at a dangling `.` so the next line survives:
  fixes `xo check` output too, but changes diagnostics and possibly the
  front end recordings (decision 0217: not re-recorded), and still
  leaves unclosed calls. Rejected for now; the text patch needs no
  parser change and is what gopls does.
- Complete from the text alone (words in the file): what editors do
  without a server; no types, no members.
- Ask the checker for the expected type through a new query (the type
  it wants for an expression): more exact, but a checker change;
  revisit with the struct literal and assignment cases.
