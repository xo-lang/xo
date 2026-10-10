# 0125: Per definition content hashes (decision 0086 step 5)

Status: Accepted (2026-10-10), implemented for `xo query hash` and `xo
publish`. Proposed, not built: recording them in xo.lock (below). Code:
`internal/defhash` (`Module`, `Diff`, `Find`, `Nearest`),
`cmd/xo/hash.go` (`xo query hash`), `internal/release/publish.go`
(`Result.Definitions`), the MCP `query` tool (kind `hash`). Tests:
`internal/defhash/defhash_test.go` (TestStable, TestSensitive,
TestRecursion), `cmd/xo/hash_test.go` (TestQueryHashAndPublish). Spec:
core 4.3 (the new [P1] bullet), 10.3.

## Context

Core 4.3 sketches, as [P3], hashes per definition: "the normalized AST
with local names erased and referenced definitions replaced by their
hashes", with consequences that need the checker and the lock to work
per definition (versions coexisting, types equal by hash, XO0601 for
names that resolve to no hash). Decision 0086 orders it as step 5. The
first useful consumer is `xo publish`: the API comparison of step 4
sees only the exported surface, so a release whose private helper now
computes something else looks like "no changes".

## Decision

The hash (`d1:` and base64 SHA-256) of a top level definition is over a
canonical encoding of its syntax tree:

- Dropped: positions, comments, doc comments, layout; the definition's
  own name for functions, methods, and constants (a type's name stays:
  Xo types are nominal).
- Erased: local names, numbered by first binding in tree order: `let`
  and `var`, pattern bindings, loop variables, closure parameters, type
  parameters (and effect variables), receivers, labels, local
  constants. `Point{x}` encodes as `Point{x: x}`, so a renamed binding
  keeps the hash.
- Kept by name: parameters of declared functions and interface methods
  (named arguments use them, so renaming one is a change callers see),
  fields, variants, methods called on values, effect labels, prelude
  and builtin names, and definitions of other modules (by import path,
  never by local import name).
- Referenced definitions of the same module (any package, through
  `alias.name`, `Type.method`, type names, struct literals, patterns)
  are replaced by their hashes; the strongly connected components of
  the reference graph (mutual recursion) are hashed together, so every
  member covers all of them; self references are a fixed marker. The
  result is deterministic and independent of the module path (a /v2
  path hashes as v1).

Two hashes per definition: `hash` (the closure above) and `own` (the
definition alone, references by name). `xo publish` reports
`changed` when `own` differs and `changed_via` when only `hash` does,
naming the referenced definitions whose hashes changed; plus `added` and
`removed`. The list is informational: it never fails a release (the API
rules of step 4 decide the version).

The hashes are syntactic, as the API diff is: an old tag is hashed as
extracted, without fetching its dependencies or type checking.

xo.lock: the spec's [P1] lock is per module version and the per
definition lock is [P3], so xo.lock is unchanged. Proposal for when the
checker resolves through hashes: a section `<path> <version> d:<package
path>.<name> d1:<hash>` for each definition of a dependency the main
module uses (directly or through other definitions), written by `xo mod
tidy`. Then an upgrade that leaves every used definition's hash alone is
known to be a no-op for this module, and `xo get -u` could say which
uses an upgrade really touches. It needs type based resolution (method
calls on values resolve to definitions) and the dependency's hashes to
include its own dependencies' definitions, both outside this step.

## Consequences

- `xo publish --check` shows private changes and changes that reach an
  export only through what it calls; agents can tell a refactor (no
  hash changed) from a behavior change.
- `xo query hash` gives agents a stable identity for a definition across
  renames of locals and moves within a file.
- Known gaps, all conservative (a hash may change without a semantic
  change, never the reverse in practice): shadowing a top level name
  with a local can change the numbering; methods called on values are
  by name, so a changed method body does not change its callers'
  `hash` (their `own` is unaffected either way); a parameter rename
  changes the hash on purpose.

## Alternatives

- Hash the checked program (types resolved): precise method references
  and type identity, but `xo publish` would have to fetch and check the
  old tag's dependencies. Left for the [P3] checker integration.
- Hash the formatted source (`xo fmt` output): renaming a local changes
  it, which defeats the purpose.
- Erase parameter names too (as Unison): breaks named arguments
  silently; rejected for declared functions, kept for closures.
