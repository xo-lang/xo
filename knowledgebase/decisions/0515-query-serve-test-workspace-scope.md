# 0515: xo query and xo serve load what the question needs; xo test ./... per member

Status: Proposed (Xo Lang 1, 2026-11-05, at the lead's request after
decisions 0511 and 0513). A draft: no code yet. Amends decision 0289's
"a workspace or repository root checks the modules under it" for the
query kinds below, and decision 0086's workspace rule for `xo query`,
`xo serve` and `xo test ./...`.

## Context

Decision 0511 scoped `xo check` to what it is asked; `xo test`, `query`,
`edit`, `mcp` and `serve` kept 0086's rule. Measured on the aws SDK
repository (8fe319e: a workspace of 474 modules, 435 of them services),
linux/arm64 offload host, cold private caches, `XO_CHK_CACHE=off`,
`XO_NO_SERVE=1` except in the serve rows. 0.39.0 and branch lang1-ws11
(0511 and 0513 prototypes) gave the same figures:

| command | peak RSS | wall | result |
|---|---|---|---|
| `xo lsp` at the root, open service/s3/client.xo, hover | 335 MB | 0.37 s | works (per directory, decision 0284) |
| `xo type service/s3/client.xo:57:8` | 333 MB | 0.21 s | works |
| `xo def new --dir=service/s3` | 107 MB | 0.13 s | works |
| `xo test ./service/s3/...` | 630 MB | 1.8 s | works |
| `xo def from_config` at the root | 8,324 MB | 10.5 s | works, heavy |
| `xo query type service/s3/client.xo:57:8` | over 30 GB | 40 s | dies at the memory budget |
| `xo serve`, then that query through it | over 30 GB | | the server dies; every later check and query sent to it fails |
| `xo test ./...` | over 30 GB | 38 s | dies at the memory budget |

The editor path holds: an editor runs `xo lsp`, which checks per
directory. What dies is what agents use most. `xo query type` on one s3
file dies because load_session_pc (compiler/cli/semantic/query.xo)
sets walk_all and calls workspace_dirs, so it loads all 474 modules to
answer a question about one. `xo type` (decision 0261) answers it in
0.21 s. `xo serve` runs the same load in its own process, so one query
kills the server, and every check and query routed to it after that
fails. Forwarding to serve is automatic, so a single agent query makes
the whole repository uncheckable until someone notices. `xo test ./...`
is the walk 0513 fixes for `xo check`, unfixed for test.

## Decision

1. **Point queries load the closure.** `type`, `def` (with --dir or a
   file), `effects`, `holes`, `api`, `imports`, `deps` and `hash` load
   the asked directory's module and its imports, transitively, as
   `xo check DIR` does under 0511. A workspace module nothing imports
   cannot change their answer: `type`'s file and `effects`' function
   depend only on what they import.
2. **Reverse queries load the reverse closure.** `callers` (and `xo
   refs` and `xo edit`'s rename, which ask the same question) load the
   target's module, every workspace module that imports it, directly or
   transitively, and those modules' imports. The importers come from
   the members' `use` lines, as 0513's shared_imports finds them,
   without checking anything. For a service that is the service and
   aws's integration module. For the core's sigv4 it is every service,
   checked one member at a time on 0513's shared base and merged, since
   a caller is found in one member's bodies.
3. **A bare name at a root** (`xo def from_config` with no --dir) finds
   its declaring modules from a declarations-only pass over the members
   (decls_only, no bodies), then answers as in point 1 from those
   modules. Today's 8.3 GB holds every member's bodies.
4. **`xo serve` keeps one shared base** (0513) and per-member entries in
   its existing LRU of checked programs. An entry is the member's
   closure, the unit its check and query requests need. A request whose
   load would pass the memory budget is answered with an error that
   names the narrower command and does not stop the server. One query
   never takes the server down for the others.
5. **`xo test ./...` checks per member** as 0513's `check ./...` does,
   then builds and runs each member's test packages from that member's
   checked program. The test binaries are per package already. Only the
   check phase holds the workspace at once today.

## Consequences

- Point queries in a large workspace cost what the asked module costs:
  `xo query type` on s3 should land near `xo type`'s 333 MB.
- The answer of a point query cannot change. A reverse query's answer
  can only lose callers outside the reverse closure, and by definition
  those are not callers. The test for it: `callers` of a core function
  in a workspace gives the same list with the old load and the new one.
- `--dir=ROOT` keeps meaning the whole workspace, and for a reverse
  query it is the default. The scope narrows where the question allows,
  not because the flag changed.
- serve's memory is bounded by its LRU entries plus the base, not by
  the workspace. Its budget error is new client-visible output, a new
  XO code, documented in spec section 10.
- Tests: cmd/xo tests on a synthetic workspace in which an unimported
  module holds a planted type error and 300 large unused members (0511's
  TestCheckWorkspaceSizeMemory pattern), asserting peak RSS for `query
  type`, `def`, `callers` and `test ./...`, and that a server survives
  a request past the budget.

## Alternatives

- **Keep 0086/0289 and raise the budget.** No: `query type` on s3 would
  still hold 474 modules, about 30 GB, to answer a 333 MB question.
- **Route query to `xo type`/`xo def` internally.** Those are point
  lookups only; callers and effects need the semantic session. Point 1
  gives the session the same scope instead.
- **An on-disk index of the workspace's declarations and references**
  (gopls style). It may come later for `refs` at the root, and would
  serve point 3 better, but it is a cache with invalidation work of its
  own. The scoping here is needed with or without it.

## Work, in order

1. Point 1 in load_session_pc, with the peak RSS test. This is the
   largest win: query type dies today.
2. Point 4's budget error in serve, so one query cannot kill the server.
3. Point 5, on 0513's code once the owner accepts 0513.
4. Points 2 and 3.
