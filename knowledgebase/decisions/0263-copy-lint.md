# 0263: A copy lint under xo vet

Status: Accepted (owner, 2026-10-18, through the lead: "a perf lint for
copy-prone shapes ... as warnings under xo vet"); not implemented yet

## Context

Xo's value semantics make some copies invisible in the source. On
2026-10-18 the HTTP/2 and TLS work found, by profiling, shapes that each
cost a fixed walk over every reference counted field of a large value,
per call (bench/results/http-callgrind-2026-10-17.md):

- `match c.s12 { Some(x) => { var y = x ... c.s12 = Some(y) } None => ...}`
  on a field of a live value: binding the payload shares (counts up) every
  heap field and the Option is dropped (counts down) when the field is
  written back, even when it was None: about 4,000 instructions an HTTP/2
  request until tlssrv tested `is_none()` first.
- A helper whose parameter is only stored into a new value
  (`fn save(h, c: Conn) { put(h, rt.any_of(c)) }`): the parameter is
  borrowed, so the store shares every field and the caller drops its own
  copy after: about 5,600 instructions a call on arm64, gone when the
  store moved to the caller's last use.
- `var c = match f() { Some(x) => x ... }` on a dying temporary and large
  `var` parameters on arm64: fixed in the compiler (19a3540d, Xo lang
  4's frame homes), so not linted.

Nothing pointed at these but a profile; an agent writing such code cannot
see the cost.

## Decision

Two warnings in `xo vet` (advisory, never errors; codes in the moves
group), each naming the type, its number of heap fields, and the fix:

- `XO0530`: a `match` or `if let` binds the payload of an Option or enum
  that lives on after it (a field or a variable used later), and the
  payload's type has at least 8 heap fields (Str, List, Map, Bytes,
  boxes, handles, recursively through structs): "binding `x` shares the
  N heap fields of T and dropping `c.s12` later releases them; test
  `c.s12.is_none()` first, or take the value out of `c` before matching".
- `XO0531`: a parameter of a type with at least 8 heap fields is only
  passed to a consuming position in the function (stored into a new
  value, `rt.any_of`, a list push) and every caller passes a value it does
  not use afterward: "`c` is borrowed, so storing it shares its N heap
  fields and each caller drops its own copy; store at the caller's last
  use, or return the value".

The threshold (8) and the field count come from the checker's types;
`xo vet --copy-threshold=N` changes it. Both are best effort (the vet
pass sees the AST and types, not the RC pass's decisions): a warning that
does not cost anything after optimization is acceptable, a silent
expensive copy is the problem being solved.

## Consequences

- vet stays fast (no SSA): the checks walk the checked AST once.
- When the RC pass learns to avoid one of the shapes (a payload view
  instead of a share, Xo Lang 2's next items), the warning for it is
  retired; the code stays reserved (codes are stable forever).
- spec/core.md 10.6 lists XO0530 and XO0531 when they are implemented.

## Alternatives

- Fix every shape in the compiler first: the right end state, and in
  progress (19a3540d, the frame homes), but each takes days, and agents
  write new code meanwhile.
- A profiler annotation (`xo dev compare` shows the share and drop
  helpers per function, decision 0264): finds the cost after the fact,
  not while writing.
