# Definition hashes encode internal/ast's Go field order

Status: Anticipated (2026-10-12, decision 0178 stage 3)

Symptom: after a field is added to, removed from, or reordered in a
struct of internal/ast (or a node type is renamed), every `xo query hash`
and `xo publish --check` definition hash changes, and
TestSelfhostDefhash (internal/selfhost) fails for most modules.

Cause: internal/defhash encodes a declaration by reflection over the Go
syntax tree: `(` and the Go type name, then every exported field in
declaration order (Base and Doc skipped; nil fields as `0`, false
booleans as `b1:0`). compiler/cli/semantic/defhash.xo writes the same
encoding out by hand over compiler/parser's tree, so it has to change in
the same place in the same commit. A change to the hash scheme also
needs a new prefix (`d1:`, decision 0125), or old and new hashes compare
as different definitions.

Fix: change internal/ast, defhash.xo (the matching encoder function, at
the field's position), and the `d1:` prefix together; run
`go test -run TestSelfhostDefhash ./internal/selfhost/`.
