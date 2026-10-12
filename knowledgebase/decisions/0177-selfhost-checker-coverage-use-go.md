# 0177: The self hosted checker binds `use go` from the binding cache and compares every diagnostic of the Go checker

Status: Accepted, implemented (2026-10-12). Implemented in
`selfhost/check/goimport.xo` (`load_go_import`, `go_skipped`),
`vouch.xo` (vouch lines, `escape_hatch`), `escape.xo` (XO0802),
`modfile.xo` (the effect lock, XO1317), `load.xo` (`edition` aliases,
XO1315), and the tests of internal/selfhost (TestSelfhostChecker,
TestSelfhostCheckerHatches). Figures:
bench/results/selfhost-check-coverage-2026-10-12.md. Extends decision
0152 and closes the coverage gaps decision 0150 (as amended on
2026-10-11) needs before the Go front end is retired.

## Context

After module loading (decision 0141) and `use c` (decision 0152), the
self hosted checker matched the Go checker on 610 of 626 repository
runs. The 16 others had `use go` (the Xo checker marked them
unsupported) or `use c` (loaded, but on the Go backend both checkers
reject the line with XO1201, a code the comparison did not list, so
the runs counted as rejected for other reasons). Seven diagnostics of
internal/check had no port: XO0802 (scope handles and task groups
escaping), XO1101 and XO1102 (`use go` bindings), XO1315 (old edition
name), XO1316 (unvouched escape hatches) and, through internal/mod,
XO1317 (the effect lock); XO1104, XO1105, and XO1201 were ported but
not compared.

## Decision

- `use go "<path>"` loads the declaration file the Go checker's binder
  (internal/gobind) wrote: `<root>/.xo/gobind/<path>.xo` (or
  `$XO_GOBIND_DIR`; `.rerun-<8 hex>` for a `replay rerun` list), where
  root is the module root, made absolute, or the main module's for a
  dependency (goBindRoot), with gobind Load's staleness rules (binder
  version 5, package name, rerun key, a `require go` pin against
  `go-module:`, the adapter next to it). A missing or stale file makes
  the run unsupported (`use go without a current binding of ...`): the
  Xo checker does not run `go list`. The module is std like, under the
  key `go:<path>`, its skipped names from the `// skip:` lines (XO1102
  at module members and at fields and methods of its types).
- The escape hatch rule of decision 0156 is ported: `vouch` lines of
  xo.mod (XO1301 when malformed), the consumer's vouches (main modules
  and workspace modules), XO1316 with the notes of stale vouches, and
  a vouch's `uses` deciding a dependency's `use c` binding.
- The effect lock: xo.lock `effects` lines are kept, every selected
  module version's effects are computed from the `uses` of its pub
  functions (mod.ModuleEffects), and XO1317 reports the labels a line
  does not admit, in the normal and the vendored build list.
- XO1315 (`edition 2026`, a note with its fix) and XO0802 (the value of
  a scope holding a scope handle, closures capturing the handle and
  values holding a group created by `s.group()` that leave the scope)
  are ported; XO0802 runs after the scope's body on the recorded names
  and types.
- TestSelfhostChecker compares XO0802, XO1102, XO1104, XO1105, XO1201,
  XO1315, XO1316, XO1317 too, compares the runs with `use c` a second
  time as a native build checks them (`--native`), and logs the
  unsupported runs by reason. XO1101 (a Go package that cannot be
  bound) stays uncompared: the Xo checker cannot bind, so such a run is
  unsupported.
- The escape hatch fixture (internal/selfhost/testdata/hatches: a
  dependency with `use go` in its own module cache, a main module
  without the vouch and one with it) is left out of the repository
  walk and compared in a temporary copy by TestSelfhostCheckerHatches
  (gotchas/gobind-cache-in-checked-dependency.md).
- internal/gobind `Load` drops the Go name at the end of a `// skip:`
  reason, as `Bind` records it (gotchas/gobind-skip-reason-load-vs-bind.md).

## Consequences

- TestSelfhostChecker (go build): all 685 repository runs compared and
  identical (398 accepted, 287 rejected only with compared
  diagnostics), 0 unsupported; the 3 `use c` runs identical as native
  builds too; TestSelfhostCheckerHatches 2 of 2; 400 module mutations
  identical (results file).
- The binder (go list, go/types, the adapter generator) stays in Go,
  like cbind's clang step (decision 0152). When the self hosted
  compiler must bind on its own (the switch-over of decision 0150) it
  runs `xo bind`, or the binder is ported.
- The Xo checker now reads `XO_GOBIND_DIR` as the Go checker does.

## Alternatives

- **Port internal/gobind**: `go list -json`, go/types export data, and
  the adapter generator. Far more code than the checker change, and
  the comparison would not change: the declaration file is the binder's
  whole output that the checker reads.
- **Compare `use c` runs only natively**: the Go backend's XO1201 is
  part of the Go checker's behavior on those runs, so both are
  compared.
- **Keep the hatch fixture in the repository walk with
  `XO_GOBIND_DIR` set for every run**: one shared binding cache would
  keep cached modules clean, but every `use go` golden would bind into
  it concurrently (one `_mod` workspace for parallel `go get`), and its
  paths would change the notes of every compared XO1102.
