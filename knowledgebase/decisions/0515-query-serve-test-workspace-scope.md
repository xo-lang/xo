# 0515: xo query loads what the question needs; xo test ./... per member

Status: Proposed (Xo Lang 1, 2026-11-05, at the lead's request after
decisions 0511 and 0513). Point 1 is prototyped on branch lang1-qscope
(on lang1-ws11; not on main), measured below. Amends decision 0289's
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
| that query through `xo serve` | over 30 GB in the child | 40 s | fails; the server lives (below) |
| `xo test ./...` | over 30 GB | 38 s | dies at the memory budget |

The editor path holds: an editor runs `xo lsp`, which checks per
directory. What dies is what agents use most. `xo query type` on one s3
file dies because load_session_pc (compiler/cli/semantic/query.xo)
sets walk_all and calls workspace_dirs, so it loads all 474 modules to
answer a question about one. `xo type` (decision 0261) answers it in
0.21 s. `xo test ./...` is the walk 0513 fixes for `xo check`, unfixed
for test.

`xo serve` is not at risk itself (corrected 2026-11-05: the first draft
said a query killed the server; the 30 GB was its child's, which GNU
time counts). The Xo server runs each request as a child process
(compiler/cli/servers/serve.xo) and caches the response, unlike the Go
server it replaced, which checked in process. Checked: `check .` through
serve failed at the budget after 34.8 s, the server stayed up at 14 MB,
and the next requests answered (below).

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
4. **`xo serve` needs no change**: its children load what points 1 and
   5 load, and a child that dies at the budget fails only its request.
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
- serve's own memory stays small (a child per request); its response
  cache keys and stamps are unchanged.
- Tests: cmd/xo tests on a synthetic workspace in which an unimported
  module holds a planted type error and 300 large unused members (0511's
  TestCheckWorkspaceSizeMemory pattern), asserting peak RSS for `query
  type`, `def`, `callers` and `test ./...`.

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
2. Point 5, on 0513's code once the owner accepts 0513.
3. Points 2 and 3. `api`, `deps` and `imports` of a directory argument
   now check that directory (done on the branch, below); a bare name
   at the root (`def from_config`, 6.5 GB) is point 3's
   declarations-only pass, still open.

## Point 1 measured (2026-11-05, branch lang1-qscope)

`api`, `imports` and `deps` with no `--dir` check the argument's
directory when it holds `.xo` files (before decision 0437's "not inside
a module" guard, which then tests that directory). Every query kind but
`callers` goes through Checker.check_workspace_dirs
(0511's rule) when every asked directory holds `.xo` files of its own; a
directory without them (a module or repository root, 0289) still walks
the modules below it. XO_QUERY_WIDE=1 restores the old load. aws
8fe319e, linux/arm64, cold:

| query | 0.39.0 | branch |
|---|---|---|
| `type service/s3/client.xo:57:8` | over 30 GB, 40 s, fails | 282 MB, 0.22 s |
| `def from_config --dir=service/s3` | (whole workspace) | 99 MB, 0.08 s |
| `effects s3.new --dir=service/s3` | (whole workspace) | 283 MB, 0.20 s |
| `holes --dir=service/s3` | (whole workspace) | 282 MB, 0.20 s |
| `api service/s3`, `imports service/s3`, `deps service/s3` | 6.5 GB, 9.3 s | 99 MB, 0.09 s, byte-identical output (871 KB, 5 KB, 18 KB) |
| `def from_config` at the root | 6.5 GB, 8.9 s | unchanged (point 3's pass, open) |
| `type` through `xo serve` | fails, server lives | 286 ms; `check ./service/s3` 1.0 s, cached 2 ms |

TestCheckWorkspaceSizeMemory now also measures `query type` and `query
api` in a 301 module workspace: 36 MB against 35 MB and 30 MB against
30 MB for one module with the change; `query type`
1,792 MB without it (XO_QUERY_WIDE=1; the test fails). cmd/xo's
workspace, TestModules and TestQuery tests pass, and
TestCodegenInputsClassified.
