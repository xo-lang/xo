# //line directives

Status: Observed (2026-10-07, P1 code generation)

- Must start at column 1 with no space: `//line users.xo:18:3`.
- A directive sets the position of the next line only; the lines after it
  count up from there (Go numbers them line+1, line+2, ...). A statement
  that expands to several Go lines therefore named the following Xo lines
  (Observed 2026-10-08: record and replay logged `fs.read` of line 14 at
  `main.xo:22`). Since 2026-10-08 the generator repeats the statement's
  directive before every Go line it emits (`fnGen.line`, and captured
  statement text starts with one, `fnGen.capture`); a directive inside
  the text (a closure body) holds for the lines after it. Consecutive
  duplicate directives are harmless.
- Relative paths are kept verbatim in stack traces (also with `-trimpath`);
  the generator writes paths relative to the xo.mod root, or to the module
  directory when there is none, so traces read `main.xo:4:5`.
- `runtime.Caller` reports the line of the directive, not the column; rt
  faults raised without an explicit position (`FaultHere`) therefore show
  `file.xo:line`. Operations the generator emits pass `file:line:col`.

Test: `TestFaultStackPointsAtXo` in `internal/gen/build_test.go`.
