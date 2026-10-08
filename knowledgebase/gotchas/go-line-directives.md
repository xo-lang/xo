# //line directives

Status: Observed (2026-10-07, P1 code generation)

- Must start at column 1 with no space: `//line users.xo:18:3`.
- A directive applies to every following line until the next one, so the
  generator emits one before each statement and each function; helper code
  after the last directive inherits its position.
- Relative paths are kept verbatim in stack traces (also with `-trimpath`);
  the generator writes paths relative to the xo.mod root, or to the module
  directory when there is none, so traces read `main.xo:4:5`.
- `runtime.Caller` reports the line of the directive, not the column; rt
  faults raised without an explicit position (`FaultHere`) therefore show
  `file.xo:line`. Operations the generator emits pass `file:line:col`.

Test: `TestFaultStackPointsAtXo` in `internal/gen/build_test.go`.
