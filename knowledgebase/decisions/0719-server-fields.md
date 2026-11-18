# 0719: server fields and the server effect

Status: Accepted (Xo Lang 1, 2026-11-18; task #369, decision 0659 point 5, owner question #159 option 2; surface agreed with the std/ui track, xo-4c).

## Context

std/ui (decision 0659) runs one model on the server and sends a copy to
each client. Some fields (a session token, an API key, a database
handle's secret) must never reach a client, and a review cannot find
every place that copies or logs them. Decision 0659 point 5 asked the
compiler to mark such fields and make every use of them visible in
signatures, as effects already make I/O visible.

## Decision

- A struct field may be marked `server`: `[pub] server name: T =
  default`. `server` is contextual (only before a field's name), so
  existing fields and variables named `server` keep working.
- A server field needs a default (XO0441): a client's copy holds the
  default in its place.
- A new builtin effect label, `server`, needs no capability. Code that
  names a server field performs it: a read, an assignment, a struct
  literal or `with` that sets the field, and a pattern that binds it. A
  `pub` function that does so declares `uses server` (XO0701, its note
  naming `Type.field`); other functions infer it.
- Pure: a literal that leaves the field to its default, a `with` that
  does not name it, `==`, hashing, and derived code (Debug, Eq, Hash,
  Json, Proto).
- Derived Debug and `${x:?}` print `<server>` for the value, as
  `Secret[T]` prints `<redacted>` (native, LLVM and Go backends).
- Function types may say `uses server`, so a model can carry a function
  that reads server fields (std/ui's `with_server`).
- Planned next, in its own push: `json.encode` and friends take
  `public: Bool = false`; with `true` they leave server fields out, which
  is what std/ui sends to a client.

## Consequences

- AST codec v12 and export format v15 carry the field's flag; the
  definition hash changes only for a struct that has a server field.
- `xo fix` adds `uses server` like any other effect.
- Derived code reads server fields without the effect, so a hand
  written encoder for a client must use the field through a function
  that declares `uses server`, which shows in review.

## Alternatives

- A `Server[T]` wrapper type, as `Secret[T]`: every read becomes
  `.reveal()`, every model type changes, and the wrapper alone does not
  show which functions read it. Rejected (owner question #159 option 1).
- An attribute checked only by std/ui at run time: leaks are found in
  production, not at compile time. Rejected.
