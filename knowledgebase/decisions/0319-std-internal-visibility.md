# 0319: Std-internal names are `pub` items of native implementation modules

Status: Accepted (owner, 2026-10-21, from the agents' poll: "a
std-internal visibility so std modules can call each other's internals,
with websocket's hand-off as the example"; Xo Lang 3)

## Context

std/websocket's server takes the HTTP connection from std/http's native
server (decision 0313). A native std module cannot name anything of
another std module that std/M.xo does not declare (`use std/http` sees
the declarations: `http.handoff` is XO0601), nor rt.cast its own struct
to a declared type (XO1201). Stage 1 therefore handed the connection
over with a 101 response whose body was a marker string and a registry
id that http.xo's respond parsed: a side channel that any handler
returning such a body could imitate, invented for one feature, and the
pattern every later cross-module std feature (SSE, an HTTP/2 upgrade, a
reverse proxy's connection reuse) would copy.

## Decision

No new syntax. The language already has the scope the poll asked for:

- A native std implementation module, std/native/M.xo, is the module
  `native/M`; only std modules may import `native/...` (core 4.1: for
  any other module it is XO0601, module not found).
- `use std/M` resolves to std/M.xo's declarations only, so a program
  never sees anything else of the implementation.
- So a `pub` item of std/native/M.xo is std-internal: visible to every
  native std module through `use native/M` (XO0605 if not `pub`),
  invisible to programs. Spec core 4.1 says so, and std.md lists none
  of these names.

std/websocket's hand-off uses it: std/native/http.xo has
`pub fn handoff_response(f: fn(rthttp.HConn) -> ()) -> http.Response`
(a 101 whose stream is f; respond runs f with the connection), and
std/native/websocket.xo calls `nhttp.handoff_response(hand)` after
`use native/http as nhttp`. The marker body and hserve.HANDOFF are
gone.

## Consequences

- Cross-module std features call each other's internals directly, type
  checked, with no registries keyed by strings.
- Cycles between native modules stay errors; a pair that needs both
  directions moves the shared part into std/native/llrt (as the HTTP
  connection code is).
- The Go backend has no such need: std/rt is one Go package.

## Alternatives

- **`pub(std)` on declarations**: a new visibility in the grammar, the
  checker, and every tool, for what `native/` scoping already gives.
- **Declaring the internals in std/M.xo** (hidden by convention): they
  would be visible to programs.
- **Keep the marker body**: imitable, untyped, and copied by each new
  feature.
