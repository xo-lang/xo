# 0261: Agent-facing commands on xo lsp's engine

Status: Accepted (owner, 2026-10-18, through the lead: "agent-facing
commands on xo lsp's engine ... stateless, arguments up front, JSON
output, a diff preview by default and --write to apply"); stage (a) in
progress (Xo Lang 3)

## Context

Agents read and change Xo code with grep and search-and-replace, which
miss shadowed names, methods of the same name on other types, and
qualified uses, and say nothing about types. The checker already resolves
every identifier for `xo lsp` (compiler/cli/servers resolve.xo:
definition, references, hover, rename), and two command line tools sit on
the checker already (2026-10-18 inventory):

- `xo query KIND [--json] [--dir=D] ARG` (compiler/cli/semantic
  query_cmd.xo): `type FILE:LINE:COL`, `def NAME` (NAME, Type.member,
  module.name), `callers module.fn`, `effects module.fn`, `holes`, `api
  MODULE`, `imports`, `deps`, `hash NAME`.
- `xo edit [--dry-run] [--dir=D] OP ARGS` (edit.xo): `rename` (function,
  method, constant), `add-field`, `change-sig`; every reference rewritten
  at once, rejected unless the program checks clean before and after.
- `xo fix` applies the checker's quick fixes; `xo doc [--json] MODULE`
  documents a module; `xo mcp` wraps query and edit as MCP tools.

What is missing for an agent: references of any symbol (callers covers
functions only), the documentation of one declaration, every symbol of a
module (api is the pub surface), short names, a diff preview, renames
beyond functions, methods and constants, and the refactors of stage (c).

## Decision

One command family, every member stateless (arguments up front, the
program checked from disk on each run), JSON by default when stdout is
not a terminal (as query), exit 0 on an answer, 1 when nothing matches,
2 on a usage error, and conflicts as structured errors (a JSON object
with `error`, `code`, and the sites).

(a) Lookups, on query's engine and the LSP resolver:
- `xo def NAME|FILE:LINE:COL`: `xo query def`, plus the position form
  (the LSP's definition).
- `xo refs NAME|FILE:LINE:COL`: every reference of the symbol (function,
  method, type, field, variant, constant, local), each with file, line,
  column, and whether it is the declaration (the LSP's references).
- `xo type FILE:LINE:COL`: `xo query type`.
- `xo doc NAME`: one declaration's signature, doc comment, file and
  line, and for a type its methods (with `xo doc MODULE` as today when
  the argument is a module directory or import path).
- `xo symbols MODULE [--all]`: the module's declarations with kind,
  signature, and location (`api` plus the private ones with --all).

(b) Refactors through `xo edit`, renamed `xo refactor`: `rename` for every
symbol the resolver renames (types, fields, variants, locals, as the
LSP's rename) and `signature` (change-sig) that also rewrites every caller
it can (a new parameter with a default needs no change; one without
becomes a hole `???` at each call, listed). Output a unified diff by
default; `--write` applies it and re-checks.

(c) `xo refactor move` (a declaration and the private helpers only it
uses, to another file or module, fixing `use` lines), `fill-match` (the
missing arms of a match), `implement` (the missing methods of an
interface for a type), and `xo fix` with the same preview and --write.

When (a) lands, the lead adds to CLAUDE.md that agents use these instead
of search-and-replace; an MCP wrapper (`xo mcp` already wraps query and
edit) follows the commands.

## Consequences

- The short names are aliases of the existing tools where they exist,
  so there is one engine per question: query's for def and type, the LSP
  resolver's for refs and positions.
- Every command checks the program on each run (no daemon): a few
  hundred milliseconds on a module of a few thousand lines, acceptable for
  agents and simpler than a server's lifecycle.

## Alternatives

A long-lived server for agents (the LSP over a socket): faster per query,
but stateful and one more process to manage; the MCP wrapper can keep one
later. Extending grep-like tools with type filters: still textual, and
every rule duplicates the checker.
